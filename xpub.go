package loop

import (
	"context"
	"errors"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/lightninglabs/loop/loopdb"
)

type XpubService struct {
	network *chaincfg.Params
	db      loopdb.XpubStore
}

func NewXpubService(db loopdb.XpubStore,
	network *chaincfg.Params) *XpubService {

	return &XpubService{
		db:      db,
		network: network,
	}
}

// GetNextAddressForXpub returns the next unused address for the given xpub.
func (s *XpubService) GetNextAddressForXpub(ctx context.Context,
	xpub string) (btcutil.Address, error) {

	extendedKey, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return nil, err
	}

	// First check that this is a public xpub.
	if extendedKey.IsPrivate() {
		return nil, errors.New("xpub must be public")
	}

	// Next check that this is a valid xpub.
	if !extendedKey.IsForNet(s.network) {
		return nil, errors.New("invalid xpub")
	}

	// Get the next unused index for the xpub.
	index, err := s.db.GetNextExternalIndex(ctx, xpub)
	if err != nil {
		return nil, err
	}

	// Derive the key for the next address.
	derivedKey, err := extendedKey.Derive(index)
	if err != nil {
		return nil, err
	}

	// Convert the key to a p2tr address.
	pubkey, err := derivedKey.ECPubKey()
	if err != nil {
		return nil, err
	}

	// Calculate the toplevel p2tr pubkey
	pubkey = txscript.ComputeTaprootKeyNoScript(pubkey)

	address, err := btcutil.NewAddressTaproot(
		schnorr.SerializePubKey(pubkey), s.network,
	)
	if err != nil {
		return nil, err
	}

	return address, nil
}
