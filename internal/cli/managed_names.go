package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/ra1phdd/zev/internal/core/filestore"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
	aliasmigration "github.com/ra1phdd/zev/internal/migrations/aliases"
	scriptmigration "github.com/ra1phdd/zev/internal/migrations/scripts"
	"github.com/spf13/cobra"
)

// createManagedEntity serializes cross-store name checks with managed writes.
func createManagedEntity(ctx context.Context, root *cobra.Command, dir, kind, name string, create func() error) error {
	lockPath := filepath.Join(dir, ".managed-names")
	if err := filestore.WithLockContext(ctx, lockPath, func() error {
		if err := rejectBuiltinName(root, name); err != nil {
			return err
		}
		if err := aliasmigration.Migrate(ctx, dir); err != nil {
			return fmt.Errorf("migrate alias store: %w", err)
		}
		if err := scriptmigration.Migrate(ctx, dir); err != nil {
			return fmt.Errorf("migrate script store: %w", err)
		}
		aliases, err := aliasrepo.New(dir).LoadAllContext(ctx)
		if err != nil {
			return fmt.Errorf("load aliases: %w", err)
		}
		scripts, err := scriptrepo.New(dir).LoadAllContext(ctx)
		if err != nil {
			return fmt.Errorf("load scripts: %w", err)
		}
		if _, exists := aliases[name]; exists && kind != managedKindAlias {
			return fmt.Errorf("name %q is already used by an alias", name)
		}
		if _, exists := scripts[name]; exists && kind != managedKindScript {
			return fmt.Errorf("name %q is already used by a script", name)
		}
		return create()
	}); err != nil {
		return fmt.Errorf("lock managed entity names: %w", err)
	}
	return nil
}

func rejectBuiltinName(root *cobra.Command, name string) error {
	if root == nil {
		return nil
	}
	for _, command := range root.Root().Commands() {
		if name == command.Name() {
			return fmt.Errorf("name %q conflicts with built-in command %q", name, command.Name())
		}
		if slices.Contains(command.Aliases, name) {
			return fmt.Errorf("name %q conflicts with built-in command %q", name, command.Name())
		}
	}
	return nil
}
