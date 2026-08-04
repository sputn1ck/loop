package wasm

import (
	"context"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRecoveryEntries(t *testing.T) {
	t.Parallel()

	params := &chaincfg.RegressionNetParams
	seed := make([]byte, 32)
	seed[0] = 7
	bundle, err := BuildRecoveryBundle(t.Context(), RecoveryBundleConfig{
		Network: RecoveryNetwork{
			Name:        params.Name,
			GenesisHash: params.GenesisHash.String(),
		},
		CreatedAt: time.Date(
			2026, time.August, 4, 12, 0, 0, 0, time.UTC,
		),
		Databases: []DatabaseSource{
			{
				Name:          LoopDatabaseRecoveryName,
				SchemaVersion: runtimeRecoverySchemaVersion,
				Encoding:      SQLiteSQLDumpEncoding,
				Source: DumpSourceFunc(
					func(_ context.Context) ([]byte, error) {
						return []byte("BEGIN; COMMIT;"), nil
					},
				),
			},
		},
		Artifacts: []ArtifactSource{
			{
				Name:          WalletSeedArtifactName,
				SchemaVersion: runtimeRecoverySchemaVersion,
				Encoding:      OpaqueArtifactEncoding,
				Source: DumpSourceFunc(
					func(_ context.Context) ([]byte, error) {
						return seed, nil
					},
				),
			},
		},
	})
	require.NoError(t, err)

	dump, recoveredSeed, err := recoveryEntries(
		bundle, params,
	)
	require.NoError(t, err)
	require.Equal(t, []byte("BEGIN; COMMIT;"), dump)
	require.Equal(t, seed, recoveredSeed)

	_, _, err = recoveryEntries(bundle, &chaincfg.MainNetParams)
	require.ErrorContains(t, err, "does not match")
}

func TestRestoreRecoveryRequiresFreshTarget(t *testing.T) {
	t.Parallel()

	_, err := RestoreRecoveryBundle(t.Context(), RestoreRecoveryConfig{
		DatabasePath: "loop.db",
		ChainParams:  &chaincfg.RegressionNetParams,
	})
	require.ErrorContains(t, err, "fresh target")
}

func TestEnsureUnusedRestoreTarget(t *testing.T) {
	t.Parallel()

	db := loopdb.NewTestSqliteDB(t)
	require.NoError(t, ensureUnusedRestoreTarget(t.Context(), db.DB))

	_, err := db.ExecContext(t.Context(), `
		CREATE TABLE restore_used_marker (value TEXT NOT NULL)
	`)
	require.NoError(t, err)
	require.NoError(t, ensureUnusedRestoreTarget(t.Context(), db.DB))

	_, err = db.ExecContext(t.Context(), `
		INSERT INTO restore_used_marker(value) VALUES ('used')
	`)
	require.NoError(t, err)
	err = ensureUnusedRestoreTarget(t.Context(), db.DB)
	require.ErrorContains(t, err, "already in use")
}
