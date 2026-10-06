// Package postgres connects to PostgreSQL. From phase S2 it also holds the
// sqlc-generated queries and the repositories built on them.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// errInvalidURL is deliberately static: pgx's own parse errors quote the
// connection string and only redact the password on a best-effort basis.
var errInvalidURL = errors.New("database.url: not a valid PostgreSQL connection URL")

// Open returns a connection pool without connecting: connections are made on
// first use. The server can therefore start before the database is up, and
// report the situation through its readiness probe.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errInvalidURL
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "magpie"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errInvalidURL
	}
	return pool, nil
}
