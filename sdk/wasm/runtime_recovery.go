package wasm

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/loop/internal/browserwallet"
	"github.com/lightninglabs/loop/loopdb"
)

const (
	// LoopDatabaseRecoveryName identifies the runtime SQLite database in a
	// recovery bundle.
	LoopDatabaseRecoveryName = "loop_sqlite"

	runtimeRecoverySchemaVersion = 1
)

// RestoreRecoveryConfig configures a browser database restore. Restore loads
// into a fresh or otherwise unused database name and refuses to overwrite a
// profile containing application state.
type RestoreRecoveryConfig struct {
	DatabasePath string
	ChainParams  *chaincfg.Params
	Bundle       []byte
	FreshTarget  bool
}

// RestoredRuntimeState contains the secret startup material recovered outside
// the SQLite database.
type RestoredRuntimeState struct {
	Seed []byte
}

// ExportRecoveryBundle returns a plaintext, checksummed recovery bundle. It
// contains the wallet seed and may contain a reusable L402 credential. The
// caller MUST encrypt and authenticate it before it leaves trusted browser
// storage.
func (r *Runtime) ExportRecoveryBundle(ctx context.Context) ([]byte, error) {
	if r == nil || r.db == nil || r.network == nil {
		return nil, errors.New("runtime recovery is not initialized")
	}
	if ctx == nil {
		return nil, errors.New("recovery context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.recoveryMu.Lock()
	defer r.recoveryMu.Unlock()

	select {
	case <-r.done:
		return nil, errors.New("runtime is stopped")
	default:
	}

	databaseDump, err := dumpSQLiteDatabase(ctx, r.db)
	if err != nil {
		return nil, fmt.Errorf("dump runtime SQLite database: %w", err)
	}

	artifacts := []ArtifactSource{
		{
			Name:          WalletSeedArtifactName,
			SchemaVersion: runtimeRecoverySchemaVersion,
			Encoding:      OpaqueArtifactEncoding,
			Source: DumpSourceFunc(func(context.Context) ([]byte, error) {
				return append([]byte(nil), r.seed...), nil
			}),
		},
	}
	bundle, err := BuildRecoveryBundle(ctx, RecoveryBundleConfig{
		Network: RecoveryNetwork{
			Name:        r.network.Name,
			GenesisHash: r.network.GenesisHash.String(),
		},
		Databases: []DatabaseSource{
			{
				Name:          LoopDatabaseRecoveryName,
				SchemaVersion: runtimeRecoverySchemaVersion,
				Encoding:      SQLiteSQLDumpEncoding,
				Source: DumpSourceFunc(
					func(context.Context) ([]byte, error) {
						return databaseDump, nil
					},
				),
			},
		},
		Artifacts: artifacts,
	})
	if err != nil {
		return nil, err
	}

	return MarshalRecoveryBundle(bundle)
}

// RestoreRecoveryBundle loads a browser SQL dump into an unused database and
// returns the recovered wallet seed needed by Start. L402 state is part of the
// SQLite dump and remains bound to its original gateway.
func RestoreRecoveryBundle(ctx context.Context,
	config RestoreRecoveryConfig) (*RestoredRuntimeState, error) {

	if ctx == nil {
		return nil, errors.New("restore context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.DatabasePath) == "" {
		return nil, errors.New("restore database path is required")
	}
	if config.ChainParams == nil {
		return nil, errors.New("restore chain parameters are required")
	}
	if !config.FreshTarget {
		return nil, errors.New(
			"restore requires explicit confirmation of a fresh target",
		)
	}

	bundle, err := ParseRecoveryBundle(config.Bundle)
	if err != nil {
		return nil, err
	}
	databaseDump, seed, err := recoveryEntries(
		bundle, config.ChainParams,
	)
	if err != nil {
		return nil, err
	}

	store, err := loopdb.NewSqliteStore(&loopdb.SqliteConfig{
		DatabaseFileName: config.DatabasePath,
	}, config.ChainParams)
	if err != nil {
		return nil, fmt.Errorf("open restore database: %w", err)
	}
	if err := ensureUnusedRestoreTarget(ctx, store.DB); err != nil {
		_ = store.Close()

		return nil, err
	}

	if err := loadSQLiteDatabase(ctx, store.DB, databaseDump); err != nil {
		_ = store.Close()

		return nil, fmt.Errorf("restore runtime SQLite database: %w", err)
	}
	if err := store.Close(); err != nil {
		return nil, fmt.Errorf("close loaded restore database: %w", err)
	}

	// The dump can come from an earlier runtime schema. Reopening the store
	// applies every migration that was added after the backup was created.
	store, err = loopdb.NewSqliteStore(&loopdb.SqliteConfig{
		DatabaseFileName: config.DatabasePath,
	}, config.ChainParams)
	if err != nil {
		return nil, fmt.Errorf("migrate restored database: %w", err)
	}
	defer func() {
		_ = store.Close()
	}()

	if err := validatePersistedSwaps(ctx, store); err != nil {
		return nil, fmt.Errorf("validate restored swaps: %w", err)
	}
	keyIndexes, err := browserwallet.NewSQLKeyIndexStore(ctx, store.DB)
	if err != nil {
		return nil, fmt.Errorf("open restored wallet key indexes: %w", err)
	}
	if err := validatePersistedSwapKeys(
		ctx, store, seed, config.ChainParams, keyIndexes,
	); err != nil {

		return nil, err
	}
	if err := keyIndexes.BindWallet(
		ctx, seed, config.ChainParams,
	); err != nil {

		return nil, fmt.Errorf("bind restored wallet identity: %w", err)
	}

	return &RestoredRuntimeState{
		Seed: append([]byte(nil), seed...),
	}, nil
}

func recoveryEntries(bundle *RecoveryBundle,
	params *chaincfg.Params) ([]byte, []byte, error) {

	if !strings.EqualFold(
		bundle.Network.GenesisHash, params.GenesisHash.String(),
	) {

		return nil, nil, fmt.Errorf(
			"recovery network %q does not match %q",
			bundle.Network.Name, params.Name,
		)
	}

	var databaseDump []byte
	for _, database := range bundle.Databases {
		if database.Name != LoopDatabaseRecoveryName {
			continue
		}
		if database.Encoding != SQLiteSQLDumpEncoding {
			return nil, nil, fmt.Errorf(
				"unsupported runtime database encoding %q",
				database.Encoding,
			)
		}
		if database.SchemaVersion != runtimeRecoverySchemaVersion {
			return nil, nil, fmt.Errorf(
				"unsupported runtime database schema version %d",
				database.SchemaVersion,
			)
		}
		databaseDump = append([]byte(nil), database.Data...)
	}
	if len(databaseDump) == 0 {
		return nil, nil, errors.New(
			"recovery bundle has no Loop SQLite database",
		)
	}

	var seed []byte
	for _, artifact := range bundle.Artifacts {
		switch artifact.Name {
		case WalletSeedArtifactName:
			if artifact.Encoding != OpaqueArtifactEncoding {
				return nil, nil, fmt.Errorf(
					"unsupported wallet seed encoding %q",
					artifact.Encoding,
				)
			}
			seed = append([]byte(nil), artifact.Data...)
		}
	}
	if len(seed) < 16 || len(seed) > 64 {
		return nil, nil, fmt.Errorf(
			"wallet seed must contain between 16 and 64 bytes",
		)
	}
	return databaseDump, seed, nil
}

func ensureUnusedRestoreTarget(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table'
			AND name NOT LIKE 'sqlite_%'
			AND name != 'schema_migrations'
	`)
	if err != nil {
		return fmt.Errorf("inspect restore target schema: %w", err)
	}

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			_ = rows.Close()

			return fmt.Errorf("inspect restore target table: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close restore target inspection: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect restore target tables: %w", err)
	}

	for _, table := range tables {
		quotedTable := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		var populated bool
		err := db.QueryRowContext(
			ctx, "SELECT EXISTS(SELECT 1 FROM "+quotedTable+" LIMIT 1)",
		).Scan(&populated)
		if err != nil {
			return fmt.Errorf(
				"inspect restore target table %q: %w", table, err,
			)
		}
		if populated {
			return fmt.Errorf(
				"restore target is already in use (table %q has data)",
				table,
			)
		}
	}

	return nil
}

func validatePersistedSwapKeys(ctx context.Context,
	store *loopdb.SqliteSwapStore, seed []byte, params *chaincfg.Params,
	indexes browserwallet.KeyIndexStore) error {

	keyRing, err := browserwallet.NewKeyRing(seed, params, indexes)
	if err != nil {
		return fmt.Errorf("open browser wallet key ring: %w", err)
	}
	loopOuts, err := store.FetchLoopOutSwaps(ctx)
	if err != nil {
		return fmt.Errorf("inspect persisted Loop Out keys: %w", err)
	}
	for _, loopOut := range loopOuts {
		if !loopOut.State().State.IsPending() {
			continue
		}

		key, err := keyRing.DeriveKey(
			ctx, &loopOut.Contract.HtlcKeys.ClientScriptKeyLocator,
		)
		if err != nil {
			return fmt.Errorf(
				"derive pending Loop Out %x receiver key: %w",
				loopOut.Hash[:], err,
			)
		}
		if !bytes.Equal(
			key.PubKey.SerializeCompressed(),
			loopOut.Contract.HtlcKeys.ReceiverScriptKey[:],
		) {

			return fmt.Errorf(
				"wallet seed does not match pending Loop Out %x",
				loopOut.Hash[:],
			)
		}
	}

	return nil
}
