package contracttest_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/testsupport/contracttest"
)

const spec = `
openapi: 3.1.0
info: {title: test, version: 1.0.0}
paths:
  /things:
    get:
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                required: [name]
                properties:
                  name: {type: string}
        "500":
          description: error
          content:
            application/problem+json:
              schema:
                type: object
                required: [type, title, status]
                properties:
                  type: {type: string}
                  title: {type: string}
                  status: {type: integer}
`

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(spec))
	require.NoError(t, err)
	return doc
}

func respond(status int, contentType, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func check(t *testing.T, h http.Handler, path string) error {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return contracttest.Check(loadSpec(t), req, rec.Result())
}

func TestCheck_AcceptsAConformingResponse(t *testing.T) {
	require.NoError(t, check(t, respond(http.StatusOK, "application/json", `{"name":"a"}`), "/things"))
	require.NoError(t, check(t, respond(http.StatusInternalServerError, "application/problem+json",
		`{"type":"about:blank","title":"Internal Server Error","status":500}`), "/things"))
}

// TestCheck_RejectsNonConformingResponses proves the S1 rule "a response that
// does not match the contract fails the test".
func TestCheck_RejectsNonConformingResponses(t *testing.T) {
	tests := map[string]struct {
		handler http.Handler
		path    string
	}{
		"missing required member": {respond(http.StatusOK, "application/json", `{}`), "/things"},
		"wrong member type":       {respond(http.StatusOK, "application/json", `{"name":42}`), "/things"},
		"undeclared status":       {respond(http.StatusTeapot, "application/json", `{"name":"a"}`), "/things"},
		"undeclared content type": {respond(http.StatusOK, "text/plain", `name`), "/things"},
		"error not a problem":     {respond(http.StatusNotFound, "text/plain", `404 page not found`), "/unknown"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, check(t, tt.handler, tt.path))
		})
	}
}

func TestCheck_UnknownRoutesMustAnswerWithAProblem(t *testing.T) {
	require.NoError(t, check(t, respond(http.StatusNotFound, "application/problem+json",
		`{"type":"about:blank","title":"Not Found","status":404}`), "/unknown"))
}
