package loopdb

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/lightninglabs/loop/test"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/stretchr/testify/require"
)

// StoreMock implements a mock client swap store.
type StoreMock struct {
	LoopOutSwaps      map[lntypes.Hash]*LoopOutContract
	LoopOutUpdates    map[lntypes.Hash][]SwapStateData
	loopOutStoreChan  chan LoopOutContract
	loopOutUpdateChan chan SwapStateData

	LoopInSwaps      map[lntypes.Hash]*LoopInContract
	LoopInUpdates    map[lntypes.Hash][]SwapStateData
	loopInStoreChan  chan LoopInContract
	loopInUpdateChan chan SwapStateData

	batches map[int32]Batch
	sweeps  map[lntypes.Hash]Sweep

	t *testing.T
}

// NewStoreMock instantiates a new mock store.
func NewStoreMock(t *testing.T) *StoreMock {
	return &StoreMock{
		loopOutStoreChan:  make(chan LoopOutContract, 1),
		loopOutUpdateChan: make(chan SwapStateData, 1),
		LoopOutSwaps:      make(map[lntypes.Hash]*LoopOutContract),
		LoopOutUpdates:    make(map[lntypes.Hash][]SwapStateData),

		loopInStoreChan:  make(chan LoopInContract, 1),
		loopInUpdateChan: make(chan SwapStateData, 1),
		LoopInSwaps:      make(map[lntypes.Hash]*LoopInContract),
		LoopInUpdates:    make(map[lntypes.Hash][]SwapStateData),

		batches: make(map[int32]Batch),
		sweeps:  make(map[lntypes.Hash]Sweep),

		t: t,
	}
}

// FetchLoopOutSwaps returns all swaps currently in the store.
//
// NOTE: Part of the SwapStore interface.
func (s *StoreMock) FetchLoopOutSwaps(ctx context.Context) ([]*LoopOut, error) {
	result := []*LoopOut{}

	for hash, contract := range s.LoopOutSwaps {
		updates := s.LoopOutUpdates[hash]
		events := make([]*LoopEvent, len(updates))
		for i, u := range updates {
			events[i] = &LoopEvent{
				SwapStateData: u,
			}
		}

		swap := &LoopOut{
			Loop: Loop{
				Hash:   hash,
				Events: events,
			},
			Contract: contract,
		}
		result = append(result, swap)
	}

	return result, nil
}

// FetchLoopOutSwaps returns all swaps currently in the store.
//
// NOTE: Part of the SwapStore interface.
func (s *StoreMock) FetchLoopOutSwap(ctx context.Context,
	hash lntypes.Hash) (*LoopOut, error) {

	contract, ok := s.LoopOutSwaps[hash]
	if !ok {
		return nil, errors.New("swap not found")
	}

	updates := s.LoopOutUpdates[hash]
	events := make([]*LoopEvent, len(updates))
	for i, u := range updates {
		events[i] = &LoopEvent{
			SwapStateData: u,
		}
	}

	swap := &LoopOut{
		Loop: Loop{
			Hash:   hash,
			Events: events,
		},
		Contract: contract,
	}

	return swap, nil
}

// CreateLoopOut adds an initiated swap to the store.
//
// NOTE: Part of the SwapStore interface.
func (s *StoreMock) CreateLoopOut(ctx context.Context, hash lntypes.Hash,
	swap *LoopOutContract) error {

	_, ok := s.LoopOutSwaps[hash]
	if ok {
		return errors.New("swap already exists")
	}

	s.LoopOutSwaps[hash] = swap
	s.LoopOutUpdates[hash] = []SwapStateData{}
	s.loopOutStoreChan <- *swap

	return nil
}

// FetchLoopInSwaps returns all in swaps currently in the store.
func (s *StoreMock) FetchLoopInSwaps(ctx context.Context) ([]*LoopIn,
	error) {

	result := []*LoopIn{}

	for hash, contract := range s.LoopInSwaps {
		updates := s.LoopInUpdates[hash]
		events := make([]*LoopEvent, len(updates))
		for i, u := range updates {
			events[i] = &LoopEvent{
				SwapStateData: u,
			}
		}

		swap := &LoopIn{
			Loop: Loop{
				Hash:   hash,
				Events: events,
			},
			Contract: contract,
		}
		result = append(result, swap)
	}

	return result, nil
}

// CreateLoopIn adds an initiated loop in swap to the store.
//
// NOTE: Part of the SwapStore interface.
func (s *StoreMock) CreateLoopIn(ctx context.Context, hash lntypes.Hash,
	swap *LoopInContract) error {

	_, ok := s.LoopInSwaps[hash]
	if ok {
		return errors.New("swap already exists")
	}

	s.LoopInSwaps[hash] = swap
	s.LoopInUpdates[hash] = []SwapStateData{}
	s.loopInStoreChan <- *swap

	return nil
}

// UpdateLoopOut stores a new event for a target loop out swap. This appends to
// the event log for a particular swap as it goes through the various stages in
// its lifetime.
//
// NOTE: Part of the SwapStore interface.
func (s *StoreMock) UpdateLoopOut(ctx context.Context, hash lntypes.Hash,
	time time.Time, state SwapStateData) error {

	updates, ok := s.LoopOutUpdates[hash]
	if !ok {
		return errors.New("swap does not exists")
	}

	updates = append(updates, state)
	s.LoopOutUpdates[hash] = updates
	s.loopOutUpdateChan <- state

	return nil
}

// UpdateLoopIn stores a new event for a target loop in swap. This appends to
// the event log for a particular swap as it goes through the various stages in
// its lifetime.
//
// NOTE: Part of the SwapStore interface.
func (s *StoreMock) UpdateLoopIn(ctx context.Context, hash lntypes.Hash,
	time time.Time, state SwapStateData) error {

	updates, ok := s.LoopInUpdates[hash]
	if !ok {
		return errors.New("swap does not exists")
	}

	updates = append(updates, state)
	s.LoopInUpdates[hash] = updates
	s.loopInUpdateChan <- state

	return nil
}

// PutLiquidityParams writes the serialized `manager.Parameters` bytes into the
// bucket.
//
// NOTE: Part of the SwapStore interface.
func (s *StoreMock) PutLiquidityParams(ctx context.Context,
	params []byte) error {

	return nil
}

// FetchLiquidityParams reads the serialized `manager.Parameters` bytes from
// the bucket.
//
// NOTE: Part of the SwapStore interface.
func (s *StoreMock) FetchLiquidityParams(ctx context.Context) ([]byte, error) {
	return nil, nil
}

// FetchUnconfirmedBatches fetches all the loop out sweep batches from the
// database that are not in a confirmed state.
func (s *StoreMock) FetchUnconfirmedSweepBatches(ctx context.Context) (
	[]*Batch, error) {

	result := []*Batch{}
	for _, batch := range s.batches {
		batch := batch
		if batch.State != "confirmed" {
			result = append(result, &batch)
		}
	}

	return result, nil
}

// InsertSweepBatch inserts a batch into the database, returning the id of the
// inserted batch.
func (s *StoreMock) InsertSweepBatch(ctx context.Context,
	batch *Batch) (int32, error) {

	var id int32

	if len(s.batches) == 0 {
		id = 0
	} else {
		id = int32(len(s.batches))
	}

	s.batches[id] = *batch
	return id, nil
}

// UpdateSweepBatch updates a batch in the database.
func (s *StoreMock) UpdateSweepBatch(ctx context.Context,
	batch *Batch) error {

	s.batches[batch.ID] = *batch
	return nil
}

// ConfirmBatch confirms a batch.
func (s *StoreMock) ConfirmBatch(ctx context.Context, id int32) error {
	batch, ok := s.batches[id]
	if !ok {
		return errors.New("batch not found")
	}

	batch.State = "confirmed"
	s.batches[batch.ID] = batch

	return nil
}

// FetchBatchSweeps fetches all the sweeps that belong to a batch.
func (s *StoreMock) FetchBatchSweeps(ctx context.Context,
	id int32) ([]*Sweep, error) {

	result := []*Sweep{}
	for _, sweep := range s.sweeps {
		sweep := sweep
		if sweep.BatchID == id {
			result = append(result, &sweep)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// UpsertSweep inserts a sweep into the database, or updates an existing sweep.
func (s *StoreMock) UpsertSweep(ctx context.Context, sweep *Sweep) error {
	s.sweeps[sweep.SwapHash] = *sweep
	return nil
}

// GetSweepStatus returns the status of a sweep.
func (s *StoreMock) GetSweepStatus(ctx context.Context,
	swapHash lntypes.Hash) (bool, error) {

	sweep, ok := s.sweeps[swapHash]
	if !ok {
		return false, nil
	}

	return sweep.Completed, nil
}

// Close closes the store.
func (s *StoreMock) Close() error {
	return nil
}

// isDone asserts that the store mock has no pending operations.
func (s *StoreMock) IsDone() error {
	select {
	case <-s.loopOutStoreChan:
		return errors.New("storeChan not empty")
	default:
	}

	select {
	case <-s.loopOutUpdateChan:
		return errors.New("updateChan not empty")
	default:
	}
	return nil
}

// AssertLoopOutStored asserts that a swap is stored.
func (s *StoreMock) AssertLoopOutStored() {
	s.t.Helper()

	select {
	case <-s.loopOutStoreChan:
	case <-time.After(test.Timeout):
		s.t.Fatalf("expected swap to be stored")
	}
}

// AssertLoopOutState asserts that a specified state transition is persisted to
// disk.
func (s *StoreMock) AssertLoopOutState(expectedState SwapState) {
	s.t.Helper()

	select {
	case state := <-s.loopOutUpdateChan:
		require.Equal(s.t, expectedState, state.State)
	case <-time.After(test.Timeout):
		s.t.Fatalf("expected swap state to be stored")
	}
}

// AssertLoopInStored asserts that a loop-in swap is stored.
func (s *StoreMock) AssertLoopInStored() {
	s.t.Helper()

	select {
	case <-s.loopInStoreChan:
	case <-time.After(test.Timeout):
		s.t.Fatalf("expected swap to be stored")
	}
}

// assertLoopInState asserts that a specified state transition is persisted to
// disk.
func (s *StoreMock) AssertLoopInState(
	expectedState SwapState) SwapStateData {

	s.t.Helper()

	state := <-s.loopInUpdateChan
	require.Equal(s.t, expectedState, state.State)

	return state
}

// AssertStorePreimageReveal asserts that a swap is marked as preimage revealed.
func (s *StoreMock) AssertStorePreimageReveal() {
	s.t.Helper()

	select {
	case state := <-s.loopOutUpdateChan:
		require.Equal(s.t, StatePreimageRevealed, state.State)

	case <-time.After(test.Timeout):
		s.t.Fatalf("expected swap to be marked as preimage revealed")
	}
}

// AssertStoreFinished asserts that a swap is marked as finished.
func (s *StoreMock) AssertStoreFinished(expectedResult SwapState) {
	s.t.Helper()

	select {
	case state := <-s.loopOutUpdateChan:
		require.Equal(s.t, expectedResult, state.State)

	case <-time.After(test.Timeout):
		s.t.Fatalf("expected swap to be finished")
	}
}

// AssertSweepStored asserts that a sweep is stored.
func (s *StoreMock) AssertSweepStored(id lntypes.Hash) {
	s.t.Helper()

	_, ok := s.sweeps[id]
	if !ok {
		s.t.Fatalf("expected sweep to be stored")
	}
}

// BatchCreateLoopOut creates many loop out swaps in a batch.
func (b *StoreMock) BatchCreateLoopOut(ctx context.Context,
	swaps map[lntypes.Hash]*LoopOutContract) error {

	return errors.New("not implemented")
}

// BatchCreateLoopIn creates many loop in swaps in a batch.
func (b *StoreMock) BatchCreateLoopIn(ctx context.Context,
	swaps map[lntypes.Hash]*LoopInContract) error {

	return errors.New("not implemented")
}

// BatchInsertUpdate inserts many updates for a swap in a batch.
func (b *StoreMock) BatchInsertUpdate(ctx context.Context,
	updateData map[lntypes.Hash][]BatchInsertUpdateData) error {

	return errors.New("not implemented")
}
