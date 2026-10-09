package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres/gen"
)

var (
	// ErrNotFound means the row does not exist or belongs to another user.
	ErrNotFound = errors.New("not found")
	// ErrConflict means a unique constraint refused the write.
	ErrConflict = errors.New("already exists")
	// ErrNoKeyring means a secret was handled by a store built without the
	// master keyring.
	ErrNoKeyring = errors.New("no master keyring: security.master_key is not configured")
	// ErrNestedTx means InTx was called inside a transaction.
	ErrNestedTx = errors.New("nested transactions are not supported")
)

// Store gives access to the repositories, on the pool or inside a transaction.
type Store struct {
	pool *pgxpool.Pool // nil inside a transaction
	q    *gen.Queries
	ring *envelope.Keyring
}

// NewStore returns a store on pool. ring may be nil for callers that never
// touch secrets.
func NewStore(pool *pgxpool.Pool, ring *envelope.Keyring) *Store {
	return &Store{pool: pool, q: gen.New(pool), ring: ring}
}

// InTx runs fn in a transaction, committed if fn returns nil and rolled back
// otherwise. fn receives a store whose repositories run in the transaction.
func (s *Store) InTx(ctx context.Context, fn func(tx *Store) error) error {
	if s.pool == nil {
		return ErrNestedTx
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning a transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := fn(&Store{q: s.q.WithTx(tx), ring: s.ring}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	return nil
}

// newID returns a UUIDv7: time-ordered, so new rows are appended to indexes.
func newID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

// translate maps driver errors to the store's errors.
func translate(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
		case "23503": // foreign_key_violation: the referenced row does not exist
			return fmt.Errorf("%w: %s", ErrNotFound, pgErr.ConstraintName)
		}
	}
	return err
}
