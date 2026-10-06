package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/config"
	"github.com/veHRz/MagpieMail-Server/internal/observability"
	"github.com/veHRz/MagpieMail-Server/internal/testsupport/contracttest"
	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi"
	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi/oapi"
)

// fakeDB is a database pinger whose outcome the test controls.
type fakeDB struct {
	err   error
	block bool // block until the context is done
}

func (f fakeDB) Ping(ctx context.Context) error {
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

// serverConfig returns the defaults of config.example.yaml, with rate limiting
// relaxed so that tests do not trip it unless they mean to.
func serverConfig() config.ServerConfig {
	return config.ServerConfig{
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
		MaxBodyBytes:      1 << 20,
		RateLimit: config.RateLimitConfig{
			Enabled: true,
			PerIP:   config.LimitConfig{Rate: 1000, Burst: 1000},
			PerUser: config.LimitConfig{Rate: 1000, Burst: 1000},
		},
	}
}

type routerOption func(*httpapi.Deps)

func withConfig(change func(*config.ServerConfig)) routerOption {
	return func(d *httpapi.Deps) { change(&d.Config) }
}

// newRouter returns the production router and the buffer receiving its JSON logs.
func newRouter(t *testing.T, db httpapi.Pinger, opts ...routerOption) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger, err := observability.NewLogger(&logs, "debug", "json")
	require.NoError(t, err)
	deps := httpapi.Deps{
		Logger:           logger,
		Database:         db,
		Config:           serverConfig(),
		ReadinessTimeout: 50 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(&deps)
	}
	router, err := httpapi.NewRouter(deps)
	require.NoError(t, err)
	return router, &logs
}

func contract(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := oapi.GetSpec()
	require.NoError(t, err)
	return doc
}

func request(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, body)
	req.RemoteAddr = "192.0.2.10:40000"
	return req
}

// serve sends req to h and fails the test if the response does not match the contract.
func serve(t *testing.T, h http.Handler, req *http.Request) *http.Response {
	t.Helper()
	return contracttest.Serve(t, contract(t), h, req)
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func TestInfo_DescribesTheContractVersionAndFeatures(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})

	resp := serve(t, router, request(t, http.MethodGet, "/api/v1/info", http.NoBody))

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	info := decodeJSON(t, resp)
	assert.Equal(t, contract(t).Info.Version, info["apiVersion"], "apiVersion comes from the contract itself")
	assert.Equal(t, map[string]any{"ai": false, "documents": false, "webPush": false}, info["features"])
	assert.NotContains(t, info, "serverVersion", "the exact build is not public")
}

func TestHealthz_ReportsTheProcessIsAlive(t *testing.T) {
	// The database being down must not make the liveness probe fail.
	router, _ := newRouter(t, fakeDB{err: errors.New("connection refused")})

	resp := serve(t, router, request(t, http.MethodGet, "/healthz", http.NoBody))

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "ok", decodeJSON(t, resp)["status"])
}

func TestReadyz_ReadyWhenTheDatabaseAnswers(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})

	resp := serve(t, router, request(t, http.MethodGet, "/readyz", http.NoBody))

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "ok", decodeJSON(t, resp)["status"])
}

func TestReadyz_UnavailableProblemWhenTheDatabaseFails(t *testing.T) {
	tests := map[string]fakeDB{
		"database error": {err: errors.New("dial tcp 10.0.0.7:5432: connection refused")},
		"database hangs": {block: true},
	}
	for name, db := range tests {
		t.Run(name, func(t *testing.T) {
			router, logs := newRouter(t, db)

			start := time.Now()
			resp := serve(t, router, request(t, http.MethodGet, "/readyz", http.NoBody))

			assert.Less(t, time.Since(start), time.Second, "the readiness check must be bounded by its timeout")
			assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
			problem := decodeJSON(t, resp)
			assert.Equal(t, "about:blank", problem["type"])
			assert.Equal(t, "Service Unavailable", problem["title"])
			assert.Equal(t, resp.Header.Get("X-Request-ID"), problem["requestId"])
			assert.NotContains(t, problem["detail"], "10.0.0.7", "internal error details stay in the logs")
			assert.Contains(t, logs.String(), "readiness check failed")
		})
	}
}

func TestRequestID_GeneratedAsUUIDv7WhenAbsent(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})

	resp := serve(t, router, request(t, http.MethodGet, "/healthz", http.NoBody))

	id, err := uuid.Parse(resp.Header.Get("X-Request-ID"))
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), id.Version())
}

func TestRequestID_ReusesAWellFormedIncomingID(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})
	req := request(t, http.MethodGet, "/healthz", http.NoBody)
	req.Header.Set("X-Request-ID", "proxy-4f2a.9_b")

	resp := serve(t, router, req)

	assert.Equal(t, "proxy-4f2a.9_b", resp.Header.Get("X-Request-ID"))
}

func TestRequestID_ReplacesAMalformedIncomingID(t *testing.T) {
	for name, incoming := range map[string]string{
		"control characters": "abc\ninjected=1",
		"quotes and spaces":  `a" b`,
		"too long":           strings.Repeat("a", 65),
	} {
		t.Run(name, func(t *testing.T) {
			router, logs := newRouter(t, fakeDB{})
			req := request(t, http.MethodGet, "/healthz", http.NoBody)
			req.Header.Set("X-Request-ID", incoming)

			resp := serve(t, router, req)

			id, err := uuid.Parse(resp.Header.Get("X-Request-ID"))
			require.NoError(t, err)
			assert.Equal(t, uuid.Version(7), id.Version())
			assert.NotContains(t, logs.String(), "injected")
		})
	}
}

func TestAccessLog_CarriesTheRequestIDAndNoPersonalData(t *testing.T) {
	router, logs := newRouter(t, fakeDB{})
	req := request(t, http.MethodGet, "/api/v1/info?token=s3cr3t-value", http.NoBody)
	req.RemoteAddr = "203.0.113.9:51234"
	req.Header.Set("User-Agent", "curl/8.0")

	resp := serve(t, router, req)

	var record map[string]any
	for line := range strings.Lines(logs.String()) {
		var candidate map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &candidate))
		if candidate["msg"] == "http request" {
			record = candidate
		}
	}
	require.NotNil(t, record, "no access log record in %s", logs.String())
	assert.Equal(t, resp.Header.Get("X-Request-ID"), record["request_id"])
	assert.Equal(t, "GET", record["method"])
	assert.Equal(t, "/api/v1/info", record["route"])
	assert.InDelta(t, http.StatusOK, record["status"], 0)
	assert.Contains(t, record, "duration_ms")
	for _, leaked := range []string{"s3cr3t-value", "203.0.113.9", "curl/8.0"} {
		assert.NotContains(t, logs.String(), leaked)
	}
}
