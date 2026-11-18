package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
	"github.com/ra1phdd/zev/internal/entities/script/service"
)

func TestManagedHelpWithoutBody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeCLIConfig(t, home, invalidProviderFixture())
	svc := service.New(repository.New(filepath.Join(home, ".zev")))
	if err := svc.Create(t.Context(), domain.Script{Name: "detached", ScriptFile: "detached.sh", Description: "Metadata only"}, []byte("exit 0\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, ".zev", "script_files", "detached.sh")); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"detached", localizationHelpFlag})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Metadata only")) {
		t.Fatal(out.String())
	}
}

func TestManagedUnknownEntityHelpIsUsageError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"does-not-exist", localizationHelpFlag})
	err := root.ExecuteContext(t.Context())
	if e, ok := errors.AsType[*machineExitError](err); !ok || e.code != exitArguments || out.Len() != 0 {
		t.Fatalf("missing help: stdout=%q error=%v", out.String(), err)
	}
}
