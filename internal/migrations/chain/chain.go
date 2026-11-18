// Package chain applies ordered, adjacent upgrades to one in-memory document.
package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
)

// Step converts and validates one schema version. Steps must not publish data.
type Step func(context.Context, []byte) ([]byte, error)

// Plan starts at Base. Steps[i] targets Base+i+1; append, never reorder, steps.
type Plan struct {
	Base  int
	Steps []Step
}

// Current is the highest schema version produced by this plan.
func (p Plan) Current() int { return p.Base + len(p.Steps) }

// Upgrade returns the original bytes for a current document, or the result of
// every missing step. The caller owns detection, locking and atomic publication.
func (p Plan) Upgrade(ctx context.Context, from int, data []byte) ([]byte, error) {
	if p.Base < 0 || slices.ContainsFunc(p.Steps, func(s Step) bool { return s == nil }) {
		return nil, errors.New("invalid migration chain: missing base or step")
	}
	if from < p.Base || from > p.Current() {
		return nil, fmt.Errorf("unsupported schema version %d (supported: %d..%d)", from, p.Base, p.Current())
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("plan migration: %w", err)
	}
	if from == p.Current() {
		return data, nil
	}
	next := bytes.Clone(data)
	for i, step := range p.Steps[from-p.Base:] {
		result, err := step(ctx, next)
		if err != nil {
			return nil, fmt.Errorf("migrate v%d to v%d: %w", from+i, from+i+1, err)
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("migrate v%d to v%d: %w", from+i, from+i+1, err)
		}
		next = result
	}
	return next, nil
}
