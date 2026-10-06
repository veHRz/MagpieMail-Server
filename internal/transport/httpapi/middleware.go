package httpapi

import (
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

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
