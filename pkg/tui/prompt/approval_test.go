package prompt

import (
	"context"
	"errors"
	"testing"
)

func TestConfirmContextCancellationNeverApproves(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	approved, err := ConfirmContext(ctx, ApprovalOptions{Prompt: "Approve?"})
	if approved || !errors.Is(err, context.Canceled) {
		t.Fatalf("approval=%t error=%v", approved, err)
	}
}
