package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	updateservice "github.com/ra1phdd/zev/internal/entities/update/service"
	"github.com/spf13/cobra"
)

type updateRunnerFake struct {
	result      updateservice.Result
	err         error
	calls       int
	hasDeadline bool
	deadline    time.Time
}

func (f *updateRunnerFake) Update(ctx context.Context) (updateservice.Result, error) {
	f.calls++
	f.deadline, f.hasDeadline = ctx.Deadline()
	return f.result, f.err
}

func TestUpdateOutputOnlyReportsVerifiedSuccess(t *testing.T) {
	t.Setenv("ZEV_LANG", "en")
	failure := errors.New("download unavailable")
	for _, tc := range []struct {
		name   string
		result updateservice.Result
		err    error
		want   string
	}{
		{name: "installed", result: updateservice.Result{Version: "1.2.3", Updated: true}, want: "Zev 1.2.3 installed; all migrations completed."},
		{name: "current", result: updateservice.Result{Version: "2.0.0"}, want: "Zev 2.0.0 is current or newer; all migrations completed."},
		{name: "failure", err: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setFixtureLanguage(t, newCLIConfigHome(t), "en")
			var output, diagnostics bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			cmd.SetOut(&output)
			cmd.SetErr(&diagnostics)
			updater := &updateRunnerFake{result: tc.result, err: tc.err}
			err := runUpdate(cmd, updater)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error=%v, want %v", err, tc.err)
			}
			if updater.calls != 1 || !updater.hasDeadline {
				t.Fatalf("unbounded or missing update: %#v", updater)
			}
			remaining := time.Until(updater.deadline)
			if remaining <= 59*time.Second || remaining > time.Minute {
				t.Fatalf("update timeout = %s, want 1m", remaining)
			}
			if tc.err != nil && output.Len() != 0 {
				t.Fatalf("failure reported success: %s", output.String())
			}
			if tc.want != "" && !strings.Contains(output.String(), tc.want) {
				t.Fatalf("output=%q, want %q", output.String(), tc.want)
			}
			if !strings.Contains(diagnostics.String(), "Checking the latest stable Zev release") {
				t.Fatalf("missing progress: %s", diagnostics.String())
			}
		})
	}
}

func TestUpdateOutputErrorsAreReturned(t *testing.T) {
	t.Setenv("ZEV_LANG", "en")
	failure := errors.New("output unavailable")
	for _, progressFails := range []bool{false, true} {
		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if progressFails {
			cmd.SetErr(failingOutput{err: failure})
		} else {
			cmd.SetOut(failingOutput{err: failure})
		}
		updater := &updateRunnerFake{result: updateservice.Result{Version: "1.2.3", Updated: true}}
		if err := runUpdate(cmd, updater); !errors.Is(err, failure) {
			t.Fatalf("error=%v, want output failure", err)
		}
		if progressFails && updater.calls != 0 {
			t.Fatal("updated after progress output failed")
		}
	}
}

//nolint:paralleltest // The fixture changes process-wide HOME and language state.
func TestMainUpdateReportsMainCheckingStatus(t *testing.T) {
	setFixtureLanguage(t, newCLIConfigHome(t), "en")
	var output, diagnostics bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetOut(&output)
	cmd.SetErr(&diagnostics)
	updater := &branchUpdateRunnerFake{result: updateservice.Result{Branch: updateservice.Main, Version: "main-commit"}}
	if err := runUpdateWithBranch(cmd, updater.UpdateBranch, updateservice.Main, updateservice.Main); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagnostics.String(), "latest main") {
		t.Fatalf("main checking status = %q", diagnostics.String())
	}
}

type branchUpdateRunnerFake struct {
	result updateservice.Result
}

func (f *branchUpdateRunnerFake) UpdateBranch(context.Context, updateservice.Branch) (updateservice.Result, error) {
	return f.result, nil
}
