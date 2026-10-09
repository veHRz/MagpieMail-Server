package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres/gen"
)

// User is a person who signs in to MagpieMail.
type User = gen.User

// NewUser describes a user to create.
type NewUser struct {
	Email       string
	DisplayName string
	Role        string // "admin" or "user"
}

// Users is the repository of users.
type Users struct{ s *Store }

// Users returns the user repository.
func (s *Store) Users() Users { return Users{s} }

// Create adds a user. An email already used, in any case, is ErrConflict.
func (r Users) Create(ctx context.Context, u NewUser) (User, error) {
	created, err := r.s.q.CreateUser(ctx, gen.CreateUserParams{
		ID: newID(), Email: u.Email, DisplayName: u.DisplayName, Role: u.Role,
	})
	return created, translate(err)
}

// Get returns a user by ID.
func (r Users) Get(ctx context.Context, id uuid.UUID) (User, error) {
	u, err := r.s.q.GetUser(ctx, id)
	return u, translate(err)
}

// GetByEmail returns a user by email, matched case-insensitively.
func (r Users) GetByEmail(ctx context.Context, email string) (User, error) {
	u, err := r.s.q.GetUserByEmail(ctx, email)
	return u, translate(err)
}
