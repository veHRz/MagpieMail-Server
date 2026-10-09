package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres/gen"
)

// Account is a provider mailbox connected by a user. Its credentials are only
// available, decrypted, through Accounts.Credentials.
type Account = gen.Account

// NewAccount describes an account to connect.
type NewAccount struct {
	UserID      uuid.UUID
	ProviderID  uuid.UUID
	Email       string
	DisplayName string
	AuthMethod  string // "password" or "oauth2"
	// Credentials (a password or an OAuth refresh token) are encrypted with the
	// user's data key before they reach the database.
	Credentials []byte
}

// Accounts is the repository of accounts. Phase S5 completes it.
type Accounts struct{ s *Store }

// Accounts returns the account repository.
func (s *Store) Accounts() Accounts { return Accounts{s} }

// credentialsAAD binds sealed credentials to their account and owner.
func credentialsAAD(userID, accountID uuid.UUID) []byte {
	return envelope.AAD("accounts", "credentials_encrypted", accountID.String(), userID.String())
}

// Create connects an account. An unknown provider is ErrNotFound.
func (r Accounts) Create(ctx context.Context, a NewAccount) (Account, error) {
	id := newID()
	sealed, err := r.seal(ctx, a.UserID, id, a.Credentials)
	if err != nil {
		return Account{}, err
	}
	created, err := r.s.q.CreateAccount(ctx, gen.CreateAccountParams{
		ID: id, UserID: a.UserID, ProviderID: a.ProviderID, Email: a.Email,
		DisplayName: a.DisplayName, AuthMethod: a.AuthMethod, CredentialsEncrypted: sealed,
	})
	return created, translate(err)
}

// Get returns one of the user's accounts.
func (r Accounts) Get(ctx context.Context, userID, id uuid.UUID) (Account, error) {
	a, err := r.s.q.GetAccount(ctx, gen.GetAccountParams{ID: id, UserID: userID})
	return a, translate(err)
}

// Credentials returns the decrypted credentials of one of the user's accounts.
func (r Accounts) Credentials(ctx context.Context, userID, id uuid.UUID) ([]byte, error) {
	a, err := r.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if a.CredentialsEncrypted == nil {
		return nil, nil
	}
	key, err := r.s.Keys().ForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return key.Open(a.CredentialsEncrypted, credentialsAAD(userID, id))
}

// SetCredentials replaces the credentials of one of the user's accounts.
func (r Accounts) SetCredentials(ctx context.Context, userID, id uuid.UUID, credentials []byte) error {
	sealed, err := r.seal(ctx, userID, id, credentials)
	if err != nil {
		return err
	}
	n, err := r.s.q.SetAccountCredentials(ctx, gen.SetAccountCredentialsParams{
		ID: id, UserID: userID, CredentialsEncrypted: sealed,
	})
	if err != nil {
		return translate(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r Accounts) seal(ctx context.Context, userID, accountID uuid.UUID, credentials []byte) ([]byte, error) {
	if credentials == nil {
		return nil, nil
	}
	key, err := r.s.Keys().ForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return key.Seal(credentials, credentialsAAD(userID, accountID))
}
