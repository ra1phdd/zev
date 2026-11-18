package filetxn

import (
	"errors"
	"fmt"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

// Replace atomically publishes validated data in place, without archiving the
// original. Caller holds the repository lock from the initial read onwards.
// A changed file identity or content returns ErrConflict.
func (s *Snapshot) Replace(data []byte) error {
	if len(data) > MaxBytes {
		return errors.New("converted document exceeds size limit")
	}
	if err := s.verifySource(); err != nil {
		return err
	}
	if err := filestore.AtomicWrite(s.path, data, 0o600); err != nil {
		return fmt.Errorf("publish upgraded document: %w", err)
	}
	if err := verify(s.path, data); err != nil {
		return errors.Join(filestore.ErrCommitted, err)
	}
	return nil
}
