package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/veHRz/MagpieMail-Server/migrations"
)

// Migration identifies one migration file.
type Migration struct {
	Version int64
	Name    string
}

// MigrationStatus tells whether a migration is applied.
type MigrationStatus struct {
	Migration
	Applied   bool
	AppliedAt time.Time
}

// Migrator applies the embedded migrations.
type Migrator struct {
	db       *sql.DB
	provider *goose.Provider
}

// NewMigrator connects to the database (lazily) and loads the migrations.
func NewMigrator(databaseURL string) (*Migrator, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, errInvalidURL
	}
	db := stdlib.OpenDB(*cfg)
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("loading migrations: %w", err)
	}
	return &Migrator{db: db, provider: provider}, nil
}

// Up applies every pending migration and returns them.
func (m *Migrator) Up(ctx context.Context) ([]Migration, error) {
	results, err := m.provider.Up(ctx)
	return applied(results), wrapMigration(err)
}

// Down rolls back the last applied migration.
func (m *Migrator) Down(ctx context.Context) (*Migration, error) {
	result, err := m.provider.Down(ctx)
	if err != nil {
		return nil, wrapMigration(err)
	}
	migration := toMigration(result.Source)
	return &migration, nil
}

// DownAll rolls back every applied migration, newest first.
func (m *Migrator) DownAll(ctx context.Context) ([]Migration, error) {
	results, err := m.provider.DownTo(ctx, 0)
	return applied(results), wrapMigration(err)
}

// Status lists every migration and whether it is applied.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return nil, wrapMigration(err)
	}
	out := make([]MigrationStatus, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, MigrationStatus{
			Migration: toMigration(s.Source),
			Applied:   s.State == goose.StateApplied,
			AppliedAt: s.AppliedAt,
		})
	}
	return out, nil
}

// Close closes the database connections.
func (m *Migrator) Close() error { return m.db.Close() }

func applied(results []*goose.MigrationResult) []Migration {
	out := make([]Migration, 0, len(results))
	for _, r := range results {
		out = append(out, toMigration(r.Source))
	}
	return out
}

func toMigration(s *goose.Source) Migration {
	return Migration{Version: s.Version, Name: s.Path}
}

func wrapMigration(err error) error {
	if err == nil {
		return nil
	}
	var partial *goose.PartialError
	if errors.As(err, &partial) && partial.Failed != nil {
		return fmt.Errorf("migration %d (%s) failed: %w", partial.Failed.Source.Version, partial.Failed.Source.Path, partial.Err)
	}
	return fmt.Errorf("migrating: %w", err)
}
