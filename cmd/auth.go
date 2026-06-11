package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/clickfunnels/clickfunnels-cli/internal/api"
	"github.com/clickfunnels/clickfunnels-cli/internal/auth"
	"github.com/clickfunnels/clickfunnels-cli/internal/config"
	"github.com/clickfunnels/clickfunnels-cli/internal/ui"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with ClickFunnels",
	}
	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthLogoutCmd())
	cmd.AddCommand(newAuthStatusCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var (
		clientID     string
		clientSecret string
		host         string
		scopes       []string
		installation bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in via your browser (OAuth2)",
		Long: "Authorize ClickFunnels and store the token. By default you authorize " +
			"as yourself — your token reaches every workspace you belong to, and each " +
			"is recorded so you can switch between them with --workspace.\n\n" +
			"Pass --installation to use the workspace-scoped installation flow instead: " +
			"you pick one workspace in the browser and get a persistent token for it. " +
			"That suits shared/service automation, but the authorization outlives your " +
			"own access, so prefer the default for personal use.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Resolve inputs: flag > env > saved default. The client id is never
			// prompted for — it comes from the build/env/config.
			if clientID == "" {
				clientID = firstNonEmpty(os.Getenv("CF_CLI_CLIENT_ID"), cfg.ClientID, defaultClientID)
			}
			if clientID == "" {
				return fmt.Errorf("no OAuth client id — set CF_CLI_CLIENT_ID or pass --client-id")
			}
			if clientSecret == "" {
				clientSecret = os.Getenv("CF_CLI_CLIENT_SECRET")
			}
			if host == "" {
				host = firstNonEmpty(os.Getenv("CF_CLI_HOST"), cfg.Host)
			}

			// OAuth happens on the workspace-agnostic accounts host. The default
			// (user) flow authorizes as the human; --installation uses the legacy
			// workspace-scoped flow, where the user picks one workspace in the
			// browser (new_installation=true).
			oauthBase := config.OAuthBaseURL(host)

			var authParams map[string]string
			if installation {
				authParams = map[string]string{"new_installation": "true"}
			}

			fmt.Println(ui.Subtle.Render("Opening your browser to authorize…"))
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
			defer cancel()

			result, err := auth.Login(ctx, auth.Options{
				OAuthBaseURL: oauthBase,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				Scopes:       scopes,
				AuthParams:   authParams,
			})
			if err != nil {
				return err
			}

			// IDM resources (teams/workspaces) are served on the accounts host.
			client, err := api.New(oauthBase+"/api/v2", result.AccessToken)
			if err != nil {
				return err
			}

			// Record the workspace(s) the token can reach. An installation token is
			// scoped to one workspace; a user token reaches all of the user's, so we
			// store one Account each (sharing the token) and select with --workspace.
			var added []config.Account
			if installation {
				ws, err := resolveLoginWorkspace(ctx, client)
				if err != nil {
					return fmt.Errorf("authorized, but couldn't identify the workspace: %w", err)
				}
				added = append(added, newAccount(ws, host, clientID, result, true))
			} else {
				all, err := listLoginWorkspaces(ctx, client)
				if err != nil {
					return fmt.Errorf("authorized, but couldn't list your workspaces: %w", err)
				}
				for _, ws := range all {
					added = append(added, newAccount(ws, host, clientID, result, false))
				}
			}

			store, err := config.LoadStore()
			if err != nil {
				return err
			}
			for _, a := range added {
				store.Upsert(a)
			}
			if err := store.Save(); err != nil {
				return err
			}

			// Remember login defaults for next time.
			cfg.ClientID, cfg.Host = clientID, host
			if err := cfg.Save(); err != nil {
				return err
			}

			check := ui.Success.Render(ui.Check)
			if len(added) == 1 {
				fmt.Printf("\n%s Logged in to %s\n", check, ui.Accent.Render(added[0].Label()))
			} else {
				fmt.Printf("\n%s Logged in — %s workspaces available.\n", check, ui.Accent.Render(fmt.Sprintf("%d", len(added))))
			}
			if len(store.Accounts) > 1 {
				fmt.Println(ui.Subtle.Render("Use --workspace <id|public-id|subdomain> to choose (or set CF_CLI_WORKSPACE)."))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client id (or set CF_CLI_CLIENT_ID)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "OAuth client secret (or set CF_CLI_CLIENT_SECRET); optional once server-side PKCE is enabled")
	cmd.Flags().StringVar(&host, "host", "", "override API host (default myclickfunnels.com; or set CF_CLI_HOST)")
	cmd.Flags().StringSliceVar(&scopes, "scope", []string{"read", "write"}, "OAuth scopes to request")
	cmd.Flags().BoolVar(&installation, "installation", false, "use the workspace-scoped installation flow (a persistent per-workspace token) instead of authorizing as yourself")
	return cmd
}

// newAccount builds an Account from a workspace and the freshly-issued token.
func newAccount(ws api.WorkspaceAttributes, host, clientID string, result *auth.LoginResult, installation bool) config.Account {
	return config.Account{
		Subdomain:    ws.Subdomain,
		Host:         host,
		ClientID:     clientID,
		WorkspaceID:  int64(ws.Id),
		PublicID:     str(ws.PublicId),
		Name:         ws.Name,
		AccessToken:  result.AccessToken,
		TokenType:    result.TokenType,
		Scope:        result.Scope,
		Installation: installation,
	}
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// listLoginWorkspaces returns every workspace a freshly-issued token can reach,
// across the user's teams.
func listLoginWorkspaces(ctx context.Context, client *api.ClientWithResponses) ([]api.WorkspaceAttributes, error) {
	teamsResp, err := client.ListTeamsWithResponse(ctx, &api.ListTeamsParams{})
	if err != nil {
		return nil, err
	}
	var all []api.WorkspaceAttributes
	for _, t := range derefSlice(teamsResp.JSON200) {
		wsResp, err := client.ListWorkspacesWithResponse(ctx, t.Id, &api.ListWorkspacesParams{})
		if err != nil {
			return nil, err
		}
		all = append(all, derefSlice(wsResp.JSON200)...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no workspaces accessible with this authorization")
	}
	return all, nil
}

// resolveLoginWorkspace identifies the single workspace an installation token is
// scoped to. It's normally exactly one; if several come back, prompt to pick.
func resolveLoginWorkspace(ctx context.Context, client *api.ClientWithResponses) (api.WorkspaceAttributes, error) {
	all, err := listLoginWorkspaces(ctx, client)
	if err != nil {
		return api.WorkspaceAttributes{}, err
	}
	if len(all) == 1 {
		return all[0], nil
	}

	options := make([]huh.Option[int], 0, len(all))
	byID := make(map[int]api.WorkspaceAttributes, len(all))
	for _, w := range all {
		options = append(options, huh.NewOption(fmt.Sprintf("%s (%s)", w.Name, w.Subdomain), w.Id))
		byID[w.Id] = w
	}
	var selected int
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[int]().Title("Select the workspace you authorized").Options(options...).Value(&selected),
	)).Run(); err != nil {
		return api.WorkspaceAttributes{}, err
	}
	return byID[selected], nil
}

func newAuthLogoutCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove a signed-in workspace (or all of them)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.LoadStore()
			if err != nil {
				return err
			}
			if len(store.Accounts) == 0 {
				fmt.Println(ui.Subtle.Render("No signed-in workspaces."))
				return nil
			}

			if all {
				n := len(store.Accounts)
				store.Accounts = nil
				if err := store.Save(); err != nil {
					return err
				}
				fmt.Printf("%s Logged out of %d workspace(s).\n", ui.Success.Render(ui.Check), n)
				return nil
			}

			sel := workspaceSelector(cmd)
			if sel == "" {
				if len(store.Accounts) == 1 {
					sel = store.Accounts[0].Subdomain
				} else {
					return fmt.Errorf("multiple workspaces signed in (%s); pass --workspace <id|public-id|subdomain> or --all", store.Labels())
				}
			}
			removed := store.Remove(sel)
			if removed == 0 {
				return fmt.Errorf("no signed-in workspace matches %q", sel)
			}
			if err := store.Save(); err != nil {
				return err
			}
			fmt.Printf("%s Logged out of %s.\n", ui.Success.Render(ui.Check), ui.Accent.Render(sel))
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "remove all signed-in workspaces")
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "List signed-in workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.LoadStore()
			if err != nil {
				return err
			}
			if len(store.Accounts) == 0 {
				fmt.Printf("%s Not logged in. Run %s.\n", ui.Error.Render(ui.Cross), ui.Accent.Render("cf auth login"))
				return nil
			}

			fmt.Println(ui.Title.Render("Signed-in workspaces"))
			rows := make([][]string, 0, len(store.Accounts))
			for _, a := range store.Accounts {
				rows = append(rows, []string{
					a.Name,
					a.Subdomain,
					a.PublicID,
					fmt.Sprintf("%d", a.WorkspaceID),
					accountType(a),
					maskToken(a.AccessToken),
				})
			}
			fmt.Println(ui.RenderTable([]string{"NAME", "SUBDOMAIN", "PUBLIC ID", "ID", "TYPE", "TOKEN"}, rows))

			if len(store.Accounts) == 1 {
				fmt.Println(ui.Subtle.Render("This is the default workspace (only one signed in)."))
			} else {
				fmt.Println(ui.Subtle.Render("Select one with --workspace <id|public-id|subdomain> or CF_CLI_WORKSPACE."))
			}
			return nil
		},
	}
}

// accountType labels how an account was authorized, for `cf auth status`.
func accountType(a config.Account) string {
	if a.Installation {
		return "installation"
	}
	return "user"
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return strings.Repeat("•", len(t))
	}
	return t[:4] + strings.Repeat("•", 8) + t[len(t)-4:]
}
