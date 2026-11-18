package repository

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestUpdateMetadataPreservesIdentityAndBody(t *testing.T) {
	t.Parallel()
	r := New(testDir(t))
	if err := r.Create(t.Context(), domain.Script{Name: safeName, ScriptFile: safeFile}, []byte(scriptBody)); err != nil {
		t.Fatal(err)
	}
	before, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Update(t.Context(), UpdateParams{Change: func(all map[string]domain.Script) error {
		s := all[safeName]
		s.Description = "updated"
		all[safeName] = s
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	after, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if after[safeName].RecordID != before[safeName].RecordID || after[safeName].Description != "updated" {
		t.Fatalf("after: %#v", after)
	}
	body, err := os.ReadFile(r.scriptsDir + "/" + safeFile)
	if err != nil || string(body) != scriptBody {
		t.Fatalf("body: %q, %v", body, err)
	}
	if err := r.Update(t.Context(), UpdateParams{Change: func(all map[string]domain.Script) error {
		s := all[safeName]
		s.RecordID = uuid.NewString()
		all[safeName] = s
		return nil
	}}); err == nil {
		t.Fatal("identity change accepted")
	}
	rejected := errors.New("rejected")
	if err := r.Update(t.Context(), UpdateParams{Change: func(map[string]domain.Script) error { return rejected }}); !errors.Is(err, rejected) {
		t.Fatalf("callback: %v", err)
	}
}
