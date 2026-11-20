package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

func TestExecuteTreatsInteractiveCancellationAsSuccess(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "context canceled", err: context.Canceled},
		{name: "wrapped context canceled", err: fmt.Errorf("select history: %w", context.Canceled)},
		{name: "tui canceled", err: errors.New(queryCancelled)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := &cobra.Command{Use: "zev", SilenceErrors: true, SilenceUsage: true, RunE: func(*cobra.Command, []string) error { return tc.err }}
			var stderr bytes.Buffer
			root.SetErr(&stderr)
			if code := Execute(t.Context(), root); code != 0 {
				t.Fatalf("Execute() code = %d, want 0; stderr = %q", code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("cancellation wrote an error: %q", stderr.String())
			}
		})
	}
}
