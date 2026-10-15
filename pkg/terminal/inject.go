package terminal

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Inject вставляет текст в input buffer терминала.
// Использует TIOCSTI ioctl для "впрыскивания" символов.
// Работает только на Unix-системах (Linux, macOS).
func Inject(text string) error {
	// Получаем file descriptor stdin
	fd := os.Stdin.Fd()

	// TIOCSTI позволяет записать символы в input buffer
	const TIOCSTI = 0x5412 // для Linux и macOS

	// Записываем каждый байт в input buffer
	for i := range len(text) {
		b := text[i]
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			fd,
			uintptr(TIOCSTI),
			uintptr(unsafe.Pointer(&b)),
		)
		if errno != 0 {
			return fmt.Errorf("failed to inject character: %w", errno)
		}
	}

	return nil
}
