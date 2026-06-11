// Package cmd defines the cobra command tree for the `cf` CLI.
package cmd

import (
	"github.com/spf13/cobra"
)

// Build metadata, overridden at release time via -ldflags. See CLAUDE.md
// for how versioning works (the CLI versions independently of the API).
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// defaultClientID is the first-party OAuth client id, so users are never asked
// for one. It's the production "ClickFunnels CLI" public client (confidential:
// false — a public client id is not a secret and is safe to ship). Override per
// login with --client-id or CF_CLI_CLIENT_ID (e.g. against a dev server). Can
// also be replaced at build time via -ldflags "-X .../cmd.defaultClientID=<uid>".
var defaultClientID = "clickfunnels_cli"

// NewRootCmd builds the root command and wires up subcommands.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cf",
		Short: "ClickFunnels from your terminal",
		Long: "cf is a command-line client for the ClickFunnels API.\n\n" +
			"Authenticate once with `cf auth login`, then manage your workspace\n" +
			"data — teams, contacts, and more — without leaving the terminal.",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Bare `cf` prints help (cobra's default with no RunE).
	}

	// Global selector for which signed-in workspace a command targets. Falls
	// back to CF_CLI_WORKSPACE, then to the only signed-in workspace.
	root.PersistentFlags().StringP("workspace", "w", "",
		"workspace to target: id, public id, or subdomain (or set CF_CLI_WORKSPACE)")

	// Global output format for command results.
	root.PersistentFlags().StringP("output", "o", "table",
		"output format: table, json, yaml, csv")

	// Curated, polished commands (tables, interactive forms).
	root.AddGroup(&cobra.Group{ID: "core", Title: "Common commands (curated):"})
	for _, c := range []*cobra.Command{
		newAuthCmd(), newTeamsCmd(), newContactsCmd(), newBlogsCmd(), newAPICmd(),
	} {
		c.GroupID = "core"
		root.AddCommand(c)
	}

	// Full API surface, generated from the spec (one group per tag).
	mountGenerated(root)

	return root
}
