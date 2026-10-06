package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// validateRequests checks every request against the contract before it reaches
// a handler: an unknown path gets 404, an unknown method 405, a missing
// credential 401, an oversized body 413, and any other mismatch (parameters,
// body, content type) 400. Handlers can therefore trust their input's shape.
func validateRequests(doc *openapi3.T) (func(http.Handler) http.Handler, error) {
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("building the request router from the contract: %w", err)
	}
	options := &openapi3filter.Options{AuthenticationFunc: checkSecurityScheme}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, pathParams, err := router.FindRoute(r)
			switch {
			case errors.Is(err, routers.ErrMethodNotAllowed):
				w.Header().Set("Allow", strings.Join(allowedMethods(router, r), ", "))
				writeProblem(w, r, http.StatusMethodNotAllowed, detailNotAllowed)
				return
			case err != nil:
				writeProblem(w, r, http.StatusNotFound, detailNotFound)
				return
			}

			err = openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
				Request:    r,
				PathParams: pathParams,
				Route:      route,
				Options:    options,
			})
			if err != nil {
				writeValidationProblem(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// allowedMethods lists the methods the contract defines for the request's path.
func allowedMethods(router routers.Router, r *http.Request) []string {
	var allowed []string
	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
	} {
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, _, err := router.FindRoute(probe); err == nil {
			allowed = append(allowed, method)
		}
	}
	return slices.Sorted(slices.Values(allowed))
}

func writeValidationProblem(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	var unauthenticated *openapi3filter.SecurityRequirementsError
	switch {
	case errors.As(err, &tooLarge):
		writeProblem(w, r, http.StatusRequestEntityTooLarge, bodyTooLargeDetail(tooLarge.Limit))
	case errors.As(err, &unauthenticated):
		w.Header().Set("WWW-Authenticate", `Bearer realm="magpiemail"`)
		writeProblem(w, r, http.StatusUnauthorized, detailUnauthorized)
	default:
		writeProblem(w, r, http.StatusBadRequest, validationDetail(err))
	}
}

func bodyTooLargeDetail(limit int64) string {
	return fmt.Sprintf("The request body exceeds %d bytes.", limit)
}

// validationDetail explains a validation failure without quoting the schema
// (an implementation detail) nor dumping the offending value.
func validationDetail(err error) string {
	var reqErr *openapi3filter.RequestError
	if !errors.As(err, &reqErr) {
		return "The request does not match the API contract."
	}

	where := "request"
	switch {
	case reqErr.Parameter != nil:
		where = fmt.Sprintf("%s parameter %q", reqErr.Parameter.In, reqErr.Parameter.Name)
	case reqErr.RequestBody != nil:
		where = "request body"
	}

	var schemaErr *openapi3.SchemaError
	if errors.As(reqErr.Err, &schemaErr) {
		if pointer := schemaErr.JSONPointer(); len(pointer) > 0 {
			where += " at /" + strings.Join(pointer, "/")
		}
		return where + ": " + schemaErr.Reason
	}
	if reqErr.Reason != "" {
		return where + ": " + reqErr.Reason
	}
	return where + " is invalid"
}
