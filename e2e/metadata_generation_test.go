//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const (
	metadataAliasKind       = "alias"
	metadataScriptKind      = "script"
	metadataContentKey      = "content"
	metadataRoleKey         = "role"
	metadataExistingTarget  = "Existing target"
	metadataPriorDesc       = "Prior description"
	metadataEntityName      = "inspect"
	metadataTargetArgument  = "target"
	metadataDescriptionFlag = "--generate-description"
	metadataArgumentsFlag   = "--generate-arguments"
	metadataJSONFlag        = "--json"
	metadataTemplate        = "printf %s {{.arg1}}"
	metadataBody            = "#!/bin/bash\nprintf '%s' \"$1\"\n"
	metadataOriginalArgs    = `[{"name":"target","description":"Existing target","required":true}]`
	metadataGeneratedArgs   = `[{"name":"destination","description":"Generated target","required":false}]`
	metadataInstruction     = "Return exactly one JSON object with no unknown fields, nulls, prose, or trailing data."
	metadataDescriptionRule = ` The object must be {"description": string} with no more than 4096 UTF-8 bytes.`
	metadataArgumentsRule   = ` The object must be {"arguments": array} with no more than 32 items; each item has name string, optional description string, and optional required boolean.`
)

type metadataReply struct {
	status  int
	content string
}

type metadataRequest struct {
	method, uri, authorization, contentType string
	body                                    []byte
}

type metadataFixture struct {
	home     string
	server   *httptest.Server
	mu       sync.Mutex
	requests []metadataRequest
}

type metadataArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type metadataItem struct {
	Name        string             `json:"name"`
	Template    string             `json:"template"`
	Script      string             `json:"script"`
	RecordID    string             `json:"recordID"` //nolint:tagliatelle // Public management JSON uses recordID.
	Description string             `json:"description"`
	Arguments   []metadataArgument `json:"arguments"`
}

func newMetadataFixture(t *testing.T, replies []metadataReply) *metadataFixture {
	t.Helper()
	f := &metadataFixture{home: t.TempDir()}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			t.Errorf("read metadata fixture request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		index := len(f.requests)
		f.requests = append(f.requests, metadataRequest{r.Method, r.RequestURI, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), body})
		f.mu.Unlock()
		if index >= len(replies) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		reply := replies[index]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{metadataContentKey: reply.content}}}}); err != nil {
			t.Errorf("write metadata fixture response: %v", err)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *metadataFixture) run(t *testing.T, input string, args ...string) (string, string) {
	t.Helper()
	cmd := cliCommand(t, f.home, args...)
	setCLIEnv(cmd, map[string]string{envProvider: providerOpenAI, envBaseURL: f.server.URL + "/v1", envAPIKey: fixtureOnly, envModel: fixtureModel, "ZEV_LANG": "en"})
	cmd.Stdin = strings.NewReader(input)
	var out, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &diagnostic
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %v; stdout=%s stderr=%s", args, err, out.String(), diagnostic.String())
	}
	if strings.Contains(out.String()+diagnostic.String(), fixtureOnly) {
		t.Fatal("metadata command leaked synthetic credential")
	}
	return out.String(), diagnostic.String()
}

func (f *metadataFixture) read(t *testing.T, kind string) metadataItem {
	t.Helper()
	out, diagnostic := f.run(t, "", kind, "list", metadataJSONFlag)
	if diagnostic != "" {
		t.Fatalf("ordinary read emitted diagnostic: %s", diagnostic)
	}
	var envelope struct {
		SchemaVersion int            `json:"schemaVersion"`
		Status        string         `json:"status"`
		Type          string         `json:"type"`
		Items         []metadataItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != 1 || envelope.Status != "ok" || envelope.Type != kind || len(envelope.Items) != 1 {
		t.Fatalf("invalid readback envelope: %s", out)
	}
	return envelope.Items[0]
}

func (f *metadataFixture) assertRequests(t *testing.T, prompts []string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != len(prompts) {
		t.Fatalf("metadata requests=%d want %d", len(f.requests), len(prompts))
	}
	for i, prompt := range prompts {
		r := f.requests[i]
		if r.method != http.MethodPost || r.uri != "/v1/chat/completions" || r.authorization != "Bearer "+fixtureOnly || r.contentType != "application/json" {
			t.Errorf("request %d has unexpected method, URL or headers", i)
		}
		want := map[string]any{
			"model": fixtureModel,
			"messages": []any{
				map[string]any{metadataRoleKey: "system", metadataContentKey: "You generate managed alias and script metadata. Do not use outside context."},
				map[string]any{metadataRoleKey: "user", metadataContentKey: prompt},
			},
			"response_format": map[string]any{"type": "json_object"},
		}
		var got any
		if err := json.Unmarshal(r.body, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("request %d payload=%s; want %#v", i, r.body, want)
		}
	}
}

func metadataPrompts(kind, description, arguments string) (string, string) {
	source := `"template":"printf %s {{.arg1}}"`
	if kind == metadataScriptKind {
		source = `"body":"#!/bin/bash\nprintf '%s' \"$1\"\n"`
	}
	prefix := `{"entity_name":"inspect","entity_kind":"` + kind + `",` + source
	return metadataInstruction + metadataDescriptionRule + "\nfield=description\ninput=" + prefix + `,"arguments":` + arguments + `}`,
		metadataInstruction + metadataArgumentsRule + "\nfield=arguments\ninput=" + prefix + `,"description":"` + description + `"}`
}

func (f *metadataFixture) create(t *testing.T, kind string, flags ...string) {
	t.Helper()
	args := make([]string, 0, 7+len(flags))
	input := ""
	if kind == metadataScriptKind {
		args = append(args, kind, "create", metadataEntityName, "--body-stdin")
		input = metadataBody
	} else {
		args = append(args, metadataAliasKind, "add", metadataEntityName, metadataTemplate)
	}
	args = append(args, "--arguments-json", metadataOriginalArgs, metadataJSONFlag)
	f.run(t, input, append(args, flags...)...)
}

func TestMetadataGenerationPersistsAcrossCLIProcesses(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{metadataAliasKind, metadataScriptKind} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newMetadataFixture(t, []metadataReply{
				{http.StatusOK, `{"description":"Generated description"}`},
				{http.StatusOK, `{"arguments":` + metadataGeneratedArgs + `}`},
				{http.StatusOK, `{"description":"Refreshed description"}`},
				{http.StatusOK, `{"arguments":` + metadataOriginalArgs + `}`},
			})
			f.create(t, kind, metadataDescriptionFlag)
			created := f.read(t, kind)
			if created.Name != metadataEntityName || created.Description != "Generated description" || !reflect.DeepEqual(created.Arguments, []metadataArgument{{metadataTargetArgument, metadataExistingTarget, true}}) {
				t.Fatalf("generated description readback: %#v", created)
			}
			f.run(t, "", kind, "edit", metadataEntityName, metadataArgumentsFlag, metadataJSONFlag)
			updated := f.read(t, kind)
			if updated.Description != created.Description || !reflect.DeepEqual(updated.Arguments, []metadataArgument{{"destination", "Generated target", false}}) {
				t.Fatalf("generated arguments readback: %#v", updated)
			}
			f.run(t, "", kind, "edit", metadataEntityName, metadataDescriptionFlag, metadataArgumentsFlag, metadataJSONFlag)
			refreshed := f.read(t, kind)
			if refreshed.Description != "Refreshed description" || !reflect.DeepEqual(refreshed.Arguments, created.Arguments) || refreshed.Template != created.Template || refreshed.Script != created.Script || refreshed.RecordID != created.RecordID {
				t.Fatalf("combined generation readback: %#v", refreshed)
			}
			if kind == metadataScriptKind {
				body, _ := f.run(t, "", kind, "show", "inspect")
				if body != metadataBody || created.Script == "" || created.RecordID == "" {
					t.Fatalf("script source or identity changed: body=%q item=%#v", body, created)
				}
			} else if created.Template != metadataTemplate {
				t.Fatalf("template changed: %q", created.Template)
			}
			description, arguments := metadataPrompts(kind, "Generated description", metadataOriginalArgs)
			refreshedDescription, refreshedArguments := metadataPrompts(kind, "Refreshed description", metadataGeneratedArgs)
			f.assertRequests(t, []string{description, arguments, refreshedDescription, refreshedArguments})
		})
	}
}

func TestMetadataGenerationFallbackPreservesPriorFields(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{metadataAliasKind, metadataScriptKind} {
		for _, tc := range []struct {
			name, descriptionResponse, argumentsResponse string
			status                                       int
			wantDescription                              string
			wantArguments                                []metadataArgument
		}{
			{"invalid responses", `{"description":null}`, `{"arguments":null}`, http.StatusOK, metadataPriorDesc, []metadataArgument{{metadataTargetArgument, metadataExistingTarget, true}}},
			{"non-2xx", `{"description":"Discarded"}`, `{"arguments":[]}`, http.StatusBadRequest, metadataPriorDesc, []metadataArgument{{metadataTargetArgument, metadataExistingTarget, true}}},
			{"description fallback only", `{"description":null}`, `{"arguments":` + metadataGeneratedArgs + `}`, http.StatusOK, metadataPriorDesc, []metadataArgument{{"destination", "Generated target", false}}},
			{"arguments fallback only", `{"description":"New description"}`, `{"arguments":null}`, http.StatusOK, "New description", []metadataArgument{{metadataTargetArgument, metadataExistingTarget, true}}},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				f := newMetadataFixture(t, []metadataReply{{tc.status, tc.descriptionResponse}, {tc.status, tc.argumentsResponse}})
				f.create(t, kind, "--description", metadataPriorDesc)
				prior := f.read(t, kind)
				_, diagnostic := f.run(t, "", kind, "edit", metadataEntityName, metadataDescriptionFlag, metadataArgumentsFlag, metadataJSONFlag)
				if !strings.Contains(diagnostic, "metadata generation") {
					t.Fatalf("missing generation warning: %q", diagnostic)
				}
				got := f.read(t, kind)
				want := prior
				want.Description, want.Arguments = tc.wantDescription, tc.wantArguments
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("fallback readback=%#v; want %#v", got, want)
				}
				description, arguments := metadataPrompts(kind, tc.wantDescription, metadataOriginalArgs)
				f.assertRequests(t, []string{description, arguments})
			})
		}
	}
}

func TestMetadataOrdinaryCRUDDoesNotContactProvider(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{metadataAliasKind, metadataScriptKind} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newMetadataFixture(t, nil)
			f.create(t, kind, "--description", "Local description")
			f.read(t, kind)
			f.run(t, "", kind, "edit", metadataEntityName, "--description", "Local edit", metadataJSONFlag)
			if got := f.read(t, kind); got.Description != "Local edit" {
				t.Fatalf("local edit did not persist: %#v", got)
			}
			f.run(t, "", metadataEntityName, "--help")
			if kind == metadataScriptKind {
				f.run(t, "", kind, "show", "inspect")
			}
			f.run(t, "", kind, "remove", metadataEntityName, "--yes", metadataJSONFlag)
			out, _ := f.run(t, "", kind, "list", metadataJSONFlag)
			if !strings.Contains(out, `"items":[]`) {
				t.Fatalf("removed record remains: %s", out)
			}
			f.assertRequests(t, nil)
		})
	}
}
