package cli

import (
	"log/slog"
	"testing"
)

func TestDefaultLoggerShowsWarnings(t *testing.T) {
	t.Parallel()
	logger := setupLogger(0)
	if !logger.Enabled(t.Context(), slog.LevelWarn) {
		t.Fatal("warnings are hidden without -v")
	}
	if logger.Enabled(t.Context(), slog.LevelInfo) {
		t.Fatal("default logging should not enable info messages")
	}
}
