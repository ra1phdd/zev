package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	testName = "deploy"
	testKind = "alias"
	testEcho = "echo"
)

func TestValidateInputsUsesOnlyBoundedEntityData(t *testing.T) {
	t.Parallel()
	validAlias := DescriptionInput{
		EntityName: testName,
		EntityKind: testKind,
		Template:   "kubectl get {{.arg1}}",
		Arguments:  []domain.Argument{{Name: "resource", Required: true}},
	}
	if err := ValidateDescriptionInput(validAlias); err != nil {
		t.Fatalf("valid description input rejected: %v", err)
	}

	validScript := ArgumentsInput{
		EntityName:  testScriptName,
		EntityKind:  entityKindScript,
		Body:        "#!/usr/bin/env bash\necho backup",
		Description: "Creates a backup",
	}
	if err := ValidateArgumentsInput(validScript); err != nil {
		t.Fatalf("valid arguments input rejected: %v", err)
	}

	cases := []struct {
		name  string
		check func() error
	}{
		{"unknown entity kind", func() error { input := validAlias; input.EntityKind = "query"; return ValidateDescriptionInput(input) }},
		{"wrong alias source", func() error { input := validAlias; input.Body = "body"; return ValidateDescriptionInput(input) }},
		{"wrong script source", func() error { input := validScript; input.Template = testEcho; return ValidateArgumentsInput(input) }},
		{"invalid name", func() error {
			input := validAlias
			input.EntityName = "../secret"
			return ValidateDescriptionInput(input)
		}},
		{"oversized body", func() error {
			input := validScript
			input.Body = strings.Repeat("x", MaxGenerationBytes+1)
			return ValidateArgumentsInput(input)
		}},
		{"invalid supplied arguments", func() error {
			input := validAlias
			input.Arguments = []domain.Argument{{Name: "x", Required: true}, {Name: "x", Required: true}}
			return ValidateDescriptionInput(input)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.check(); err == nil {
				t.Fatal("input was accepted")
			}
		})
	}
}

func TestDecodeDescriptionResponseIsStrict(t *testing.T) {
	t.Parallel()
	got, err := DecodeDescriptionResponse([]byte(`{"description":"lists resources"}`))
	if err != nil || got != "lists resources" {
		t.Fatalf("decode = %q, %v", got, err)
	}
	for _, body := range []string{
		`{"description":"one","description":"two"}`,
		`{"description":"ok","extra":1}`,
		`{"description":null}`,
		`{"description":1}`,
		`{"description":"\u001b[31mred"}`,
		`{"description":"ok"} {"description":"trailing"}`,
		`{"description":"` + string([]byte{0xff}) + `"}`,
	} {
		if _, err := DecodeDescriptionResponse([]byte(body)); err == nil {
			t.Errorf("accepted invalid description response %q", body)
		}
	}
}

func TestDecodeArgumentsResponseRejectsMalformedMetadata(t *testing.T) {
	t.Parallel()
	valid := `{"arguments":[{"name":"service","description":"service id","required":true},{"name":"file","description":"output","required":false}]}`
	got, err := DecodeArgumentsResponse([]byte(valid))
	want := []domain.Argument{{Name: "service", Description: "service id", Required: true}, {Name: "file", Description: "output", Required: false}}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("decode = %#v, %v", got, err)
	}
	for _, body := range []string{
		`{"arguments":[{"name":"x","required":true,"unknown":false}]}`,
		`{"arguments":[{"name":"x","description":null}]}`,
		`{"arguments":[{"name":"x","required":"true"}]}`,
		`{"arguments":[{"name":"x"},{"name":"x"}]}`,
		`{"arguments":null}`,
		`{"arguments":{}}`,
		`{"arguments":[null]}`,
		`{"arguments":[{"name":"x","required":true,"required":false}]}`,
	} {
		if _, err := DecodeArgumentsResponse([]byte(body)); err == nil {
			t.Errorf("accepted invalid arguments response %q", body)
		}
	}
}

func TestDecodeDescriptionResponseByteBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		size    int
		wantErr error
	}{
		{size: 4096},
		{size: 4097, wantErr: ErrInvalidResponse},
	} {
		t.Run(fmt.Sprintf("%d bytes", tc.size), func(t *testing.T) {
			t.Parallel()
			description := strings.Repeat("é", 2048) + strings.Repeat("x", tc.size-4096)
			got, err := DecodeDescriptionResponse([]byte(`{"description":"` + description + `"}`))
			want := description
			if tc.wantErr != nil {
				want = ""
			}
			if !errors.Is(err, tc.wantErr) || got != want {
				t.Fatalf("decode: length=%d, error=%v; want length=%d, error=%v", len(got), err, len(want), tc.wantErr)
			}
		})
	}
}

func TestDecodeArgumentsResponseBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		count   int
		wantErr error
	}{
		{count: 32},
		{count: 33, wantErr: ErrInvalidResponse},
	} {
		t.Run(fmt.Sprintf("%d arguments", tc.count), func(t *testing.T) {
			t.Parallel()
			want := make([]domain.Argument, tc.count)
			items := make([]string, tc.count)
			for i := range want {
				want[i] = domain.Argument{Name: fmt.Sprintf("arg%d", i+1), Description: fmt.Sprintf("value %d", i+1), Required: i < 16}
				items[i] = fmt.Sprintf(`{"name":%q,"description":%q,"required":%t}`, want[i].Name, want[i].Description, want[i].Required)
			}
			got, err := DecodeArgumentsResponse([]byte(`{"arguments":[` + strings.Join(items, ",") + `]}`))
			if tc.wantErr != nil {
				want = nil
			}
			if !errors.Is(err, tc.wantErr) || !slices.Equal(got, want) {
				t.Fatalf("decode = %#v, %v; want %#v, %v", got, err, want, tc.wantErr)
			}
		})
	}
}

func TestDecodeErrorsAreSecretFree(t *testing.T) {
	t.Parallel()
	secret := "provider-secret-payload"
	_, err := DecodeDescriptionResponse([]byte(`{"description":null,"secret":"` + secret + `"}`))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked response data: %v", err)
	}
	if errors.Is(err, ErrInvalidResponse) == false {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
}
