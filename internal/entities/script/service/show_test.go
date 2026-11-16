package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestShowWritesExactBodyWithoutExecutingIt(t *testing.T) {
	svc, _ := newTestService(t)
	body := "#!/usr/bin/env bash\nprintf 'marker' > marker\n"
	if err := svc.repo.Create(t.Context(), domain.Script{Name: "safe", ScriptFile: safeScriptFile}, []byte(body)); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := svc.Show(t.Context(), "safe", &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != body {
		t.Fatalf("Show() body = %q, want %q", got, body)
	}
	if _, err := os.Lstat(filepath.Join("marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("show executed script marker: %v", err)
	}
}

func TestShowRejectsMissingScriptBeforeReading(t *testing.T) {
	svc, _ := newTestService(t)
	var out bytes.Buffer

	err := svc.Show(t.Context(), "missing", &out)
	if !errors.Is(err, ErrScriptNotFound) {
		t.Fatalf("Show() error = %v, want ErrScriptNotFound", err)
	}
	if out.Len() != 0 {
		t.Fatalf("Show() wrote %d bytes for missing script", out.Len())
	}
}

func TestShowRejectsSymlinkScript(t *testing.T) {
	svc, dir := newTestService(t)
	outside := filepath.Join(t.TempDir(), "outside.sh")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	scriptsDir := filepath.Join(dir, "script_files")
	if err := os.MkdirAll(scriptsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(scriptsDir, safeScriptFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts"), []byte("safe:\n  script: "+safeScriptFile+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := svc.Show(t.Context(), "safe", &out); err == nil {
		t.Fatal("Show() accepted symlink script")
	}
	if out.Len() != 0 {
		t.Fatalf("Show() wrote %d bytes for rejected symlink", out.Len())
	}
}

func TestShowStopsOnCancellationDuringOutput(t *testing.T) {
	svc, _ := newTestService(t)
	body := "#!/usr/bin/env bash\nprintf marker\n"
	if err := svc.repo.Create(t.Context(), domain.Script{Name: "safe", ScriptFile: safeScriptFile}, []byte(body)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out cancelWriter
	out.cancel = cancel

	err := svc.Show(ctx, "safe", &out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Show() error = %v, want context.Canceled", err)
	}
	if got := out.buffer.String(); got != body {
		t.Fatalf("Show() wrote %q, want exact body before cancellation", got)
	}
}

func TestShowReturnsOutputError(t *testing.T) {
	svc, _ := newTestService(t)
	if err := svc.repo.Create(t.Context(), domain.Script{Name: "safe", ScriptFile: safeScriptFile}, []byte("body")); err != nil {
		t.Fatal(err)
	}
	outputErr := errors.New("output unavailable")

	err := svc.Show(t.Context(), "safe", failingWriter{err: outputErr})
	if !errors.Is(err, outputErr) {
		t.Fatalf("Show() error = %v, want output error", err)
	}
}

type cancelWriter struct {
	buffer bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	n, err := w.buffer.Write(p)
	w.cancel()
	return n, err
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

var _ io.Writer = (*cancelWriter)(nil)
var _ io.Writer = failingWriter{}
