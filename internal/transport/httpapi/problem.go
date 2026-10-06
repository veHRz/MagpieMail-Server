package httpapi

import (
	"encoding/json"
	"net/http"
)

// problem is an RFC 9457 problem details object. Phase S1 defines the problem
// types in the API contract; until then only "about:blank" is used.
type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// writeProblem writes an application/problem+json response. The detail is shown
// to clients: it must not contain internal error messages.
func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}
