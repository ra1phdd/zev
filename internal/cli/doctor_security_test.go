package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

func TestDoctorDisclosesGlobalBypass(t *testing.T) {
	t.Parallel()
	cfg := doctorFixtureConfig()
	cfg.Security.Mode = config.SecurityModeBypass
	cfg.Security.WebFetchEnabled = true
	var out bytes.Buffer
	cmd := newDoctorCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	deps := doctorDeps{
		loadConfig:     func() (*config.Config, error) { return cfg, nil },
		checkDirectory: func(string) error { return nil },
		checkHistory:   func(context.Context, string) error { return nil },
		findExecutable: func(string) (string, error) { return doctorFixturePath, nil },
		networkCheck:   func(context.Context, *config.Config) error { return nil },
	}
	if err := runDoctor(cmd, doctorOptions{JSON: true}, deps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name":"security_mode"`) || !strings.Contains(out.String(), "command-delivery bypass") || !strings.Contains(out.String(), `"name":"web_fetch"`) {
		t.Fatalf("report = %q", out.String())
	}
}
