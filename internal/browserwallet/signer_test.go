package browserwallet

import (
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/stretchr/testify/require"
)

// TestSignOutputRaw verifies the non-cooperative script-path signature that
// remains available if the server cannot complete a cooperative sweep.
func TestSignOutputRaw(t *testing.T) {
	t.Parallel()

	seed := make([]byte, 32)
	for index := range seed {
		seed[index] = byte(index + 1)
	}

	ring, err := NewKeyRing(
		seed, &chaincfg.RegressionNetParams, NewMemoryKeyIndexStore(),
	)
	require.NoError(t, err)
	keyDesc, err := ring.DeriveNextKey(
		t.Context(), keychain.KeyFamily(99),
	)
	require.NoError(t, err)
	signer, err := NewSigner(ring)
	require.NoError(t, err)

	witnessScript, err := txscript.NewScriptBuilder().
		AddData(keyDesc.PubKey.SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).Script()
	require.NoError(t, err)
	scriptHash := sha256.Sum256(witnessScript)
	pkScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).AddData(scriptHash[:]).Script()
	require.NoError(t, err)

	previousOutput := &wire.TxOut{
		Value:    50_000,
		PkScript: pkScript,
	}
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash: chainhash.Hash{1},
		},
	})
	tx.AddTxOut(&wire.TxOut{Value: 49_000, PkScript: []byte{txscript.OP_TRUE}})

	rawSigs, err := signer.SignOutputRaw(
		t.Context(), tx, []*lndclient.SignDescriptor{{
			KeyDesc:       *keyDesc,
			WitnessScript: witnessScript,
			Output:        previousOutput,
			HashType:      txscript.SigHashAll,
			InputIndex:    0,
		}}, []*wire.TxOut{previousOutput},
	)
	require.NoError(t, err)
	require.Len(t, rawSigs, 1)

	parsedSig, err := ecdsa.ParseDERSignature(rawSigs[0])
	require.NoError(t, err)
	prevFetcher := txscript.NewCannedPrevOutputFetcher(
		previousOutput.PkScript, previousOutput.Value,
	)
	sigHashes := txscript.NewTxSigHashes(tx, prevFetcher)
	digest, err := txscript.CalcWitnessSigHash(
		witnessScript, sigHashes, txscript.SigHashAll, tx, 0,
		previousOutput.Value,
	)
	require.NoError(t, err)
	require.True(t, parsedSig.Verify(digest, keyDesc.PubKey))
}

// TestMuSig2Signer verifies the cooperative Taproot signing operations used by
// Loop's sweep batcher without an external signer daemon.
func TestMuSig2Signer(t *testing.T) {
	t.Parallel()

	newParticipant := func(seedByte byte) (*Signer,
		*keychain.KeyDescriptor) {

		seed := make([]byte, 32)
		for index := range seed {
			seed[index] = seedByte + byte(index)
		}

		ring, err := NewKeyRing(
			seed, &chaincfg.RegressionNetParams,
			NewMemoryKeyIndexStore(),
		)
		require.NoError(t, err)

		desc, err := ring.DeriveNextKey(
			t.Context(), keychain.KeyFamily(99),
		)
		require.NoError(t, err)

		signer, err := NewSigner(ring)
		require.NoError(t, err)

		return signer, desc
	}

	alice, aliceKey := newParticipant(1)
	bob, bobKey := newParticipant(101)
	signerKeys := [][]byte{
		aliceKey.PubKey.SerializeCompressed(),
		bobKey.PubKey.SerializeCompressed(),
	}

	aliceSession, err := alice.MuSig2CreateSession(
		t.Context(), input.MuSig2Version100RC2,
		&aliceKey.KeyLocator, signerKeys,
	)
	require.NoError(t, err)
	bobSession, err := bob.MuSig2CreateSession(
		t.Context(), input.MuSig2Version100RC2,
		&bobKey.KeyLocator, signerKeys,
	)
	require.NoError(t, err)

	haveAll, err := alice.MuSig2RegisterNonces(
		t.Context(), aliceSession.SessionID,
		[][66]byte{bobSession.PublicNonce},
	)
	require.NoError(t, err)
	require.True(t, haveAll)
	haveAll, err = bob.MuSig2RegisterNonces(
		t.Context(), bobSession.SessionID,
		[][66]byte{aliceSession.PublicNonce},
	)
	require.NoError(t, err)
	require.True(t, haveAll)

	digest := sha256.Sum256([]byte("browser Loop sweep"))
	_, err = alice.MuSig2Sign(
		t.Context(), aliceSession.SessionID, digest, false,
	)
	require.NoError(t, err)
	bobPartial, err := bob.MuSig2Sign(
		t.Context(), bobSession.SessionID, digest, false,
	)
	require.NoError(t, err)

	haveAll, finalSigBytes, err := alice.MuSig2CombineSig(
		t.Context(), aliceSession.SessionID, [][]byte{bobPartial},
	)
	require.NoError(t, err)
	require.True(t, haveAll)

	finalSig, err := schnorr.ParseSignature(finalSigBytes)
	require.NoError(t, err)
	require.True(t, finalSig.Verify(digest[:], aliceSession.CombinedKey))
}
