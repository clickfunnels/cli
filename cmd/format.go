package cmd

import (
	"github.com/spf13/cobra"

	"github.com/clickfunnels/cli/internal/output"
)

// outputFormat reads and validates the global --output/-o flag.
func outputFormat(cmd *cobra.Command) (output.Format, error) {
	s, _ := cmd.Flags().GetString("output")
	return output.Parse(s)
}
