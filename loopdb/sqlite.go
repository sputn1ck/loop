package loopdb

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/snacl"
	sqlite_migrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/jackc/pgx/v4"
	"github.com/lightninglabs/loop/loopdb/sqlc"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // Register relevant drivers.
)

const (
	// sqliteOptionPrefix is the string prefix sqlite uses to set various
	// options. This is used in the following format:
	//   * sqliteOptionPrefix || option_name = option_value.
	sqliteOptionPrefix = "_pragma"
)

// SqliteConfig holds all the config arguments needed to interact with our
// sqlite DB.
type SqliteConfig struct {
	// SkipMigrations if true, then all the tables will be created on start
	// up if they don't already exist.
	SkipMigrations bool `long:"skipmigrations" description:"Skip applying migrations on startup."`

	// DatabaseFileName is the full file path where the database file can be
	// found.
	DatabaseFileName string `long:"dbfile" description:"The full path to the database."`
}

// SqliteSwapStore is a sqlite3 based database for the loop daemon.
type SqliteSwapStore struct {
	cfg *SqliteConfig

	*BaseDB
}

// NewSqliteStore attempts to open a new sqlite database based on the passed
// config.
func NewSqliteStore(cfg *SqliteConfig, network *chaincfg.Params,
	opts ...BaseDBOptionsOptionFunc) (*SqliteSwapStore, error) {
	// The set of pragma options are accepted using query options. For now
	// we only want to ensure that foreign key constraints are properly
	// enforced.
	pragmaOptions := []struct {
		name  string
		value string
	}{
		{
			name:  "foreign_keys",
			value: "on",
		},
		{
			name:  "journal_mode",
			value: "WAL",
		},
		{
			name:  "busy_timeout",
			value: "5000",
		},
	}
	sqliteOptions := make(url.Values)
	for _, option := range pragmaOptions {
		sqliteOptions.Add(
			sqliteOptionPrefix,
			fmt.Sprintf("%v=%v", option.name, option.value),
		)
	}

	// Construct the DSN which is just the database file name, appended
	// with the series of pragma options as a query URL string. For more
	// details on the formatting here, see the modernc.org/sqlite docs:
	// https://pkg.go.dev/modernc.org/sqlite#Driver.Open.
	dsn := fmt.Sprintf(
		"%v?%v", cfg.DatabaseFileName, sqliteOptions.Encode(),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if !cfg.SkipMigrations {
		// Now that the database is open, populate the database with
		// our set of schemas based on our embedded in-memory file
		// system.
		//
		// First, we'll need to open up a new migration instance for
		// our current target database: sqlite.
		driver, err := sqlite_migrate.WithInstance(
			db, &sqlite_migrate.Config{},
		)
		if err != nil {
			return nil, err
		}

		err = applyMigrations(
			sqlSchemas, driver, "sqlc/migrations", "sqlc",
		)
		if err != nil {
			return nil, err
		}
	}

	return &SqliteSwapStore{
		cfg:    cfg,
		BaseDB: NewBaseDB(db, network, opts...),
	}, nil
}

// NewTestSqliteDB is a helper function that creates an SQLite database for
// testing.
func NewTestSqliteDB(t *testing.T) *SqliteSwapStore {

	t.Helper()

	t.Logf("Creating new SQLite DB for testing")

	dbFileName := filepath.Join(t.TempDir(), "tmp.db")
	sqlDB, err := NewSqliteStore(&SqliteConfig{
		DatabaseFileName: dbFileName,
		SkipMigrations:   false,
	}, &chaincfg.MainNetParams, WithEncryptionMiddleware("loop"),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, sqlDB.DB.Close())
	})

	err = sqlDB.Init(context.Background())
	require.NoError(t, err)

	return sqlDB
}

// BaseDB is the base database struct that each implementation can embed to
// gain some common functionality.
type BaseDB struct {
	network *chaincfg.Params

	options *BaseDBOptions

	*sql.DB

	*sqlc.Queries
}

func NewBaseDB(db *sql.DB, network *chaincfg.Params,
	opts ...BaseDBOptionsOptionFunc) *BaseDB {

	options := &BaseDBOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return &BaseDB{
		DB:      db,
		Queries: sqlc.New(db),
		network: network,
		options: options,
	}
}

type BaseDBOptions struct {
	// password is the password used to encrypt sensitive data stored in the
	// database. The password alone is not enough to decrypt the data, the
	// salt which is stored in the database on first run is also needed.
	password []byte

	// secret is used to encypt and decrypt sensitive data stored in the
	// database. It is derived from the password and the stored salt.
	secret *snacl.SecretKey
}

// PostgresStoreOptionFunc is a functional option that can be used to
// configure the passed PostgresStoreOptions.
type BaseDBOptionsOptionFunc func(*BaseDBOptions)

// WithSecretKey is a PostgresStoreMiddlewareOption that can be used to set the
// secret key used to encrypt sensitive data stored in the database.
func WithEncryptionMiddleware(password string) BaseDBOptionsOptionFunc {
	return func(m *BaseDBOptions) {
		// Copy of the password.
		m.password = []byte(password)
	}
}

// Init initializes the necessary versioning state if the database hasn't
// already been created in the past.
func (p *BaseDB) Init(ctx context.Context) error {
	return p.initSecretKey(ctx)
}

func (p *BaseDB) initSecretKey(ctx context.Context) error {
	// Make sure that we don't store the password in memory for too long.
	defer func() {
		p.options.password = nil
	}()

	// Once migrations are done we're ready to initialize the secret key
	// middleware.
	return p.ExecTx(ctx, &SqliteTxOptions{}, func(tx *sqlc.Queries) error {
		params, err := tx.GetSecretKeyParams(ctx)
		if err == pgx.ErrNoRows || err == sql.ErrNoRows {
			// If there's no secret key params stored in the db,
			// then we first create a new secret key with our
			// password and save the marshaled parameters.
			secret, err := snacl.NewSecretKey(
				&p.options.password,
				snacl.DefaultN, snacl.DefaultR, snacl.DefaultP,
			)
			if err != nil {
				return err
			}

			err = tx.InsertSecretKeyParams(ctx, secret.Marshal())
			if err != nil {
				return err
			}

			p.options.secret = secret

			return nil
		}
		if err != nil {
			return err
		}

		// If there's already a marshaled secret key parameter stored
		// in the db, then we will unmarshal it and use it to create
		// a new secret key instance.
		var secret snacl.SecretKey
		err = secret.Unmarshal(params)
		if err != nil {
			return err
		}
		err = secret.DeriveKey(&p.options.password)
		if err != nil {
			return err
		}

		p.options.secret = &secret

		return nil
	})
}

// BeginTx wraps the normal sql specific BeginTx method with the TxOptions
// interface. This interface is then mapped to the concrete sql tx options
// struct.
func (db *BaseDB) BeginTx(ctx context.Context, opts TxOptions) (*sql.Tx, error) {
	sqlOptions := sql.TxOptions{
		ReadOnly: opts.ReadOnly(),
	}
	return db.DB.BeginTx(ctx, &sqlOptions)
}

// ExecTx is a wrapper for txBody to abstract the creation and commit of a db
// transaction. The db transaction is embedded in a `*postgres.Queries` that
// txBody needs to use when executing each one of the queries that need to be
// applied atomically.
func (db *BaseDB) ExecTx(ctx context.Context, txOptions TxOptions,
	txBody func(*sqlc.Queries) error) error {

	// Create the db transaction.
	tx, err := db.BeginTx(ctx, txOptions)
	if err != nil {
		return err
	}

	// Rollback is safe to call even if the tx is already closed, so if
	// the tx commits successfully, this is a no-op.
	defer tx.Rollback() //nolint: errcheck

	if err := txBody(db.Queries.WithTx(tx)); err != nil {
		return err
	}

	// Commit transaction.
	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}

// encrypt will encrypt the passed slice with the store's SecretKey.
func (p *BaseDB) encrypt(data []byte) ([]byte, error) {
	// Make sure that we have a secret key ready for encryption.
	if p.options.secret == nil {
		return nil, fmt.Errorf("invalid SecretKey")
	}

	return p.options.secret.Encrypt(data)
}

// decryptOrNil will decrypt the passed slice with the store's SecretKey.
// Returns the decrypted slice unless the store's SecretKey is unset.
func (p *BaseDB) decryptOrNil(data []byte) ([]byte, error) {
	// If the secret key is not set, we'll simply return nil. This way we
	// can make decryption optional.
	if p.options.secret == nil {
		return nil, nil
	}

	return p.options.secret.Decrypt(data)
}

// TxOptions represents a set of options one can use to control what type of
// database transaction is created. Transaction can wither be read or write.
type TxOptions interface {
	// ReadOnly returns true if the transaction should be read only.
	ReadOnly() bool
}

// SqliteTxOptions defines the set of db txn options the KeyStore
// understands.
type SqliteTxOptions struct {
	// readOnly governs if a read only transaction is needed or not.
	readOnly bool
}

// NewKeyStoreReadOpts returns a new KeyStoreTxOptions instance triggers a read
// transaction.
func NewSqlReadOpts() *SqliteTxOptions {
	return &SqliteTxOptions{
		readOnly: true,
	}
}

// ReadOnly returns true if the transaction should be read only.
//
// NOTE: This implements the TxOptions
func (r *SqliteTxOptions) ReadOnly() bool {
	return r.readOnly
}
