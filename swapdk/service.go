package swapdk

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnrpc/signrpc"
)

// MuSig2CreateSessionRequest is the request for a MuSig2 session.
type MuSig2CreateSessionRequest struct {
	Version    input.MuSig2Version
	KeyLocator *keychain.KeyLocator
	Signers    [][]byte
}

// MuSig2RegisterNoncesRequest is the request to register nonces for a MuSig2
// session.
type MuSig2RegisterNoncesRequest struct {
	SessionID [32]byte
	Nonces    [][66]byte
}

// MuSig2SignRequest is the request to sign a message with a MuSig2 session.
type MuSig2SignRequest struct {
	SessionID [32]byte
	Message   [32]byte
	Cleanup   bool
}

// MuSig2CombineSigRequest is the request to combine partial signatures for a
// MuSig2 session.
type MuSig2CombineSigRequest struct {
	SessionID        [32]byte
	OtherPartialSigs [][]byte
}

// SignOutputRawRequest is the request to sign a raw output.
type SignOutputRawRequest struct {
	Tx              *wire.MsgTx
	SignDescriptors []*lndclient.SignDescriptor
	PrevOutputs     []*wire.TxOut
}

// SignMessageRequest is the request to sign a message.
type SignMessageRequest struct {
	Msg     []byte
	Locator keychain.KeyLocator
	Opts    []lndclient.SignMessageOption
}

// DeriveSharedKeyRequest is the request to derive a shared key.
type DeriveSharedKeyRequest struct {
	EphemeralPubKey *btcec.PublicKey
	KeyLocator      *keychain.KeyLocator
}

// MuSig2CleanupRequest is the request to clean up a MuSig2 session.
type MuSig2CleanupRequest struct {
	SessionID [32]byte
}

// VerifyMessageRequest is the request to verify a message.
type VerifyMessageRequest struct {
	Msg    []byte
	Sig    []byte
	Pubkey [33]byte
	Opts   []lndclient.VerifyMessageOption
}

// SignOutputRawKeyLocatorRequest is the request to sign a raw output with a key
// locator.
type SignOutputRawKeyLocatorRequest struct {
	Tx         *wire.MsgTx
	KeyLocator keychain.KeyLocator
	PrevOutput *wire.TxOut
}

// ComputeInputScriptRequest is the request to compute an input script.
type ComputeInputScriptRequest struct {
	Tx              *wire.MsgTx
	SignDescriptors []*lndclient.SignDescriptor
	PrevOutputs     []*wire.TxOut
}

// SigningRequest represents a request to sign a message or transaction.
type SigningRequest struct {
	Id       [32]byte
	Request  interface{}
	Response chan *SigningResponse
}

// SigningResponse represents the response to a signing request.
type SigningResponse struct {
	Response interface{}
	Err      error
}

// SwapDKService is an implementation of the SwapDKSigner interface that
// delegates signing to a remote client.
type SwapDKService struct {
	// mu is a mutex that protects the pending signing requests.
	mu sync.Mutex

	// pendingSignatures is a map of pending signing requests.
	pendingSignatures map[[32]byte]*SigningRequest

	manager SwapDKManager
}

// NewSwapDKService creates a new SwapDKService.
func NewSwapDKService(manager SwapDKManager) *SwapDKService {
	return &SwapDKService{
		pendingSignatures: make(map[[32]byte]*SigningRequest),
		manager:           manager,
	}
}

// GetPendingSignatureRequests returns all pending signing requests.
func (s *SwapDKService) GetPendingSignatureRequests() []*SigningRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	var requests []*SigningRequest
	for _, req := range s.pendingSignatures {
		requests = append(requests, req)
	}

	return requests
}

// RespondToSignatureRequest responds to a pending signing request.
func (s *SwapDKService) RespondToSignatureRequest(id [32]byte,
	response *SigningResponse) error {

	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.pendingSignatures[id]
	if !ok {
		return fmt.Errorf("request not found")
	}

	req.Response <- response
	delete(s.pendingSignatures, id)

	return nil
}

// Unimplemented methods will be added in subsequent steps.

func (s *SwapDKService) SignOutputRaw(ctx context.Context, tx *wire.MsgTx,
	signDescriptors []*lndclient.SignDescriptor,
	prevOutputs []*wire.TxOut) ([][]byte, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &SignOutputRawRequest{
			Tx:              tx,
			SignDescriptors: signDescriptors,
			PrevOutputs:     prevOutputs,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return nil, resp.Err
		}

		sigs, ok := resp.Response.([][]byte)
		if !ok {
			return nil, fmt.Errorf("invalid response type")
		}

		return sigs, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *SwapDKService) SignMessage(ctx context.Context, msg []byte,
	locator keychain.KeyLocator,
	opts ...lndclient.SignMessageOption) ([]byte, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &SignMessageRequest{
			Msg:     msg,
			Locator: locator,
			Opts:    opts,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return nil, resp.Err
		}

		sig, ok := resp.Response.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid response type")
		}

		return sig, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *SwapDKService) DeriveSharedKey(ctx context.Context,
	ephemeralPubKey *btcec.PublicKey,
	keyLocator *keychain.KeyLocator) ([32]byte, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &DeriveSharedKeyRequest{
			EphemeralPubKey: ephemeralPubKey,
			KeyLocator:      keyLocator,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return [32]byte{}, resp.Err
		}

		sharedKey, ok := resp.Response.([32]byte)
		if !ok {
			return [32]byte{}, fmt.Errorf("invalid response type")
		}

		return sharedKey, nil
	case <-ctx.Done():
		return [32]byte{}, ctx.Err()
	}
}

func (s *SwapDKService) MuSig2CreateSession(ctx context.Context,
	version input.MuSig2Version, keyLocator *keychain.KeyLocator,
	signers [][]byte, opts ...lndclient.MuSig2SessionOpts) (*input.MuSig2SessionInfo, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &MuSig2CreateSessionRequest{
			Version:    version,
			KeyLocator: keyLocator,
			Signers:    signers,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return nil, resp.Err
		}

		sessionInfo, ok := resp.Response.(*input.MuSig2SessionInfo)
		if !ok {
			return nil, fmt.Errorf("invalid response type")
		}

		return sessionInfo, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *SwapDKService) MuSig2RegisterNonces(ctx context.Context,
	sessionID [32]byte, nonces [][66]byte) (bool, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &MuSig2RegisterNoncesRequest{
			SessionID: sessionID,
			Nonces:    nonces,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return false, resp.Err
		}

		haveAllNonces, ok := resp.Response.(bool)
		if !ok {
			return false, fmt.Errorf("invalid response type")
		}

		return haveAllNonces, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (s *SwapDKService) VerifyMessage(ctx context.Context, msg, sig []byte,
	pubkey [33]byte, opts ...lndclient.VerifyMessageOption) (bool, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &VerifyMessageRequest{
			Msg:    msg,
			Sig:    sig,
			Pubkey: pubkey,
			Opts:   opts,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return false, resp.Err
		}

		valid, ok := resp.Response.(bool)
		if !ok {
			return false, fmt.Errorf("invalid response type")
		}

		return valid, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (s *SwapDKService) SignOutputRawKeyLocator(ctx context.Context,
	tx *wire.MsgTx, keyLocator keychain.KeyLocator,
	prevOutput *wire.TxOut) ([]byte, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &SignOutputRawKeyLocatorRequest{
			Tx:         tx,
			KeyLocator: keyLocator,
			PrevOutput: prevOutput,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return nil, resp.Err
		}

		sig, ok := resp.Response.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid response type")
		}

		return sig, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *SwapDKService) MuSig2Sign(ctx context.Context, sessionID [32]byte,
	message [32]byte, cleanup bool) ([]byte, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &MuSig2SignRequest{
			SessionID: sessionID,
			Message:   message,
			Cleanup:   cleanup,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return nil, resp.Err
		}

		sig, ok := resp.Response.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid response type")
		}

		return sig, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *SwapDKService) MuSig2CombineSig(ctx context.Context,
	sessionID [32]byte, otherPartialSigs [][]byte) (bool, []byte, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &MuSig2CombineSigRequest{
			SessionID:        sessionID,
			OtherPartialSigs: otherPartialSigs,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return false, nil, resp.Err
		}

		type combineSigResponse struct {
			HaveAllSigs bool
			FinalSig    []byte
		}

		combineResp, ok := resp.Response.(*combineSigResponse)
		if !ok {
			return false, nil, fmt.Errorf("invalid response type")
		}

		return combineResp.HaveAllSigs, combineResp.FinalSig, nil
	case <-ctx.Done():
		return false, nil, ctx.Err()
	}
}

func (s *SwapDKService) MuSig2Cleanup(ctx context.Context,
	sessionID [32]byte) error {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &MuSig2CleanupRequest{
			SessionID: sessionID,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		return resp.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *SwapDKService) ComputeInputScript(ctx context.Context, tx *wire.MsgTx,
	signDescriptors []*lndclient.SignDescriptor,
	prevOutputs []*wire.TxOut) ([]*input.Script, error) {

	id := sha256.Sum256([]byte(fmt.Sprintf("%v", time.Now().UnixNano())))

	req := &SigningRequest{
		Id: id,
		Request: &ComputeInputScriptRequest{
			Tx:              tx,
			SignDescriptors: signDescriptors,
			PrevOutputs:     prevOutputs,
		},
		Response: make(chan *SigningResponse),
	}

	s.mu.Lock()
	s.pendingSignatures[id] = req
	s.mu.Unlock()

	select {
	case resp := <-req.Response:
		if resp.Err != nil {
			return nil, resp.Err
		}

		scripts, ok := resp.Response.([]*input.Script)
		if !ok {
			return nil, fmt.Errorf("invalid response type")
		}

		return scripts, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *SwapDKService) RawClientWithMacAuth(ctx context.Context) (context.Context,
	time.Duration, signrpc.SignerClient, error) {

	return nil, 0, nil, fmt.Errorf("not implemented")
}

// GetBalance returns the balance of the static address.
func (s *SwapDKService) GetBalance(ctx context.Context) (int64, error) {
	return s.manager.GetBalance(ctx)
}

// GetTransactions returns a list of transactions for the static address.
func (s *SwapDKService) GetTransactions(ctx context.Context) ([]string, error) {
	return s.manager.GetTransactions(ctx)
}
