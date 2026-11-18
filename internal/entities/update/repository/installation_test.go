package repository

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const windowsPlatform = "windows"

const fixtureSource = `package main
import("fmt";"os")
func main(){
 if len(os.Args)==3&&os.Args[1]=="version"&&os.Args[2]=="--update-build" {fmt.Print("1.2.3\n");return}
 if len(os.Args)==3&&os.Args[1]=="version"&&os.Args[2]=="--update-protocol" {fmt.Print("zev-update-v1\n");return}
 if len(os.Args)==3&&os.Args[1]=="update"&&os.Args[2]=="--migrate-only"{if err:=os.WriteFile(os.Getenv("UPDATE_FIXTURE_MARKER"),[]byte("new binary migrated"),0600);err!=nil{panic(err)};return}
 panic("unexpected argv")
}`

func fixtureBinary(t *testing.T, supported bool) []byte {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "fixture.go")
	source := fixtureSource
	if !supported {
		source = strings.ReplaceAll(source, "zev-update-v1", "unsupported")
	}
	if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	bin, err := filepath.Abs("../../../../bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	artifacts, err := os.MkdirTemp(bin, "zev-update-test-*") //nolint:usetesting // Compiled artifacts must stay in the repository bin directory.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(artifacts); err != nil {
			t.Errorf("clean fixture binaries: %v", err)
		}
	})
	path := filepath.Join(artifacts, "zev")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", path, src) //nolint:gosec // Test fixture invokes the pinned Go tool with temporary paths.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, out)
	}
	data, err := os.ReadFile(path) //nolint:gosec // Path is a test-owned temporary fixture.
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestInstallProbesBeforeReplacement(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == windowsPlatform {
		t.Skip("atomic replacement unsupported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "zev")
	original := []byte("old executable")
	if err := os.WriteFile(path, original, 0o700); err != nil { //nolint:gosec // The executable fixture must be runnable.
		t.Fatal(err)
	}
	r := New("")
	r.executable = func() (string, error) { return path, nil }
	err := r.WithLock(t.Context(), func() error { _, err := r.Install(t.Context(), fixtureBinary(t, false), "1.2.3"); return err })
	if err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("err=%v", err)
	}
	got, err := os.ReadFile(path) //nolint:gosec // Path is a test-owned temporary fixture.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("unsupported binary replaced original")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("leaked stage: %v", entries)
	}
}

func TestInstallAndRunNewBinary(t *testing.T) {
	if runtime.GOOS == windowsPlatform {
		t.Skip("atomic replacement unsupported on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "zev")
	link := filepath.Join(dir, "link")
	marker := filepath.Join(dir, "migrated")
	t.Setenv("UPDATE_FIXTURE_MARKER", marker)
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil { //nolint:gosec // The executable fixture must be runnable.
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	r := New("")
	r.executable = func() (string, error) { return link, nil }
	data := fixtureBinary(t, true)
	err := r.WithLock(t.Context(), func() error {
		path, err := r.Install(t.Context(), data, "1.2.3")
		if err != nil {
			return err
		}
		if path != target {
			resolvedTarget, resolveErr := filepath.EvalSymlinks(target)
			if resolveErr != nil || path != resolvedTarget {
				t.Errorf("installed path=%q", path)
			}
		}
		return r.RunMigrations(t.Context(), path)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker) //nolint:gosec // Path is a test-owned temporary fixture.
	if err != nil || string(got) != "new binary migrated" {
		t.Fatalf("marker=%q err=%v", got, err)
	}
	got, err = os.ReadFile(target) //nolint:gosec // Path is a test-owned temporary fixture.
	if err != nil || !bytes.Equal(data, got) {
		t.Fatal("new binary not installed", err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("installation permissions were not preserved: %v, %v", info, err)
	}
}

func TestInstallRejectsWrongCandidateVersion(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == windowsPlatform {
		t.Skip("atomic replacement unsupported on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "zev")
	original := []byte("old executable")
	if err := os.WriteFile(target, original, 0o700); err != nil { //nolint:gosec // Executable installation fixture.
		t.Fatal(err)
	}
	r := New("")
	r.executable = func() (string, error) { return target, nil }
	data := fixtureBinary(t, true)
	err := r.WithLock(t.Context(), func() error { _, err := r.Install(t.Context(), data, "2.0.0"); return err })
	if err == nil {
		t.Fatal("candidate reporting 1.2.3 was installed as 2.0.0")
	}
	got, readErr := os.ReadFile(target) //nolint:gosec // Reads the test-owned installation fixture.
	if readErr != nil || !bytes.Equal(got, original) {
		t.Fatalf("wrong-version candidate replaced installation: %v", readErr)
	}
}

func TestUpdateRefusesStaleRunningVersion(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == windowsPlatform {
		t.Skip("atomic replacement unsupported on Windows")
	}
	target := filepath.Join(t.TempDir(), "zev")
	if err := os.WriteFile(target, fixtureBinary(t, true), 0o700); err != nil { //nolint:gosec // Executable installation fixture.
		t.Fatal(err)
	}
	r := New("1.0.0")
	r.executable = func() (string, error) { return target, nil }
	called := false
	err := r.WithLock(t.Context(), func() error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("stale running binary authorized update: callback=%t, error=%v", called, err)
	}
}

func TestProtocolCaptureCannotBypassLimitThroughCopy(t *testing.T) {
	t.Parallel()
	var out limitedOutput
	_, err := io.Copy(&out, io.LimitReader(strings.NewReader(strings.Repeat("x", 4096)), 4096))
	if err == nil || out.Len() > 1024 {
		t.Fatalf("capture bypassed bound: len=%d, err=%v", out.Len(), err)
	}
}

func TestInstallRequiresLockAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	r := New("")
	if _, err := r.Install(t.Context(), nil, "1.2.3"); err == nil {
		t.Fatal("unlocked installation accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.WithLock(ctx, func() error { t.Error("canceled callback"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}
