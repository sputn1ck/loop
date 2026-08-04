//go:build js && wasm

package loopdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	wasmsqlite "github.com/lightninglabs/go-wasmsqlite"
)

const (
	wasmSQLiteOpenAttempts = 25
	wasmSQLiteOpenDelay    = 200 * time.Millisecond
)

// openSqliteDatabase opens a persistent browser SQLite database in OPFS.
func openSqliteDatabase(cfg *SqliteConfig,
	pragmas []string) (*sql.DB, error) {

	options := &wasmsqlite.Options{
		File:              wasmSQLiteFileName(cfg.DatabaseFileName),
		VFS:               "auto",
		BusyTimeout:       5000,
		RequirePersistent: true,
		DisallowMemory:    true,
		ParseTime:         true,
		JournalMode:       "WAL",
		Mode:              "rwc",
	}

	for _, pragma := range pragmas {
		switch {
		case strings.HasPrefix(pragma, "busy_timeout="):
			// BusyTimeout is set through the driver's typed option.

		case strings.HasPrefix(pragma, "journal_mode="):
			// JournalMode is set through the driver's typed option.

		case strings.HasPrefix(pragma, "fullfsync="):
			// fullfsync is a native filesystem durability hint and is
			// not meaningful for browser OPFS.

		default:
			options.Pragma = append(options.Pragma, pragma)
		}
	}

	// Loop owns one connection to its OPFS file. Exclusive mode prevents a
	// second SQL connection in the same runtime from silently racing it.
	options.Pragma = append(options.Pragma, "locking_mode=EXCLUSIVE")

	return openWASMSQLiteWithRetry(context.Background(), options)
}

// openWASMSQLiteWithRetry handles the brief OPFS release race after a browser
// page reload. Each attempt uses a fresh database/sql handle.
func openWASMSQLiteWithRetry(ctx context.Context,
	options *wasmsqlite.Options) (*sql.DB, error) {

	var lastErr error
	for attempt := 0; attempt < wasmSQLiteOpenAttempts; attempt++ {
		db, err := wasmsqlite.Open(options)
		if err != nil {
			return nil, err
		}

		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)

		err = db.PingContext(ctx)
		if err == nil {
			return db, nil
		}

		_ = db.Close()
		if !isWASMSQLiteOpenRace(err) {
			return nil, err
		}

		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-time.After(wasmSQLiteOpenDelay):
		}
	}

	return nil, fmt.Errorf("open OPFS SQLite database: %w", lastErr)
}

// isWASMSQLiteOpenRace identifies transient errors seen while a previous page
// runtime is still releasing its OPFS database handle.
func isWASMSQLiteOpenRace(err error) bool {
	if errors.Is(err, wasmsqlite.ErrDuplicateOpen) {
		return true
	}

	message := strings.ToLower(err.Error())

	return strings.Contains(message, "sqlite_cantopen") ||
		strings.Contains(message, "unable to open database file") ||
		strings.Contains(message, "database is locked")
}

// wasmSQLiteFileName maps a native data path to a stable origin-local OPFS
// filename. Hashing the full path prevents network/profile databases with the
// same basename from colliding within one browser origin.
func wasmSQLiteFileName(name string) string {
	normalized := filepath.ToSlash(filepath.Clean(name))
	base := filepath.Base(normalized)
	if base == "." || base == "/" || base == "" {
		base = "loop.db"
		normalized = base
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(normalized))

	extension := filepath.Ext(base)
	stem := strings.TrimSuffix(base, extension)

	return fmt.Sprintf("/%s-%016x%s", stem, hasher.Sum64(), extension)
}

// newSqliteMigrationDriver constructs the official WASM SQLite migration
// driver around the already-open OPFS database.
func newSqliteMigrationDriver(db *sql.DB) (database.Driver, error) {
	return wasmsqlite.NewMigrateDriver(db)
}
