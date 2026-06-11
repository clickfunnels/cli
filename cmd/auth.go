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
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Add a workspace by logging in via your browser (OAuth2)",
		Long: "Authorize a workspace and store its token. ClickFunnels tokens are " +
			"workspace-scoped, so run this once per workspace you want to use; they " +
			"accumulate and you switch between them with --workspace.",
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

			// OAuth happens on the accounts host; the user picks the workspace in
			// the browser (new_installation=true), so no subdomain is needed here.
			oauthBase := config.OAuthBaseURL(host)

			fmt.Println(ui.Subtle.Render("Opening your browser to authorize…"))
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
			defer cancel()

			result, err := auth.Login(ctx, auth.Options{
				OAuthBaseURL: oauthBase,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				Scopes:       scopes,
				AuthParams:   map[string]string{"new_installation": "true"},
			})
			if err != nil {
				return err
			}

			// Identify which workspace the token was scoped to (IDM resources are
			// served on the accounts host).
			client, err := api.New(oauthBase+"/api/v2", result.AccessToken)
			if err != nil {
				return err
			}
			ws, err := resolveLoginWorkspace(ctx, client)
			if err != nil {
				return fmt.Errorf("authorized, but couldn't identify the workspace: %w", err)
			}

			account := config.Account{
				Subdomain:   ws.Subdomain,
				Host:        host,
				ClientID:    clientID,
				WorkspaceID: int64(ws.Id),
				PublicID:    str(ws.PublicId),
				Name:        ws.Name,
				AccessToken: result.AccessToken,
				TokenType:   result.TokenType,
				Scope:       result.Scope,
			}

			store, err := config.LoadStore()
			if err != nil {
				return err
			}
			store.Upsert(account)
			if err := store.Save(); err != nil {
				return err
			}

			// Remember login defaults for next time.
			cfg.ClientID, cfg.Host = clientID, host
			if err := cfg.Save(); err != nil {
				return err
			}

			fmt.Printf("\n%s Logged in to %s\n", ui.Success.Render(ui.Check), ui.Accent.Render(account.Label()))
			if len(store.Accounts) > 1 {
				fmt.Println(ui.Subtle.Render(fmt.Sprintf("%d workspaces signed in — use --workspace to choose.", len(store.Accounts))))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client id (or set CF_CLI_CLIENT_ID)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "OAuth client secret (or set CF_CLI_CLIENT_SECRET); optional once server-side PKCE is enabled")
	cmd.Flags().StringVar(&host, "host", "", "override API host (default myclickfunnels.com; or set CF_CLI_HOST)")
	cmd.Flags().StringSliceVar(&scopes, "scope", []string{"read", "write"}, "OAuth scopes to request")
	return cmd
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

// resolveLoginWorkspace identifies the workspace a freshly-issued token is
// scoped to. The user picked it during the browser flow, so the token usually
// grants one workspace; if it somehow grants several, prompt to pick.
func resolveLoginWorkspace(ctx context.Context, client *api.ClientWithResponses) (api.WorkspaceAttributes, error) {
	teamsResp, err := client.ListTeamsWithResponse(ctx, &api.ListTeamsParams{})
	if err != nil {
		return api.WorkspaceAttributes{}, err
	}
	var all []api.WorkspaceAttributes
	for _, t := range derefSlice(teamsResp.JSON200) {
		wsResp, err := client.ListWorkspacesWithResponse(ctx, t.Id, &api.ListWorkspacesParams{})
		if err != nil {
			return api.WorkspaceAttributes{}, err
		}
		all = append(all, derefSlice(wsResp.JSON200)...)
	}
	if len(all) == 0 {
		return api.WorkspaceAttributes{}, fmt.Errorf("no workspaces accessible with this authorization")
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
					maskToken(a.AccessToken),
				})
			}
			fmt.Println(ui.RenderTable([]string{"NAME", "SUBDOMAIN", "PUBLIC ID", "ID", "TOKEN"}, rows))

			if len(store.Accounts) == 1 {
				fmt.Println(ui.Subtle.Render("This is the default workspace (only one signed in)."))
			} else {
				fmt.Println(ui.Subtle.Render("Select one with --workspace <id|public-id|subdomain> or CF_CLI_WORKSPACE."))
			}
			return nil
		},
	}
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return strings.Repeat("•", len(t))
	}
	return t[:4] + strings.Repeat("•", 8) + t[len(t)-4:]
}
