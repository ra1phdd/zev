package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/entities/query/repository/openai"
	"github.com/spf13/cobra"
)

//nolint:paralleltest // setFixtureLanguage, HOME, and t.Chdir mutate process-global state.
func TestMachineNativeToolLoop(t *testing.T) {
	setFixtureLanguage(t, newCLIConfigHome(t), "en")
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("project fixture detail"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		enabled bool
		mode    string
		denied  bool
	}{{"disabled", false, toolModeBypass, false}, {"manual without terminal", true, toolModeManual, true}, {"automatic review", true, toolModeAuto, false}, {"explicit bypass", true, toolModeBypass, false}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct {
						Role, Content string
						ToolCallID    string `json:"tool_call_id"` //nolint:tagliatelle // Native OpenAI wire format.
					}
					Tools []json.RawMessage
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				calls++
				if len(request.Messages) == 0 {
					t.Error("no messages")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				system := request.Messages[0].Content
				if strings.Contains(system, "isolated command safety reviewer") {
					if tc.mode != toolModeAuto || len(request.Tools) != 0 || len(request.Messages) != 2 {
						t.Errorf("unexpected safety review: %+v", request)
					}
					writeToolHTTPResponse(t, w, `{"allowed":true,"reason":"bounded project read"}`)
					return
				}
				last := request.Messages[len(request.Messages)-1]
				if last.Role == "tool" {
					if last.ToolCallID != "call-context" {
						t.Errorf("bad correlation %q", last.ToolCallID)
					}
					if tc.denied && !strings.Contains(last.Content, `"denied":true`) {
						t.Errorf("missing manual refusal %q", last.Content)
					}
					if !tc.denied && !strings.Contains(last.Content, "project fixture detail") {
						t.Errorf("real context absent: %q", last.Content)
					}
					writeToolHTTPResponse(t, w, `{"is_valid":true,"commands":[{"command":"printf final-only","short_explanation":"result","is_dangerous":false}]}`)
					return
				}
				if !tc.enabled {
					if len(request.Tools) != 0 {
						t.Error("machine default advertised tools")
					}
					writeToolHTTPResponse(t, w, `{"is_valid":true,"commands":[{"command":"printf final-only","short_explanation":"result","is_dangerous":false}]}`)
					return
				}
				if len(request.Tools) != 1 {
					t.Errorf("native exec not advertised: %d", len(request.Tools))
				}
				_, err := io.WriteString(w, `{"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call-context","type":"function","function":{"name":"exec","arguments":"{\"command\":\"cat\",\"args\":[\"README.md\"],\"reason\":\"Read project documentation\"}"}}]}}]}`)
				if err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			provider, err := openai.New("fixture", cliFixtureModel, server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			cfg := config.DefaultConfig()
			cfg.ToolApprovalMode = tc.mode
			var out, diagnostics bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			cmd.SetOut(&out)
			cmd.SetErr(&diagnostics)
			cmd.SetIn(strings.NewReader("yes\n"))
			flags := parsedFlags{Print: true, Option: 1, Tools: tc.enabled}
			err = runMachineProvider(cmd, flags, machineProviderParams{ToolConfig: cfg, Provider: provider, Prompt: "inspect project", Context: func() (string, error) { return "test environment", nil }, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			if err != nil {
				t.Fatal(err)
			}
			if out.String() != "printf final-only\n" {
				t.Fatalf("machine stdout polluted %q", out.String())
			}
			expected := 1
			if tc.enabled {
				expected = 2
			}
			if tc.mode == toolModeAuto {
				expected = 3
			}
			if calls != expected {
				t.Fatalf("calls=%d want=%d", calls, expected)
			}
			if tc.mode == toolModeBypass && tc.enabled && !strings.Contains(diagnostics.String(), toolModeBypass) {
				t.Error("missing bypass warning")
			}
		})
	}
}

func writeToolHTTPResponse(t *testing.T, w io.Writer, content string) {
	t.Helper()
	response := map[string]any{"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Error(err)
	}
}

func TestToolSettingsRespectPrivacyAndExplicitOptIn(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	enabled, mode := resolveToolSettings(cfg, parsedFlags{NoContext: true})
	if enabled || mode != toolModeManual {
		t.Fatalf("privacy mode=%t %s", enabled, mode)
	}
	enabled, _ = resolveToolSettings(cfg, parsedFlags{NoContext: true, Tools: true})
	if !enabled {
		t.Fatal("explicit tools not honored")
	}
}
