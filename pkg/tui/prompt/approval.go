package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ra1phdd/zev/internal/core/locale"
)

// ApprovalOptions identifies the exact action and the diagnostic output stream.
// Prompt must escape any untrusted terminal control characters.
type ApprovalOptions struct {
	Prompt string
	Output io.Writer
	Input  io.Reader
}

// ConfirmContext accepts y or Enter as approval, n or Esc as cancellation, and
// Ctrl+C as a terminal cancellation. All UI output goes to Output.
func ConfirmContext(ctx context.Context, options ApprovalOptions) (bool, error) {
	return confirmContext(ctx, options)
}

func confirmContext(ctx context.Context, options ApprovalOptions, programOpts ...tea.ProgramOption) (bool, error) {
	if options.Output == nil {
		options.Output = io.Discard
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("confirm approval: %w", err)
	}
	m := approvalModel{prompt: options.Prompt}
	inputCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	input := options.Input
	if input == nil {
		input = os.Stdin
	}
	opts := make([]tea.ProgramOption, 0, len(programOpts)+3)
	opts = append(opts, tea.WithContext(inputCtx), tea.WithOutput(options.Output), tea.WithInput(readerForProgram(input)))
	opts = append(opts, programOpts...)
	result, err := tea.NewProgram(m, opts...).Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, fmt.Errorf("confirm approval: %w", ctxErr)
	}
	if inputCtx.Err() != nil && ctx.Err() == nil {
		return false, fmt.Errorf("confirm approval: %w", inputCtx.Err())
	}
	if err != nil {
		return false, fmt.Errorf("confirm tool: %w", err)
	}
	final, ok := result.(approvalModel)
	if !ok {
		return false, fmt.Errorf("unexpected approval model type %T", result)
	}
	if final.cancelled {
		return false, errors.New("cancelled")
	}
	return final.approved, nil
}

type cancelOnEOFReader struct {
	io.Reader
	eofSent  bool
	hadInput bool
}

func (r *cancelOnEOFReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.hadInput = true
	}
	if err == nil {
		return n, nil
	}
	if errors.Is(err, io.EOF) && !r.eofSent && !r.hadInput && len(p) > 0 {
		r.eofSent = true
		p[0] = 3
		return 1, nil
	}
	return n, fmt.Errorf("read input: %w", err)
}

type approvalModel struct {
	prompt    string
	approved  bool
	denied    bool
	cancelled bool
}

func (m approvalModel) Init() tea.Cmd { return nil }

func (m approvalModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyEnter:
		m.approved = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyCtrlC:
		m.cancelled = true
		return m, tea.Quit
	case tea.KeyRunes:
		if strings.EqualFold(string(key.Runes), "y") {
			m.approved = true
			return m, tea.Quit
		}
		if strings.EqualFold(string(key.Runes), "n") {
			m.denied = true
			return m, tea.Quit
		}
	default:
		return m, nil
	}
	return m, nil
}

func (m approvalModel) View() string {
	return fmt.Sprintf("\n%s\n%s", m.prompt, locale.Text(locale.KeyToolsApprovalPrompt))
}
