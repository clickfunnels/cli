package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/clickfunnels/clickfunnels-cli/internal/api"
	"github.com/clickfunnels/clickfunnels-cli/internal/config"
)

// workspaceSelector returns the active-workspace selector: the --workspace flag
// if set, else the CF_CLI_WORKSPACE environment variable.
func workspaceSelector(cmd *cobra.Command) string {
	if sel, _ := cmd.Flags().GetString("workspace"); sel != "" {
		return sel
	}
	return os.Getenv("CF_CLI_WORKSPACE")
}

// authedClient resolves the active account (honoring --workspace / CF_CLI_WORKSPACE,
// or the only signed-in workspace) and returns a generated, auth-wired API
// client for it plus the account.
func authedClient(cmd *cobra.Command) (*api.ClientWithResponses, *config.Account, error) {
	store, err := config.LoadStore()
	if err != nil {
		return nil, nil, err
	}
	account, err := store.Resolve(workspaceSelector(cmd))
	if err != nil {
		return nil, nil, err
	}
	client, err := api.New(account.BaseURL(), account.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	return client, account, nil
}
