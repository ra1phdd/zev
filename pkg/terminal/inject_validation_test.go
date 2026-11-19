package terminal

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

// This test replaces process-wide stdin to ensure no input can reach a real TTY.
func TestInjectValidatesBeforeTerminalAccess(t *testing.T) { //nolint:paralleltest // os.Stdin is process-wide.
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() {
		os.Stdin = original
		if err := stdin.Close(); err != nil {
			t.Error(err)
		}
	})

	tests := []struct {
		name string
		text string
	}{
		{name: "empty"},
		{name: "whitespace", text: "   "},
		{name: "NUL", text: "echo safe\x00echo unsafe"},
		{name: "ESC", text: "echo safe\x1b[2J"},
		{name: "LF", text: "echo safe\necho unsafe"},
		{name: "CR", text: "echo safe\recho unsafe"},
		{name: "CRLF", text: "echo safe\r\necho unsafe"},
		{name: "trailing LF", text: "echo unsafe\n"},
		{name: "tab completion", text: "echo\tunsafe"},
		{name: "backspace", text: "echo safe\bunsafe"},
		{name: "DEL", text: "echo safe\x7f"},
		{name: "C1 control", text: "echo safe\u0085"},
		{name: "invalid UTF-8", text: "echo \xff"},
		{name: "oversized", text: strings.Repeat("x", domain.MaxCommandLength+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Inject(tt.text); !errors.Is(err, ErrInjectionFailed) {
				t.Fatalf("Inject() error = %v, want ErrInjectionFailed before TTY access", err)
			}
		})
	}

	for _, text := range []string{"echo hello", "printf '%s' 'Привет 世界'", strings.Repeat("x", domain.MaxCommandLength)} {
		want := ErrNotTTY
		if runtime.GOOS == "windows" {
			want = ErrInjectionUnsupported
		}
		if err := Inject(text); !errors.Is(err, want) {
			t.Errorf("safe Inject() error = %v, want %v", err, want)
		}
	}
}

func TestPartialInjectionErrorPreservesCauseAndWrittenCount(t *testing.T) {
	t.Parallel()
	cause := errors.New("ioctl failed")
	err := &PartialInjectionError{Written: 2, Err: cause}
	if !errors.Is(err, cause) || err.Written != 2 {
		t.Fatalf("partial error = %v, written=%d", err, err.Written)
	}
}
