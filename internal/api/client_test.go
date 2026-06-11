package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthDoerInjectsBearer verifies the bearer token is added to every request
// and a 2xx flows through the generated method as a decoded body.
func TestAuthDoerInjectsBearer(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[{"id":1,"public_id":"abc"}]`)
	}))
	defer srv.Close()

	client, err := New(srv.URL, "tok123")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ListTeamsWithResponse(context.Background(), &ListTeamsParams{})
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q, want Bearer tok123", gotAuth)
	}
	if gotPath != "/teams" {
		t.Errorf("path = %q, want /teams", gotPath)
	}
	if resp.JSON200 == nil || len(*resp.JSON200) != 1 {
		t.Fatalf("expected one team decoded, got %v", resp.JSON200)
	}
}

// TestAuthDoerNormalizesErrors is the key behavior: a non-2xx response surfaces
// as a single *APIError through the generated method's error return — no
// per-call status-code inspection needed by callers.
func TestAuthDoerNormalizesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"API key missing or invalid"}`)
	}))
	defer srv.Close()

	client, _ := New(srv.URL, "tok")
	_, err := client.ListTeamsWithResponse(context.Background(), &ListTeamsParams{})
	if err == nil {
		t.Fatal("expected an error on 401")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", apiErr.StatusCode)
	}
	if apiErr.Message != "API key missing or invalid" {
		t.Errorf("message = %q", apiErr.Message)
	}
}

func TestCursor(t *testing.T) {
	if Cursor(nil) != "" {
		t.Error("nil response should yield empty cursor")
	}
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Pagination-Next", "cur_123")
	if got := Cursor(resp); got != "cur_123" {
		t.Errorf("cursor = %q, want cur_123", got)
	}
}

// TestToContactUpdate verifies create params convert cleanly to the update body
// type, preserving set fields and the omit semantics.
func TestToContactUpdate(t *testing.T) {
	email := "a@b.com"
	tags := []int{1, 2}
	u := ToContactUpdate(ContactParameters{EmailAddress: &email, TagIds: &tags})
	if u.EmailAddress == nil || *u.EmailAddress != "a@b.com" {
		t.Errorf("email not carried over: %v", u.EmailAddress)
	}
	if u.TagIds == nil || len(*u.TagIds) != 2 {
		t.Errorf("tag_ids not carried over: %v", u.TagIds)
	}
	if u.FirstName != nil {
		t.Errorf("unset field should stay nil, got %v", u.FirstName)
	}
}

// TestRawRequest shows the escape hatch returns the body verbatim even on a
// non-2xx (unlike the error-normalizing client).
func TestRawRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer auth")
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		io.WriteString(w, `{"error":"nope"}`)
	}))
	defer srv.Close()

	resp, err := RawRequest(context.Background(), srv.URL, "tok", "POST", "/contacts", []byte(`{}`))
	if err != nil {
		t.Fatalf("RawRequest returned transport error: %v", err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
	if string(resp.Body) != `{"error":"nope"}` {
		t.Errorf("body = %q", resp.Body)
	}
}
