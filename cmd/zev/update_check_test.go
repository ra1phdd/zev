package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/locale"
	updateservice "github.com/ra1phdd/zev/internal/entities/update/service"
)

const testStableVersion = "1.2.3"

func TestUpdateCheckLifecycleWaitsForCompletionAndReturnsErrors(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	finished := make(chan struct{})
	finish := startBoundedBackgroundCheck(ctx, time.Second, func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return "", ctx.Err()
	})
	<-started
	cancel()
	_, err := finish()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("finish error = %v, want canceled check error", err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("background check remained running after finish")
	}
}

func TestUpdateCheckLifecycleReturnsSuccessfulValue(t *testing.T) {
	t.Parallel()
	finish := startBoundedBackgroundCheck(t.Context(), time.Second, func(context.Context) (string, error) {
		return testStableVersion, nil
	})
	got, err := finish()
	if err != nil || got != testStableVersion {
		t.Fatalf("finish() = (%q, %v), want version", got, err)
	}
}

func TestUpdateCheckLifecycleReturnsDeadlineError(t *testing.T) {
	t.Parallel()
	finished := make(chan struct{})
	finish := startBoundedBackgroundCheck(t.Context(), time.Millisecond, func(ctx context.Context) (string, error) {
		<-ctx.Done()
		close(finished)
		return "", ctx.Err()
	})
	<-finished
	_, err := finish()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("finish error = %v, want deadline exceeded", err)
	}
}

func TestUpdateCheckFinishCancelsInFlightCheck(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	finish := startBoundedBackgroundCheck(ctx, time.Minute, func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	<-started

	done := make(chan error, 1)
	go func() {
		_, err := finish()
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("finish error = %v, want canceled check", err)
		}
	case <-time.After(time.Second):
		t.Fatal("finish waited for the update check instead of canceling it")
	}
}

func TestBranchFromVersion(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		version string
		want    updateservice.Branch
	}{
		{version: "main-0123456789abcdef", want: updateservice.Main},
		{version: testStableVersion, want: updateservice.Stable},
		{version: "dev", want: updateservice.Stable},
	} {
		t.Run(test.version, func(t *testing.T) {
			t.Parallel()
			if got := branchFromVersion(test.version); got != test.want {
				t.Fatalf("branchFromVersion(%q) = %q, want %q", test.version, got, test.want)
			}
		})
	}
}

//nolint:paralleltest // The test changes the process-wide locale.
func TestUpdateAvailableMessageMatchesBranch(t *testing.T) {
	restore := locale.SetLanguage("en", "")
	t.Cleanup(restore)
	stable := updateAvailableMessage(updateservice.Stable, testStableVersion)
	main := updateAvailableMessage(updateservice.Main, "main-abc123")
	if !strings.Contains(stable, "zev update`") || !strings.Contains(stable, testStableVersion) {
		t.Fatalf("stable update message = %q", stable)
	}
	if !strings.Contains(main, "zev update main`") || !strings.Contains(main, "main-abc123") {
		t.Fatalf("main update message = %q", main)
	}
}

func TestStartupProxy(t *testing.T) {
	t.Parallel()
	const updateCommand = "update"
	const proxyFlag = "--proxy"
	for _, test := range []struct {
		name     string
		args     []string
		want     string
		explicit bool
	}{
		{name: "separate root flag", args: []string{proxyFlag, "socks5://proxy.example:1080", "query"}, want: "socks5://proxy.example:1080", explicit: true},
		{name: "equals child flag", args: []string{updateCommand, "--proxy=https://proxy.example:8443"}, want: "https://proxy.example:8443", explicit: true},
		{name: "empty explicit", args: []string{"update", "--proxy="}, explicit: true},
		{name: "missing explicit", args: []string{updateCommand, proxyFlag}, explicit: true},
		{name: "after delimiter is child data", args: []string{"query", "--", proxyFlag, "not-a-proxy"}},
		{name: "not configured", args: []string{"version"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, explicit := startupProxy(test.args)
			if got != test.want || explicit != test.explicit {
				t.Fatalf("startupProxy(%q) = (%q, %t), want (%q, %t)", test.args, got, explicit, test.want, test.explicit)
			}
		})
	}
}
