// Package api is the ClickFunnels API client. The bulk of it — types, request
// and response shapes, path/query assembly, and one typed method per endpoint —
// is generated from the OpenAPI spec into api.gen.go (`make generate`).
//
// This file is the small hand-written transport that sits *under* the generated
// client and applies cross-cutting concerns once, for every endpoint:
//
//   - auth: inject the bearer token on each request
//   - errors: turn any non-2xx response into a single *APIError, so callers get
//     a normal Go error instead of having to inspect status-code-keyed fields
//   - pagination: a helper to read the cursor header off any list response
//
// These are transport concerns, independent of which endpoints exist — adding
// an endpoint is a regen, not new transport code.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIError is a non-2xx response from the API.
type APIError struct {
	StatusCode int
	Message    string
}

const userAgent = "ClickFunnelsCLI"

func (e *APIError) Error() string {
	return fmt.Sprintf("api error (%d): %s", e.StatusCode, e.Message)
}

// authDoer is the HttpRequestDoer the generated client calls. It adds the bearer
// token and normalizes non-2xx responses into *APIError — so the generated
// per-endpoint methods surface HTTP failures through their plain `error` return,
// with no per-call status handling.
type authDoer struct {
	token string
	http  *http.Client
}

func (d *authDoer) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &APIError{StatusCode: resp.StatusCode, Message: extractError(body)}
	}
	return resp, nil
}

// New builds a typed client for a workspace, wired to the auth/error transport.
// baseURL is e.g. https://myworkspace.myclickfunnels.com/api/v2.
func New(baseURL, token string) (*ClientWithResponses, error) {
	doer := &authDoer{token: token, http: &http.Client{Timeout: 30 * time.Second}}
	return NewClientWithResponses(baseURL, WithHTTPClient(doer))
}

// Cursor returns the next-page cursor from a list response, or "" if there are
// no more pages. Pass the WithResponse result's HTTPResponse.
func Cursor(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	return resp.Header.Get("Pagination-Next")
}

// extractError pulls a human-readable message from an error response body.
func extractError(body []byte) string {
	var payload struct {
		Error  string `json:"error"`
		Errors any    `json:"errors"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Error != "" {
			return payload.Error
		}
		if payload.Errors != nil {
			return fmt.Sprintf("%v", payload.Errors)
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "(empty response body)"
	}
	return s
}
