//go:build windows

package terminal

// Inject не поддерживается на Windows: TIOCSTI ioctl там отсутствует.
// Используйте --execute или скопируйте команду из буфера обмена вручную.
func Inject(_ string) error {
	return ErrInjectionUnsupported
}
