package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

const (
	doctorTestInvalidMode = "invalid"
	doctorFixturePath     = "/bin/fixture"
)

func TestDoctorStaticChecksDoNotCallNetwork(t *testing.T) {
	t.Parallel()
	networkCalls := 0
	var out bytes.Buffer
	cmd := newDoctorCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{historyJSONFlag})
	deps := doctorDeps{
		loadConfig:     func() (*config.Config, error) { return doctorFixtureConfig(), nil },
		checkDirectory: func(string) error { return nil },
		checkHistory:   func(context.Context, string) error { return nil },
		findExecutable: func(string) (string, error) { return doctorFixturePath, nil },
		networkCheck:   func(context.Context, *config.Config) error { networkCalls++; return nil },
	}
	if err := runDoctor(cmd, doctorOptions{JSON: true}, deps); err != nil {
		t.Fatal(err)
	}
	if networkCalls != 0 {
		t.Fatalf("network calls = %d, want 0", networkCalls)
	}
	if !strings.Contains(out.String(), `"checks"`) || !strings.Contains(out.String(), `"status":"warning"`) {
		t.Fatalf("JSON report = %q", out.String())
	}
	for _, secret := range []string{"fixture-key", "Authorization", "Bearer", "http://provider.invalid"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("report leaked %q: %q", secret, out.String())
		}
	}
}

func TestDoctorConfirmRunsNetworkWithoutLeakingResponse(t *testing.T) {
	t.Parallel()
	networkCalls := 0
	var out bytes.Buffer
	cmd := newDoctorCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{historyJSONFlag, "--confirm"})
	deps := doctorDeps{
		loadConfig:     func() (*config.Config, error) { return doctorFixtureConfig(), nil },
		checkDirectory: func(string) error { return nil },
		checkHistory:   func(context.Context, string) error { return nil },
		findExecutable: func(string) (string, error) { return doctorFixturePath, nil },
		networkCheck: func(context.Context, *config.Config) error {
			networkCalls++
			return errors.New("private provider response")
		},
	}
	if err := runDoctor(cmd, doctorOptions{JSON: true, Confirm: true}, deps); err == nil {
		t.Fatal("doctor succeeded after failed network check")
	}
	if networkCalls != 1 || !strings.Contains(out.String(), `"name":"network"`) || !strings.Contains(out.String(), `"status":"error"`) {
		t.Fatalf("calls=%d report=%q", networkCalls, out.String())
	}
	if strings.Contains(out.String(), "private provider response") {
		t.Fatalf("report leaked provider error: %q", out.String())
	}
}

func TestDoctorStaticValidationAndHumanPrint(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cmd := newDoctorCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--print"})
	deps := doctorDeps{
		loadConfig: func() (*config.Config, error) {
			cfg := doctorFixtureConfig()
			cfg.CommandExecutionMode = doctorTestInvalidMode
			cfg.OpenAIModel = ""
			return cfg, nil
		},
		checkDirectory: func(string) error { return nil },
		checkHistory:   func(context.Context, string) error { return nil },
		findExecutable: func(string) (string, error) { return "", errors.New("missing") },
		networkCheck:   func(context.Context, *config.Config) error { t.Fatal("network called"); return nil },
	}
	if err := runDoctor(cmd, doctorOptions{Print: true}, deps); err == nil {
		t.Fatal("doctor succeeded with invalid static configuration")
	}
	if !strings.Contains(out.String(), "execution_mode") || !strings.Contains(out.String(), "action") {
		t.Fatalf("human report = %q", out.String())
	}
}

func TestDoctorInteractiveConfirmationRunsNetwork(t *testing.T) {
	t.Parallel()
	networkCalls := 0
	var out bytes.Buffer
	cmd := newDoctorCmd()
	cmd.SetOut(&out)
	deps := doctorDeps{
		loadConfig:      func() (*config.Config, error) { return doctorFixtureConfig(), nil },
		checkDirectory:  func(string) error { return nil },
		checkHistory:    func(context.Context, string) error { return nil },
		findExecutable:  func(string) (string, error) { return doctorFixturePath, nil },
		askConfirmation: func(context.Context) (bool, error) { return true, nil },
		networkCheck: func(context.Context, *config.Config) error {
			networkCalls++
			return nil
		},
	}
	if err := runDoctor(cmd, doctorOptions{}, deps); err != nil {
		t.Fatal(err)
	}
	if networkCalls != 1 || !strings.Contains(out.String(), "real provider request succeeded") {
		t.Fatalf("calls=%d report=%q", networkCalls, out.String())
	}
}

func doctorFixtureConfig() *config.Config {
	return &config.Config{
		LLMProvider: "openai", OpenAIModel: "fixture-model", OpenAIBaseURL: "https://provider.invalid/v1", OpenAIAPIKey: "fixture-key",
		CommandExecutionMode: "inject", ScriptEditor: "fixture-editor", HistoryEnabled: true, ContextEnabled: true,
		HTTPHeaders: map[string]string{"Authorization": "Bearer fixture-key"},
	}
}
