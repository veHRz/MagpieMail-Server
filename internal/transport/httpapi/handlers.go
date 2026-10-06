package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi/oapi"
)

// handlers implements the operations of the contract.
type handlers struct {
	apiVersion       string
	db               Pinger
	readinessTimeout time.Duration
	logger           *slog.Logger
}

var _ oapi.StrictServerInterface = (*handlers)(nil)

// GetInfo describes the instance. No optional feature exists yet.
func (h *handlers) GetInfo(context.Context, oapi.GetInfoRequestObject) (oapi.GetInfoResponseObject, error) {
	return oapi.GetInfo200JSONResponse{ApiVersion: h.apiVersion, Features: oapi.Features{}}, nil
}

// GetLiveness reports that the process is alive. It checks nothing else, so
// that an orchestrator never restarts the server because a dependency is down.
func (h *handlers) GetLiveness(context.Context, oapi.GetLivenessRequestObject) (oapi.GetLivenessResponseObject, error) {
	return oapi.GetLiveness200JSONResponse{HealthOKJSONResponse: oapi.HealthOKJSONResponse{Status: oapi.Ok}}, nil
}

// GetReadiness reports whether the database answers within the timeout.
func (h *handlers) GetReadiness(ctx context.Context, _ oapi.GetReadinessRequestObject) (oapi.GetReadinessResponseObject, error) {
	pingCtx, cancel := context.WithTimeout(ctx, h.readinessTimeout)
	defer cancel()

	if err := h.db.Ping(pingCtx); err != nil {
		h.logger.WarnContext(ctx, "readiness check failed", slog.String("check", "database"), slog.Any("error", err))
		problem := newProblem(ctx, http.StatusServiceUnavailable, detailUnavailableDB)
		return oapi.GetReadiness503ApplicationProblemPlusJSONResponse{
			ServiceUnavailableApplicationProblemPlusJSONResponse: oapi.ServiceUnavailableApplicationProblemPlusJSONResponse(problem),
		}, nil
	}
	return oapi.GetReadiness200JSONResponse{HealthOKJSONResponse: oapi.HealthOKJSONResponse{Status: oapi.Ok}}, nil
}
