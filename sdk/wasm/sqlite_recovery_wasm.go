//go:build js && wasm

package wasm

import (
	"context"
	"database/sql"

	wasmsqlite "github.com/lightninglabs/go-wasmsqlite"
)

func dumpSQLiteDatabase(ctx context.Context, db *sql.DB) ([]byte, error) {
	dump, err := wasmsqlite.DumpDatabaseContext(ctx, db)
	if err != nil {
		return nil, err
	}

	return []byte(dump), nil
}

func loadSQLiteDatabase(ctx context.Context, db *sql.DB, dump []byte) error {
	return wasmsqlite.LoadDatabaseContext(ctx, db, string(dump))
}
