// Package auth implements the OAuth2 authorization-code login flow used by the
// CLI to obtain a bearer token for the ClickFunnels API.
//
// # Design notes
//
// ClickFunnels uses Doorkeeper. The OAuth endpoints are served on the
// workspace-agnostic "accounts" host (e.g. https://accounts.myclickfunnels.com)
// — the user signs in there and *picks the target workspace during consent*, so
// the CLI does not need a subdomain up front. We pass new_installation=true so
// the server shows the workspace picker and scopes the issued token to the
// chosen workspace.
//
// It's a native-app loopback flow: bind a localhost web server on one of a few
// fixed ports (which the server's OAuth app registers as redirect URIs, since
// Doorkeeper matches the redirect exactly), open the browser to the authorize
// URL, and capture the redirect at http://localhost:<port>/callback.
//
// We always send a PKCE code_challenge (S256). With the CLI registered as a
// public (confidential: false) client, the token exchange needs no client
// secret; PKCE is sent regardless and is honored the moment the backend
// migrates the columns. See CLAUDE.md.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// LoopbackPorts are the fixed localhost ports the CLI binds for the OAuth
// redirect, in preference order. They must match the redirect URIs registered
// on the server's OAuth application (Doorkeeper matches the redirect exactly,
// including the port).
var LoopbackPorts = []int{8976, 8977, 8978}

// LoginResult is returned to the caller after a successful browser login.
type LoginResult struct {
	AccessToken string
	TokenType   string
	Scope       string
}

// Options configure a single login attempt.
type Options struct {
	OAuthBaseURL string // the accounts host, e.g. https://accounts.myclickfunnels.com
	ClientID     string
	ClientSecret string            // optional; unused for a public client
	Scopes       []string          // e.g. ["read", "write"]
	AuthParams   map[string]string // extra authorize params, e.g. new_installation=true
	// OpenBrowser is called with the authorize URL. Tests inject a no-op; the
	// CLI opens the system browser.
	OpenBrowser func(url string) error
}

// Login runs the full authorization-code + PKCE flow and blocks until the user
// completes (or cancels) the browser consent. The provided context bounds the
// total wait.
func Login(ctx context.Context, opts Options) (*LoginResult, error) {
	if opts.ClientID == "" {
		return nil, errors.New("missing OAuth client id")
	}

	// Bind one of the fixed loopback ports (the server registers these exact
	// redirect URIs). Try them in order so a busy port falls back to the next.
	var listener net.Listener
	var port int
	for _, p := range LoopbackPorts {
		if l, lerr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); lerr == nil {
			listener, port = l, p
			break
		}
	}
	if listener == nil {
		return nil, fmt.Errorf("none of the loopback ports %v are free; close whatever is using them and retry", LoopbackPorts)
	}
	defer listener.Close()

	// Doorkeeper only exempts non-SSL redirect URIs whose host is literally
	// "localhost" (not 127.0.0.1), so advertise localhost.
	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)

	conf := &oauth2.Config{
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		Scopes:       opts.Scopes,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  opts.OAuthBaseURL + "/oauth/authorize",
			TokenURL: opts.OAuthBaseURL + "/oauth/token",
		},
	}

	state, err := randomString(24)
	if err != nil {
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()

	authOpts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	}
	for k, v := range opts.AuthParams {
		authOpts = append(authOpts, oauth2.SetAuthURLParam(k, v))
	}
	authURL := conf.AuthCodeURL(state, authOpts...)

	// callbackResult carries the outcome from the HTTP handler goroutine.
	type callbackResult struct {
		code string
		err  error
	}
	resultCh := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			writeBrowserPage(w, false, q.Get("error_description"))
			resultCh <- callbackResult{err: fmt.Errorf("authorization denied: %s", e)}
			return
		}
		if q.Get("state") != state {
			writeBrowserPage(w, false, "state mismatch")
			resultCh <- callbackResult{err: errors.New("state mismatch (possible CSRF); aborting")}
			return
		}
		code := q.Get("code")
		if code == "" {
			writeBrowserPage(w, false, "no authorization code returned")
			resultCh <- callbackResult{err: errors.New("no authorization code in callback")}
			return
		}
		writeBrowserPage(w, true, "")
		resultCh <- callbackResult{code: code}
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	open := opts.OpenBrowser
	if open == nil {
		open = openBrowser
	}
	if err := open(authURL); err != nil {
		// Non-fatal: print the URL so the user can open it manually.
		fmt.Printf("\nOpen this URL in your browser to continue:\n\n  %s\n\n", authURL)
	}

	var code string
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("login timed out or canceled: %w", ctx.Err())
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}
		code = res.code
	}

	token, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging authorization code for token: %w", err)
	}

	scope, _ := token.Extra("scope").(string)
	return &LoginResult{
		AccessToken: token.AccessToken,
		TokenType:   tokenTypeOrBearer(token.TokenType),
		Scope:       scope,
	}, nil
}

func tokenTypeOrBearer(t string) string {
	if t == "" {
		return "Bearer"
	}
	return t
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
