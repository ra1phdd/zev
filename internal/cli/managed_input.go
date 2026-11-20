package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type managedInput struct {
	input     textinput.Model
	title     string
	value     string
	submitted bool
	cancelled bool
}

func newManagedInput(title, current string) managedInput {
	input := textinput.New()
	input.SetValue(current)
	input.Focus()
	input.CharLimit = 65536
	return managedInput{input: input, title: title}
}
func (m managedInput) Init() tea.Cmd { return textinput.Blink }
func (m managedInput) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyCtrlC, tea.KeyEsc, tea.KeyCtrlD:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEnter:
			m.value = m.input.Value()
			m.submitted = true
			return m, tea.Quit
		default:
			// textinput handles editing and rune input below.
		}
	}
	var command tea.Cmd
	m.input, command = m.input.Update(message)
	return m, command
}

func (m managedInput) View() string {
	return "\n" + m.title + "\n" + m.input.View() + "\n\n" + locale.Text(locale.KeyUIConfirmHint) + "\n"
}

func readManagedTTY(cmd *cobra.Command, title, current string) (string, error) {
	model := newManagedInput(title, current)
	result, err := tea.NewProgram(model, tea.WithContext(cmd.Context()), tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.ErrOrStderr())).Run()
	if contextErr := cmd.Context().Err(); contextErr != nil {
		return "", fmt.Errorf("%s: %w", locale.Text(locale.KeyManagedCancelled), contextErr)
	}
	if err != nil {
		return "", fmt.Errorf("read managed form: %w", err)
	}
	final, ok := result.(managedInput)
	if !ok || !final.submitted || final.cancelled {
		return "", errors.New(locale.Text(locale.KeyManagedCancelled))
	}
	return final.value, nil
}

func realManagedTTY(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
