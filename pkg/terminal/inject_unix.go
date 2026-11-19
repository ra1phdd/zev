//go:build !windows && !darwin

package terminal

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode"
	"unsafe"

	"github.com/ra1phdd/zev/internal/core/domain"
	"golang.org/x/term"
)

// Inject inserts a nonempty single-line command into terminal input without executing it.
// It returns ErrInjectionFailed for invalid command text or any control character,
// including newlines and tabs, before accessing the terminal. Callers can use
// errors.Is to distinguish ErrNotTTY and ioctl failures wrapping ErrInjectionFailed.
func Inject(text string) error {
	response := domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: text}}}
	if err := response.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInjectionFailed, err)
	}
	// TIOCSTI feeds terminal input processing: newlines execute and tabs complete.
	if strings.ContainsFunc(text, unicode.IsControl) {
		return fmt.Errorf("%w: terminal control characters are not allowed", ErrInjectionFailed)
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return ErrNotTTY
	}

	// TIOCSTI writes each byte through the terminal's input processing.
	const TIOCSTI = 0x5412

	for i := range len(text) {
		b := text[i]
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			uintptr(fd),
			uintptr(TIOCSTI),
			uintptr(unsafe.Pointer(&b)),
		)
		if errno != 0 {
			return &PartialInjectionError{Written: i, Err: fmt.Errorf("%w: %w", ErrInjectionFailed, errno)}
		}
	}

	return nil
}
