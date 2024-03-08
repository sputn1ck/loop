package assets

import (
	"context"
	"errors"
	"sync"

	"github.com/lightningnetwork/lnd/lntypes"
)

type InmemStore struct {
	// swaps is a map of swap hashes to swap outs.
	swaps map[lntypes.Hash]*SwapOut

	sync.Mutex
}

// NewInmemStore creates a new in-memory store for asset swaps.
func NewInmemStore() *InmemStore {
	return &InmemStore{
		swaps: make(map[lntypes.Hash]*SwapOut),
	}
}

// CreateAssetSwapOut creates a new asset swap out in the store.
func (i *InmemStore) CreateAssetSwapOut(_ context.Context, swap *SwapOut) error {
	i.Lock()
	defer i.Unlock()

	i.swaps[swap.SwapPreimage.Hash()] = swap
	return nil
}

// GetAssetSwapOut gets an asset swap out from the store.
func (i *InmemStore) GetAssetSwapOut(_ context.Context, swapHash []byte) (
	*SwapOut, error) {

	i.Lock()
	defer i.Unlock()

	swap, ok := i.swaps[lntypes.Hash(swapHash)]
	if !ok {
		return nil, errors.New("swap not found")
	}

	return swap, nil
}

// UpdateAssetSwapOut updates an asset swap out in the store.
func (i *InmemStore) UpdateAssetSwapOut(ctx context.Context, swap *SwapOut) error {
	i.Lock()
	defer i.Unlock()

	i.swaps[swap.SwapPreimage.Hash()] = swap
	return nil
}
