// Package postgres is the PostgreSQL store: the connection pool, the schema
// migrations, and one repository per aggregate over the sqlc-generated queries
// of package gen. Repositories always filter on the owning user.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// errInvalidURL is deliberately static: pgx's own parse errors quote the
// connection string and only redact the password on a best-effort basis.
var errInvalidURL = errors.New("database.url: not a valid PostgreSQL connection URL")

// Open returns a connection pool without connecting: connections are made on
// first use. The server can therefore start before the database is up, and
// report the situation through its readiness probe. Timestamps are read in UTC.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errInvalidURL
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "magpie"
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name:  "timestamptz",
			OID:   pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
		})
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errInvalidURL
	}
	return pool, nil
}
