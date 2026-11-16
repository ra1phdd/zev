package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ra1phdd/zev/internal/entities/history/service"
	"modernc.org/sqlite"
)

const (
	sqliteBusy       = 5
	sqliteLocked     = 6
	sqliteConstraint = 19

	// SQLite extended result codes keep the primary result code in the low byte.
	sqliteConstraintForeignKey = sqliteConstraint | 3<<8
	sqliteConstraintPrimaryKey = sqliteConstraint | 6<<8
	sqliteConstraintUnique     = sqliteConstraint | 8<<8
)

// storageError keeps diagnostic causes inspectable without rendering SQL or data.
type storageError struct {
	class error
	cause error
	text  string
}

func (e *storageError) Error() string { return e.text }

func (e *storageError) Unwrap() []error { return []error{e.class, e.cause} }

// classifyStorageError maps low-level history failures to stable service errors.
// The original error remains available through errors.Is and errors.As, while
// Error deliberately contains no SQL statements, paths, or bound values.
func classifyStorageError(err error) error {
	if err == nil {
		return nil
	}
	text, class := classifyStorageCause(err)
	return &storageError{class: class, cause: err, text: text}
}

func classifyStorageCause(err error) (string, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "history operation cancelled", service.ErrCancelled
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "history not found", service.ErrNotFound
	}
	for _, typed := range []struct {
		class error
		text  string
	}{
		{service.ErrNotFound, "history not found"},
		{service.ErrConflict, "history conflict"},
		{service.ErrInvalid, "history invalid input"},
		{service.ErrUnsupportedSchema, "history unsupported schema"},
		{service.ErrBusy, "history storage busy"},
		{service.ErrCancelled, "history operation cancelled"},
		{service.ErrStorageFailure, "history storage failure"},
		{service.ErrEmbeddingUnavailable, "history embedding unavailable"},
	} {
		if errors.Is(err, typed.class) {
			return typed.text, typed.class
		}
	}

	sqliteErr, ok := errors.AsType[*sqlite.Error](err)
	if ok {
		code := sqliteErr.Code()
		primary := code & 0xff
		switch primary {
		case sqliteBusy, sqliteLocked:
			return "history storage busy", service.ErrBusy
		case sqliteConstraint:
			switch code {
			case sqliteConstraintForeignKey:
				return "history invalid input", service.ErrInvalid
			case sqliteConstraintPrimaryKey, sqliteConstraintUnique, sqliteConstraint:
				return "history conflict", service.ErrConflict
			}
		}
	}
	return "history storage failure", service.ErrStorageFailure
}
