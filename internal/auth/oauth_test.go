package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestLoginFlow exercises the full authorization-code + PKCE flow against a
// stand-in Doorkeeper server, with the "browser" automated to hit the loopback
// callback directly.
func TestLoginFlow(t *testing.T) {
	const wantToken = "test-access-token"
	var gotVerifier string

	// Stand-in OAuth server: /oauth/authorize redirects back to the loopback
	// redirect_uri with a code; /oauth/token exchanges it for a token.
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/authorize":
			q := r.URL.Query()
			if q.Get("code_challenge") == "" {
				t.Errorf("expected PKCE code_challenge in authorize request")
			}
			if q.Get("code_challenge_method") != "S256" {
				t.Errorf("expected S256 challenge method, got %q", q.Get("code_challenge_method"))
			}
			redirect := q.Get("redirect_uri") + "?code=the-code&state=" + q.Get("state")
			http.Redirect(w, r, redirect, http.StatusFound)
		case "/oauth/token":
			_ = r.ParseForm()
			gotVerifier = r.Form.Get("code_verifier")
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"access_token":"`+wantToken+`","token_type":"Bearer","scope":"read write"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// The "browser" just performs an HTTP GET against the authorize URL and
	// follows the redirect to our loopback callback server.
	openBrowser := func(authURL string) error {
		go func() {
			client := &http.Client{}
			req, _ := http.NewRequest(http.MethodGet, authURL, nil)
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := Login(ctx, Options{
		OAuthBaseURL: server.URL,
		ClientID:     "client-123",
		Scopes:       []string{"read", "write"},
		OpenBrowser:  openBrowser,
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if result.AccessToken != wantToken {
		t.Errorf("access token = %q, want %q", result.AccessToken, wantToken)
	}
	if result.Scope != "read write" {
		t.Errorf("scope = %q, want %q", result.Scope, "read write")
	}
	if gotVerifier == "" {
		t.Errorf("expected code_verifier to be sent in token exchange")
	}
}

// TestLoginRequiresClientID verifies we fail fast without a client id.
func TestLoginRequiresClientID(t *testing.T) {
	_, err := Login(context.Background(), Options{OAuthBaseURL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error when client id is missing")
	}
}
