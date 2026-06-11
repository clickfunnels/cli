// Command cf is a terminal client for the ClickFunnels API, built on the Charm
// stack (Fang + Cobra + Bubble Tea + Lip Gloss).
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/charmbracelet/fang"

	"github.com/clickfunnels/cli/cmd"
)

func main() {
	// Cancel the context on SIGINT/SIGTERM so in-flight requests and the TUI
	// shut down cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	root := cmd.NewRootCmd()

	// Fang gives us styled help, errors, --version, and completion for free.
	if err := fang.Execute(ctx, root,
		fang.WithVersion(cmd.Version),
	); err != nil {
		os.Exit(1)
	}
}
