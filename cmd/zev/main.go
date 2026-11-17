// Package main starts the Zev command-line application.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ra1phdd/zev/internal/cli"
)

var version = "dev"

func main() {
	root := cli.NewRootCommand(version)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Execute(ctx, root)
	stop()
	os.Exit(code)
}
