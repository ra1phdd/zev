package migrations

import (
	"context"
	"errors"
	"testing"
)

func TestRunStoresAttemptsAllAndPreservesErrors(t *testing.T) {
	t.Parallel()
	first := errors.New("first migration failed")
	second := errors.New("second migration failed")
	calls := 0
	stores := []store{
		{name: "first", run: func(context.Context, string) error { calls++; return first }},
		{name: "second", run: func(context.Context, string) error { calls++; return second }},
		{name: "third", run: func(context.Context, string) error { calls++; return nil }},
	}
	err := runStores(t.Context(), nil, stores)
	if !errors.Is(err, first) || !errors.Is(err, second) || calls != 3 {
		t.Fatalf("error=%v, calls=%d; want both failures and all three attempts", err, calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls = 0
	if err := runStores(ctx, nil, stores); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("canceled migrations: %v, calls=%d", err, calls)
	}
	if err := runStores(t.Context(), func(string) error { return second }, stores); !errors.Is(err, second) || calls != 0 {
		t.Fatalf("report failure: %v, calls=%d", err, calls)
	}
}
