package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	updateclient "github.com/ra1phdd/zev/internal/entities/update/client"
	updaterepo "github.com/ra1phdd/zev/internal/entities/update/repository"
	updateservice "github.com/ra1phdd/zev/internal/entities/update/service"
)

const updateCheckTimeout = 5 * time.Second

func runSynchronousStartupCheck(ctx context.Context, check func(context.Context) (string, error)) (string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	return check(checkCtx)
}

func checkUpdatesOnStartup(ctx context.Context, proxyURL string, explicitProxy bool) (string, error) {
	if explicitProxy && proxyURL == "" {
		return "", nil
	}

	dir, err := config.GetZevDir()
	if err != nil {
		return "", fmt.Errorf("resolve update check directory: %w", err)
	}
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect update check directory: %w", err)
	}
	branch := branchFromVersion(version)
	cache := updaterepo.NewCheckCache(filepath.Join(dir, "update-check.json"))
	return runSynchronousStartupCheck(ctx, func(checkCtx context.Context) (string, error) {
		if entry, ok, err := cache.Load(); err == nil && ok && entry.Branch == branch && cache.IsFresh(entry, time.Now()) && !entry.Failed && updateservice.UpdateAvailable(branch, version, entry.Latest) {
			return entry.Latest, nil
		}
		client, err := updateclient.NewWithProxy(proxyURL)
		if err != nil {
			return "", fmt.Errorf("configure update proxy: %w", err)
		}
		entry, _, err := cache.Refresh(checkCtx, time.Now(), branch, version, func(checkCtx context.Context) (updateservice.CheckResult, error) {
			return updateservice.CheckLatest(checkCtx, client, branch, version)
		})
		if err != nil {
			return "", fmt.Errorf("refresh startup update check: %w", err)
		}
		if entry.Available {
			return entry.Latest, nil
		}
		return "", nil
	})
}

func updateAvailableMessage(branch updateservice.Branch, latest string) string {
	key := locale.KeyUpdateAvailable
	if branch == updateservice.Main {
		key = locale.KeyUpdateAvailableMain
	}
	return fmt.Sprintf(locale.Text(key), latest)
}

func writeUpdateAvailable(branch updateservice.Branch, latest string) {
	_, err := fmt.Fprint(os.Stderr, updateAvailableMessage(branch, latest))
	if err == nil {
		return
	}
	slog.Debug("Failed to report available update", "error", err)
}

func startupProxy(args []string) (string, bool) {
	for i, arg := range args {
		if arg == "--" {
			return "", false
		}
		if args[i] == startupProxyFlag {
			if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "--") {
				return "", true
			}
			return args[i+1], true
		}
		if value, ok := strings.CutPrefix(args[i], startupProxyFlag+"="); ok {
			return value, true
		}
	}
	return "", false
}
