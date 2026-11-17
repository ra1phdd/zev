package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/spf13/cobra"
)

const (
	machineFixtureName = "fixture"
	machinePrintFlag   = "--print"
	machineOptionFlag  = "--option"
	machineSafeName    = "safe"
	machineDeployName  = "deploy"
)

type machineFixtureProvider struct {
	response *domain.OptionsResponse
}

func (p machineFixtureProvider) GetOptions(context.Context, string, string) (*domain.OptionsResponse, error) {
	return p.response, nil
}

func (p machineFixtureProvider) GetOptionsWithHistory(context.Context, *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	return p.response, nil
}
func (p machineFixtureProvider) Name() string  { return machineFixtureName }
func (p machineFixtureProvider) Model() string { return "fixture-model" }

func TestMachineProviderWritesOnlyResultToStdout(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	err := runMachineProvider(cmd, parsedFlags{Print: true, Option: 2}, machineProviderParams{
		Context:  func() (string, error) { return "fixture context", nil },
		Provider: machineFixtureProvider{response: &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "echo first"}, {Command: "echo second"}}}},
		Prompt:   "fixture query",
		Logger:   slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "echo second\n" {
		t.Fatalf("stdout=%q", out.String())
	}
	if !strings.Contains(stderr.String(), "Fetching suggestions...") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestMachineProviderJSONOutput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		json   bool
		option int
		want   string
		code   int
	}{
		{name: "default", option: 1, want: "touch never-executed\n"},
		{name: "second", option: 2, want: "echo second\n"},
		{name: "third", option: 3, want: "echo third\n"},
		{name: "missing", option: 4, code: 2},
		{name: "json", json: true, option: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			response := &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "touch never-executed"}, {Command: "echo second", IsDangerous: true, DangerousExplanation: "fixture warning"}, {Command: "echo third"}}}
			var out, stderr bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&out)
			cmd.SetErr(&stderr)
			err := writeMachineResult(cmd, parsedFlags{Print: !tc.json, JSON: tc.json, Option: tc.option}, response)
			if got := cliExitCode(err); got != tc.code {
				t.Fatalf("code=%d error=%v", got, err)
			}
			if tc.code != 0 {
				if out.Len() != 0 {
					t.Fatalf("failure stdout=%q", out.String())
				}
				return
			}
			if tc.json {
				var obj map[string]json.RawMessage
				if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
					t.Fatal(err)
				}
				if string(obj["schema_version"]) != "1" || string(obj["status"]) != `"ok"` || string(obj["selected_option"]) != "2" {
					t.Fatalf("schema=%s", out.Bytes())
				}
				var commands []domain.Command
				if err := json.Unmarshal(obj["commands"], &commands); err != nil || len(commands) != 3 {
					t.Fatalf("commands=%s err=%v", obj["commands"], err)
				}
			} else if out.String() != tc.want {
				t.Fatalf("stdout=%q", out.String())
			}
			if tc.option == 2 && !bytes.Contains(stderr.Bytes(), []byte("fixture warning")) {
				t.Fatalf("warning absent: %q", stderr.String())
			}
		})
	}
}

func TestMachineFlags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args   []string
		option int
		code   int
	}{
		{args: []string{machinePrintFlag, historyValidationQuery}, option: 1},
		{args: []string{historyJSONFlag, machineOptionFlag, "2", historyValidationQuery}, option: 2},
		{args: []string{machinePrintFlag, "--option=3", historyValidationQuery}, option: 3},
		{args: []string{machinePrintFlag, machineOptionFlag, "0", historyValidationQuery}, code: 2},
		{args: []string{machinePrintFlag, machineOptionFlag, "oops", historyValidationQuery}, code: 2},
		{args: []string{machinePrintFlag, machineOptionFlag}, code: 2},
		{args: []string{machinePrintFlag, "--json", historyValidationQuery}, code: 2},
		{args: []string{machinePrintFlag, "--execute", historyValidationQuery}, code: 2},
		{args: []string{machineOptionFlag, "2", historyValidationQuery}, code: 2},
	} {
		p := parseArgsFlags(tc.args)
		err := validateMachineFlags(p)
		if cliExitCode(err) != tc.code {
			t.Errorf("%v: %v", tc.args, err)
		}
		if tc.code == 0 && p.Option != tc.option {
			t.Errorf("option=%d want=%d", p.Option, tc.option)
		}
	}
}

func TestMachineProviderAndContextErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		code int
	}{
		{err: errors.New("provider failed"), code: 3},
		{err: domain.ErrInvalidOptionsResponse, code: 4},
		{err: domain.ErrNoResponse, code: 4},
	} {
		if got := cliExitCode(machineProviderError(tc.err)); got != tc.code {
			t.Errorf("error=%v code=%d", tc.err, got)
		}
	}
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	err := runMachineProvider(cmd, parsedFlags{Print: true, Option: 1}, machineProviderParams{Context: func() (string, error) { return "", errors.New("context unavailable") }, Logger: slog.New(slog.DiscardHandler)})
	if cliExitCode(err) != 5 || out.Len() != 0 {
		t.Fatalf("context error=%v stdout=%q", err, out.String())
	}
}

func TestMachineNamedQueriesNeverExecute(t *testing.T) {
	// HOME and provider settings are process-wide.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home, "executed")
	if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte("safe: touch "+marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts"), []byte("deploy:\n  script: deploy.sh\n  description: fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{machineSafeName, machineDeployName} {
		var out, stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		cmd.SetOut(&out)
		cmd.SetErr(&stderr)
		handled, err := machineNamedQuery(cmd, parsedFlags{Print: true, Option: 1, Args: []string{name}})
		if !handled {
			t.Fatalf("not handled: %s", name)
		}
		if name == machineSafeName {
			if err != nil || out.String() != "touch "+marker+"\n" {
				t.Fatalf("alias: %q %v", out.String(), err)
			}
		} else if cliExitCode(err) != 2 || out.Len() != 0 {
			t.Fatalf("script: %q %v", out.String(), err)
		}
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("command executed: %v", err)
	}
}
