package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3filter"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	// ID identifies the caller.
	ID string
	// Scheme is the contract's security scheme that authenticated the request:
	// "bearerAuth" or "cookieAuth".
	Scheme string
}

// Authenticator finds the principal of a request from its credentials. It
// reports false when the request carries no valid credential, and an error
// only when the check itself failed. Phase S3 provides the implementation.
type Authenticator interface {
	Authenticate(r *http.Request) (Principal, bool, error)
}

type principalKey struct{}

// PrincipalFrom returns the authenticated caller, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// authenticate stores the request's principal in its context. It rejects
// nothing: the request validator enforces each operation's security
// requirements, so public operations stay reachable without credentials.
func authenticate(a Authenticator, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a == nil {
				next.ServeHTTP(w, r)
				return
			}
			p, ok, err := a.Authenticate(r)
			if err != nil {
				logger.ErrorContext(r.Context(), "authentication failed", slog.Any("error", err))
				writeProblem(w, r, http.StatusInternalServerError, detailInternal)
				return
			}
			if ok {
				r = r.WithContext(context.WithValue(r.Context(), principalKey{}, p))
			}
			next.ServeHTTP(w, r)
		})
	}
}

var errNotAuthenticated = errors.New("no valid credential for this security scheme")

// checkSecurityScheme is the validator's authentication function: a security
// requirement is met when the request was authenticated through that scheme.
func checkSecurityScheme(_ context.Context, in *openapi3filter.AuthenticationInput) error {
	p, ok := PrincipalFrom(in.RequestValidationInput.Request.Context())
	if !ok || p.Scheme != in.SecuritySchemeName {
		return errNotAuthenticated
	}
	return nil
}
