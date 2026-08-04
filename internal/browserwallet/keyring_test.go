package browserwallet

import (
	"context"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/stretchr/testify/require"
)

// TestKeyRingRestore verifies that a restored seed and key-index store derive
// the pending swap key again without reusing its index.
func TestKeyRingRestore(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	seed := make([]byte, 32)
	for index := range seed {
		seed[index] = byte(index + 1)
	}

	indexes := NewMemoryKeyIndexStore()
	firstRing, err := NewKeyRing(seed, &chaincfg.RegressionNetParams, indexes)
	require.NoError(t, err)

	first, err := firstRing.DeriveNextKey(ctx, keychain.KeyFamily(99))
	require.NoError(t, err)
	require.Equal(t, uint32(0), first.Index)

	restoredRing, err := NewKeyRing(
		seed, &chaincfg.RegressionNetParams, indexes,
	)
	require.NoError(t, err)

	restored, err := restoredRing.DeriveKey(ctx, &first.KeyLocator)
	require.NoError(t, err)
	require.Equal(
		t, first.PubKey.SerializeCompressed(),
		restored.PubKey.SerializeCompressed(),
	)

	privateKey, err := restoredRing.PrivateKey(
		context.Background(), &keychain.KeyDescriptor{
			PubKey: first.PubKey,
		},
	)
	require.NoError(t, err)
	require.Equal(
		t, first.PubKey.SerializeCompressed(),
		privateKey.PubKey().SerializeCompressed(),
	)

	second, err := restoredRing.DeriveNextKey(ctx, keychain.KeyFamily(99))
	require.NoError(t, err)
	require.Equal(t, uint32(1), second.Index)
	require.NotEqual(
		t, first.PubKey.SerializeCompressed(),
		second.PubKey.SerializeCompressed(),
	)
}

// TestKeyRingPrivateKeyRejectsLocatorMismatch verifies that a caller cannot
// silently sign with a locator that derives a different public key.
func TestKeyRingPrivateKeyRejectsLocatorMismatch(t *testing.T) {
	t.Parallel()

	seed := make([]byte, 32)
	seed[0] = 1
	ring, err := NewKeyRing(
		seed, &chaincfg.RegressionNetParams, NewMemoryKeyIndexStore(),
	)
	require.NoError(t, err)

	first, err := ring.DeriveNextKey(t.Context(), keychain.KeyFamily(99))
	require.NoError(t, err)
	second, err := ring.DeriveNextKey(t.Context(), keychain.KeyFamily(99))
	require.NoError(t, err)

	_, err = ring.PrivateKey(t.Context(), &keychain.KeyDescriptor{
		KeyLocator: first.KeyLocator,
		PubKey:     second.PubKey,
	})
	require.ErrorContains(t, err, "does not match public key")
}
