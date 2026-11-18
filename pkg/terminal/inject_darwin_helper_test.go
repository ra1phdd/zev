//go:build darwin

package terminal

import "syscall"

// sysProcAttrSetsid puts the child in a new session so its stdin TTY becomes
// its controlling terminal, which the Darwin kernel requires for TIOCSTI.
func sysProcAttrSetsid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
