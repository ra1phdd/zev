package metadatafile_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/ra1phdd/zev/internal/migrations/aliases"
	"github.com/ra1phdd/zev/internal/migrations/metadatafile"
	"github.com/ra1phdd/zev/internal/migrations/scripts"
)

func TestMetadataChainsAcceptAnotherStep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		schema metadatafile.Schema
		legacy string
	}{
		{aliases.Schema(), "show: echo\n"},
		{scripts.Schema(), "show: {script: show.sh}\n"},
	} {
		t.Run(tc.schema.Entity, func(t *testing.T) {
			schema := tc.schema
			v2, changed, err := schema.Upgrade(t.Context(), []byte(tc.legacy))
			if err != nil || !changed {
				t.Fatalf("legacy upgrade: changed=%v, err=%v", changed, err)
			}
			original := append([]byte("# preserve this comment\n"), v2...)
			got, changed, err := schema.Upgrade(t.Context(), original)
			if err != nil || changed || !bytes.Equal(original, got) {
				t.Fatalf("current v2 changed: %v", err)
			}
			// Append a step with a different envelope. Starting from v2 must skip the
			// legacy transformation (including script ID allocation).
			schema.Plan.Steps = append(schema.Plan.Steps, func(_ context.Context, data []byte) ([]byte, error) {
				if !bytes.Contains(data, []byte("version: 2\n")) {
					return nil, fmt.Errorf("expected v2 input: %q", data)
				}
				return []byte("version: 3\nentries: []\n"), nil
			})
			for _, source := range [][]byte{[]byte(tc.legacy), original, []byte("version: 3\nentries: []\n")} {
				got, changed, err := schema.Upgrade(t.Context(), source)
				if err != nil || string(got) != "version: 3\nentries: []\n" || changed == bytes.Equal(source, got) {
					t.Fatalf("extend from %q: got %q, changed=%v err=%v", source, got, changed, err)
				}
			}
		})
	}
}

func TestUpgradeRejectsUnrecognizedShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		schema metadatafile.Schema
		bad    []string
	}{
		{aliases.Schema(), []string{"show: {template: echo}", "show: true", "show: ''", "show: echo | sh", "version: echo", "show: echo\nshow: ls", "5: echo"}},
		{scripts.Schema(), []string{"show: show.sh", "show: {script: true}", "show: {description: text}", "show: {script: x, recordID: id}", "show: {script: x, arguments: []}", "show: {script: x, description: null}", "version: {script: show.sh}", "show: {script: a, script: b}"}},
	} {
		for _, input := range tc.bad {
			t.Run(tc.schema.Entity+"/"+input, func(t *testing.T) {
				if _, _, err := tc.schema.Upgrade(t.Context(), []byte(input)); err == nil {
					t.Fatal("accepted unknown input")
				}
			})
		}
	}
}
