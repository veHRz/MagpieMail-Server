// Package contracttest checks HTTP responses against the OpenAPI contract, so
// that a response that does not match api/openapi.yaml fails the test.
package contracttest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

var routerCache sync.Map // *openapi3.T -> routers.Router

func routerFor(doc *openapi3.T) (routers.Router, error) {
	if r, ok := routerCache.Load(doc); ok {
		return r.(routers.Router), nil //nolint:forcetypeassert // Only routers are stored.
	}
	r, err := gorillamux.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("building a router from the contract: %w", err)
	}
	routerCache.Store(doc, r)
	return r, nil
}

// Check validates resp, the answer to req, against doc: status code, content
// type, headers and body must match the operation's declared responses. A
// request that matches no operation must be answered by an RFC 9457 problem.
// Check consumes resp.Body and replaces it with an equivalent reader.
func Check(doc *openapi3.T, req *http.Request, resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading the response body: %w", err)
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))

	router, err := routerFor(doc)
	if err != nil {
		return err
	}
	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		return checkProblem(resp, body)
	}

	return openapi3filter.ValidateResponse(req.Context(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status:  resp.StatusCode,
		Header:  resp.Header,
		Body:    io.NopCloser(bytes.NewReader(body)),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	})
}

// checkProblem verifies an RFC 9457 problem response.
func checkProblem(resp *http.Response, body []byte) error {
	if resp.StatusCode < http.StatusBadRequest {
		return fmt.Errorf("status %d for a request that matches no operation", resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/problem+json" {
		return fmt.Errorf("content type %q, want application/problem+json", resp.Header.Get("Content-Type"))
	}
	var problem struct {
		Type   *string `json:"type"`
		Title  *string `json:"title"`
		Status *int    `json:"status"`
	}
	if err := json.Unmarshal(body, &problem); err != nil {
		return fmt.Errorf("problem body is not JSON: %w", err)
	}
	if problem.Type == nil || problem.Title == nil || problem.Status == nil {
		return errors.New("problem body lacks type, title or status")
	}
	if *problem.Status != resp.StatusCode {
		return fmt.Errorf("problem status %d differs from the response status %d", *problem.Status, resp.StatusCode)
	}
	return nil
}

// Serve sends req to h, checks the response against doc and reports any
// mismatch as a test failure. The returned response has a readable body.
func Serve(t testing.TB, doc *openapi3.T, h http.Handler, req *http.Request) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	resp := rec.Result()
	if err := Check(doc, req, resp); err != nil {
		t.Errorf("%s %s: response does not match the contract: %v", req.Method, req.URL.Path, err)
	}
	return resp
}
