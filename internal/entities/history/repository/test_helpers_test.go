package repository

import (
	"io"
	"testing"
)

func closeImportTest(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Error(err)
	}
}
