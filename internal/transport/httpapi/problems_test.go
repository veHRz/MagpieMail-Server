package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/config"
	"github.com/veHRz/MagpieMail-Server/internal/observability"
	"github.com/veHRz/MagpieMail-Server/internal/testsupport/contracttest"
	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi"
)

// The S1 criterion "400, 401, 404, 413, 429 and 500 responses are all
// application/problem+json" is proven by the tests of this file: 404, 405, 413
// and 429 on the production router; 400, 401, 413 and 500 on test operations
// mounted on the same middleware chain (testdata/chain.yaml), because the real
// contract has no operation with input or authentication yet.

// tokenAuthenticator accepts "Bearer good-token" and the session cookie "good-session".
type tokenAuthenticator struct{}

func (tokenAuthenticator) Authenticate(r *http.Request) (httpapi.Principal, bool, error) {
	if r.Header.Get("Authorization") == "Bearer good-token" {
		return httpapi.Principal{ID: "user-1", Scheme: "bearerAuth"}, true, nil
	}
	if c, err := r.Cookie("__Host-magpie_session"); err == nil && c.Value == "good-session" {
		return httpapi.Principal{ID: "user-2", Scheme: "cookieAuth"}, true, nil
	}
	return httpapi.Principal{}, false, nil
}

func chainContract(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("testdata/chain.yaml")
	require.NoError(t, err)
	require.NoError(t, doc.Validate(t.Context()))
	return doc
}

// newTestChain mounts the operations of testdata/chain.yaml on the production
// middleware chain.
func newTestChain(t *testing.T, opts ...routerOption) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger, err := observability.NewLogger(&logs, "debug", "json")
	require.NoError(t, err)
	deps := httpapi.Deps{
		Logger:        logger,
		Database:      fakeDB{},
		Config:        serverConfig(),
		Authenticator: tokenAuthenticator{},
	}
	for _, opt := range opts {
		opt(&deps)
	}

	r, err := httpapi.NewChainForTest(deps, chainContract(t))
	require.NoError(t, err)
	r.Post("/test/echo", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Text string }
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Errorf("the validator let an invalid body through: %v", err)
		}
		times, _ := strconv.Atoi(r.URL.Query().Get("times"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": strings.Repeat(in.Text, max(times, 1))})
	})
	r.Get("/test/protected", func(w http.ResponseWriter, r *http.Request) {
		p, _ := httpapi.PrincipalFrom(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"principal": p.ID})
	})
	r.Get("/test/cookie-only", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	r.Get("/test/panic", func(http.ResponseWriter, *http.Request) {
		panic("something broke")
	})
	return r, &logs
}

func serveChain(t *testing.T, h http.Handler, req *http.Request) *http.Response {
	t.Helper()
	return contracttest.Serve(t, chainContract(t), h, req)
}

// assertProblem checks the RFC 9457 shape shared by every error response.
func assertProblem(t *testing.T, resp *http.Response, status int) map[string]any {
	t.Helper()
	assert.Equal(t, status, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	problem := decodeJSON(t, resp)
	assert.Equal(t, "about:blank", problem["type"])
	assert.Equal(t, http.StatusText(status), problem["title"])
	assert.InDelta(t, status, problem["status"], 0)
	assert.Equal(t, resp.Header.Get("X-Request-ID"), problem["requestId"])
	return problem
}

func TestProblem_400_InvalidParameter(t *testing.T) {
	h, _ := newTestChain(t)

	resp := serveChain(t, h, request(t, http.MethodPost, "/test/echo?times=99", strings.NewReader(`{"text":"a"}`)))

	problem := assertProblem(t, resp, http.StatusBadRequest)
	assert.Contains(t, problem["detail"], "times")
}

func TestProblem_400_InvalidBody(t *testing.T) {
	h, _ := newTestChain(t)
	for name, body := range map[string]string{
		"missing member": `{}`,
		"wrong type":     `{"text":42}`,
		"not JSON":       `{"text":`,
	} {
		t.Run(name, func(t *testing.T) {
			req := request(t, http.MethodPost, "/test/echo", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			assertProblem(t, serveChain(t, h, req), http.StatusBadRequest)
		})
	}
}

func TestEcho_ValidRequestReachesTheHandler(t *testing.T) {
	h, _ := newTestChain(t)
	req := request(t, http.MethodPost, "/test/echo?times=2", strings.NewReader(`{"text":"ab"}`))
	req.Header.Set("Content-Type", "application/json")

	resp := serveChain(t, h, req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "abab", decodeJSON(t, resp)["text"])
}

func TestProblem_401_MissingOrInvalidCredentials(t *testing.T) {
	h, _ := newTestChain(t)
	for name, header := range map[string]string{
		"no credentials": "",
		"invalid token":  "Bearer bad-token",
	} {
		t.Run(name, func(t *testing.T) {
			req := request(t, http.MethodGet, "/test/protected", http.NoBody)
			if header != "" {
				req.Header.Set("Authorization", header)
			}

			resp := serveChain(t, h, req)

			assertProblem(t, resp, http.StatusUnauthorized)
			assert.Equal(t, `Bearer realm="magpiemail"`, resp.Header.Get("WWW-Authenticate"))
		})
	}
}

func TestAuthentication_AcceptsEitherDeclaredScheme(t *testing.T) {
	h, _ := newTestChain(t)

	bearer := request(t, http.MethodGet, "/test/protected", http.NoBody)
	bearer.Header.Set("Authorization", "Bearer good-token")
	resp := serveChain(t, h, bearer)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "user-1", decodeJSON(t, resp)["principal"])

	cookie := request(t, http.MethodGet, "/test/protected", http.NoBody)
	cookie.AddCookie(&http.Cookie{Name: "__Host-magpie_session", Value: "good-session"})
	resp = serveChain(t, h, cookie)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "user-2", decodeJSON(t, resp)["principal"])
}

func TestAuthentication_SchemeMustBeOneTheOperationAccepts(t *testing.T) {
	h, _ := newTestChain(t)
	req := request(t, http.MethodGet, "/test/cookie-only", http.NoBody)
	req.Header.Set("Authorization", "Bearer good-token")

	assertProblem(t, serveChain(t, h, req), http.StatusUnauthorized)
}

func TestAuthentication_NobodyIsAuthenticatedUntilS3(t *testing.T) {
	// Without an Authenticator, every protected operation answers 401.
	h, _ := newTestChain(t, func(d *httpapi.Deps) { d.Authenticator = nil })
	req := request(t, http.MethodGet, "/test/protected", http.NoBody)
	req.Header.Set("Authorization", "Bearer good-token")

	assertProblem(t, serveChain(t, h, req), http.StatusUnauthorized)
}

func TestProblem_413_BodyTooLarge(t *testing.T) {
	small := withConfig(func(c *config.ServerConfig) { c.MaxBodyBytes = 64 })
	body := `{"text":"` + strings.Repeat("a", 100) + `"}`

	t.Run("declared length", func(t *testing.T) {
		h, _ := newTestChain(t, small)
		req := request(t, http.MethodPost, "/test/echo", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		problem := assertProblem(t, serveChain(t, h, req), http.StatusRequestEntityTooLarge)
		assert.Contains(t, problem["detail"], "64 bytes")
	})

	t.Run("streamed body without length", func(t *testing.T) {
		h, _ := newTestChain(t, small)
		req := request(t, http.MethodPost, "/test/echo", io.MultiReader(strings.NewReader(body)))
		req.ContentLength = -1
		req.Header.Set("Content-Type", "application/json")

		assertProblem(t, serveChain(t, h, req), http.StatusRequestEntityTooLarge)
	})

	t.Run("production router", func(t *testing.T) {
		router, _ := newRouter(t, fakeDB{}, small)
		req := request(t, http.MethodGet, "/api/v1/info", strings.NewReader(body))

		// Checked for its shape only: getInfo takes no body, so its contract
		// rightly declares no 413.
		resp := httpRecord(router, req)
		assertProblem(t, resp, http.StatusRequestEntityTooLarge)
	})
}

func TestProblem_404_UnknownRoute(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})

	resp := serve(t, router, request(t, http.MethodGet, "/api/v1/nothing-here", http.NoBody))

	assertProblem(t, resp, http.StatusNotFound)
}

func TestProblem_405_WrongMethod(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})

	resp := serve(t, router, request(t, http.MethodDelete, "/api/v1/info", http.NoBody))

	assertProblem(t, resp, http.StatusMethodNotAllowed)
	assert.Equal(t, "GET", resp.Header.Get("Allow"))
}

func TestProblem_429_RateLimitedWithRetryAfter(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, withConfig(func(c *config.ServerConfig) {
		c.RateLimit.PerIP = config.LimitConfig{Rate: 0.5, Burst: 2}
	}))
	for range 2 {
		resp := serve(t, router, request(t, http.MethodGet, "/api/v1/info", http.NoBody))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	resp := serve(t, router, request(t, http.MethodGet, "/api/v1/info", http.NoBody))

	assertProblem(t, resp, http.StatusTooManyRequests)
	retryAfter, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	require.NoError(t, err, "Retry-After must be a number of seconds")
	assert.Equal(t, 2, retryAfter, "one token every 2 seconds")
}

func TestProblem_500_PanicIsRecovered(t *testing.T) {
	h, logs := newTestChain(t)

	resp := serveChain(t, h, request(t, http.MethodGet, "/test/panic", http.NoBody))

	problem := assertProblem(t, resp, http.StatusInternalServerError)
	assert.NotContains(t, problem["detail"], "something broke", "the panic stays in the logs")
	assert.Contains(t, logs.String(), "something broke")
	assert.Contains(t, logs.String(), `"level":"ERROR"`)
}

func TestProblem_500_LoggedWithoutBreakingTheAccessLog(t *testing.T) {
	h, logs := newTestChain(t)

	serveChain(t, h, request(t, http.MethodGet, "/test/panic", http.NoBody))

	assert.Contains(t, logs.String(), `"msg":"http request"`)
	assert.Contains(t, logs.String(), `"status":500`)
}

// httpRecord serves req without contract checks.
func httpRecord(h http.Handler, req *http.Request) *http.Response {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}
