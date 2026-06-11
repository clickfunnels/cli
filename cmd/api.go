package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/clickfunnels/cli/internal/api"
)

func newAPICmd() *cobra.Command {
	var (
		data  string
		input string
	)
	cmd := &cobra.Command{
		Use:   "api [METHOD] <path>",
		Short: "Make an authenticated request to any API endpoint",
		Long: "Escape hatch for endpoints the CLI doesn't model yet. The path is " +
			"relative to the workspace API base (/api/v2).\n\n" +
			"Examples:\n" +
			"  cf api /teams\n" +
			"  cf api GET /workspaces/42/contacts\n" +
			"  cf api POST /workspaces/42/contacts -d '{\"contact\":{\"email_address\":\"a@b.com\"}}'\n" +
			"  cf api PUT /contacts/gjDMvQ --input contact.json",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, account, err := authedClient(cmd)
			if err != nil {
				return err
			}

			// One arg → GET <path>; two args → <METHOD> <path>.
			method, path := "GET", args[0]
			if len(args) == 2 {
				method, path = strings.ToUpper(args[0]), args[1]
			}

			var body []byte
			switch {
			case data != "" && input != "":
				return fmt.Errorf("pass only one of --data or --input")
			case input != "":
				if body, err = readInput(input); err != nil {
					return err
				}
			case data != "":
				body = []byte(data)
			}

			resp, err := api.RawRequest(cmd.Context(), account.BaseURL(), account.AccessToken, method, path, body)
			if err != nil {
				return err
			}

			// Pretty-print JSON bodies; pass anything else through verbatim.
			out := resp.Body
			var pretty bytes.Buffer
			if json.Indent(&pretty, resp.Body, "", "  ") == nil {
				out = pretty.Bytes()
			}
			fmt.Println(strings.TrimRight(string(out), "\n"))

			// Body went to stdout; signal failure (to stderr/exit code) on non-2xx.
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&data, "data", "d", "", "request body as an inline JSON string")
	cmd.Flags().StringVar(&input, "input", "", "request body from a file, or '-' for stdin")
	return cmd
}
