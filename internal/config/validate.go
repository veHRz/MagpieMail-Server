package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	logLevels  = []string{"debug", "info", "warn", "error"}
	logFormats = []string{"json", "text"}
)

// validate records a problem for every setting with an invalid value. Keys that
// already failed to decode are skipped so that each mistake is reported once.
func (c Config) validate(found *problems, origins map[string]string) {
	check := func(key, message string) {
		if message != "" && !found.has(key) {
			found.add(key, origins[key], message)
		}
	}

	check("server.listen", checkListen(c.Server.Listen))
	check("server.read_header_timeout", checkPositive(c.Server.ReadHeaderTimeout))
	check("server.read_timeout", checkPositive(c.Server.ReadTimeout))
	check("server.write_timeout", checkPositive(c.Server.WriteTimeout))
	check("server.idle_timeout", checkPositive(c.Server.IdleTimeout))
	check("server.shutdown_timeout", checkPositive(c.Server.ShutdownTimeout))
	check("server.max_body_bytes", checkPositive(c.Server.MaxBodyBytes))
	check("server.trusted_proxies", checkEach(c.Server.TrustedProxies, checkAddressOrPrefix))
	check("server.hsts_max_age", checkNotNegative(c.Server.HSTSMaxAge))
	check("server.cors.allowed_origins", checkEach(c.Server.CORS.AllowedOrigins, checkOrigin))
	if c.Server.RateLimit.Enabled {
		check("server.rate_limit.per_ip.rate", checkPositive(c.Server.RateLimit.PerIP.Rate))
		check("server.rate_limit.per_ip.burst", checkAtLeastOne(c.Server.RateLimit.PerIP.Burst))
		check("server.rate_limit.per_user.rate", checkPositive(c.Server.RateLimit.PerUser.Rate))
		check("server.rate_limit.per_user.burst", checkAtLeastOne(c.Server.RateLimit.PerUser.Burst))
	}
	if !found.has("security.master_key_file") {
		check("security.master_key", c.Security.checkMasterKey())
	}
	check("security.previous_master_keys", c.Security.checkPreviousMasterKeys())
	check("storage.path", checkAbsolutePath(c.Storage.Path))
	check("storage.purge_grace", checkNotNegative(c.Storage.PurgeGrace))
	check("log.level", checkOneOf(c.Log.Level, logLevels))
	check("log.format", checkOneOf(c.Log.Format, logFormats))
	check("database.url", checkPostgresURL(c.Database.URL.Reveal()))
}

func checkListen(addr string) string {
	if addr == "" {
		return "is required"
	}
	_, port, err := net.SplitHostPort(addr)
	if err == nil {
		_, err = strconv.ParseUint(port, 10, 16)
	}
	if err != nil {
		return fmt.Sprintf(`must be a host:port address such as ":8080" or "127.0.0.1:8080", got %q`, addr)
	}
	return ""
}

func checkPositive[N time.Duration | int64 | float64](n N) string {
	if n <= 0 {
		return "must be greater than zero"
	}
	return ""
}

func checkNotNegative(d time.Duration) string {
	if d < 0 {
		return "must be zero or more"
	}
	return ""
}

func checkAtLeastOne(n int) string {
	if n < 1 {
		return "must be at least 1"
	}
	return ""
}

// checkEach applies check to every item and reports the first invalid one.
func checkEach(items []string, check func(string) string) string {
	for _, item := range items {
		if message := check(item); message != "" {
			return message
		}
	}
	return ""
}

func checkAddressOrPrefix(s string) string {
	if _, err := netip.ParsePrefix(s); err == nil {
		return ""
	}
	if _, err := netip.ParseAddr(s); err == nil {
		return ""
	}
	return fmt.Sprintf(`must list IP addresses or CIDR prefixes such as "10.0.0.0/8", got %q`, s)
}

// checkOrigin accepts a browser origin. A wildcard is refused: the API allows
// credentials, which browsers never send to a wildcard origin.
func checkOrigin(s string) string {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Sprintf("must list origins as scheme://host[:port], got %q", s)
	}
	return ""
}

func checkOneOf(value string, allowed []string) string {
	if !slices.Contains(allowed, value) {
		return fmt.Sprintf("must be one of %s, got %q", strings.Join(allowed, ", "), value)
	}
	return ""
}

// checkPostgresURL never quotes the value nor the parser error: both may
// contain the password.
func checkPostgresURL(raw string) string {
	if raw == "" {
		return "is required"
	}
	u, err := url.Parse(raw)
	if err == nil && u.Port() != "" {
		_, err = strconv.ParseUint(u.Port(), 10, 16)
	}
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return `must be a PostgreSQL URL such as "postgres://user:password@host:5432/dbname"`
	}
	return ""
}

// redactURL hides the password of a connection URL, including password
// parameters in the query string. Unparsable values are hidden entirely.
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return redacted
	}
	query := u.Query()
	for name := range query {
		if strings.Contains(strings.ToLower(name), "password") {
			query.Set(name, "xxxxx")
		}
	}
	u.RawQuery = query.Encode()
	return u.Redacted()
}

func checkAbsolutePath(p string) string {
	if !filepath.IsAbs(p) {
		return fmt.Sprintf("must be an absolute path, got %q", p)
	}
	return ""
}
