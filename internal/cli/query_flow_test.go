package cli

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

// Query flags and configuration use process-wide state.
func TestQueryStopsOnProgressOutputFailure(t *testing.T) {
	verbosity, execute, inject := verbosityLevel, executeFlag, injectFlag
	t.Cleanup(func() {
		verbosityLevel, executeFlag, injectFlag = verbosity, execute, inject
	})
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, err := io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"is_valid\":false}"}}]}`)
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)
	outputErr := errors.New("progress output unavailable")
	cmd := &cobra.Command{}
	cmd.SetOut(failingOutput{err: outputErr})
	cmd.SetErr(io.Discard)
	cmd.SetContext(t.Context())
	if err := runQuery(cmd, []string{"show files"}); !errors.Is(err, outputErr) {
		t.Errorf("runQuery() error = %v, want output failure", err)
	}
	if calls != 0 {
		t.Errorf("provider requests = %d, want none after output failure", calls)
	}
}
