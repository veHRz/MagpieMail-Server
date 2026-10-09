package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres/gen"
)

// Provider is an entry of the provider catalogue.
type Provider = gen.Provider

// NewProvider describes a provider to add to the catalogue.
type NewProvider struct {
	Slug     string
	Name     string
	Kind     string // "builtin" or "custom"
	Settings map[string]any
}

// Providers is the repository of the provider catalogue. Phase S4 completes it.
type Providers struct{ s *Store }

// Providers returns the provider repository.
func (s *Store) Providers() Providers { return Providers{s} }

// Create adds a provider. A slug already used is ErrConflict.
func (r Providers) Create(ctx context.Context, p NewProvider) (Provider, error) {
	settings, err := json.Marshal(p.Settings)
	if err != nil {
		return Provider{}, fmt.Errorf("encoding provider settings: %w", err)
	}
	created, err := r.s.q.CreateProvider(ctx, gen.CreateProviderParams{
		ID: newID(), Slug: p.Slug, Name: p.Name, Kind: p.Kind, Settings: settings,
	})
	return created, translate(err)
}
