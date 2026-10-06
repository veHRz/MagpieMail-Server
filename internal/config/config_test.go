package config_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/config"
)

const (
	testPassword = "hunter2-do-not-leak"
	validDBURL   = "postgres://magpie:" + testPassword + "@db.internal:5432/magpie?sslmode=disable"
)

// writeFile writes a YAML configuration file in a temporary directory and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoad_AppliesDefaults(t *testing.T) {
	cfg, err := config.Load(config.Source{Environ: []string{"MAGPIE_DATABASE__URL=" + validDBURL}})
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.Server.Listen)
	assert.Equal(t, 5*time.Second, cfg.Server.ReadHeaderTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.WriteTimeout)
	assert.Equal(t, 120*time.Second, cfg.Server.IdleTimeout)
	assert.Equal(t, 15*time.Second, cfg.Server.ShutdownTimeout)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
	assert.Equal(t, validDBURL, cfg.Database.URL.Reveal())
}

func TestLoad_FileOverridesDefaults(t *testing.T) {
	path := writeFile(t, `
server:
  listen: "127.0.0.1:9000"
  read_timeout: 45s
log:
  level: debug
  format: text
database:
  url: "`+validDBURL+`"
`)

	cfg, err := config.Load(config.Source{File: path})
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9000", cfg.Server.Listen)
	assert.Equal(t, 45*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.WriteTimeout, "keys absent from the file keep their default")
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "text", cfg.Log.Format)
}

func TestLoad_EnvironmentOverridesFile(t *testing.T) {
	path := writeFile(t, `
server:
  listen: "127.0.0.1:9000"
log:
  level: debug
`)

	cfg, err := config.Load(config.Source{
		File: path,
		Environ: []string{
			"MAGPIE_SERVER__LISTEN=0.0.0.0:7000",
			"MAGPIE_SERVER__SHUTDOWN_TIMEOUT=1m",
			"MAGPIE_DATABASE__URL=" + validDBURL,
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0:7000", cfg.Server.Listen)
	assert.Equal(t, time.Minute, cfg.Server.ShutdownTimeout)
	assert.Equal(t, "debug", cfg.Log.Level, "keys absent from the environment keep the file value")
}

func TestLoad_IgnoresUnrelatedVariables(t *testing.T) {
	_, err := config.Load(config.Source{Environ: []string{
		"PATH=/usr/bin",
		"HOME=/home/magpie",
		"MAGPIEMAIL_DB_PASSWORD=not-a-server-setting",
		"MAGPIE_CONFIG=/etc/magpie/config.yaml",
		"MAGPIE_DATABASE__URL=" + validDBURL,
	}})
	require.NoError(t, err)
}

// TestLoad_InvalidConfigurationNamesTheKey is the proof for the S0 criterion
// "an invalid configuration fails startup with a message naming the faulty key".
func TestLoad_InvalidConfigurationNamesTheKey(t *testing.T) {
	dbURL := "MAGPIE_DATABASE__URL=" + validDBURL

	tests := []struct {
		name     string
		file     string
		environ  []string
		contains []string
	}{
		{
			name:     "unknown key in file",
			file:     "server:\n  lisen: \":8080\"\n",
			environ:  []string{dbURL},
			contains: []string{`server.lisen`, "unknown configuration key", "config.yaml"},
		},
		{
			name:     "unknown variable in environment",
			environ:  []string{dbURL, "MAGPIE_SERVER__LISEN=:8080"},
			contains: []string{"MAGPIE_SERVER__LISEN", "unknown configuration variable"},
		},
		{
			name:     "section given a scalar",
			file:     "server: 8080\n",
			environ:  []string{dbURL},
			contains: []string{"server", "must be a mapping"},
		},
		{
			name:     "malformed duration in environment",
			environ:  []string{dbURL, "MAGPIE_SERVER__READ_TIMEOUT=soon"},
			contains: []string{"server.read_timeout", "MAGPIE_SERVER__READ_TIMEOUT", "duration"},
		},
		{
			name:     "duration without unit in file",
			file:     "server:\n  read_timeout: 30\n",
			environ:  []string{dbURL},
			contains: []string{"server.read_timeout", "duration"},
		},
		{
			name:     "non-positive duration",
			environ:  []string{dbURL, "MAGPIE_SERVER__IDLE_TIMEOUT=0s"},
			contains: []string{"server.idle_timeout", "greater than zero"},
		},
		{
			name:     "invalid listen address",
			environ:  []string{dbURL, "MAGPIE_SERVER__LISTEN=nope"},
			contains: []string{"server.listen", `"nope"`},
		},
		{
			name:     "listen port out of range",
			environ:  []string{dbURL, "MAGPIE_SERVER__LISTEN=:70000"},
			contains: []string{"server.listen"},
		},
		{
			name:     "unknown log level",
			environ:  []string{dbURL, "MAGPIE_LOG__LEVEL=verbose"},
			contains: []string{"log.level", "debug, info, warn, error", `"verbose"`},
		},
		{
			name:     "unknown log format",
			file:     "log:\n  format: xml\n",
			environ:  []string{dbURL},
			contains: []string{"log.format", "json, text"},
		},
		{
			name:     "missing database url",
			contains: []string{"database.url", "is required"},
		},
		{
			name:     "database url with another scheme",
			environ:  []string{"MAGPIE_DATABASE__URL=mysql://magpie:secret@db/magpie"},
			contains: []string{"database.url", "PostgreSQL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := config.Source{Environ: tt.environ}
			if tt.file != "" {
				src.File = writeFile(t, tt.file)
			}

			_, err := config.Load(src)
			require.Error(t, err)
			for _, want := range tt.contains {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestLoad_ReportsEveryProblemAtOnce(t *testing.T) {
	_, err := config.Load(config.Source{Environ: []string{
		"MAGPIE_LOG__LEVEL=verbose",
		"MAGPIE_SERVER__LISTEN=nope",
	}})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "log.level")
	assert.Contains(t, err.Error(), "server.listen")
	assert.Contains(t, err.Error(), "database.url")
}

func TestLoad_ErrorsNeverRevealTheDatabaseURL(t *testing.T) {
	for _, value := range []string{
		"mysql://magpie:" + testPassword + "@db/magpie",
		"postgres://magpie:" + testPassword + "@db:notaport/magpie",
		"postgres://magpie:" + testPassword + "@db/magpie\x7f",
	} {
		_, err := config.Load(config.Source{Environ: []string{"MAGPIE_DATABASE__URL=" + value}})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), testPassword)
	}
}

func TestLoad_FileErrorsNameThePath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	_, err := config.Load(config.Source{File: missing})
	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)

	malformed := writeFile(t, "server: [unclosed\n")
	_, err = config.Load(config.Source{File: malformed})
	require.Error(t, err)
	assert.Contains(t, err.Error(), malformed)
}

func TestConfig_NeverPrintsSecrets(t *testing.T) {
	cfg, err := config.Load(config.Source{Environ: []string{"MAGPIE_DATABASE__URL=" + validDBURL}})
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(cfg)
	require.NoError(t, err)

	var logged bytes.Buffer
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("config loaded", "config", cfg)

	for name, out := range map[string]string{
		"%v":   fmt.Sprintf("%v", cfg),
		"%+v":  fmt.Sprintf("%+v", cfg),
		"%#v":  fmt.Sprintf("%#v", cfg),
		"%s":   fmt.Sprintf("url=%s", cfg.Database.URL),
		"json": string(jsonBytes),
		"slog": logged.String(),
	} {
		assert.NotContains(t, out, testPassword, "secret leaked through %s", name)
	}
}

func TestConfig_LogValueKeepsUsefulFields(t *testing.T) {
	cfg, err := config.Load(config.Source{Environ: []string{"MAGPIE_DATABASE__URL=" + validDBURL}})
	require.NoError(t, err)

	var logged bytes.Buffer
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("config loaded", "config", cfg)

	assert.Contains(t, logged.String(), `"listen":":8080"`)
	assert.Contains(t, logged.String(), "db.internal:5432", "the database host is useful and not secret")
	assert.NotContains(t, logged.String(), testPassword)
}
