package service

import (
	"bytes"
	"testing"
)

func TestMultilineDescriptionRemainsCommentData(t *testing.T) {
	t.Setenv("SKIP_EDITOR", "1")
	svc, _ := newTestService(t)
	description := "first line\r\nprintf should-not-run\necho also-data"
	if err := svc.CreateContext(t.Context(), safeScriptName, description); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := svc.Show(t.Context(), safeScriptName, &out); err != nil {
		t.Fatal(err)
	}
	want := "#!/usr/bin/env bash\n\n# first line\n# printf should-not-run\n# echo also-data\n\n"
	if out.String() != want {
		t.Fatalf("description became script code: %q", out.String())
	}
}
