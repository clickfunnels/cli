package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/clickfunnels/cli/internal/api"
	"github.com/clickfunnels/cli/internal/output"
	"github.com/clickfunnels/cli/internal/ui"
)

func newTeamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teams",
		Short: "Manage teams",
	}
	cmd.AddCommand(newTeamsListCmd())
	return cmd
}

func newTeamsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List teams for your account",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			client, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListTeamsWithResponse(cmd.Context(), &api.ListTeamsParams{})
			if err != nil {
				return err
			}
			teams := derefSlice(resp.JSON200)

			if format == output.Table && len(teams) == 0 {
				fmt.Println(ui.Subtle.Render("No teams found."))
				return nil
			}
			return output.Collection(os.Stdout, format, teamColumns(), teams)
		},
	}
	return cmd
}

func teamColumns() []output.Column[api.TeamAttributes] {
	return []output.Column[api.TeamAttributes]{
		{Header: "ID", Value: func(t api.TeamAttributes) string { return str(t.PublicId) }},
		{Header: "NAME", Value: func(t api.TeamAttributes) string { return str(t.Name) }},
		{Header: "TIME ZONE", Value: func(t api.TeamAttributes) string { return str(t.TimeZone) }},
		{Header: "LOCALE", Value: func(t api.TeamAttributes) string {
			if t.Locale != nil {
				return string(*t.Locale)
			}
			return ""
		}},
	}
}
