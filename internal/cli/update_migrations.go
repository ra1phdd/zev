package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/migrations"
)

func runUpdateMigrations(ctx context.Context, output io.Writer) error {
	if err := migrations.Run(ctx, func(name string) error {
		return writeQueryOutput(output, locale.Text(locale.KeyUpdateMigrating), name)
	}); err != nil {
		return fmt.Errorf("upgrade local data: %w", err)
	}
	return nil
}
