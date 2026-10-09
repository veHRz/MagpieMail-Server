//go:build integration

package pgtest

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
)

const templateDB = "magpie_migrated_template"

// shared is one PostgreSQL container per test binary, removed by
// Testcontainers' reaper when the binary exits.
var shared struct {
	once     sync.Once
	adminURL string // the "postgres" maintenance database
	err      error
}

func sharedServer(t testing.TB) string {
	t.Helper()
	shared.once.Do(func() {
		ctx := context.Background()
		ctr, err := tcpostgres.Run(ctx, Image,
			tcpostgres.WithDatabase("postgres"),
			tcpostgres.WithUsername("magpie"),
			tcpostgres.WithPassword("magpie"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			shared.err = err
			return
		}
		shared.adminURL, shared.err = ctr.ConnectionString(ctx, "sslmode=disable")
		if shared.err == nil {
			shared.err = createTemplate(ctx, shared.adminURL)
		}
	})
	if shared.err != nil {
		t.Fatalf("starting the shared PostgreSQL: %v", shared.err)
	}
	return shared.adminURL
}

// createTemplate builds a database with every migration applied, copied by
// NewMigratedDatabase: copying is much faster than migrating each time.
func createTemplate(ctx context.Context, adminURL string) error {
	if err := exec(ctx, adminURL, "CREATE DATABASE "+templateDB); err != nil {
		return err
	}
	m, err := postgres.NewMigrator(withDatabase(adminURL, templateDB))
	if err != nil {
		return err
	}
	_, err = m.Up(ctx)
	if closeErr := m.Close(); err == nil {
		err = closeErr
	}
	return err
}

// NewDatabase returns the URL of a new, empty database on the shared server.
func NewDatabase(t testing.TB) string {
	t.Helper()
	return newDatabase(t, "")
}

// NewMigratedDatabase returns the URL of a new database with every migration
// applied.
func NewMigratedDatabase(t testing.TB) string {
	t.Helper()
	return newDatabase(t, " TEMPLATE "+templateDB)
}

func newDatabase(t testing.TB, template string) string {
	t.Helper()
	adminURL := sharedServer(t)
	name := "t_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := exec(t.Context(), adminURL, "CREATE DATABASE "+name+template); err != nil {
		t.Fatalf("creating database %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = exec(context.Background(), adminURL, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return withDatabase(adminURL, name)
}

func exec(ctx context.Context, url, sql string) error {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, sql); err != nil {
		return fmt.Errorf("%s: %w", sql, err)
	}
	return nil
}

func withDatabase(serverURL, name string) string {
	u, err := url.Parse(serverURL)
	if err != nil {
		panic(err) // built by Testcontainers: always valid
	}
	u.Path = "/" + name
	return u.String()
}
