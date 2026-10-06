package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/cors"

	"github.com/veHRz/MagpieMail-Server/internal/observability"
)

// RequestIDHeader carries the request ID in both directions.
const RequestIDHeader = "X-Request-ID"

// validRequestID accepts IDs set by a trusted reverse proxy, but only when they
// cannot inject anything into the logs.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// requestID gives every request an ID, stored in the context (and so in every
// log record of the request) and echoed in the response.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !validRequestID.MatchString(id) {
			id = uuid.Must(uuid.NewV7()).String()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(observability.WithRequestID(r.Context(), id)))
	})
}

// quietRoutes are polled by orchestrators; their successes are logged at debug level.
var quietRoutes = map[string]bool{"/healthz": true, "/readyz": true}

// accessLog writes one record per request. It logs the route pattern, never the
// raw path or query string, and no client address or user agent.
func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			level := slog.LevelInfo
			if quietRoutes[route] && status < http.StatusBadRequest {
				level = slog.LevelDebug
			}
			logger.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			)
		})
	}
}

// recoverPanics turns a panic in a handler into a logged error and a 500
// problem, so that one faulty request never takes the server down.
func recoverPanics(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v) // net/http's way to abort a response; it logs nothing
				}
				logger.ErrorContext(r.Context(), "panic while serving a request",
					slog.Any("panic", v), slog.String("stack", string(debug.Stack())))
				if started, ok := w.(interface{ Status() int }); ok && started.Status() != 0 {
					return // the response is already on its way: nothing more to send
				}
				writeProblem(w, r, http.StatusInternalServerError, detailInternal)
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// securityHeaders hardens every response. The API serves JSON only: nothing may
// be framed, sniffed, cached or loaded from it.
func securityHeaders(hstsMaxAge time.Duration) func(http.Handler) http.Handler {
	hsts := ""
	if hstsMaxAge > 0 {
		hsts = "max-age=" + strconv.FormatInt(int64(hstsMaxAge.Seconds()), 10)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Cache-Control", "no-store")
			if hsts != "" {
				h.Set("Strict-Transport-Security", hsts)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// limitBody rejects bodies larger than limit with 413: at once when the
// declared length is too large, otherwise when reading passes the limit.
func limitBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				writeProblem(w, r, http.StatusRequestEntityTooLarge, bodyTooLargeDetail(limit))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// allowCORS lets the configured browser origins call the API with credentials.
// Without origins it returns nil: browsers then keep their same-origin policy.
func allowCORS(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		return nil
	}
	normalized := make([]string, len(origins))
	for i, origin := range origins {
		normalized[i] = strings.TrimSuffix(origin, "/")
	}
	return cors.New(cors.Options{
		AllowedOrigins:   normalized,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowedHeaders:   []string{"Authorization", "Content-Type", RequestIDHeader},
		ExposedHeaders:   []string{RequestIDHeader, "Retry-After"},
		AllowCredentials: true,
		MaxAge:           600,
	}).Handler
}
