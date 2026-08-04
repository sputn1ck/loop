//go:build js && wasm

package loopdb

import "strings"

// mapSqliteError attempts to parse a WASM sqlite error as a database agnostic
// SQL error. The browser driver exposes SQLite result codes in error strings.
func mapSqliteError(err error) (error, bool) {
	message := strings.ToLower(err.Error())
	isSQLite := strings.Contains(message, "sqlite") ||
		strings.Contains(message, "constraint")
	if !isSQLite {
		return nil, false
	}

	isUnique := strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "sqlite_constraint_unique") ||
		strings.Contains(message, "(2067)")
	if isUnique {
		return &ErrSqlUniqueConstraintViolation{
			DbError: err,
		}, true
	}

	return err, true
}
