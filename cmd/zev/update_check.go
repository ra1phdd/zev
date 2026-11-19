package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	updateclient "github.com/ra1phdd/zev/internal/entities/update/client"
	updaterepo "github.com/ra1phdd/zev/internal/entities/update/repository"
	updateservice "github.com/ra1phdd/zev/internal/entities/update/service"
)

const (
	updateCheckTimeout         = 55 * time.Second
	updateCheckShutdownTimeout = 250 * time.Millisecond
)

type (
	backgroundCheck       func(context.Context) (string, error)
	backgroundCheckResult struct {
		value string
		err   error
	}
)

func startBoundedBackgroundCheck(ctx context.Context, timeout time.Duration, check backgroundCheck) func() (string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	result := make(chan backgroundCheckResult, 1)
	go func() {
		defer cancel()
		value, err := check(checkCtx)
		result <- backgroundCheckResult{value: value, err: err}
	}()
	var once sync.Once
	var completed backgroundCheckResult
	return func() (string, error) {
		once.Do(func() {
			cancel()
			timer := time.NewTimer(updateCheckShutdownTimeout)
			defer timer.Stop()
			select {
			case completed = <-result:
			case <-timer.C:
				completed.err = context.DeadlineExceeded
			}
		})
		return completed.value, completed.err
	}
}

func checkUpdatesOnStartup(ctx context.Context, proxyURL string, explicitProxy bool) func() (string, error) {
	if explicitProxy && proxyURL == "" {
		return func() (string, error) { return "", nil }
	}
	dir, err := config.GetZevDir()
	if err != nil {
		return func() (string, error) { return "", fmt.Errorf("resolve update check directory: %w", err) }
	}
	branch := branchFromVersion(version)
	cache := updaterepo.NewCheckCache(filepath.Join(dir, "update-check.json"))
	return startBoundedBackgroundCheck(ctx, updateCheckTimeout, func(checkCtx context.Context) (string, error) {
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
		if args[i] == "--proxy" {
			if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "--") {
				return "", true
			}
			return args[i+1], true
		}
		if value, ok := strings.CutPrefix(args[i], "--proxy="); ok {
			return value, true
		}
	}
	return "", false
}
