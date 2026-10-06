// Package config loads and validates the server configuration.
//
// Settings are layered, each layer overriding the previous one:
// built-in defaults, then an optional YAML file, then MAGPIE_* environment
// variables. Environment variable names map to keys by dropping the prefix,
// lower-casing, and using "__" as the level separator:
// MAGPIE_SERVER__READ_TIMEOUT sets server.read_timeout.
//
// Loading is strict: unknown keys and unknown MAGPIE_* variables are errors, so
// that a typo can never be silently ignored. Every problem is reported at once,
// naming the faulty key and where its value came from.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

const (
	// EnvPrefix starts every environment variable that sets a configuration key.
	// The whole MAGPIE_ namespace is reserved for the server.
	EnvPrefix = "MAGPIE_"
	// EnvConfigFile names the environment variable holding the YAML file path.
	// It selects the file and is not itself a configuration key.
	EnvConfigFile = "MAGPIE_CONFIG"

	envLevelSeparator = "__"
	keyDelimiter      = "."
)

// Config is the complete, validated server configuration.
type Config struct {
	Server   ServerConfig   `koanf:"server"`
	Log      LogConfig      `koanf:"log"`
	Database DatabaseConfig `koanf:"database"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	// Listen is the TCP address the HTTP server binds to, as host:port.
	Listen            string        `koanf:"listen"`
	ReadHeaderTimeout time.Duration `koanf:"read_header_timeout"`
	ReadTimeout       time.Duration `koanf:"read_timeout"`
	WriteTimeout      time.Duration `koanf:"write_timeout"`
	IdleTimeout       time.Duration `koanf:"idle_timeout"`
	// ShutdownTimeout bounds how long in-flight requests may run after a stop signal.
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// DatabaseConfig configures the PostgreSQL connection.
type DatabaseConfig struct {
	// URL is a PostgreSQL connection URL. It usually embeds a password.
	URL Secret `koanf:"url"`
}

// Source tells Load where to read configuration from.
type Source struct {
	// File is the path of an optional YAML configuration file.
	File string
	// Environ is the process environment in "KEY=value" form, usually os.Environ().
	Environ []string
}

func defaults() map[string]any {
	return map[string]any{
		"server.listen":              ":8080",
		"server.read_header_timeout": "5s",
		"server.read_timeout":        "30s",
		"server.write_timeout":       "30s",
		"server.idle_timeout":        "120s",
		"server.shutdown_timeout":    "15s",
		"log.level":                  "info",
		"log.format":                 "json",
	}
}

// Load reads, merges, decodes and validates the configuration. On failure it
// returns a *ValidationError listing every problem, or an error naming the
// configuration file when that file cannot be read or parsed.
func Load(src Source) (Config, error) {
	schema := schemaOf(reflect.TypeFor[Config]())
	origins := make(map[string]string)
	var found problems

	k := koanf.New(keyDelimiter)
	if err := k.Load(confmap.Provider(defaults(), keyDelimiter), nil); err != nil {
		return Config{}, fmt.Errorf("loading configuration defaults: %w", err)
	}

	if src.File != "" {
		values, err := readFile(src.File, schema, &found)
		if err != nil {
			return Config{}, err
		}
		if err := k.Load(confmap.Provider(values, keyDelimiter), nil); err != nil {
			return Config{}, fmt.Errorf("loading configuration file %q: %w", src.File, err)
		}
		for key := range values {
			origins[key] = src.File
		}
	}

	envValues := readEnviron(src.Environ, schema, &found, origins)
	if err := k.Load(confmap.Provider(envValues, keyDelimiter), nil); err != nil {
		return Config{}, fmt.Errorf("loading configuration from the environment: %w", err)
	}

	var cfg Config
	err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook:       durationHook,
			WeaklyTypedInput: true,
		},
	})
	for key, message := range decodeProblems(err, schema) {
		found.add(key, origins[key], message)
	}

	cfg.validate(&found, origins)

	if err := found.err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// readFile parses the YAML file and returns the values of known keys. Unknown
// keys are recorded as problems.
func readFile(path string, schema schema, found *problems) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading configuration file %q: %w", path, err)
	}

	fk := koanf.New(keyDelimiter)
	if err := fk.Load(rawbytes.Provider(data), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("parsing configuration file %q: %w", path, err)
	}

	values := make(map[string]any)
	for _, key := range fk.Keys() {
		switch {
		case schema.leaves[key] != nil:
			values[key] = fk.Get(key)
		case schema.sections[key]:
			found.add(key, path, "must be a mapping of settings")
		default:
			found.add(key, path, "unknown configuration key")
		}
	}
	return values, nil
}

// readEnviron returns the values set by MAGPIE_* variables. Unknown variables
// are recorded as problems.
func readEnviron(environ []string, schema schema, found *problems, origins map[string]string) map[string]any {
	values := make(map[string]any)
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, EnvPrefix) || name == EnvConfigFile {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(name, EnvPrefix), envLevelSeparator, keyDelimiter))
		if schema.leaves[key] == nil {
			found.add(name, "", fmt.Sprintf("unknown configuration variable (no setting named %q)", key))
			continue
		}
		values[key] = value
		origins[key] = name
	}
	return values
}

var errNotDuration = errors.New(`must be a duration with a unit, such as "30s" or "2m"`)

// durationHook parses durations from strings only. Bare numbers are rejected:
// mapstructure would otherwise read "30" as 30 nanoseconds.
func durationHook(_, to reflect.Type, data any) (any, error) {
	if to != reflect.TypeFor[time.Duration]() {
		return data, nil
	}
	s, ok := data.(string)
	if !ok {
		return nil, errNotDuration
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, errNotDuration
	}
	return d, nil
}

// decodeProblems turns mapstructure errors into messages keyed by setting.
// Messages for secret settings never include the underlying error, which may
// quote the value.
func decodeProblems(err error, schema schema) map[string]string {
	out := make(map[string]string)
	var visit func(error)
	visit = func(err error) {
		if err == nil {
			return
		}
		if de, ok := err.(*mapstructure.DecodeError); ok && !containsDecodeError(de.Unwrap()) { //nolint:errorlint // Walking the tree by hand.
			key := de.Name()
			switch {
			case errors.Is(de, errNotDuration):
				out[key] = errNotDuration.Error()
			case schema.leaves[key] == reflect.TypeFor[Secret]():
				out[key] = "has an invalid value"
			default:
				out[key] = "has an invalid value: " + de.Unwrap().Error()
			}
			return
		}
		switch e := err.(type) { //nolint:errorlint // Walking the tree by hand.
		case interface{ Unwrap() []error }:
			for _, inner := range e.Unwrap() {
				visit(inner)
			}
		case interface{ Unwrap() error }:
			visit(e.Unwrap())
		default:
			out[""] = err.Error()
		}
	}
	visit(err)
	return out
}

func containsDecodeError(err error) bool {
	var de *mapstructure.DecodeError
	return errors.As(err, &de)
}

// schema lists the valid keys of a configuration struct.
type schema struct {
	leaves   map[string]reflect.Type // setting key -> Go type
	sections map[string]bool         // keys that group other settings
}

func schemaOf(t reflect.Type) schema {
	s := schema{leaves: make(map[string]reflect.Type), sections: make(map[string]bool)}
	var walk func(prefix string, t reflect.Type)
	walk = func(prefix string, t reflect.Type) {
		for i := range t.NumField() {
			field := t.Field(i)
			key := prefix + field.Tag.Get("koanf")
			if field.Type.Kind() == reflect.Struct {
				s.sections[key] = true
				walk(key+keyDelimiter, field.Type)
				continue
			}
			s.leaves[key] = field.Type
		}
	}
	walk("", t)
	return s
}

// LogValue implements slog.LogValuer. Secrets are redacted; the database URL
// keeps its host and database name, which help diagnose connection issues.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Group("server",
			slog.String("listen", c.Server.Listen),
			slog.String("read_header_timeout", c.Server.ReadHeaderTimeout.String()),
			slog.String("read_timeout", c.Server.ReadTimeout.String()),
			slog.String("write_timeout", c.Server.WriteTimeout.String()),
			slog.String("idle_timeout", c.Server.IdleTimeout.String()),
			slog.String("shutdown_timeout", c.Server.ShutdownTimeout.String()),
		),
		slog.Group("log",
			slog.String("level", c.Log.Level),
			slog.String("format", c.Log.Format),
		),
		slog.Group("database",
			slog.String("url", redactURL(c.Database.URL.Reveal())),
		),
	)
}
