// Package aliases defines the ordered upgrades for aliases metadata.
package aliases

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/migrations/chain"
	"github.com/ra1phdd/zev/internal/migrations/metadatafile"
)

// Schema returns the migration chain beginning with the unversioned legacy map.
func Schema() metadatafile.Schema {
	return metadatafile.Schema{Entity: "aliases", Plan: chain.Plan{Base: 1, Steps: []chain.Step{toV2}}}
}

// Migrate upgrades aliases in dir under the same lock as repository updates.
func Migrate(ctx context.Context, dir string) error {
	if err := filestore.ValidateDirectory(dir); err != nil {
		return fmt.Errorf("validate aliases directory: %w", err)
	}
	if err := filestore.WithLockContext(ctx, filepath.Join(dir, "aliases"), func() error {
		return Schema().Apply(ctx, dir, func(data []byte, _ bool) error {
			_, err := metadata.DecodeAliases(data)
			if err != nil {
				return fmt.Errorf("decode aliases: %w", err)
			}
			return nil
		})
	}); err != nil {
		return fmt.Errorf("migrate aliases: %w", err)
	}
	return nil
}
