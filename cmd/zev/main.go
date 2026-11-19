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

const startupProxyFlag = "--proxy"

func branchFromVersion(buildVersion string) updateservice.Branch {
	if strings.HasPrefix(buildVersion, "main-") {
		return updateservice.Main
	}
	return updateservice.Stable
}

func executeAfterStartupUpdateCheck(
	branch updateservice.Branch,
	finish func() (string, error),
	writeNotice func(updateservice.Branch, string),
	execute func() int,
) int {
	latest, err := finish()
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("Startup update check failed", "error", err)
	}
	if latest != "" {
		writeNotice(branch, latest)
	}
	return execute()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if skipStartupUpdateCheck(os.Args[1:], version) {
		code := cli.Execute(ctx, cli.NewRootCommandContext(ctx, version))
		stop()
		os.Exit(code)
	}
	proxyURL, explicitProxy := startupProxy(os.Args[1:])
	code := executeAfterStartupUpdateCheck(
		branchFromVersion(version),
		func() (string, error) { return checkUpdatesOnStartup(ctx, proxyURL, explicitProxy) },
		writeUpdateAvailable,
		func() int { return cli.Execute(ctx, cli.NewRootCommandContext(ctx, version)) },
	)
	stop()
	os.Exit(code)
}

func skipStartupUpdateCheck(args []string, buildVersion string) bool {
	if buildVersion == "dev" {
		return true
	}
	if len(args) == 0 {
		return false
	}
	positionals, machineOutput, help := startupPositionals(args)
	if help || machineOutput {
		return true
	}
	if len(positionals) == 0 {
		return false
	}
	if positionals[0] == "version" {
		return true
	}
	return len(positionals) == 2 &&
		(positionals[0] == "alias" || positionals[0] == "script" || positionals[0] == "provider") &&
		positionals[1] == "list"
}

func startupPositionals(args []string) ([]string, bool, bool) {
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return nil, false, false
		}
		if arg == "-h" || arg == "--help" {
			return nil, false, true
		}
		if takesStartupValue(arg) {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		if hasStartupValue(arg) {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if arg == "--print" || arg == "--json" {
				return nil, true, false
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return positionals, false, false
}

func takesStartupValue(arg string) bool {
	return arg == startupProxyFlag || arg == "--context-file" || arg == "-cf" || arg == "--option"
}

func hasStartupValue(arg string) bool {
	return strings.HasPrefix(arg, startupProxyFlag+"=") || strings.HasPrefix(arg, "--context-file=") || strings.HasPrefix(arg, "--option=")
}
