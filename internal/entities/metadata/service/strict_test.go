package service

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestJSONResponseSafety(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"description":"\ud800"}`,
		`{"description":"\udc00"}`,
		`{"description":"\ud800\u0041"}`,
		`{"description":"\u009b31m"}`,
		`{"description":"line\nfeed"}`,
		`{"description":"tab\tvalue"}`,
		`{"description":"` + strings.Repeat("é", 2049) + `"}`,
	} {
		if _, err := DecodeDescriptionResponse([]byte(body)); err == nil {
			t.Errorf("accepted invalid response: %.80s", body)
		}
	}
	for _, text := range []string{"emoji 😀", "replacement �", "literal \\ud800"} {
		body := fmt.Appendf(nil, `{"description":%q}`, text)
		if got, err := DecodeDescriptionResponse(body); err != nil || got != text {
			t.Errorf("valid description rejected: %q %v", got, err)
		}
	}
}

func TestResponseByteBoundary(t *testing.T) {
	t.Parallel()
	for _, size := range []int{MaxGenerationBytes, MaxGenerationBytes + 1} {
		body := []byte(`{"description":"ok"}`)
		body = append(body, bytes.Repeat([]byte(" "), size-len(body))...)
		got, err := DecodeDescriptionResponse(body)
		if size == MaxGenerationBytes && (err != nil || got != "ok") {
			t.Fatalf("response at boundary: %q %v", got, err)
		}
		if size > MaxGenerationBytes && err == nil {
			t.Fatal("response beyond byte boundary accepted")
		}
	}
}

func TestSuppliedArgumentsMatchAliasTemplate(t *testing.T) {
	t.Parallel()
	input := DescriptionInput{EntityName: testName, EntityKind: testKind, Template: testEcho, Arguments: []domain.Argument{{Name: "value", Required: true}}}
	if err := ValidateDescriptionInput(input); err == nil {
		t.Fatal("arguments outside template positions accepted")
	}
}

func TestMetadataPromptsSpecifySeparateSchemas(t *testing.T) {
	t.Parallel()
	description, err := DescriptionPrompt(DescriptionInput{EntityName: testName, EntityKind: testKind, Template: testEcho})
	if err != nil || !strings.Contains(description, `field=description`) || !strings.Contains(description, `description`) {
		t.Fatalf("description schema missing: %q %v", description, err)
	}
	arguments, err := ArgumentsPrompt(ArgumentsInput{EntityName: testName, EntityKind: testKind, Template: "echo {{.arg1}}"})
	if err != nil || !strings.Contains(arguments, `required boolean`) || !strings.Contains(arguments, "32") {
		t.Fatalf("argument schema/limits missing: %q %v", arguments, err)
	}
}

func TestArgumentsDefaultsAndMetadataLimits(t *testing.T) {
	t.Parallel()
	args, err := DecodeArgumentsResponse([]byte(`{"arguments":[{"name":"x"},{"name":"y","required":false}]}`))
	if err != nil || len(args) != 2 || !args[0].Required || args[1].Required {
		t.Fatalf("defaults = %#v %v", args, err)
	}
	for _, content := range []string{
		`{"arguments":[{"name":"x","required":null}]}`,
		`{"arguments":[{"name":"x","required":false},{"name":"y","required":true}]}`,
		`{"arguments":[{"name":"` + strings.Repeat("x", 65) + `"}]}`,
		`{"arguments":[{"name":"x","description":"` + strings.Repeat("x", 1025) + `"}]}`,
		`{"arguments":[{"name":"x","description":"\u001b[2J"}]}`,
	} {
		if _, err := DecodeArgumentsResponse([]byte(content)); err == nil {
			t.Errorf("invalid argument metadata accepted: %.100s", content)
		}
	}
}
