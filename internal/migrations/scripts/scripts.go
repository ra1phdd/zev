// Package scripts defines the ordered upgrades for scripts metadata.
package scripts

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
	"github.com/ra1phdd/zev/internal/migrations/chain"
	"github.com/ra1phdd/zev/internal/migrations/metadatafile"
)

// Schema returns the migration chain beginning with the unversioned legacy map.
func Schema() metadatafile.Schema {
	return metadatafile.Schema{Entity: "scripts", Plan: chain.Plan{Base: 1, Steps: []chain.Step{toV2}}}
}

// Migrate upgrades scripts in dir only after validating every referenced body
// under the repository's writer lock. Script bodies are never rewritten.
func Migrate(ctx context.Context, dir string) error {
	if err := repository.New(dir).WithLockedFiles(ctx, func(path string, checkBody func(string) error) error {
		return Schema().Apply(ctx, filepath.Dir(path), func(data []byte, publishing bool) error {
			scripts, err := metadata.DecodeScripts(data)
			if err != nil {
				return fmt.Errorf("decode scripts: %w", err)
			}
			// A current metadata-only lookup need not open an absent body.
			if !publishing {
				return nil
			}
			for name, script := range scripts {
				if err := checkBody(script.ScriptFile); err != nil {
					return fmt.Errorf("script %q body: %w", name, err)
				}
			}
			return nil
		})
	}); err != nil {
		return fmt.Errorf("migrate scripts: %w", err)
	}
	return nil
}
