package assets

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/loop/fsm"
	"github.com/lightninglabs/taproot-assets/address"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/commitment"

	"github.com/lightninglabs/taproot-assets/tappsbt"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lntypes"
)

type SwapState fsm.StateType

const (
	// StateInit is the initial state of the swap.
	StateInit SwapState = "Init"

	// StatePrepayAccepted is the state where the prepay invoice has been
	// accepted.
	StatePrepayAccepted SwapState = "PrepayAccepted"

	// StateOpeningTxBroadcast is the state where the opening transaction
	// has been broadcast.
	StateOpeningTxBroadcast SwapState = "OpeningTxBroadcast"

	StateFailed SwapState = "Failed"
)

type SwapOut struct {
	SwapPreimage lntypes.Preimage
	State        SwapState
	Amount       btcutil.Amount

	SenderPubkey   *btcec.PublicKey
	ReceiverPubkey *btcec.PublicKey
	Expiry         int64
	AssetId        []byte

	PrepayPreimage lntypes.Preimage
	PrepayInvoice  string

	ServerKeyDesc *keychain.KeyDescriptor

	TaprootAssetRoot []byte
	HtlcOutpoint     *wire.OutPoint
}

func NewSwapOut(swapPreimage, prepayPreimage lntypes.Preimage, amt btcutil.Amount,
	serverKeyDesc *keychain.KeyDescriptor, receiverPubkey *btcec.PublicKey,
	expiry int64, prepayInvoice string) *SwapOut {

	return &SwapOut{
		SwapPreimage:   swapPreimage,
		State:          StateInit,
		Amount:         amt,
		SenderPubkey:   serverKeyDesc.PubKey,
		ReceiverPubkey: receiverPubkey,
		Expiry:         expiry,
		ServerKeyDesc:  serverKeyDesc,
		PrepayPreimage: prepayPreimage,
		PrepayInvoice:  prepayInvoice,
	}
}

func (s *SwapOut) getSuccesScript() ([]byte, error) {
	return GenSuccessPathScript(s.ReceiverPubkey, s.SwapPreimage.Hash())
}

func (s *SwapOut) getTimeoutScript() ([]byte, error) {
	return GenTimeoutPathScript(s.SenderPubkey, s.Expiry)
}

func (s *SwapOut) getAggregateKey() (*btcec.PublicKey, error) {
	aggregateKey, err := input.MuSig2CombineKeys(
		input.MuSig2Version100RC2,
		[]*btcec.PublicKey{
			s.SenderPubkey, s.ReceiverPubkey,
		},
		true,
		&input.MuSig2Tweaks{},
	)
	if err != nil {
		return nil, err
	}

	return aggregateKey.PreTweakedKey, nil
}

func (s *SwapOut) getTimeOutLeaf() (txscript.TapLeaf, error) {
	timeoutScript, err := s.getTimeoutScript()
	if err != nil {
		return txscript.TapLeaf{}, err
	}

	timeoutLeaf := txscript.NewBaseTapLeaf(timeoutScript)

	return timeoutLeaf, nil
}

func (s *SwapOut) getSuccessLeaf() (txscript.TapLeaf, error) {
	successScript, err := s.getSuccesScript()
	if err != nil {
		return txscript.TapLeaf{}, err
	}

	successLeaf := txscript.NewBaseTapLeaf(successScript)

	return successLeaf, nil
}

func (s *SwapOut) getSiblingPreimage() (commitment.TapscriptPreimage, error) {
	timeOutLeaf, err := s.getTimeOutLeaf()
	if err != nil {
		return commitment.TapscriptPreimage{}, err
	}

	successLeaf, err := s.getSuccessLeaf()
	if err != nil {
		return commitment.TapscriptPreimage{}, err
	}

	branch := txscript.NewTapBranch(timeOutLeaf, successLeaf)

	siblingPreimage := commitment.NewPreimageFromBranch(branch)

	return siblingPreimage, nil
}

func (s *SwapOut) createHtlcVpkt() (*tappsbt.VPacket, error) {
	assetId := asset.ID{}
	copy(assetId[:], s.AssetId)

	btcInternalKey, err := s.getAggregateKey()
	if err != nil {
		return nil, err
	}

	siblingPreimage, err := s.getSiblingPreimage()
	if err != nil {
		return nil, err
	}

	tapScriptKey, _, _, _, err := createOpTrueLeaf()
	if err != nil {
		return nil, err
	}

	pkt := &tappsbt.VPacket{
		Inputs: []*tappsbt.VInput{{
			PrevID: asset.PrevID{
				ID: assetId,
			},
		}},
		Outputs:     make([]*tappsbt.VOutput, 0, 2),
		ChainParams: &address.RegressionNetTap,
	}
	pkt.Outputs = append(pkt.Outputs, &tappsbt.VOutput{
		Amount:            0,
		Type:              tappsbt.TypeSplitRoot,
		AnchorOutputIndex: 0,
		ScriptKey:         asset.NUMSScriptKey,
	})
	pkt.Outputs = append(pkt.Outputs, &tappsbt.VOutput{
		// todo(sputn1ck) assetversion
		AssetVersion:      asset.Version(1),
		Amount:            uint64(s.Amount),
		Interactive:       true,
		AnchorOutputIndex: 1,
		ScriptKey: asset.NewScriptKey(
			tapScriptKey.PubKey,
		),
		AnchorOutputInternalKey:      btcInternalKey,
		AnchorOutputTapscriptSibling: &siblingPreimage,
	})

	return pkt, nil
}
