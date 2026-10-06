// Package httpapi is the HTTP transport: the middleware chain shared by every
// request, and the handlers of the operations generated from api/openapi.yaml
// (package oapi).
package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"

	"github.com/veHRz/MagpieMail-Server/internal/config"
	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi/oapi"
)

const defaultReadinessTimeout = 2 * time.Second

// Pinger reports whether a dependency answers. *pgxpool.Pool implements it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators and settings of the HTTP transport.
type Deps struct {
	Logger *slog.Logger
	// Database is checked by the readiness probe.
	Database Pinger
	Config   config.ServerConfig
	// Authenticator identifies callers. Nil authenticates nobody, so every
	// protected operation answers 401; phase S3 provides the real one.
	Authenticator Authenticator
	// ReadinessTimeout bounds the readiness check. Zero means 2 seconds.
	ReadinessTimeout time.Duration
}

// NewRouter returns the server's HTTP handler: the operations of the embedded
// contract behind the shared middleware chain.
func NewRouter(deps Deps) (http.Handler, error) {
	doc, err := oapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("loading the embedded API contract: %w", err)
	}
	r, err := newChain(deps, doc)
	if err != nil {
		return nil, err
	}

	if deps.ReadinessTimeout <= 0 {
		deps.ReadinessTimeout = defaultReadinessTimeout
	}
	server := oapi.NewStrictHandlerWithOptions(&handlers{
		apiVersion:       doc.Info.Version,
		db:               deps.Database,
		readinessTimeout: deps.ReadinessTimeout,
		logger:           deps.Logger,
	}, nil, oapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, r, http.StatusBadRequest, err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			deps.Logger.ErrorContext(r.Context(), "handler failed", slog.Any("error", err))
			writeProblem(w, r, http.StatusInternalServerError, detailInternal)
		},
	})
	oapi.HandlerWithOptions(server, oapi.ChiServerOptions{
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, r, http.StatusBadRequest, err.Error())
		},
	})
	return r, nil
}

// newChain returns a router carrying the middleware chain that every request
// goes through, validated against doc. Routes are added by the caller.
//
// Order matters: each request gets an ID and an access log record first; then
// panics are recovered and security headers set; browsers' CORS preflights are
// answered; the client address is rate limited before any expensive work; the
// body size is capped; the caller is identified and rate limited as a user;
// finally the request is checked against the contract.
func newChain(deps Deps, doc *openapi3.T) (chi.Router, error) {
	cfg := deps.Config
	trusted, err := parseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}
	validate, err := validateRequests(doc)
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	r.Use(requestID, accessLog(deps.Logger), recoverPanics(deps.Logger), securityHeaders(cfg.HSTSMaxAge))
	if cors := allowCORS(cfg.CORS.AllowedOrigins); cors != nil {
		r.Use(cors)
	}
	if cfg.RateLimit.Enabled {
		r.Use(rateLimit(newLimiter(cfg.RateLimit.PerIP), func(r *http.Request) (string, bool) {
			if probes[r.URL.Path] {
				return "", false
			}
			return "ip:" + clientAddr(r, trusted).String(), true
		}))
	}
	r.Use(limitBody(cfg.MaxBodyBytes), authenticate(deps.Authenticator, deps.Logger))
	if cfg.RateLimit.Enabled {
		r.Use(rateLimit(newLimiter(cfg.RateLimit.PerUser), func(r *http.Request) (string, bool) {
			p, ok := PrincipalFrom(r.Context())
			return "user:" + p.ID, ok
		}))
	}
	r.Use(validate)

	// Every path and method is checked by the validator first; these only catch
	// what slips through, such as a route registered outside the contract.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, http.StatusNotFound, detailNotFound)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, http.StatusMethodNotAllowed, detailNotAllowed)
	})
	return r, nil
}
