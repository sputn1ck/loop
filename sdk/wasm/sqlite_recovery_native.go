//go:build !js || !wasm

package wasm

import (
	"context"
	"database/sql"
	"errors"
)

var errBrowserSQLiteRecovery = errors.New(
	"OPFS SQLite recovery is only available in a browser WASM build",
)

func dumpSQLiteDatabase(context.Context, *sql.DB) ([]byte, error) {
	return nil, errBrowserSQLiteRecovery
}

func loadSQLiteDatabase(context.Context, *sql.DB, []byte) error {
	return errBrowserSQLiteRecovery
}
