package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/veHRz/MagpieMail-Server/internal/observability"
	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi/oapi"
)

// Details shown to clients. They never contain internal error messages.
const (
	detailInternal      = "An unexpected error occurred."
	detailNotFound      = "No resource exists at this address."
	detailUnauthorized  = "Authentication is required."
	detailRateLimited   = "Rate limit exceeded; retry later."
	detailNotAllowed    = "This method is not allowed on this resource."
	detailUnavailableDB = "The database is not reachable."
)

// newProblem returns an RFC 9457 problem for a generic HTTP error. Problem types
// specific to MagpieMail arrive with the domains that need them.
func newProblem(ctx context.Context, status int, detail string) oapi.Problem {
	p := oapi.Problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
	}
	if detail != "" {
		p.Detail = &detail
	}
	if id := observability.RequestID(ctx); id != "" {
		p.RequestId = &id
	}
	return p
}

// writeProblem writes an application/problem+json response.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(newProblem(r.Context(), status, detail))
}
