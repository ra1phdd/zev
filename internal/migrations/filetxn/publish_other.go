//go:build !darwin && !linux && !windows

package filetxn

import "errors"

func moveExclusive(_, _ string) error {
	return errors.New("no-overwrite configuration migration unsupported on this platform")
}
