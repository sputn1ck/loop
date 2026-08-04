//go:build !js || !wasm

package browserwallet

import (
	"database/sql"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestSQLKeyIndexStoreBindWallet verifies that persistent wallet metadata
// prevents a profile from resuming with a different seed or network.
func TestSQLKeyIndexStoreBindWallet(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	store, err := NewSQLKeyIndexStore(t.Context(), db)
	require.NoError(t, err)

	seed := make([]byte, 32)
	seed[0] = 1
	require.NoError(t, store.BindWallet(
		t.Context(), seed, &chaincfg.RegressionNetParams,
	))
	require.NoError(t, store.BindWallet(
		t.Context(), seed, &chaincfg.RegressionNetParams,
	))

	otherSeed := append([]byte(nil), seed...)
	otherSeed[0]++
	err = store.BindWallet(
		t.Context(), otherSeed, &chaincfg.RegressionNetParams,
	)
	require.ErrorContains(t, err, "different seed")

	err = store.BindWallet(t.Context(), seed, &chaincfg.MainNetParams)
	require.ErrorContains(t, err, "different Bitcoin network")
}
