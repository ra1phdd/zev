//go:build e2e

package e2e_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestHistoryMigrationRespectsLocalAndPrivacyModes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"is_valid\":true,\"commands\":[{\"command\":\"echo privacy-fixture\",\"short_explanation\":\"fixture\"}]}"}}]}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	for _, tc := range []struct {
		name    string
		args    []string
		enabled bool
	}{
		{name: "help", args: []string{"--help"}, enabled: true},
		{name: "version", args: []string{"version"}, enabled: true},
		{name: "doctor", args: []string{"doctor", "--json"}, enabled: true},
		{name: "query flag", args: []string{"query", "--print", "--no-history", "privacy fixture"}, enabled: true},
		{name: "query config", args: []string{"query", "--print", "privacy fixture"}, enabled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := seedLegacyHistory(t)
			cmd := cliCommand(t, home, tc.args...)
			setCLIEnv(cmd, map[string]string{
				envHistoryEnabled: strconv.FormatBool(tc.enabled), envProvider: providerOpenAI,
				envBaseURL: server.URL, envModel: fixtureModel,
				envAPIKey: fixtureOnly, "SHELL": "/bin/sh",
			})
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("CLI failed: %v; output: %s", err, output)
			}
			source := filepath.Join(home, ".zev", "history")
			data, err := os.ReadFile(filepath.Clean(source))
			if err != nil || string(data) != legacyLine {
				t.Fatalf("local/privacy invocation changed legacy history: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(home, ".zev", "history.db")); !os.IsNotExist(err) {
				t.Fatalf("local/privacy invocation created database: %v", err)
			}
			if archives, err := filepath.Glob(source + ".migrated.*.jsonl"); err != nil || len(archives) != 0 {
				t.Fatalf("local/privacy invocation archived history: %v, %v", archives, err)
			}
		})
	}
}
