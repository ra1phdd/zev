package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const testScriptName = "backup"

func TestArgumentsPromptExactPayload(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		input   ArgumentsInput
		payload string
	}{
		{name: "alias", input: ArgumentsInput{EntityName: testName, EntityKind: testKind, Template: "echo {{.arg1}}", Description: `Prints "value"`}, payload: `{"entity_name":"deploy","entity_kind":"alias","template":"echo {{.arg1}}","description":"Prints \"value\""}`},
		{name: "script", input: ArgumentsInput{EntityName: testScriptName, EntityKind: entityKindScript, Body: "#!/bin/bash\necho \"$1\"", Description: "Backs up данные"}, payload: `{"entity_name":"backup","entity_kind":"script","body":"#!/bin/bash\necho \"$1\"","description":"Backs up данные"}`},
		{name: "empty description retained", input: ArgumentsInput{EntityName: testName, EntityKind: testKind, Template: testEcho}, payload: `{"entity_name":"deploy","entity_kind":"alias","template":"echo","description":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := "Return exactly one JSON object with no unknown fields, nulls, prose, or trailing data. The object must be {\"arguments\": array} with no more than 32 items; each item has name string, optional description string, and optional required boolean.\nfield=arguments\ninput=" + tc.payload
			got, err := ArgumentsPrompt(tc.input)
			if err != nil || got != want {
				t.Fatalf("ArgumentsPrompt = %q, %v; want %q", got, err, want)
			}
		})
	}
}

func TestValidateResponseDocumentDepthBoundary(t *testing.T) {
	t.Parallel()
	for _, shape := range []struct {
		name  string
		open  string
		close string
	}{
		{name: "arrays", open: "[", close: "]"},
		{name: "objects", open: `{"nested":`, close: "}"},
	} {
		for _, depth := range []int{31, 32, 33} {
			t.Run(fmt.Sprintf("%s/depth_%d", shape.name, depth), func(t *testing.T) {
				t.Parallel()
				body := strings.Repeat(shape.open, depth) + "0" + strings.Repeat(shape.close, depth)
				err := ValidateResponseDocument([]byte(body))
				if depth <= 32 {
					if err != nil {
						t.Fatalf("valid document at depth %d rejected: %v", depth, err)
					}
				} else if !errors.Is(err, ErrInvalidResponse) {
					t.Fatalf("depth %d: error = %v, want ErrInvalidResponse", depth, err)
				}
			})
		}
	}
}

func TestValidateResponseDocumentBoundsAndSyntax(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		body    string
		wantErr error
	}{
		{name: "extension fields allowed", body: `{"provider_extension":{"text":"ok"}}`},
		{name: "exact byte boundary", body: `{}` + strings.Repeat(" ", MaxGenerationBytes-2)},
		{name: "byte boundary plus one", body: `{}` + strings.Repeat(" ", MaxGenerationBytes-1), wantErr: ErrInvalidResponse},
		{name: "empty", wantErr: ErrInvalidResponse},
		{name: "whitespace only", body: " \n\t", wantErr: ErrInvalidResponse},
		{name: "invalid UTF8", body: `{"text":"` + string([]byte{0xff}) + `"}`, wantErr: ErrInvalidResponse},
		{name: "unclosed object", body: `{"text":"ok"`, wantErr: ErrInvalidResponse},
		{name: "trailing document", body: `{} []`, wantErr: ErrInvalidResponse},
		{name: "duplicate nested field", body: `{"extension":{"name":1,"name":2}}`, wantErr: ErrInvalidResponse},
		{name: "escaped duplicate field", body: `{"name":1,"\u006eame":2}`, wantErr: ErrInvalidResponse},
		{name: "lone surrogate", body: `{"text":"\ud800"}`, wantErr: ErrInvalidResponse},
		{name: "valid surrogate pair", body: `{"text":"\ud83d\ude00"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateResponseDocument([]byte(tc.body)); !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateResponseDocument = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestArgumentsInputUTF8AndByteBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		body        string
		description string
		wantErr     error
	}{
		{name: "body exact boundary", body: strings.Repeat("é", MaxGenerationBytes/2)},
		{name: "body boundary plus one", body: strings.Repeat("é", MaxGenerationBytes/2) + "x", wantErr: ErrInvalidInput},
		{name: "body invalid UTF8", body: string([]byte{0xff}), wantErr: ErrInvalidInput},
		{name: "description exact boundary", body: testEcho, description: strings.Repeat("é", 2048)},
		{name: "description boundary plus one", body: testEcho, description: strings.Repeat("é", 2048) + "x", wantErr: ErrInvalidInput},
		{name: "description invalid UTF8", body: testEcho, description: string([]byte{0xff}), wantErr: ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := ArgumentsInput{EntityName: testScriptName, EntityKind: entityKindScript, Body: tc.body, Description: tc.description}
			if err := ValidateArgumentsInput(input); !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateArgumentsInput = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestArgumentsPromptEncodedPayloadBounds(t *testing.T) {
	t.Parallel()
	const payloadOverhead = len(`{"entity_name":"backup","entity_kind":"script","body":"","description":""}`)
	for _, tc := range []struct {
		name    string
		body    string
		wantErr error
	}{
		{name: "exact payload boundary", body: strings.Repeat("x", MaxGenerationBytes-payloadOverhead)},
		{name: "payload boundary plus one", body: strings.Repeat("x", MaxGenerationBytes-payloadOverhead+1), wantErr: ErrInvalidInput},
		{name: "escaping expands payload", body: strings.Repeat("<", MaxGenerationBytes/2), wantErr: ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ArgumentsPrompt(ArgumentsInput{EntityName: testScriptName, EntityKind: entityKindScript, Body: tc.body})
			if !errors.Is(err, tc.wantErr) || (err != nil && got != "") {
				t.Fatalf("ArgumentsPrompt length=%d, error=%v; want %v", len(got), err, tc.wantErr)
			}
			if tc.wantErr == nil {
				_, payload, ok := strings.Cut(got, "\ninput=")
				if !ok || len(payload) != MaxGenerationBytes {
					t.Fatalf("payload length = %d, want %d", len(payload), MaxGenerationBytes)
				}
			}
		})
	}
}
