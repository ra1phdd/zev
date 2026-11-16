package main

import (
	"log/slog"
	"testing"
)

func TestDefaultLoggerShowsRetries(t *testing.T) {
	t.Parallel()
	logger := setupLogger(0)
	if !logger.Enabled(t.Context(), slog.LevelWarn) {
		t.Fatal("retry warnings are hidden without -v")
	}
	if logger.Enabled(t.Context(), slog.LevelInfo) {
		t.Fatal("default logging should not enable info messages")
	}
}
