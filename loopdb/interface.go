package loopdb

import (
	"context"
	"time"

	"github.com/lightningnetwork/lnd/lntypes"
)

// SwapStore is the primary database interface used by the loopd system. It
// houses information for all pending completed/failed swaps.
type SwapStore interface {
	// FetchLoopOutSwaps returns all swaps currently in the store.
	FetchLoopOutSwaps(ctx context.Context) ([]*LoopOut, error)

	// FetchLoopOutSwap returns the loop out swap with the given hash.
	FetchLoopOutSwap(ctx context.Context, hash lntypes.Hash) (*LoopOut, error)

	// CreateLoopOut adds an initiated swap to the store.
	CreateLoopOut(ctx context.Context, hash lntypes.Hash,
		swap *LoopOutContract) error

	// BatchCreateLoopOut creates a batch of loop out swaps to the store.
	BatchCreateLoopOut(ctx context.Context, hashes []lntypes.Hash,
		swaps []*LoopOutContract) error

	// UpdateLoopOut stores a new event for a target loop out swap. This
	// appends to the event log for a particular swap as it goes through
	// the various stages in its lifetime.
	UpdateLoopOut(ctx context.Context, hash lntypes.Hash, time time.Time,
		state SwapStateData) error

	// FetchLoopInSwaps returns all swaps currently in the store.
	FetchLoopInSwaps(ctx context.Context) ([]*LoopIn, error)

	// CreateLoopIn adds an initiated swap to the store.
	CreateLoopIn(ctx context.Context, hash lntypes.Hash,
		swap *LoopInContract) error

	// BatchCreateLoopIn creates a batch of loop in swaps to the store.
	BatchCreateLoopIn(ctx context.Context, hashes []lntypes.Hash,
		swaps []*LoopInContract) error

	// UpdateLoopIn stores a new event for a target loop in swap. This
	// appends to the event log for a particular swap as it goes through
	// the various stages in its lifetime.
	UpdateLoopIn(ctx context.Context, hash lntypes.Hash, time time.Time,
		state SwapStateData) error

	BatchInsertUpdate(ctx context.Context, hashes []lntypes.Hash,
		times []time.Time, states []SwapStateData) error

	// PutLiquidityParams writes the serialized `manager.Parameters` bytes
	// into the bucket.
	//
	// NOTE: it's the caller's responsibility to encode the param. Atm,
	// it's encoding using the proto package's `Marshal` method.
	PutLiquidityParams(ctx context.Context, params []byte) error

	// FetchLiquidityParams reads the serialized `manager.Parameters` bytes
	// from the bucket.
	//
	// NOTE: it's the caller's responsibility to decode the param. Atm,
	// it's decoding using the proto package's `Unmarshal` method.
	FetchLiquidityParams(ctx context.Context) ([]byte, error)

	// Close closes the underlying database.
	Close() error
}

type XpubStore interface {
	// GetNextExternalIndex returns the next external index to be used for
	// the given xpub.
	GetNextExternalIndex(ctx context.Context, xpub string) (uint32, error)
}

// TODO(roasbeef): back up method in interface?
