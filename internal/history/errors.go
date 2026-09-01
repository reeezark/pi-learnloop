package history

import "errors"

var (
	ErrClosed            = errors.New("history: store is closed")
	ErrConflict          = errors.New("history: conflicting update")
	ErrCorrupt           = errors.New("history: database is corrupt")
	ErrInvalid           = errors.New("history: invalid value")
	ErrNotFound          = errors.New("history: record not found")
	ErrUnsafePath        = errors.New("history: unsafe storage path")
	ErrUnsupportedSchema = errors.New("history: unsupported schema version")
)
