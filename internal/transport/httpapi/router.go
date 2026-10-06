// Package httpapi is the HTTP transport: routing, middlewares, operational
// probes and, from phase S1, the handlers generated from api/openapi.yaml.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

const defaultReadinessTimeout = 2 * time.Second

// Pinger reports whether a dependency answers. *pgxpool.Pool implements it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators the HTTP transport needs.
type Deps struct {
	Logger *slog.Logger
	// Database is checked by the readiness probe.
	Database Pinger
	// ReadinessTimeout bounds the readiness check. Zero means 2 seconds.
	ReadinessTimeout time.Duration
}

// NewRouter returns the HTTP handler of the server.
func NewRouter(deps Deps) http.Handler {
	if deps.ReadinessTimeout <= 0 {
		deps.ReadinessTimeout = defaultReadinessTimeout
	}

	r := chi.NewRouter()
	r.Use(requestID, accessLog(deps.Logger))

	// Operational probes live outside /api/v1 and need no authentication.
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(deps.Database, deps.ReadinessTimeout, deps.Logger))
	return r
}
