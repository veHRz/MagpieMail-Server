package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres/gen"
)

// Keys is the repository of per-user keys, wrapped by the master key.
type Keys struct{ s *Store }

// Keys returns the key repository.
func (s *Store) Keys() Keys { return Keys{s} }

func userKeyAAD(userID uuid.UUID) []byte { return envelope.AAD("user_keys", userID.String()) }

// ForUser returns the user's key, creating it on first use.
func (r Keys) ForUser(ctx context.Context, userID uuid.UUID) (*envelope.UserKey, error) {
	if r.s.ring == nil {
		return nil, ErrNoKeyring
	}
	row, err := r.s.q.GetUserKey(ctx, userID)
	if errors.Is(translate(err), ErrNotFound) {
		row, err = r.create(ctx, userID)
	}
	if err != nil {
		return nil, translate(err)
	}
	root, err := r.s.ring.Unwrap(row.MasterKeyID, row.WrappedKey, userKeyAAD(userID))
	if err != nil {
		return nil, fmt.Errorf("unwrapping the key of user %s: %w", userID, err)
	}
	return envelope.DeriveUserKey(root)
}

// create inserts a new root key. Two concurrent calls may race: the first
// insert wins and both read it back.
func (r Keys) create(ctx context.Context, userID uuid.UUID) (gen.UserKey, error) {
	keyID, wrapped, err := r.s.ring.Wrap(envelope.NewRootKey(), userKeyAAD(userID))
	if err != nil {
		return gen.UserKey{}, err
	}
	if err := r.s.q.InsertUserKey(ctx, gen.InsertUserKeyParams{
		UserID: userID, MasterKeyID: keyID, WrappedKey: wrapped,
	}); err != nil {
		return gen.UserKey{}, err
	}
	return r.s.q.GetUserKey(ctx, userID)
}

// CountByMasterKey tells how many user keys each master key wraps.
func (r Keys) CountByMasterKey(ctx context.Context) (map[string]int64, error) {
	rows, err := r.s.q.CountUserKeysByMasterKey(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.MasterKeyID] = row.Keys
	}
	return counts, nil
}

// RotateMasterKey rewraps, in one transaction, every user key not yet wrapped
// by the current master key, and returns how many it rewrapped. The keyring
// must still hold the previous keys. Data sealed with the user keys is not
// touched: user keys themselves do not change.
func (s *Store) RotateMasterKey(ctx context.Context) (int, error) {
	if s.ring == nil {
		return 0, ErrNoKeyring
	}
	rewrapped := 0
	err := s.InTx(ctx, func(tx *Store) error {
		rows, err := tx.q.LockUserKeys(ctx)
		if err != nil {
			return err
		}
		current := tx.ring.CurrentID()
		for _, row := range rows {
			if row.MasterKeyID == current {
				continue
			}
			aad := userKeyAAD(row.UserID)
			root, err := tx.ring.Unwrap(row.MasterKeyID, row.WrappedKey, aad)
			if err != nil {
				return fmt.Errorf("unwrapping the key of user %s: %w", row.UserID, err)
			}
			keyID, wrapped, err := tx.ring.Wrap(root, aad)
			if err != nil {
				return err
			}
			if err := tx.q.RewrapUserKey(ctx, gen.RewrapUserKeyParams{
				UserID: row.UserID, MasterKeyID: keyID, WrappedKey: wrapped,
			}); err != nil {
				return err
			}
			rewrapped++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return rewrapped, nil
}
