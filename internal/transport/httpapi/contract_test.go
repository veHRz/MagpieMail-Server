package httpapi_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContract_IsAValidOpenAPIDocument(t *testing.T) {
	doc := contract(t)

	require.NoError(t, doc.Validate(t.Context()))
	assert.Equal(t, "3.1.0", doc.OpenAPI)
}

func TestContract_VersionIsSemantic(t *testing.T) {
	assert.Regexp(t, `^\d+\.\d+\.\d+$`, contract(t).Info.Version)
}

// TestRouter_ServesExactlyTheContract proves the S1 criterion "a test checks
// that every served route exists in the contract", and the converse.
func TestRouter_ServesExactlyTheContract(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})
	routes, ok := router.(chi.Routes)
	require.True(t, ok, "the router must expose its routes")

	type operation struct{ method, path string }
	var served []operation
	require.NoError(t, chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		served = append(served, operation{method, route})
		return nil
	}))
	require.NotEmpty(t, served)

	doc := contract(t)
	var declared []operation
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			declared = append(declared, operation{method, path})
		}
	}

	for _, op := range served {
		assert.Contains(t, declared, op, "served route %s %s is not in the contract", op.method, op.path)
	}
	for _, op := range declared {
		assert.True(t, slices.Contains(served, op), "contract operation %s %s is not served", op.method, op.path)
	}
}
