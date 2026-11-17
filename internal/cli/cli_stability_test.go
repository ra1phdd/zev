package cli

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestDoctorRejectsConflictingFormatsBeforeConfig(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	loads := 0
	deps := doctorDeps{
		loadConfig:     func() (*config.Config, error) { loads++; return doctorFixtureConfig(), nil },
		checkDirectory: func(string) error { return nil },
		checkHistory:   func(context.Context, string) error { return nil },
		findExecutable: func(string) (string, error) { return doctorFixturePath, nil },
	}
	cmd := newDoctorCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := runDoctor(cmd, doctorOptions{JSON: true, Print: true}, deps); err == nil {
		t.Error("conflicting formats accepted")
	}
	if loads != 0 || out.Len() != 0 {
		t.Fatalf("conflict performed config I/O or wrote output: loads=%d bytes=%d", loads, out.Len())
	}
}

func TestHistorySearchRejectsInvalidBoundaries(t *testing.T) {
	t.Parallel()
	for _, req := range []historyservice.SearchRequest{
		{ExactCommand: " 	", Limit: 1, Mode: historySearchModeLexical, Fuzzy: toolModeAuto},
		{Query: toolTestPrompt, ExactCommand: " 	", Limit: 1, Mode: historySearchModeLexical, Fuzzy: toolModeAuto},
		{Query: toolTestPrompt, Limit: 1001, Mode: historySearchModeLexical, Fuzzy: toolModeAuto},
	} {
		if err := validateHistorySearch(&req, "", "", false, false); err == nil {
			t.Errorf("invalid request accepted: %+v", req)
		}
	}
}
