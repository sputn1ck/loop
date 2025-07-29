package swapdk

import (
	"context"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnrpc/signrpc"
)

// SwapDKSigner is an interface that abstracts the signing operations required
// by the loop daemon. This will allow us to use either a local lnd signer or a
// remote signer provided by a SwapDK client.
type SwapDKSigner interface {
	// SignOutputRaw generates a signature for a given transaction output.
	SignOutputRaw(ctx context.Context, tx *wire.MsgTx,
		signDescriptors []*lndclient.SignDescriptor,
		prevOutputs []*wire.TxOut) ([][]byte, error)

	// SignMessage signs a message with a given key locator.
	SignMessage(ctx context.Context, msg []byte,
		locator keychain.KeyLocator,
		opts ...lndclient.SignMessageOption) ([]byte, error)

	// DeriveSharedKey returns a shared secret key by performing Diffie-Hellman
	// key derivation between a remote public key and a key specified by the
	// key locator.
	DeriveSharedKey(ctx context.Context,
		ephemeralPubKey *btcec.PublicKey,
		keyLocator *keychain.KeyLocator) ([32]byte, error)

	// MuSig2CreateSession creates a new MuSig2 signing session.
	MuSig2CreateSession(ctx context.Context, version input.MuSig2Version,
		keyLocator *keychain.KeyLocator, signers [][]byte,
		opts ...lndclient.MuSig2SessionOpts) (*input.MuSig2SessionInfo, error)

	// MuSig2RegisterNonces registers the nonces of all other signing
	// participants for a MuSig2 session.
	MuSig2RegisterNonces(ctx context.Context, sessionID [32]byte,
		nonces [][66]byte) (bool, error)

	// MuSig2Sign creates a partial signature for a MuSig2 session.
	MuSig2Sign(ctx context.Context, sessionID [32]byte,
		message [32]byte, cleanup bool) ([]byte, error)

	// MuSig2CombineSig combines the partial signatures of all signing
	// participants to create the final signature.
	MuSig2CombineSig(ctx context.Context, sessionID [32]byte,
		otherPartialSigs [][]byte) (bool, []byte, error)

	// MuSig2Cleanup cleans up a MuSig2 signing session.
	MuSig2Cleanup(ctx context.Context, sessionID [32]byte) error

	// ComputeInputScript generates a witness script for the specified UTXO.
	ComputeInputScript(ctx context.Context, tx *wire.MsgTx,
		signDescriptors []*lndclient.SignDescriptor,
		prevOutputs []*wire.TxOut) ([]*input.Script, error)

	// RawClientWithMacAuth returns the raw signrpc client with the macaroon
	// authenticated context.
	RawClientWithMacAuth(ctx context.Context) (context.Context,
		time.Duration, signrpc.SignerClient)

	// SignOutputRawKeyLocator generates a signature for a given transaction
	// output.
	SignOutputRawKeyLocator(ctx context.Context, tx *wire.MsgTx,
		signDescriptors []*lndclient.SignDescriptor,
		prevOutputs []*wire.TxOut) ([][]byte, error)

	// VerifyMessage verifies a signature for a given message.
	VerifyMessage(ctx context.Context, msg, sig []byte,
		pubkey [33]byte, opts ...lndclient.VerifyMessageOption) (bool, error)
}
