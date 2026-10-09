package config_test

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/config"
	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
)

func TestLoad_StorageDefaults(t *testing.T) {
	cfg, err := config.Load(config.Source{Environ: []string{"MAGPIE_DATABASE__URL=" + validDBURL}})
	require.NoError(t, err)

	assert.Equal(t, "/var/lib/magpie/blobs", cfg.Storage.Path)
	assert.True(t, cfg.Storage.EncryptBlobs)
	assert.Equal(t, time.Hour, cfg.Storage.PurgeGrace)
}

func TestKeyring_FromTheEnvironment(t *testing.T) {
	current, previous := envelope.GenerateKey(), envelope.GenerateKey()
	cfg, err := config.Load(config.Source{Environ: []string{
		"MAGPIE_DATABASE__URL=" + validDBURL,
		"MAGPIE_SECURITY__MASTER_KEY=" + current,
		"MAGPIE_SECURITY__PREVIOUS_MASTER_KEYS=" + previous,
	}})
	require.NoError(t, err)

	ring, err := cfg.Security.Keyring()
	require.NoError(t, err)
	currentKey, err := envelope.ParseKey(current)
	require.NoError(t, err)
	assert.Equal(t, currentKey.ID(), ring.CurrentID())

	previousKey, err := envelope.ParseKey(previous)
	require.NoError(t, err)
	_, wrapped, err := envelope.NewKeyring(previousKey).Wrap(envelope.NewRootKey(), nil)
	require.NoError(t, err)
	_, err = ring.Unwrap(previousKey.ID(), wrapped, nil)
	require.NoError(t, err, "previous keys unwrap during a rotation")
}

func TestKeyring_FromASecretFile(t *testing.T) {
	key := envelope.GenerateKey()
	path := filepath.Join(t.TempDir(), "master_key")
	require.NoError(t, os.WriteFile(path, []byte(key+"\n"), 0o600))

	cfg, err := config.Load(config.Source{Environ: []string{
		"MAGPIE_DATABASE__URL=" + validDBURL,
		"MAGPIE_SECURITY__MASTER_KEY_FILE=" + path,
	}})
	require.NoError(t, err)

	ring, err := cfg.Security.Keyring()
	require.NoError(t, err)
	parsed, err := envelope.ParseKey(key)
	require.NoError(t, err)
	assert.Equal(t, parsed.ID(), ring.CurrentID())
}

func TestKeyring_RequiresAMasterKey(t *testing.T) {
	cfg, err := config.Load(config.Source{Environ: []string{"MAGPIE_DATABASE__URL=" + validDBURL}})
	require.NoError(t, err, "loading succeeds: only commands that use the key require it")

	_, err = cfg.Security.Keyring()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "security.master_key")
}

func TestLoad_InvalidSecuritySettingsNeverRevealKeys(t *testing.T) {
	dbURL := "MAGPIE_DATABASE__URL=" + validDBURL
	missing := filepath.Join(t.TempDir(), "absent")
	// Well-formed base64 of the wrong length: valid-looking but invalid keys.
	shortKey := base64.StdEncoding.EncodeToString([]byte("short-secret-key"))
	notAKey := base64.StdEncoding.EncodeToString([]byte("not-a-key-at-all"))
	tests := map[string]struct {
		environ  []string
		contains []string
		secret   string
	}{
		"malformed master key": {
			environ:  []string{dbURL, "MAGPIE_SECURITY__MASTER_KEY=" + shortKey},
			contains: []string{"security.master_key", "base64"},
			secret:   shortKey,
		},
		"malformed previous key": {
			environ: []string{
				dbURL, "MAGPIE_SECURITY__MASTER_KEY=" + envelope.GenerateKey(),
				"MAGPIE_SECURITY__PREVIOUS_MASTER_KEYS=" + notAKey,
			},
			contains: []string{"security.previous_master_keys"},
			secret:   notAKey,
		},
		"both key and key file": {
			environ: []string{
				dbURL, "MAGPIE_SECURITY__MASTER_KEY=" + envelope.GenerateKey(),
				"MAGPIE_SECURITY__MASTER_KEY_FILE=/run/secrets/master_key",
			},
			contains: []string{"security.master_key_file", "not both"},
		},
		"unreadable key file": {
			environ:  []string{dbURL, "MAGPIE_SECURITY__MASTER_KEY_FILE=" + missing},
			contains: []string{"security.master_key_file", missing},
		},
		"relative storage path": {
			environ:  []string{dbURL, "MAGPIE_STORAGE__PATH=blobs"},
			contains: []string{"storage.path", "absolute"},
		},
		"negative purge grace": {
			environ:  []string{dbURL, "MAGPIE_STORAGE__PURGE_GRACE=-1m"},
			contains: []string{"storage.purge_grace", "zero or more"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := config.Load(config.Source{Environ: tt.environ})
			require.Error(t, err)
			for _, want := range tt.contains {
				assert.Contains(t, err.Error(), want)
			}
			if tt.secret != "" {
				assert.NotContains(t, err.Error(), tt.secret)
			}
		})
	}
}

func TestConfig_NeverPrintsMasterKeys(t *testing.T) {
	current, previous := envelope.GenerateKey(), envelope.GenerateKey()
	cfg, err := config.Load(config.Source{Environ: []string{
		"MAGPIE_DATABASE__URL=" + validDBURL,
		"MAGPIE_SECURITY__MASTER_KEY=" + current,
		"MAGPIE_SECURITY__PREVIOUS_MASTER_KEYS=" + previous,
	}})
	require.NoError(t, err)

	for _, out := range []string{fmt.Sprintf("%v", cfg), fmt.Sprintf("%+v", cfg), fmt.Sprintf("%#v", cfg), logged(t, cfg)} {
		assert.NotContains(t, out, current)
		assert.NotContains(t, out, previous)
	}
}
