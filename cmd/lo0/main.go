// Command lo0 checks whether a developer's Claude Code work can run on a
// local model and harness.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"lo0.ai/lo0/internal/cli"
)

func main() {
	app, err := cli.FromOS()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lo0:", err)
		os.Exit(cli.ExitFailure)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := app.Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
