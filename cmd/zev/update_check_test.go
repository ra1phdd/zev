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

func TestRunSynchronousStartupCheckUsesFiveSecondTimeout(t *testing.T) {
	t.Parallel()
	_, err := runSynchronousStartupCheck(t.Context(), func(ctx context.Context) (string, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("startup check context has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 4*time.Second || remaining > 5*time.Second {
			t.Fatalf("startup check deadline in %s, want about 5s", remaining)
		}
		return "", nil
	})
	if err != nil {
		t.Fatalf("runSynchronousStartupCheck() error = %v", err)
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

func TestStartupUpdateNoticeIsWrittenBeforeCommandOutput(t *testing.T) {
	t.Parallel()
	var events []string
	code := executeAfterStartupUpdateCheck(
		updateservice.Stable,
		func() (string, error) { return testStableVersion, nil },
		func(updateservice.Branch, string) { events = append(events, "notice") },
		func() int {
			events = append(events, "command")
			return 7
		},
	)
	if code != 7 {
		t.Fatalf("code = %d, want 7", code)
	}
	if got := strings.Join(events, ","); got != "notice,command" {
		t.Fatalf("events = %q, want notice,command", got)
	}
}

func TestStartupUpdateNoticeWaitsForSynchronousCheck(t *testing.T) {
	t.Parallel()
	var events []string
	code := executeAfterStartupUpdateCheck(
		updateservice.Stable,
		func() (string, error) {
			events = append(events, "check-start")
			events = append(events, "check-complete")
			return testStableVersion, nil
		},
		func(updateservice.Branch, string) { events = append(events, "notice") },
		func() int {
			events = append(events, "command")
			return 0
		},
	)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if got := strings.Join(events, ","); got != "check-start,check-complete,notice,command" {
		t.Fatalf("events = %q, want synchronous check before notice and command", got)
	}
}

func TestStartupUpdateFailureDoesNotPreventCommand(t *testing.T) {
	t.Parallel()
	commandRan := false
	code := executeAfterStartupUpdateCheck(
		updateservice.Stable,
		func() (string, error) { return "", errors.New("check failed") },
		func(updateservice.Branch, string) { t.Fatal("notice written without an available update") },
		func() int {
			commandRan = true
			return 9
		},
	)
	if code != 9 || !commandRan {
		t.Fatalf("code = %d, commandRan = %t; startup check failure blocked command", code, commandRan)
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
