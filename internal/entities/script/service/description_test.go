package service

import (
	"bytes"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestDescriptionRemainsMetadataWithoutChangingBody(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	description := "first line; printf should-not-run; echo also-data"
	body := []byte("#!/usr/bin/env bash\nexit 0\n")
	if err := svc.Create(t.Context(), domain.Script{Name: safeScriptName, ScriptFile: safeScriptFile, Description: description}, body); err != nil {
		t.Fatal(err)
	}
	script, err := svc.Find(t.Context(), safeScriptName)
	if err != nil {
		t.Fatal(err)
	}
	if script.Description != description {
		t.Fatalf("description = %q, want %q", script.Description, description)
	}
	var out bytes.Buffer
	if err := svc.Show(t.Context(), safeScriptName, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), body) {
		t.Fatalf("metadata changed script body: %q, want %q", out.Bytes(), body)
	}
}
