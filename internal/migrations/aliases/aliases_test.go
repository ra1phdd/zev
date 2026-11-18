package aliases

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationRejectsUnreadableResultBeforePublication(t *testing.T) {
	t.Parallel()
	// Each legacy record is two YAML nodes; its v2 representation has more.
	var source strings.Builder
	for i := range 1500 {
		fmt.Fprintf(&source, "item%d: echo\n", i)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases")
	if err := os.WriteFile(path, []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), dir); err == nil || !strings.Contains(err.Error(), "YAML nodes") {
		t.Fatalf("expected node limit error, got %v", err)
	}
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil || string(got) != source.String() {
		t.Fatalf("migration replaced the readable source: %v", err)
	}
}
