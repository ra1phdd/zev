package terminal

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/term"
)

var (
	// ErrNotTTY возвращается, когда stdin не является TTY.
	ErrNotTTY = errors.New("stdin is not a TTY")

	// ErrInjectionFailed возвращается, когда TIOCSTI ioctl не сработал.
	ErrInjectionFailed = errors.New("failed to inject command")
)

// Inject вставляет текст в input buffer терминала.
// Использует TIOCSTI ioctl для "впрыскивания" символов.
// Работает только на Unix-системах (Linux, macOS) и только когда stdin является TTY.
func Inject(text string) error {
	// Проверяем, что stdin является TTY
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return ErrNotTTY
	}

	// TIOCSTI позволяет записать символы в input buffer
	const TIOCSTI = 0x5412 // для Linux и macOS

	// Записываем каждый байт в input buffer
	for i := range len(text) {
		b := text[i]
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			uintptr(fd),
			uintptr(TIOCSTI),
			uintptr(unsafe.Pointer(&b)),
		)
		if errno != 0 {
			return fmt.Errorf("%w: %w", ErrInjectionFailed, errno)
		}
	}

	return nil
}
