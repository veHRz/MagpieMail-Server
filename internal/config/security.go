package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
)

// errNoMasterKey tells the operator how to provide the master key.
var errNoMasterKey = errors.New("security.master_key is required: set MAGPIE_SECURITY__MASTER_KEY " +
	"or security.master_key_file (generate a key with `magpie admin generate-key`)")

// Keyring returns the master keyring. Commands that handle secrets call it;
// it fails when no master key is configured.
func (s SecurityConfig) Keyring() (*envelope.Keyring, error) {
	if s.MasterKey == "" {
		return nil, errNoMasterKey
	}
	current, err := envelope.ParseKey(s.MasterKey.Reveal())
	if err != nil {
		return nil, fmt.Errorf("security.master_key: %w", err)
	}
	previous := make([]envelope.Key, 0, len(s.PreviousMasterKeys))
	for i, encoded := range s.PreviousMasterKeys {
		k, err := envelope.ParseKey(encoded.Reveal())
		if err != nil {
			return nil, fmt.Errorf("security.previous_master_keys[%d]: %w", i, err)
		}
		previous = append(previous, k)
	}
	return envelope.NewKeyring(current, previous...), nil
}

// readKeyFile loads the master key from security.master_key_file, if set.
func (s *SecurityConfig) readKeyFile(found *problems, origins map[string]string) {
	const key = "security.master_key_file"
	if s.MasterKeyFile == "" {
		return
	}
	if s.MasterKey != "" {
		found.add(key, origins[key], "set security.master_key or security.master_key_file, not both")
		return
	}
	data, err := os.ReadFile(s.MasterKeyFile)
	if err != nil {
		found.add(key, origins[key], fmt.Sprintf("cannot read %q: %v", s.MasterKeyFile, errors.Unwrap(err)))
		return
	}
	s.MasterKey = Secret(strings.TrimSpace(string(data)))
}

// checkMasterKey validates the format of configured keys, never quoting them.
func (s SecurityConfig) checkMasterKey() string {
	if s.MasterKey == "" {
		return ""
	}
	if _, err := envelope.ParseKey(s.MasterKey.Reveal()); err != nil {
		return err.Error()
	}
	return ""
}

func (s SecurityConfig) checkPreviousMasterKeys() string {
	for i, encoded := range s.PreviousMasterKeys {
		if _, err := envelope.ParseKey(encoded.Reveal()); err != nil {
			return fmt.Sprintf("entry %d: %v", i, err)
		}
	}
	return ""
}
