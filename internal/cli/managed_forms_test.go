package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type forbiddenManagedInput struct{ t *testing.T }

func (r forbiddenManagedInput) Read([]byte) (int, error) {
	r.t.Error("stdin read despite invalid command fields")
	return 0, io.EOF
}

func TestManagedInvalidBodyFlagsDoNotReadStdin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, args := range [][]string{
		{managedKindScript, managedOpCreate, historyBadValue, managedBodyStdinFlag, "--body-file", managedMissingFixture},
		{managedKindScript, managedOpEdit, historyBadValue, managedBodyStdinFlag, "--edit-body"},
		{managedKindScript, managedOpCreate, "../bad", managedBodyStdinFlag},
		{managedKindScript, managedOpCreate, historyBadValue, managedBodyStdinFlag, managedArgumentsJSONFlag, `[{"name":"x","required":null}]`},
	} {
		root := NewRootCommand("test")
		root.SetIn(forbiddenManagedInput{t})
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(args)
		if err := root.ExecuteContext(t.Context()); cliExitCode(err) != 2 {
			t.Errorf("%v: %v", args, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".zev")); !os.IsNotExist(err) {
		t.Fatalf("invalid command created storage: %v", err)
	}
}

func TestManagedInteractiveFormsPreserveAndCancel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SKIP_EDITOR", "1")
	previous := managedTerminal
	managedTerminal = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { managedTerminal = previous })
	run := func(input string, args ...string) error {
		root := NewRootCommand("test")
		var out, diagnostic bytes.Buffer
		root.SetIn(strings.NewReader(input))
		root.SetOut(&out)
		root.SetErr(&diagnostic)
		root.SetArgs(args)
		err := root.ExecuteContext(t.Context())
		if err != nil {
			return fmt.Errorf("run managed command: %w", err)
		}
		return nil
	}
	if err := run("pending\necho\n", managedKindAlias, managedOpAdd); !errors.Is(err, io.EOF) {
		t.Fatalf("EOF: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zev", "aliases")); !os.IsNotExist(err) {
		t.Fatal("cancelled form published")
	}
	if err := run("inspect\necho {{.arg1}}\nDescription\n[{\"name\":\"target\",\"required\":false}]\n", managedKindAlias, managedOpAdd); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(home, ".zev", "aliases")) //nolint:gosec // Isolated test store under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if err := run("1\n\n\n\n", managedKindAlias, managedOpEdit); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(home, ".zev", "aliases")) //nolint:gosec // Isolated test store under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("Enter changed metadata")
	}
	if err := run("1\n", managedKindAlias, "remove", "--yes"); !errors.Is(err, io.EOF) {
		t.Fatalf("unnamed --yes bypassed confirmation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zev", "aliases")); err != nil {
		t.Fatal(err)
	}
	if err := run("1\nyes\n", managedKindAlias, "remove"); err != nil {
		t.Fatal(err)
	}
}
