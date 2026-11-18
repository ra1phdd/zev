package service

import (
	"errors"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
)

func TestMetadataPatchRejectsMismatchedBodyPrerequisite(t *testing.T) {
	t.Parallel()
	svc := New(repository.New(t.TempDir()))
	for _, name := range []string{"first", "second"} {
		if err := svc.Create(t.Context(), domain.Script{Name: name, ScriptFile: name + ".sh"}, []byte("exit 0\n")); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.Snapshot(t.Context(), "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Snapshot(t.Context(), "second")
	if err != nil {
		t.Fatal(err)
	}
	first.Body = second.Body
	description := "unbound metadata"
	if _, err := svc.Patch(t.Context(), first, Patch{Description: &description}); !errors.Is(err, ErrScriptChanged) {
		t.Fatalf("metadata accepted an unrelated body's prerequisite: %v", err)
	}
}

func TestMetadataPatchPreservesOtherFieldsAndRejectsRecreation(t *testing.T) {
	t.Parallel()
	svc := New(repository.New(t.TempDir()))
	script := domain.Script{Name: "patchable", ScriptFile: "patchable.sh", Description: "original description"}
	if err := svc.Create(t.Context(), script, []byte("exit 0\n")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.Snapshot(t.Context(), script.Name)
	if err != nil {
		t.Fatal(err)
	}
	description := "updated description"
	updated, err := svc.Patch(t.Context(), snapshot, Patch{Description: &description})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != description {
		t.Fatal(updated)
	}
	arguments := []domain.Argument{{Name: "target", Required: false}}
	updated, err = svc.Patch(t.Context(), snapshot, Patch{Arguments: &arguments})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != description || len(updated.Arguments) != 1 {
		t.Fatal(updated)
	}
	if _, err := svc.Patch(t.Context(), snapshot, Patch{Description: &description}); !errors.Is(err, ErrScriptChanged) {
		t.Fatalf("stale selected field: %v", err)
	}
	if err := svc.Remove(script.Name); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(t.Context(), script, []byte("exit 0\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Patch(t.Context(), snapshot, Patch{Arguments: &arguments}); !errors.Is(err, ErrScriptChanged) {
		t.Fatalf("recreated record: %v", err)
	}
}
