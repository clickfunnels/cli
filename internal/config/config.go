// Package config handles persistent CLI configuration and credential storage.
//
// ClickFunnels OAuth authorizations are workspace-scoped: one login grants a
// token for exactly one workspace. Power users sign in to several workspaces,
// so we store a *set* of authorized Accounts (each with its own token) and
// select between them Heroku-style — if only one is signed in it's implied,
// otherwise the user passes --workspace (or sets CF_CLI_WORKSPACE).
//
// Everything lives under an XDG-style config directory (e.g. ~/.config/cf):
//   - config.json       non-secret login defaults (client id, host)
//   - credentials.json  the Store of Accounts incl. tokens, mode 0600
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultAPIHost is the production ClickFunnels host. The workspace subdomain is
// prefixed to it at request time (e.g. myworkspace.myclickfunnels.com).
const DefaultAPIHost = "myclickfunnels.com"

// AccountsSubdomain is the workspace-agnostic host that serves sign-in and the
// OAuth endpoints. The workspace is chosen during the browser flow, so login
// needs no workspace subdomain.
const AccountsSubdomain = "accounts"

// OAuthBaseURL returns the accounts host serving the OAuth endpoints for a given
// API host, e.g. https://accounts.myclickfunnels.com.
func OAuthBaseURL(host string) string {
	if host == "" {
		host = DefaultAPIHost
	}
	return fmt.Sprintf("https://%s.%s", AccountsSubdomain, host)
}

// Config holds non-secret defaults reused across logins. Persisted to config.json.
type Config struct {
	// ClientID is the default OAuth client id offered at the next login.
	ClientID string `json:"client_id,omitempty"`
	// Host overrides DefaultAPIHost (for staging/local). Empty = production.
	Host string `json:"host,omitempty"`
}

// Account is a single authorized workspace and its token. ClickFunnels tokens
// are workspace-scoped, so there is exactly one Account per authorization.
type Account struct {
	Subdomain   string `json:"subdomain"`
	Host        string `json:"host,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	WorkspaceID int64  `json:"workspace_id"`
	PublicID    string `json:"public_id,omitempty"`
	Name        string `json:"name,omitempty"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type,omitempty"`
	Scope       string `json:"scope,omitempty"`
	// APIBaseURL, when set, overrides the constructed base URL entirely (scheme,
	// host, port, and /api/v2 path). Used to point the CLI at a local/dev/test
	// server, e.g. http://127.0.0.1:3001/api/v2. The CF_CLI_API_BASE_URL env var
	// takes precedence over this field.
	APIBaseURL string `json:"api_base_url,omitempty"`
}

// Store is the full set of authorized workspaces, persisted to credentials.json.
type Store struct {
	Accounts []Account `json:"accounts"`
}

// --- paths ---

// Dir returns the config directory: $CF_CLI_CONFIG_DIR, else $XDG_CONFIG_HOME/cf,
// else ~/.config/cf. We use the XDG path on every platform (rather than Go's
// os.UserConfigDir, which resolves to ~/Library/Application Support on macOS)
// so the location is consistent and matches the docs.
func Dir() (string, error) {
	if dir := os.Getenv("CF_CLI_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "cf"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "cf"), nil
}

func pathFor(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// --- Config ---

// Load reads config.json. A missing file yields a zero-value Config.
func Load() (*Config, error) {
	path, err := pathFor("config.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &c, nil
}

// Save writes config.json (0600).
func (c *Config) Save() error {
	return writeJSON("config.json", c)
}

// HostOrDefault returns the configured base host, or DefaultAPIHost.
func (c *Config) HostOrDefault() string {
	if c.Host == "" {
		return DefaultAPIHost
	}
	return c.Host
}

// --- Account helpers ---

// HostOrDefault returns the account's base host, or DefaultAPIHost.
func (a Account) HostOrDefault() string {
	if a.Host == "" {
		return DefaultAPIHost
	}
	return a.Host
}

// Hostname is the full workspace host, e.g. myworkspace.myclickfunnels.com.
func (a Account) Hostname() string {
	return fmt.Sprintf("%s.%s", a.Subdomain, a.HostOrDefault())
}

// BaseURL is the API base, e.g. https://myworkspace.myclickfunnels.com/api/v2.
// A CF_CLI_API_BASE_URL env var or the account's APIBaseURL field overrides it
// (used to target a local/dev/test server).
func (a Account) BaseURL() string {
	if v := os.Getenv("CF_CLI_API_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if a.APIBaseURL != "" {
		return strings.TrimRight(a.APIBaseURL, "/")
	}
	return fmt.Sprintf("https://%s/api/v2", a.Hostname())
}

// Label is a human-friendly identifier for status/error messages.
func (a Account) Label() string {
	if a.Name != "" {
		return fmt.Sprintf("%s (%s)", a.Name, a.Subdomain)
	}
	return a.Subdomain
}

// Matches reports whether sel identifies this account — by numeric workspace id,
// obfuscated public id, or subdomain.
func (a Account) Matches(sel string) bool {
	return sel != "" && (sel == a.Subdomain ||
		sel == a.PublicID ||
		sel == strconv.FormatInt(a.WorkspaceID, 10))
}

// --- Store ---

// LoadStore reads credentials.json. A missing file yields an empty Store.
func LoadStore() (*Store, error) {
	path, err := pathFor("credentials.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Store{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &s, nil
}

// Save writes credentials.json (0600).
func (s *Store) Save() error {
	return writeJSON("credentials.json", s)
}

// Upsert adds or replaces the account for a workspace (keyed by WorkspaceID,
// falling back to subdomain when the id is unknown).
func (s *Store) Upsert(a Account) {
	for i := range s.Accounts {
		same := (a.WorkspaceID != 0 && s.Accounts[i].WorkspaceID == a.WorkspaceID) ||
			(a.WorkspaceID == 0 && s.Accounts[i].Subdomain == a.Subdomain)
		if same {
			s.Accounts[i] = a
			return
		}
	}
	s.Accounts = append(s.Accounts, a)
}

// Remove deletes accounts matching sel; returns how many were removed.
func (s *Store) Remove(sel string) int {
	kept := s.Accounts[:0]
	removed := 0
	for _, a := range s.Accounts {
		if a.Matches(sel) {
			removed++
			continue
		}
		kept = append(kept, a)
	}
	s.Accounts = kept
	return removed
}

// Find returns the single account matching sel, erroring if none or many match.
func (s *Store) Find(sel string) (*Account, error) {
	var match *Account
	for i := range s.Accounts {
		if s.Accounts[i].Matches(sel) {
			if match != nil {
				return nil, fmt.Errorf("%q matches more than one signed-in workspace", sel)
			}
			match = &s.Accounts[i]
		}
	}
	if match == nil {
		return nil, fmt.Errorf("no signed-in workspace matches %q; signed in: %s", sel, s.Labels())
	}
	return match, nil
}

// Resolve selects the active account: the one matching sel, or the only signed-in
// account when sel is empty. Errors (with guidance) when ambiguous.
func (s *Store) Resolve(sel string) (*Account, error) {
	if len(s.Accounts) == 0 {
		return nil, errors.New("not logged in — run `cf auth login`")
	}
	if sel != "" {
		return s.Find(sel)
	}
	if len(s.Accounts) == 1 {
		return &s.Accounts[0], nil
	}
	return nil, fmt.Errorf("multiple workspaces signed in (%s); pass --workspace <id|public-id|subdomain> or set CF_CLI_WORKSPACE", s.Labels())
}

// Labels is a comma-joined list of account labels, for messages.
func (s *Store) Labels() string {
	parts := make([]string, 0, len(s.Accounts))
	for _, a := range s.Accounts {
		parts = append(parts, a.Label())
	}
	return strings.Join(parts, ", ")
}

// --- shared ---

func writeJSON(name string, v any) error {
	path, err := pathFor(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
