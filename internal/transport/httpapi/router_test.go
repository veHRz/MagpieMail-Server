package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/observability"
	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi"
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

// newRouter returns a router and the buffer receiving its JSON logs.
func newRouter(t *testing.T, db httpapi.Pinger) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger, err := observability.NewLogger(&logs, "debug", "json")
	require.NoError(t, err)
	return httpapi.NewRouter(httpapi.Deps{
		Logger:           logger,
		Database:         db,
		ReadinessTimeout: 50 * time.Millisecond,
	}), &logs
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out), "body is not JSON: %q", body)
	return out
}

func TestHealthz_ReportsTheProcessIsAlive(t *testing.T) {
	// The database being down must not make the liveness probe fail.
	router, _ := newRouter(t, fakeDB{err: errors.New("connection refused")})

	rec := serve(router, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "ok", decodeJSON(t, rec.Body.Bytes())["status"])
}

func TestReadyz_ReadyWhenTheDatabaseAnswers(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})

	rec := serve(router, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", http.NoBody))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "ok", decodeJSON(t, rec.Body.Bytes())["status"])
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
			rec := serve(router, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", http.NoBody))

			assert.Less(t, time.Since(start), time.Second, "the readiness check must be bounded by its timeout")
			assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			problem := decodeJSON(t, rec.Body.Bytes())
			assert.Equal(t, "about:blank", problem["type"])
			assert.Equal(t, "Service Unavailable", problem["title"])
			assert.InDelta(t, http.StatusServiceUnavailable, problem["status"], 0)
			assert.NotEmpty(t, problem["detail"])
			assert.NotContains(t, rec.Body.String(), "10.0.0.7", "internal error details stay in the logs")
			assert.Contains(t, logs.String(), "readiness check failed")
		})
	}
}

func TestRequestID_GeneratedAsUUIDv7WhenAbsent(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})

	rec := serve(router, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody))

	id, err := uuid.Parse(rec.Header().Get("X-Request-ID"))
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), id.Version())
}

func TestRequestID_ReusesAWellFormedIncomingID(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
	req.Header.Set("X-Request-ID", "proxy-4f2a.9_b")

	rec := serve(router, req)

	assert.Equal(t, "proxy-4f2a.9_b", rec.Header().Get("X-Request-ID"))
}

func TestRequestID_ReplacesAMalformedIncomingID(t *testing.T) {
	for name, incoming := range map[string]string{
		"control characters": "abc\ninjected=1",
		"quotes and spaces":  `a" b`,
		"too long":           strings.Repeat("a", 65),
	} {
		t.Run(name, func(t *testing.T) {
			router, logs := newRouter(t, fakeDB{})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
			req.Header.Set("X-Request-ID", incoming)

			rec := serve(router, req)

			id, err := uuid.Parse(rec.Header().Get("X-Request-ID"))
			require.NoError(t, err)
			assert.Equal(t, uuid.Version(7), id.Version())
			assert.NotContains(t, logs.String(), "injected")
		})
	}
}

func TestAccessLog_CarriesTheRequestIDAndNoPersonalData(t *testing.T) {
	router, logs := newRouter(t, fakeDB{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz?token=s3cr3t-value", http.NoBody)
	req.RemoteAddr = "203.0.113.9:51234"
	req.Header.Set("User-Agent", "curl/8.0")

	rec := serve(router, req)

	var record map[string]any
	for line := range strings.Lines(logs.String()) {
		candidate := decodeJSON(t, []byte(line))
		if candidate["msg"] == "http request" {
			record = candidate
		}
	}
	require.NotNil(t, record, "no access log record in %s", logs.String())
	assert.Equal(t, rec.Header().Get("X-Request-ID"), record["request_id"])
	assert.Equal(t, "GET", record["method"])
	assert.Equal(t, "/readyz", record["route"])
	assert.InDelta(t, http.StatusOK, record["status"], 0)
	assert.Contains(t, record, "duration_ms")
	for _, leaked := range []string{"s3cr3t-value", "203.0.113.9", "curl/8.0"} {
		assert.NotContains(t, logs.String(), leaked)
	}
}
