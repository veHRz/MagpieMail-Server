package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type statusBody struct {
	Status string `json:"status"`
}

// healthz reports that the process is alive. It checks nothing else, so that an
// orchestrator never restarts the server because a dependency is down.
func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusBody{Status: "ok"})
}

// readyz reports whether the server can do useful work: the database answers
// within the timeout.
func readyz(db Pinger, timeout time.Duration, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "readiness check failed", slog.String("check", "database"), slog.Any("error", err))
			writeProblem(w, http.StatusServiceUnavailable, "The database is not reachable.")
			return
		}
		writeJSON(w, http.StatusOK, statusBody{Status: "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
