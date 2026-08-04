//go:build !js || !wasm

package loopdb

import (
	"database/sql"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4/database"
	sqlite_migrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "modernc.org/sqlite"
)

const sqliteOptionPrefix = "_pragma"

// openSqliteDatabase opens the native modernc SQLite database.
func openSqliteDatabase(cfg *SqliteConfig,
	pragmas []string) (*sql.DB, error) {

	options := make(url.Values)
	for _, pragma := range pragmas {
		options.Add(sqliteOptionPrefix, pragma)
	}

	dsn := fmt.Sprintf("%v?%v", cfg.DatabaseFileName, options.Encode())

	return sql.Open("sqlite", dsn)
}

// newSqliteMigrationDriver constructs the native SQLite migration driver.
func newSqliteMigrationDriver(db *sql.DB) (database.Driver, error) {
	return sqlite_migrate.WithInstance(db, &sqlite_migrate.Config{})
}
