package prompt

import (
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ra1phdd/zev/locale"
)

// ApprovalOptions identifies the exact action and the diagnostic output stream.
// Prompt must escape any untrusted terminal control characters.
type ApprovalOptions struct {
	Prompt string
	Output io.Writer
}

// ConfirmContext requires the literal token yes. Empty input never authorizes
// an action. All UI output goes to Output; cancellation returns false and an error.
func ConfirmContext(ctx context.Context, options ApprovalOptions) (bool, error) {
	if options.Output == nil {
		options.Output = io.Discard
	}
	value, err := inputContext(ctx, options.Prompt+"\n"+locale.Text("Type yes to approve this tool call; anything else denies it."), tea.WithOutput(options.Output))
	if err != nil {
		return false, fmt.Errorf("confirm tool: %w", err)
	}
	return value == "yes", nil
}
