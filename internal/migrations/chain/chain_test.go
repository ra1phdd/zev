package chain

import (
	"context"
	"errors"
	"testing"
)

func TestUpgradeUsesOnlyMissingSteps(t *testing.T) {
	t.Parallel()
	appendByte := func(b byte) Step {
		return func(_ context.Context, data []byte) ([]byte, error) { return append(data, b), nil }
	}
	p := Plan{Base: 1, Steps: []Step{appendByte('2'), appendByte('3')}}
	for _, from := range []int{1, 2, 3} {
		input := []byte("123"[:from])
		got, err := p.Upgrade(t.Context(), from, input)
		if err != nil || string(got) != "123" {
			t.Errorf("from v%d: got %q, %v", from, got, err)
		}
	}
}

func TestUpgradeRejectsInvalidPlansBeforeRunning(t *testing.T) {
	t.Parallel()
	fail := func(context.Context, []byte) ([]byte, error) { t.Fatal("unexpected step"); return nil, nil }
	for _, tc := range []struct {
		p    Plan
		from int
	}{{Plan{1, []Step{fail, nil}}, 1}, {Plan{1, []Step{fail}}, 3}, {Plan{1, []Step{fail}}, 0}} {
		if _, err := tc.p.Upgrade(t.Context(), tc.from, nil); err == nil {
			t.Error("accepted unsupported version or gap")
		}
	}
}

func TestFailedUpgradeLeavesSourceUntouched(t *testing.T) {
	t.Parallel()
	failure := errors.New("invalid source")
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	for _, interrupt := range []bool{false, true} {
		p := Plan{Base: 0, Steps: []Step{func(_ context.Context, b []byte) ([]byte, error) {
			b[0] = '2'
			if interrupt {
				cancel()
			}
			return b, nil
		}, func(context.Context, []byte) ([]byte, error) { return nil, failure }}}
		want := failure
		if interrupt {
			want = context.Canceled
		}
		source := []byte("1")
		got, err := p.Upgrade(ctx, 0, source)
		if !errors.Is(err, want) || got != nil || string(source) != "1" {
			t.Fatalf("source %q, result %q, error %v; want %v", source, got, err, want)
		}
	}
}
