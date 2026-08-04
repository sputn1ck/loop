package browserwallet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnrpc/signrpc"
)

// Signer implements the subset of lndclient.SignerClient used by Loop Out.
type Signer struct {
	lndclient.SignerClient

	keys     *KeyRing
	sessions *input.MusigSessionManager
}

// NewSigner creates an in-process signer backed by the deterministic key ring.
func NewSigner(keys *KeyRing) (*Signer, error) {
	if keys == nil {
		return nil, errors.New("key ring is required")
	}

	signer := &Signer{keys: keys}
	signer.sessions = input.NewMusigSessionManager(
		func(desc *keychain.KeyDescriptor) (*btcec.PrivateKey, error) {
			return keys.PrivateKey(context.Background(), desc)
		},
	)

	return signer, nil
}

// RawClientWithMacAuth satisfies lndclient's service wrapper. There is no raw
// RPC client because signing runs in process.
func (s *Signer) RawClientWithMacAuth(ctx context.Context) (
	context.Context, time.Duration, signrpc.SignerClient) {

	return ctx, 0, nil
}

// SignOutputRaw signs each requested input and returns signatures in the same
// format as LND's signer RPC: DER for ECDSA and 64-byte BIP340 for Schnorr.
func (s *Signer) SignOutputRaw(ctx context.Context, tx *wire.MsgTx,
	descriptors []*lndclient.SignDescriptor,
	prevOutputs []*wire.TxOut) ([][]byte, error) {

	return s.signOutputRaw(ctx, tx, descriptors, prevOutputs)
}

// SignOutputRawKeyLocator is equivalent for the in-process signer because the
// full key descriptor is already available to it.
func (s *Signer) SignOutputRawKeyLocator(ctx context.Context,
	tx *wire.MsgTx, descriptors []*lndclient.SignDescriptor,
	prevOutputs []*wire.TxOut) ([][]byte, error) {

	return s.signOutputRaw(ctx, tx, descriptors, prevOutputs)
}

func (s *Signer) signOutputRaw(ctx context.Context, tx *wire.MsgTx,
	descriptors []*lndclient.SignDescriptor,
	prevOutputs []*wire.TxOut) ([][]byte, error) {

	if len(prevOutputs) != len(tx.TxIn) {
		return nil, fmt.Errorf("got %d previous outputs for %d inputs",
			len(prevOutputs), len(tx.TxIn))
	}

	prevFetcher := txscript.NewMultiPrevOutFetcher(nil)
	for index, txIn := range tx.TxIn {
		prevFetcher.AddPrevOut(
			txIn.PreviousOutPoint, prevOutputs[index],
		)
	}
	sigHashes := txscript.NewTxSigHashes(tx, prevFetcher)

	signatures := make([][]byte, 0, len(descriptors))
	for _, desc := range descriptors {
		if desc == nil || desc.Output == nil {
			return nil, errors.New("complete sign descriptor is required")
		}
		if desc.InputIndex < 0 || desc.InputIndex >= len(tx.TxIn) {
			return nil, fmt.Errorf("input index %d is out of range",
				desc.InputIndex)
		}

		privKey, err := s.keys.PrivateKey(ctx, &desc.KeyDesc)
		if err != nil {
			return nil, err
		}
		privKey = tweakPrivateKey(privKey, desc)

		sig, err := signDescriptor(
			tx, desc, privKey, sigHashes, prevFetcher,
		)
		if err != nil {
			return nil, err
		}

		signatures = append(signatures, sig)
	}

	return signatures, nil
}

func tweakPrivateKey(privKey *btcec.PrivateKey,
	desc *lndclient.SignDescriptor) *btcec.PrivateKey {

	switch {
	case desc.SingleTweak != nil:
		return input.TweakPrivKey(privKey, desc.SingleTweak)

	case desc.DoubleTweak != nil:
		return input.DeriveRevocationPrivKey(privKey, desc.DoubleTweak)

	default:
		return privKey
	}
}

func signDescriptor(tx *wire.MsgTx, desc *lndclient.SignDescriptor,
	privKey *btcec.PrivateKey, sigHashes *txscript.TxSigHashes,
	prevFetcher txscript.PrevOutputFetcher) ([]byte, error) {

	if txscript.IsPayToTaproot(desc.Output.PkScript) {
		var (
			rawSig []byte
			err    error
		)

		switch desc.SignMethod {
		case input.TaprootKeySpendBIP0086SignMethod,
			input.TaprootKeySpendSignMethod:

			rawSig, err = txscript.RawTxInTaprootSignature(
				tx, sigHashes, desc.InputIndex, desc.Output.Value,
				desc.Output.PkScript, desc.TapTweak, desc.HashType,
				privKey,
			)

		case input.TaprootScriptSpendSignMethod:
			leaf := txscript.NewBaseTapLeaf(desc.WitnessScript)
			rawSig, err = txscript.RawTxInTapscriptSignature(
				tx, sigHashes, desc.InputIndex, desc.Output.Value,
				desc.Output.PkScript, leaf, desc.HashType, privKey,
			)

		default:
			return nil, fmt.Errorf("unsupported taproot sign method %v",
				desc.SignMethod)
		}
		if err != nil {
			return nil, err
		}

		if len(rawSig) < schnorr.SignatureSize {
			return nil, errors.New("short Schnorr signature")
		}
		sig, err := schnorr.ParseSignature(rawSig[:schnorr.SignatureSize])
		if err != nil {
			return nil, err
		}

		return sig.Serialize(), nil
	}

	// The previous-output fetcher is required to construct the shared
	// sighash cache for mixed SegWit transactions. Retain the argument here
	// so this helper cannot accidentally be called with an incomplete view.
	if prevFetcher == nil {
		return nil, errors.New("previous-output fetcher is required")
	}

	rawSig, err := txscript.RawTxInWitnessSignature(
		tx, sigHashes, desc.InputIndex, desc.Output.Value,
		desc.WitnessScript, desc.HashType, privKey,
	)
	if err != nil {
		return nil, err
	}
	if len(rawSig) < 2 {
		return nil, errors.New("short ECDSA signature")
	}

	sig, err := ecdsa.ParseDERSignature(rawSig[:len(rawSig)-1])
	if err != nil {
		return nil, err
	}

	return sig.Serialize(), nil
}

// MuSig2CreateSession starts a stateful two-party signing session.
func (s *Signer) MuSig2CreateSession(_ context.Context,
	version input.MuSig2Version, signerLoc *keychain.KeyLocator,
	signerKeys [][]byte, opts ...lndclient.MuSig2SessionOpts) (
	*input.MuSig2SessionInfo, error) {

	if signerLoc == nil {
		return nil, errors.New("signer key locator is required")
	}

	keys, err := input.MuSig2ParsePubKeys(version, signerKeys)
	if err != nil {
		return nil, err
	}

	req := &signrpc.MuSig2SessionRequest{}
	for _, option := range opts {
		option(req)
	}
	if len(req.PregeneratedLocalNonce) != 0 {
		return nil, errors.New("pre-generated MuSig2 nonce is unsupported")
	}

	nonces := make([][musig2.PubNonceSize]byte, 0,
		len(req.OtherSignerPublicNonces))
	for _, nonceBytes := range req.OtherSignerPublicNonces {
		if len(nonceBytes) != musig2.PubNonceSize {
			return nil, fmt.Errorf("invalid MuSig2 nonce length %d",
				len(nonceBytes))
		}
		var nonce [musig2.PubNonceSize]byte
		copy(nonce[:], nonceBytes)
		nonces = append(nonces, nonce)
	}

	tweaks := &input.MuSig2Tweaks{}
	if req.TaprootTweak != nil {
		if req.TaprootTweak.KeySpendOnly {
			tweaks.TaprootBIP0086Tweak = true
		} else {
			tweaks.TaprootTweak = req.TaprootTweak.ScriptRoot
		}
	}

	return s.sessions.MuSig2CreateSession(
		version, *signerLoc, keys, tweaks, nonces, nil,
	)
}

// MuSig2RegisterNonces registers a peer's public nonce.
func (s *Signer) MuSig2RegisterNonces(_ context.Context,
	sessionID [32]byte, nonces [][musig2.PubNonceSize]byte) (bool, error) {

	return s.sessions.MuSig2RegisterNonces(sessionID, nonces)
}

// MuSig2Sign creates the local partial signature.
func (s *Signer) MuSig2Sign(_ context.Context, sessionID [32]byte,
	message [32]byte, cleanup bool) ([]byte, error) {

	partialSig, err := s.sessions.MuSig2Sign(
		sessionID, message, cleanup,
	)
	if err != nil {
		return nil, err
	}

	var encoded bytes.Buffer
	if err := partialSig.Encode(&encoded); err != nil {
		return nil, err
	}

	return encoded.Bytes(), nil
}

// MuSig2CombineSig combines peer partial signatures with the local one.
func (s *Signer) MuSig2CombineSig(_ context.Context, sessionID [32]byte,
	otherPartialSigs [][]byte) (bool, []byte, error) {

	partials := make([]*musig2.PartialSignature, 0, len(otherPartialSigs))
	for _, encoded := range otherPartialSigs {
		partial := &musig2.PartialSignature{}
		if err := partial.Decode(bytes.NewReader(encoded)); err != nil {
			return false, nil, err
		}
		partials = append(partials, partial)
	}

	finalSig, haveAll, err := s.sessions.MuSig2CombineSig(
		sessionID, partials,
	)
	if err != nil || !haveAll {
		return haveAll, nil, err
	}

	return true, finalSig.Serialize(), nil
}

// MuSig2Cleanup removes a completed or abandoned session.
func (s *Signer) MuSig2Cleanup(_ context.Context,
	sessionID [32]byte) error {

	return s.sessions.MuSig2Cleanup(sessionID)
}

// MuSig2RegisterCombinedNonce registers a coordinator-aggregated nonce.
func (s *Signer) MuSig2RegisterCombinedNonce(_ context.Context,
	sessionID [32]byte, combinedNonce [musig2.PubNonceSize]byte) error {

	return s.sessions.MuSig2RegisterCombinedNonce(
		sessionID, combinedNonce,
	)
}

// MuSig2GetCombinedNonce returns the current coordinator-aggregated nonce.
func (s *Signer) MuSig2GetCombinedNonce(_ context.Context,
	sessionID [32]byte) ([musig2.PubNonceSize]byte, error) {

	return s.sessions.MuSig2GetCombinedNonce(sessionID)
}
