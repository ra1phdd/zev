package metadatafile

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/ra1phdd/zev/internal/core/metadata/yamldoc"

	"github.com/ra1phdd/zev/internal/migrations/chain"
)

// Schema pairs a metadata document's discriminator with its ordered migrations.
type Schema struct {
	Entity string
	Plan   chain.Plan
}

// Upgrade runs missing steps in memory, under the repository lock. The caller
// validates the current payload before publishing, and skips publication if unchanged.
func (s Schema) Upgrade(ctx context.Context, source []byte) ([]byte, bool, error) {
	from, err := documentVersion(source)
	if err != nil {
		return nil, false, fmt.Errorf("detect %s version: %w", s.Entity, err)
	}
	canonical, err := s.Plan.Upgrade(ctx, from, source)
	if err != nil {
		return nil, false, fmt.Errorf("upgrade %s: %w", s.Entity, err)
	}
	return canonical, from != s.Plan.Current(), nil
}

// The only released pre-envelope format has no integer discriminator. A map
// with a non-integer version key still has to pass the full v1 step (including
// reserved-name validation); it is never accepted merely by detection.
func documentVersion(source []byte) (int, error) {
	fields, err := yamldoc.DecodeMapping(source)
	if err != nil {
		return 0, fmt.Errorf("decode metadata mapping: %w", err)
	}
	node := fields["version"]
	if node == nil || node.Tag != "!!int" {
		return 1, nil
	}
	version, err := strconv.Atoi(node.Value)
	if err != nil || version < 2 {
		return 0, errors.New("invalid metadata version")
	}
	return version, nil
}
