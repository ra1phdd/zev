package cli

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

const queryFlowFixturePrompt = "show files"

//nolint:paralleltest // The fixture changes HOME and restores package-level query flags.
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
	writeCLIConfig(t, newCLIConfigHome(t), openAITestConfig(server.URL, "test-key"))
	outputErr := errors.New("progress output unavailable")
	cmd := &cobra.Command{}
	cmd.SetOut(failingOutput{err: outputErr})
	cmd.SetErr(io.Discard)
	cmd.SetContext(t.Context())
	if err := runQuery(cmd, []string{queryFlowFixturePrompt}); !errors.Is(err, outputErr) {
		t.Errorf("runQuery() error = %v, want output failure", err)
	}
	if calls != 0 {
		t.Errorf("provider requests = %d, want none after output failure", calls)
	}
}
