//go:build windows

package cli

func isInterruptedProcess(error) bool { return false }
