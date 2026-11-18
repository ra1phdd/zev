package metadata

import (
	"bytes"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	testAliasName  = "dps"
	testCommand    = "echo"
	testScriptName = "deploy"
)

func TestEncodeDecodeAliasesUsesVersionTwoAndExplicitRequired(t *testing.T) {
	t.Parallel()
	input := map[string]domain.Alias{
		testAliasName: {
			Name:        testAliasName,
			Template:    "docker service ps {{.arg1}} {{.arg2}}",
			Description: "show tasks",
			Arguments: []domain.Argument{
				{Name: "service", Description: "service name", Required: true},
				{Name: "format", Required: false},
			},
		},
	}

	data, err := EncodeAliases(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 2") || !strings.Contains(string(data), "required: false") {
		t.Fatalf("canonical aliases = %q", data)
	}

	got, legacy, err := DecodeAliases(data)
	if err != nil {
		t.Fatal(err)
	}
	if legacy {
		t.Fatal("canonical v2 data reported as legacy")
	}
	if got[testAliasName].Description != input[testAliasName].Description || got[testAliasName].Arguments[1].Required {
		t.Fatalf("decoded aliases = %#v", got)
	}
}

func TestDecodeAliasesDefaultsOmittedRequiredToTrue(t *testing.T) {
	t.Parallel()
	got, legacy, err := DecodeAliases([]byte("version: 2\naliases:\n  dps:\n    template: docker ps {{.arg1}}\n    arguments:\n      - name: service\n"))
	if err != nil {
		t.Fatal(err)
	}
	if legacy || !got[testAliasName].Arguments[0].Required {
		t.Fatalf("legacy=%v aliases=%#v", legacy, got)
	}
}

func TestDecodeRejectsStrictYAMLErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "duplicate key", data: []byte("version: 2\naliases:\n  dps:\n    template: echo one\n    template: echo two\n")},
		{name: "unknown field", data: []byte("version: 2\naliases:\n  dps:\n    template: echo\n    extra: value\n")},
		{name: "multiple documents", data: []byte("version: 2\naliases: {}\n---\nversion: 2\naliases: {}\n")},
		{name: "anchor", data: []byte("version: 2\naliases: &items {}\n")},
		{name: "merge key", data: []byte("version: 2\naliases:\n  dps:\n    <<: {template: echo}\n    template: echo\n")},
		{name: "null root", data: []byte("null\n")},
		{name: "null field", data: []byte("version: 2\naliases:\n  dps:\n    template: null\n")},
		{name: "custom tag", data: []byte("version: 2\naliases: !custom {}\n")},
		{name: "control", data: []byte("version: 2\naliases:\n  dps:\n    template: \"echo \\u0000\"\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := DecodeAliases(tc.data); err == nil {
				t.Fatal("invalid YAML accepted")
			}
		})
	}
}

func TestDecodeAliasesClassifiesLegacyVersionAsReservedName(t *testing.T) {
	t.Parallel()
	_, _, err := DecodeAliases([]byte("version: echo {{.arg1}}\n"))
	if err == nil || !strings.Contains(err.Error(), `name "version" is reserved`) {
		t.Fatalf("expected reserved legacy name error, got %v", err)
	}
}

func TestDecodeScriptsLegacyDoesNotGenerateRecordID(t *testing.T) {
	t.Parallel()
	got, legacy, err := DecodeScripts([]byte("deploy:\n  script: deploy.sh\n  description: exact\\ntext\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !legacy || got[testScriptName].RecordID != "" {
		t.Fatalf("legacy=%v scripts=%#v", legacy, got)
	}
	if err := ValidateLegacyScript(got[testScriptName]); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeScriptsClassifiesLegacyVersionAsReservedName(t *testing.T) {
	t.Parallel()
	_, _, err := DecodeScripts([]byte("version:\n  script: deploy.sh\n"))
	if err == nil || !strings.Contains(err.Error(), `name "version" is reserved`) {
		t.Fatalf("expected reserved legacy name error, got %v", err)
	}
}

func TestTemplateGrammarRejectsShellExpansionAndRestrictedWords(t *testing.T) {
	t.Parallel()
	for _, template := range []string{
		"echo *.txt",
		"echo ~user",
		"FOO=bar echo",
		"if echo",
		"echo '{{.arg1}}'",
		"echo prefix{{.arg1}}",
		"echo {{.arg2}}",
		"echo {{.arg01}}",
	} {
		t.Run(template, func(t *testing.T) {
			if err := ValidateAliasTemplate(template, nil); err == nil {
				t.Fatal("unsupported template accepted")
			}
		})
	}
}

func TestEffectiveArgumentsDefaultMissingMetadataToRequired(t *testing.T) {
	t.Parallel()
	alias := domain.Alias{Name: testScriptName, Template: "echo {{.arg1}} {{.arg2}}", Arguments: []domain.Argument{{Name: "environment", Required: false}}}
	if _, err := EffectiveArguments(alias); err == nil {
		t.Fatal("required position after optional metadata accepted")
	}

	alias.Arguments = nil
	arguments, err := EffectiveArguments(alias)
	if err != nil || len(arguments) != 2 || !arguments[0].Required || !arguments[1].Required || arguments[1].Name != "arg2" {
		t.Fatalf("arguments=%#v err=%v", arguments, err)
	}
}

func TestRenderAliasPreservesArgvBoundariesAndValidatesContract(t *testing.T) {
	t.Parallel()
	alias := domain.Alias{
		Name:      "show",
		Template:  "printf %s {{.arg1}} {{.args}}",
		Arguments: []domain.Argument{{Name: "value", Required: true}},
	}
	got, err := RenderAlias(alias, []string{"a b", "", "x;y", "$(touch nope)", "тест\n"})
	if err != nil {
		t.Fatal(err)
	}
	want := "printf %s 'a b' 'a b' '' 'x;y' '$(touch nope)' 'тест\n'"
	if got != want {
		t.Fatalf("rendered=%q want=%q", got, want)
	}

	_, err = RenderAlias(domain.Alias{Name: "show", Template: "echo {{.arg1}}"}, nil)
	if err == nil || !strings.Contains(err.Error(), "requires argument 1") {
		t.Fatalf("missing required argument error = %v", err)
	}
}

func TestDecodeReaderAndEncodedLimit(t *testing.T) {
	t.Parallel()
	if _, _, err := DecodeAliases([]byte("version: 2\naliases: {}\n")); err != nil {
		t.Fatal(err)
	}
	tooLarge := bytes.Repeat([]byte("x"), maxMetadataBytes+1)
	if _, _, err := DecodeAliases(tooLarge); err == nil {
		t.Fatal("oversized metadata accepted")
	}
	valid := []byte("version: 2\naliases: {}\n")
	padded := valid
	padded = append(padded, bytes.Repeat([]byte("\n"), maxMetadataBytes-len(valid)+1)...)
	if _, _, err := DecodeAliases(padded); err == nil {
		t.Fatal("oversized reader accepted valid metadata padded beyond the limit")
	}
	scriptValid := []byte("version: 2\nscripts: {}\n")
	scriptPadded := scriptValid
	scriptPadded = append(scriptPadded, bytes.Repeat([]byte("\n"), maxMetadataBytes-len(scriptValid)+1)...)
	if _, _, err := DecodeScriptsReader(bytes.NewReader(scriptPadded)); err == nil {
		t.Fatal("oversized scripts reader accepted valid metadata padded beyond the limit")
	}
	aliases := map[string]domain.Alias{testCommand: {Name: testCommand, Template: testCommand}}
	if data, err := EncodeAliases(aliases); err != nil || len(data) > maxMetadataBytes {
		t.Fatalf("encoded aliases len=%d err=%v", len(data), err)
	}
}

func TestCodecRejectsC0ControlsInMetadataStrings(t *testing.T) {
	t.Parallel()
	data := []byte("version: 2\naliases:\n  dps:\n    template: echo\n    description: \"line\\ntext\"\n")
	if _, _, err := DecodeAliases(data); err == nil {
		t.Fatal("C0 control accepted in metadata scalar")
	}
	if _, err := EncodeAliases(map[string]domain.Alias{"dps": {Name: "dps", Template: "echo", Description: "line\ntext"}}); err == nil {
		t.Fatal("C0 control encoded in metadata scalar")
	}
}

func TestValidateScriptRejectsMissingOrInvalidRecordID(t *testing.T) {
	t.Parallel()
	base := domain.Script{Name: testScriptName, RecordID: "", ScriptFile: "deploy.sh"}
	if err := ValidateScript(base); err == nil {
		t.Fatal("missing record ID accepted")
	}
	base.RecordID = "not-a-uuid"
	if err := ValidateScript(base); err == nil {
		t.Fatal("invalid record ID accepted")
	}
}

func TestEncodeDecodeScriptsPreservesCanonicalFields(t *testing.T) {
	t.Parallel()
	input := map[string]domain.Script{testScriptName: {
		Name: testScriptName, RecordID: "550e8400-e29b-41d4-a716-446655440000", ScriptFile: "deploy.sh",
		Description: "deploy application", Arguments: []domain.Argument{{Name: "environment", Required: false}},
	}}
	data, err := EncodeScripts(input)
	if err != nil {
		t.Fatal(err)
	}
	got, legacy, err := DecodeScripts(data)
	if err != nil || legacy || !reflect.DeepEqual(got[testScriptName], input[testScriptName]) {
		t.Fatalf("scripts=%#v legacy=%v err=%v", got, legacy, err)
	}
}

func TestDecodeRejectsNullArgumentsAndWrongTypes(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		"version: 2\naliases:\n  x:\n    template: echo\n    arguments: null\n",
		"version: 2\naliases:\n  x:\n    template: echo\n    arguments: string\n",
		"version: 2\naliases:\n  x:\n    template: echo {{.arg1}}\n    arguments:\n      - name: value\n        required: null\n",
	} {
		if _, _, err := DecodeAliases([]byte(data)); err == nil {
			t.Fatalf("wrong type accepted: %q", data)
		}
	}
}

func TestDecodeEnforcesYAMLNodeAndDepthBounds(t *testing.T) {
	t.Parallel()
	var nodes strings.Builder
	nodes.WriteString("version: 2\naliases:\n")
	for i := range maxYAMLNodes {
		nodes.WriteString("  x" + strconv.Itoa(i) + ":\n")
		nodes.WriteString("    template: echo\n")
	}
	if _, _, err := DecodeAliases([]byte(nodes.String())); err == nil || !strings.Contains(err.Error(), "YAML nodes") {
		t.Fatalf("node limit error = %v", err)
	}

	var depth strings.Builder
	depth.WriteString("version: 2\naliases:\n")
	depth.WriteString("  x:\n")
	for i := range maxYAMLDepth + 2 {
		depth.WriteString(strings.Repeat("  ", i+2) + "x:\n")
	}
	depth.WriteString(strings.Repeat("  ", maxYAMLDepth+4) + "template: echo\n")
	if _, _, err := DecodeAliases([]byte(depth.String())); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("depth limit error = %v", err)
	}
}

func TestValidateNamesAndScriptPathsRejectUnsafeValues(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "version", strings.Repeat("a", 65), "bad name", "é"} {
		if err := ValidateName(name); err == nil {
			t.Fatalf("name accepted: %q", name)
		}
	}
	for _, path := range []string{"", "/tmp/script", "../script", "C:/script", `dir\\script`, "script\x00"} {
		if err := ValidateScriptPath(path); err == nil {
			t.Fatalf("path accepted: %q", path)
		}
	}
}
