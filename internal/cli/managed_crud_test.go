package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func managedCommand(t *testing.T, argv ...string) (string, error) {
	t.Helper()
	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(argv)
	err := root.ExecuteContext(t.Context())
	return out.String(), err
}

func TestManagedAliasCRUD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	out, err := managedCommand(t, managedKindAlias, managedOpAdd, "inspect", managedTemplateFixture, "desc", `[{"name":"target","required":false}]`, historyJSONFlag)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if string(result["schemaVersion"]) != "1" || string(result["type"]) != `"alias"` {
		t.Fatalf("schema %s", out)
	}
	out, err = managedCommand(t, managedKindAlias, managedOpEdit, "inspect", "--description", "new", historyJSONFlag)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Item struct {
			Template, Description string
			Arguments             []struct {
				Name     string
				Required bool
			}
		}
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	if body.Item.Template != managedTemplateFixture || body.Item.Description != "new" || len(body.Item.Arguments) != 1 || body.Item.Arguments[0].Required {
		t.Fatal(out)
	}
	out, err = managedCommand(t, managedKindAlias, managedOpList, historyJSONFlag)
	if err != nil || !strings.Contains(out, `"items":[`) {
		t.Fatalf("%s %v", out, err)
	}
	out, err = managedCommand(t, managedKindAlias, "rm", "inspect", "--yes", historyJSONFlag)
	if err != nil || !strings.Contains(out, `"name":"inspect"`) {
		t.Fatalf("%s %v", out, err)
	}
}

func TestManagedScriptCRUD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	file := filepath.Join(home, "body")
	if err := os.WriteFile(file, []byte("#!/bin/bash\nprintf '%s' \"$@\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := managedCommand(t, managedKindScript, managedOpCreate, "deploy", "--body-file", file, "--argument", "target=Environment", "--description", "Deploy app", historyJSONFlag)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"required":true`) {
		t.Fatal(out)
	}
	out, err = managedCommand(t, managedKindScript, managedOpEdit, "deploy", "--clear-description", historyJSONFlag)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"description":""`) {
		t.Fatal(out)
	}
	out, err = managedCommand(t, managedKindScript, "show", "deploy")
	if err != nil || out != "#!/bin/bash\nprintf '%s' \"$@\"\n" {
		t.Fatalf("%s %v", out, err)
	}
	out, err = managedCommand(t, managedKindScript, "rm", "deploy", historyJSONFlag)
	if err != nil || !strings.Contains(out, `"name":"deploy"`) {
		t.Fatalf("%s %v", out, err)
	}
}

func TestManagedConflictsAndNonInteractive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, argv := range [][]string{
		{managedKindAlias, managedOpAdd, "--non-interactive"},
		{managedKindAlias, managedOpAdd, historyBadValue, managedEchoFixture, "positional", "--description", "", historyJSONFlag},
		{managedKindAlias, managedOpAdd, historyBadValue, "echo", "--no-metadata", "--clear-arguments", historyJSONFlag},
		{managedKindAlias, managedOpEdit, historyBadValue, "--no-metadata"},
		{managedKindScript, managedOpCreate, historyBadValue, historyJSONFlag},
		{managedKindScript, managedOpEdit, historyBadValue, "--edit-body", managedBodyStdinFlag},
		{managedKindAlias, managedOpAdd, historyBadValue, managedTemplateFixture, managedArgumentsJSONFlag, `[{"name":"x","required":null}]`, historyJSONFlag},
	} {
		out, err := managedCommand(t, argv...)
		if cliExitCode(err) != 2 || out != "" {
			t.Errorf("%v: %q %v", argv, out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".zev", "scripts")); !os.IsNotExist(err) {
		t.Fatalf("partial write %v", err)
	}
}
