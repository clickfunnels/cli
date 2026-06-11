package config

import (
	"strings"
	"testing"
)

func sampleStore() *Store {
	return &Store{Accounts: []Account{
		{Subdomain: "acme", WorkspaceID: 100, PublicID: "gjDMvQ", Name: "Acme", AccessToken: "t1"},
		{Subdomain: "globex", WorkspaceID: 200, PublicID: "LAPeMn", Name: "Globex", AccessToken: "t2"},
	}}
}

func TestAccountMatches(t *testing.T) {
	a := Account{Subdomain: "acme", WorkspaceID: 100, PublicID: "gjDMvQ"}
	for _, sel := range []string{"acme", "gjDMvQ", "100"} {
		if !a.Matches(sel) {
			t.Errorf("expected %q to match", sel)
		}
	}
	for _, sel := range []string{"", "globex", "101", "GJDMVQ"} {
		if a.Matches(sel) {
			t.Errorf("did not expect %q to match", sel)
		}
	}
}

func TestResolveSingle(t *testing.T) {
	s := &Store{Accounts: []Account{{Subdomain: "solo", WorkspaceID: 1}}}
	got, err := s.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Subdomain != "solo" {
		t.Errorf("got %q, want solo", got.Subdomain)
	}
}

func TestResolveMultipleNeedsSelector(t *testing.T) {
	s := sampleStore()
	_, err := s.Resolve("")
	if err == nil || !strings.Contains(err.Error(), "multiple workspaces") {
		t.Fatalf("expected multiple-workspaces error, got %v", err)
	}
}

func TestResolveBySelector(t *testing.T) {
	s := sampleStore()
	// by subdomain, public id, and numeric id all reach the same account
	for _, sel := range []string{"globex", "LAPeMn", "200"} {
		got, err := s.Resolve(sel)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", sel, err)
		}
		if got.WorkspaceID != 200 {
			t.Errorf("Resolve(%q) = %d, want 200", sel, got.WorkspaceID)
		}
	}
}

func TestResolveUnknownSelector(t *testing.T) {
	s := sampleStore()
	if _, err := s.Resolve("nope"); err == nil {
		t.Fatal("expected error for unknown selector")
	}
}

func TestUpsertReplacesByWorkspaceID(t *testing.T) {
	s := sampleStore()
	s.Upsert(Account{Subdomain: "acme-renamed", WorkspaceID: 100, AccessToken: "t1b"})
	if len(s.Accounts) != 2 {
		t.Fatalf("len = %d, want 2 (replace not append)", len(s.Accounts))
	}
	got, _ := s.Resolve("100")
	if got.AccessToken != "t1b" || got.Subdomain != "acme-renamed" {
		t.Errorf("upsert did not replace: %+v", got)
	}
}

func TestUpsertAppendsNew(t *testing.T) {
	s := sampleStore()
	s.Upsert(Account{Subdomain: "initech", WorkspaceID: 300})
	if len(s.Accounts) != 3 {
		t.Fatalf("len = %d, want 3", len(s.Accounts))
	}
}

func TestRemove(t *testing.T) {
	s := sampleStore()
	if n := s.Remove("acme"); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if len(s.Accounts) != 1 || s.Accounts[0].Subdomain != "globex" {
		t.Errorf("unexpected remaining accounts: %+v", s.Accounts)
	}
}

func TestAccountURLs(t *testing.T) {
	a := Account{Subdomain: "acme"}
	if got := a.BaseURL(); got != "https://acme.myclickfunnels.com/api/v2" {
		t.Errorf("BaseURL = %s", got)
	}
}

func TestOAuthBaseURL(t *testing.T) {
	if got := OAuthBaseURL(""); got != "https://accounts.myclickfunnels.com" {
		t.Errorf("OAuthBaseURL(default) = %s", got)
	}
	if got := OAuthBaseURL("myclickfunnels.test"); got != "https://accounts.myclickfunnels.test" {
		t.Errorf("OAuthBaseURL(test) = %s", got)
	}
}
