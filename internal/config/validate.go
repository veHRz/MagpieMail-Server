package config

import (
	"fmt"
	"net"
	"net/url"
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

func checkPositive(d time.Duration) string {
	if d <= 0 {
		return "must be greater than zero"
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
