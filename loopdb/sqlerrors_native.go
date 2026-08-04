//go:build !js || !wasm

package loopdb

import (
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// mapSqliteError attempts to parse a native sqlite error as a database
// agnostic SQL error.
func mapSqliteError(err error) (error, bool) {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return nil, false
	}

	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE:
		return &ErrSqlUniqueConstraintViolation{
			DbError: sqliteErr,
		}, true

	default:
		return fmt.Errorf("unknown sqlite error: %w", sqliteErr), true
	}
}
