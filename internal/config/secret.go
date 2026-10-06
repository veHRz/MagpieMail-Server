package config

import "log/slog"

const redacted = "[REDACTED]"

// Secret holds a sensitive configuration value. Every formatting path (fmt, JSON,
// text marshalling, slog) prints a placeholder; only Reveal returns the value.
type Secret string

// Reveal returns the secret value. Call it only where the value is consumed.
func (s Secret) Reveal() string { return string(s) }

// String implements fmt.Stringer.
func (Secret) String() string { return redacted }

// GoString implements fmt.GoStringer, which %#v uses.
func (Secret) GoString() string { return redacted }

// MarshalText implements encoding.TextMarshaler, which encoding/json uses.
func (Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue(redacted) }
