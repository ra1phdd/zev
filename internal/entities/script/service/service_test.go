package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
)

const (
	safeScriptName        = "safe"
	testScriptName        = "test"
	testScriptDescription = "Test script"
)

// newTestService creates an isolated script service and storage directory.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return New(repository.New(tmpDir)), tmpDir
}

func TestServiceCreate(t *testing.T) {
	t.Parallel()
	svc, tmpDir := newTestService(t)

	err := svc.Create(t.Context(), domain.Script{Name: testScriptName, ScriptFile: "test.sh", Description: testScriptDescription}, []byte("#!/bin/bash\nexit 0\n"))
	if err != nil {
		t.Fatalf("Failed to create script: %v", err)
	}

	scriptPath := filepath.Join(tmpDir, "script_files", "test.sh")
	body, err := os.ReadFile(filepath.Clean(scriptPath))
	if err != nil || string(body) != "#!/bin/bash\nexit 0\n" {
		t.Fatalf("created body = %q, error = %v", body, err)
	}

	scripts, err := svc.List()
	if err != nil {
		t.Fatalf("Failed to list scripts: %v", err)
	}

	if len(scripts) != 1 {
		t.Fatalf("Expected 1 script, got %d", len(scripts))
	}

	if scripts[0].Name != testScriptName || scripts[0].Description != testScriptDescription {
		t.Errorf("Unexpected script metadata: %+v", scripts[0])
	}
}

func TestServiceRemove(t *testing.T) {
	t.Parallel()
	svc, tmpDir := newTestService(t)

	if err := svc.Create(t.Context(), domain.Script{Name: testScriptName, ScriptFile: "test.sh", Description: testScriptDescription}, []byte("#!/bin/bash\nexit 0\n")); err != nil {
		t.Fatalf("Failed to create script: %v", err)
	}

	if err := svc.Remove(testScriptName); err != nil {
		t.Fatalf("Failed to remove script: %v", err)
	}

	scriptPath := filepath.Join(tmpDir, "script_files", "test.sh")
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("Script file was not removed")
	}

	scripts, err := svc.List()
	if err != nil {
		t.Fatalf("Failed to list scripts: %v", err)
	}

	if len(scripts) != 0 {
		t.Errorf("Expected 0 scripts, got %d", len(scripts))
	}
}

func TestServiceList(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)

	if err := svc.Create(t.Context(), domain.Script{Name: "zeta", ScriptFile: "zeta.sh", Description: "Last script"}, []byte("exit 0\n")); err != nil {
		t.Fatalf("Failed to create zeta: %v", err)
	}
	if err := svc.Create(t.Context(), domain.Script{Name: "alpha", ScriptFile: "alpha.sh", Description: "First script"}, []byte("exit 0\n")); err != nil {
		t.Fatalf("Failed to create alpha: %v", err)
	}

	scripts, err := svc.List()
	if err != nil {
		t.Fatalf("Failed to list scripts: %v", err)
	}

	if len(scripts) != 2 {
		t.Errorf("Expected 2 scripts, got %d", len(scripts))
	}
	if len(scripts) == 2 && (scripts[0].Name != "alpha" || scripts[1].Name != "zeta") {
		t.Fatalf("scripts are not sorted by name: %#v", scripts)
	}
}

func TestExecuteValidatesRequiredArgumentsBeforeLaunch(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	script := domain.Script{
		Name:       safeScriptName,
		ScriptFile: safeScriptFile,
		Arguments:  []domain.Argument{{Name: "environment", Description: "target environment", Required: true}},
	}
	marker := filepath.Join(t.TempDir(), "launched")
	if err := svc.repo.Create(t.Context(), script, []byte("#!/bin/bash\ntouch "+marker+"\nexit 0\n")); err != nil {
		t.Fatal(err)
	}

	if err := svc.Execute(t.Context(), safeScriptName, nil); err == nil || !strings.Contains(err.Error(), `script "safe" requires argument 1: environment — target environment`) {
		t.Fatalf("Execute() error = %v, want required argument diagnostic", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("script launched before required-argument validation: %v", statErr)
	}
}
