// Package main starts the Zev command-line application.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ra1phdd/zev/internal/cli"
	updateservice "github.com/ra1phdd/zev/internal/entities/update/service"
)

var version = "dev"

func branchFromVersion(buildVersion string) updateservice.Branch {
	if strings.HasPrefix(buildVersion, "main-") {
		return updateservice.Main
	}
	return updateservice.Stable
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	proxyURL, explicitProxy := startupProxy(os.Args[1:])
	finishUpdateCheck := checkUpdatesOnStartup(ctx, proxyURL, explicitProxy)
	root := cli.NewRootCommand(version)
	code := cli.Execute(ctx, root)
	latest, err := finishUpdateCheck()
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("Startup update check failed", "error", err)
	}
	if latest != "" {
		writeUpdateAvailable(branchFromVersion(version), latest)
	}
	stop()
	os.Exit(code)
}
