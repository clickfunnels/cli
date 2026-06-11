package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// newTestContactCmd builds a throwaway command with contactFlags bound, so we
// can exercise the --input + flag overlay without hitting the network.
func newTestContactCmd(cf *contactFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	cf.bind(cmd)
	return cmd
}

func TestContactFlagsInputOverlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contact.json")
	if err := os.WriteFile(path, []byte(`{"email_address":"file@example.com","first_name":"FromFile","tag_ids":[1,2]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var cf contactFlags
	cmd := newTestContactCmd(&cf)
	// --input provides the base; --first-name overrides one field from it.
	cmd.SetArgs([]string{"--input", path, "--first-name", "Override"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	params, provided, err := cf.build(cmd)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !provided {
		t.Fatal("expected provided=true")
	}
	if params.EmailAddress == nil || *params.EmailAddress != "file@example.com" {
		t.Errorf("email = %v, want file@example.com (from --input)", params.EmailAddress)
	}
	if params.FirstName == nil || *params.FirstName != "Override" {
		t.Errorf("first_name = %v, want Override (flag wins over --input)", params.FirstName)
	}
	if params.TagIds == nil || len(*params.TagIds) != 2 {
		t.Errorf("tag_ids = %v, want [1 2] (from --input)", params.TagIds)
	}
}

func TestContactFlagsNothingProvided(t *testing.T) {
	var cf contactFlags
	cmd := newTestContactCmd(&cf)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	_, provided, err := cf.build(cmd)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if provided {
		t.Error("expected provided=false when no flags or input given")
	}
}

func TestContactFlagsMapAndSlice(t *testing.T) {
	var cf contactFlags
	cmd := newTestContactCmd(&cf)
	cmd.SetArgs([]string{"--tag-id", "5", "--tag-id", "6", "--attr", "plan=pro", "--attr", "seats=3"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	params, provided, err := cf.build(cmd)
	if err != nil || !provided {
		t.Fatalf("build: provided=%v err=%v", provided, err)
	}
	if params.TagIds == nil || len(*params.TagIds) != 2 || (*params.TagIds)[1] != 6 {
		t.Errorf("tag_ids = %v, want [5 6]", params.TagIds)
	}
	if params.CustomAttributes == nil || (*params.CustomAttributes)["plan"] != "pro" {
		t.Errorf("custom_attributes = %v", params.CustomAttributes)
	}
}
