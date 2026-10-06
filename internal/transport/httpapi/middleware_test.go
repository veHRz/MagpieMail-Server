package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/config"
)

func limitTo(burst int) routerOption {
	return withConfig(func(c *config.ServerConfig) {
		c.RateLimit.PerIP = config.LimitConfig{Rate: 0.001, Burst: burst}
	})
}

func statusOf(t *testing.T, h http.Handler, req *http.Request) int {
	t.Helper()
	resp := serve(t, h, req)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func fromAddr(t *testing.T, remoteAddr string) *http.Request {
	t.Helper()
	req := request(t, http.MethodGet, "/api/v1/info", http.NoBody)
	req.RemoteAddr = remoteAddr
	return req
}

func TestRateLimit_EachClientAddressHasItsOwnBucket(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, limitTo(1))

	assert.Equal(t, http.StatusOK, statusOf(t, router, fromAddr(t, "192.0.2.1:1000")))
	assert.Equal(t, http.StatusTooManyRequests, statusOf(t, router, fromAddr(t, "192.0.2.1:2000")), "same address, other port")
	assert.Equal(t, http.StatusOK, statusOf(t, router, fromAddr(t, "192.0.2.2:1000")))
}

func TestRateLimit_ProbesAreExempt(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, limitTo(1))

	for range 5 {
		assert.Equal(t, http.StatusOK, statusOf(t, router, request(t, http.MethodGet, "/healthz", http.NoBody)))
		assert.Equal(t, http.StatusOK, statusOf(t, router, request(t, http.MethodGet, "/readyz", http.NoBody)))
	}
}

func TestRateLimit_CanBeDisabled(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, limitTo(1), withConfig(func(c *config.ServerConfig) {
		c.RateLimit.Enabled = false
	}))

	for range 5 {
		assert.Equal(t, http.StatusOK, statusOf(t, router, fromAddr(t, "192.0.2.1:1000")))
	}
}

func TestRateLimit_PerUserAcrossAddresses(t *testing.T) {
	h, _ := newTestChain(t, withConfig(func(c *config.ServerConfig) {
		c.RateLimit.PerUser = config.LimitConfig{Rate: 0.001, Burst: 2}
	}))
	protected := func(remoteAddr, token string) int {
		req := request(t, http.MethodGet, "/test/protected", http.NoBody)
		req.RemoteAddr = remoteAddr
		req.Header.Set("Authorization", "Bearer "+token)
		resp := serveChain(t, h, req)
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	assert.Equal(t, http.StatusOK, protected("192.0.2.1:1000", "good-token"))
	assert.Equal(t, http.StatusOK, protected("198.51.100.7:1000", "good-token"))
	assert.Equal(t, http.StatusTooManyRequests, protected("203.0.113.5:1000", "good-token"),
		"the user's bucket is shared by all their addresses")
}

func TestRateLimit_TrustedProxyRevealsTheClientAddress(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, limitTo(1), withConfig(func(c *config.ServerConfig) {
		c.TrustedProxies = []string{"10.0.0.0/8"}
	}))
	viaProxy := func(forwardedFor string) int {
		req := fromAddr(t, "10.0.0.2:5000")
		req.Header.Set("X-Forwarded-For", forwardedFor)
		return statusOf(t, router, req)
	}

	assert.Equal(t, http.StatusOK, viaProxy("192.0.2.1"))
	assert.Equal(t, http.StatusOK, viaProxy("192.0.2.2"), "another client behind the same proxy")
	assert.Equal(t, http.StatusTooManyRequests, viaProxy("192.0.2.1"))
	assert.Equal(t, http.StatusTooManyRequests, viaProxy("198.51.100.9, 192.0.2.1"),
		"only the hop appended by the trusted proxy counts; earlier hops are client-controlled")
}

func TestRateLimit_UntrustedForwardedForIsIgnored(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, limitTo(1))
	spoof := func(forwardedFor string) int {
		req := fromAddr(t, "192.0.2.1:1000")
		req.Header.Set("X-Forwarded-For", forwardedFor)
		return statusOf(t, router, req)
	}

	assert.Equal(t, http.StatusOK, spoof("198.51.100.1"))
	assert.Equal(t, http.StatusTooManyRequests, spoof("198.51.100.2"), "a direct client cannot pick its address")
}

func TestSecurityHeaders_OnEveryResponse(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})
	for _, path := range []string{"/api/v1/info", "/healthz", "/api/v1/nothing-here"} {
		resp := serve(t, router, request(t, http.MethodGet, path, http.NoBody))
		_ = resp.Body.Close()

		assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), path)
		assert.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"), path)
		assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", resp.Header.Get("Content-Security-Policy"), path)
		assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"), path)
		assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"), path)
		assert.Empty(t, resp.Header.Get("Strict-Transport-Security"), "HSTS is off by default")
	}
}

func TestSecurityHeaders_HSTSWhenConfigured(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, withConfig(func(c *config.ServerConfig) {
		c.HSTSMaxAge = 365 * 24 * time.Hour
	}))

	resp := serve(t, router, request(t, http.MethodGet, "/api/v1/info", http.NoBody))
	defer resp.Body.Close()

	assert.Equal(t, "max-age=31536000", resp.Header.Get("Strict-Transport-Security"))
}

func TestCORS_DisabledByDefault(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})
	req := request(t, http.MethodGet, "/api/v1/info", http.NoBody)
	req.Header.Set("Origin", "https://mail.example.com")

	resp := serve(t, router, req)
	defer resp.Body.Close()

	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowsConfiguredOriginsWithCredentials(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, withConfig(func(c *config.ServerConfig) {
		c.CORS.AllowedOrigins = []string{"https://mail.example.com"}
	}))

	allowed := request(t, http.MethodGet, "/api/v1/info", http.NoBody)
	allowed.Header.Set("Origin", "https://mail.example.com")
	resp := serve(t, router, allowed)
	defer resp.Body.Close()
	assert.Equal(t, "https://mail.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, resp.Header.Get("Access-Control-Expose-Headers"), "X-Request-Id")

	other := request(t, http.MethodGet, "/api/v1/info", http.NoBody)
	other.Header.Set("Origin", "https://evil.example")
	resp = serve(t, router, other)
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORS_AnswersPreflightRequests(t *testing.T) {
	router, _ := newRouter(t, fakeDB{}, withConfig(func(c *config.ServerConfig) {
		c.CORS.AllowedOrigins = []string{"https://mail.example.com"}
	}))
	req := request(t, http.MethodOptions, "/api/v1/info", http.NoBody)
	req.Header.Set("Origin", "https://mail.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	// Browsers send the requested headers lower-cased and sorted (Fetch standard).
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")

	resp := httpRecord(router, req)

	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "https://mail.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "authorization")
}
