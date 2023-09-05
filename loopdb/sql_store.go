package loopdb

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/loop/loopdb/sqlc"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/routing/route"
)

// FetchLoopOutSwaps returns all swaps currently in the store.
func (s *BaseDB) FetchLoopOutSwaps(ctx context.Context) ([]*LoopOut,
	error) {

	var loopOuts []*LoopOut

	err := s.ExecTx(ctx, NewSqlReadOpts(), func(*sqlc.Queries) error {
		swaps, err := s.Queries.GetLoopOutSwaps(ctx)
		if err != nil {
			return err
		}

		loopOuts = make([]*LoopOut, len(swaps))

		for i, swap := range swaps {
			updates, err := s.Queries.GetSwapUpdates(ctx, swap.SwapHash)
			if err != nil {
				return err
			}

			loopOut, err := s.convertLoopOutRow(
				sqlc.GetLoopOutSwapRow(swap), updates,
			)
			if err != nil {
				return err
			}

			loopOuts[i] = loopOut
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return loopOuts, nil
}

// FetchLoopOutSwap returns the loop out swap with the given hash.
func (s *BaseDB) FetchLoopOutSwap(ctx context.Context,
	hash lntypes.Hash) (*LoopOut, error) {

	var loopOut *LoopOut

	err := s.ExecTx(ctx, NewSqlReadOpts(), func(*sqlc.Queries) error {
		swap, err := s.Queries.GetLoopOutSwap(ctx, hash[:])
		if err != nil {
			return err
		}

		updates, err := s.Queries.GetSwapUpdates(ctx, swap.SwapHash)
		if err != nil {
			return err
		}

		loopOut, err = s.convertLoopOutRow(
			swap, updates,
		)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return loopOut, nil
}

// CreateLoopOut adds an initiated swap to the store.
func (s *BaseDB) CreateLoopOut(ctx context.Context, hash lntypes.Hash,
	swap *LoopOutContract) error {

	writeOpts := &SqliteTxOptions{}
	return s.ExecTx(ctx, writeOpts, func(tx *sqlc.Queries) error {
		insertArgs := loopToInsertArgs(
			hash, &swap.SwapContract,
		)

		// First we'll insert the swap itself.
		err := tx.InsertSwap(ctx, insertArgs)
		if err != nil {
			return err
		}

		htlcKeyInsertArgs := swapToHtlcKeysInsertArgs(
			hash, &swap.SwapContract,
		)

		// Next insert the htlc keys.
		err = tx.InsertHtlcKeys(ctx, htlcKeyInsertArgs)
		if err != nil {
			return err
		}

		loopOutInsertArgs := loopOutToInsertArgs(hash, swap)

		// Next insert the loop out relevant data.
		err = tx.InsertLoopOut(ctx, loopOutInsertArgs)
		if err != nil {
			return err
		}

		return nil
	})
}

// BatchCreateLoopOut adds multiple initiated swaps to the store.
func (s *BaseDB) BatchCreateLoopOut(ctx context.Context,
	swaps map[lntypes.Hash]*LoopOutContract) error {

	writeOpts := &SqliteTxOptions{}
	return s.ExecTx(ctx, writeOpts, func(tx *sqlc.Queries) error {
		for swapHash, swap := range swaps {
			swap := swap

			insertArgs := loopToInsertArgs(
				swapHash, &swap.SwapContract,
			)

			// First we'll insert the swap itself.
			err := tx.InsertSwap(ctx, insertArgs)
			if err != nil {
				return err
			}

			htlcKeyInsertArgs := swapToHtlcKeysInsertArgs(
				swapHash, &swap.SwapContract,
			)

			// Next insert the htlc keys.
			err = tx.InsertHtlcKeys(ctx, htlcKeyInsertArgs)
			if err != nil {
				return err
			}

			loopOutInsertArgs := loopOutToInsertArgs(swapHash, swap)

			// Next insert the loop out relevant data.
			err = tx.InsertLoopOut(ctx, loopOutInsertArgs)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateLoopOut stores a new event for a target loop out swap. This
// appends to the event log for a particular swap as it goes through
// the various stages in its lifetime.
func (s *BaseDB) UpdateLoopOut(ctx context.Context, hash lntypes.Hash,
	time time.Time, state SwapStateData) error {

	return s.updateLoop(ctx, hash, time, state)
}

// FetchLoopInSwaps returns all swaps currently in the store.
func (s *BaseDB) FetchLoopInSwaps(ctx context.Context) (
	[]*LoopIn, error) {

	var loopIns []*LoopIn

	err := s.ExecTx(ctx, NewSqlReadOpts(), func(*sqlc.Queries) error {
		swaps, err := s.Queries.GetLoopInSwaps(ctx)
		if err != nil {
			return err
		}

		loopIns = make([]*LoopIn, len(swaps))

		for i, swap := range swaps {
			updates, err := s.Queries.GetSwapUpdates(ctx, swap.SwapHash)
			if err != nil {
				return err
			}

			loopIn, err := s.convertLoopInRow(
				swap, updates,
			)
			if err != nil {
				return err
			}

			loopIns[i] = loopIn
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return loopIns, nil
}

// CreateLoopIn adds an initiated swap to the store.
func (s *BaseDB) CreateLoopIn(ctx context.Context, hash lntypes.Hash,
	swap *LoopInContract) error {

	writeOpts := &SqliteTxOptions{}
	return s.ExecTx(ctx, writeOpts, func(tx *sqlc.Queries) error {
		insertArgs := loopToInsertArgs(
			hash, &swap.SwapContract,
		)

		// First we'll insert the swap itself.
		err := tx.InsertSwap(ctx, insertArgs)
		if err != nil {
			return err
		}
		htlcKeyInsertArgs := swapToHtlcKeysInsertArgs(
			hash, &swap.SwapContract,
		)

		// Next insert the htlc keys.
		err = tx.InsertHtlcKeys(ctx, htlcKeyInsertArgs)
		if err != nil {
			return err
		}

		loopInInsertArgs := loopInToInsertArgs(hash, swap)

		// Next insert the loop out relevant data.
		err = tx.InsertLoopIn(ctx, loopInInsertArgs)
		if err != nil {
			return err
		}

		return nil
	})
}

// BatchCreateLoopIn adds multiple initiated swaps to the store.
func (s *BaseDB) BatchCreateLoopIn(ctx context.Context,
	swaps map[lntypes.Hash]*LoopInContract) error {

	writeOpts := &SqliteTxOptions{}
	return s.ExecTx(ctx, writeOpts, func(tx *sqlc.Queries) error {
		for swapHash, swap := range swaps {
			swap := swap

			insertArgs := loopToInsertArgs(
				swapHash, &swap.SwapContract,
			)

			// First we'll insert the swap itself.
			err := tx.InsertSwap(ctx, insertArgs)
			if err != nil {
				return err
			}

			htlcKeyInsertArgs := swapToHtlcKeysInsertArgs(
				swapHash, &swap.SwapContract,
			)

			// Next insert the htlc keys.
			err = tx.InsertHtlcKeys(ctx, htlcKeyInsertArgs)
			if err != nil {
				return err
			}

			loopInInsertArgs := loopInToInsertArgs(swapHash, swap)

			// Next insert the loop in relevant data.
			err = tx.InsertLoopIn(ctx, loopInInsertArgs)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// UpdateLoopIn stores a new event for a target loop in swap. This
// appends to the event log for a particular swap as it goes through
// the various stages in its lifetime.
func (s *BaseDB) UpdateLoopIn(ctx context.Context, hash lntypes.Hash,
	time time.Time, state SwapStateData) error {

	return s.updateLoop(ctx, hash, time, state)
}

// PutLiquidityParams writes the serialized `manager.Parameters` bytes
// into the bucket.
//
// NOTE: it's the caller's responsibility to encode the param. Atm,
// it's encoding using the proto package's `Marshal` method.
func (s *BaseDB) PutLiquidityParams(ctx context.Context,
	params []byte) error {

	err := s.Queries.UpsertLiquidityParams(ctx, params)
	if err != nil {
		return err
	}

	return nil
}

// FetchLiquidityParams reads the serialized `manager.Parameters` bytes
// from the bucket.
//
// NOTE: it's the caller's responsibility to decode the param. Atm,
// it's decoding using the proto package's `Unmarshal` method.
func (s *BaseDB) FetchLiquidityParams(ctx context.Context) ([]byte,
	error) {

	var params []byte
	params, err := s.Queries.FetchLiquidityParams(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return params, nil
	} else if err != nil {
		return nil, err
	}

	return params, nil
}

// A compile time assertion to ensure that SqliteStore satisfies the
// SwapStore interface.
var _ SwapStore = (*BaseDB)(nil)

// updateLoop updates the swap with the given hash by inserting a new update
// in the swap_updates table.
func (s *BaseDB) updateLoop(ctx context.Context, hash lntypes.Hash,
	time time.Time, state SwapStateData) error {

	writeOpts := &SqliteTxOptions{}
	return s.ExecTx(ctx, writeOpts, func(tx *sqlc.Queries) error {
		updateParams := sqlc.InsertSwapUpdateParams{
			SwapHash:        hash[:],
			UpdateTimestamp: time.UTC(),
			UpdateState:     int32(state.State),
			ServerCost:      int64(state.Cost.Server),
			OnchainCost:     int64(state.Cost.Onchain),
			OffchainCost:    int64(state.Cost.Offchain),
		}

		if state.HtlcTxHash != nil {
			updateParams.HtlcTxhash = state.HtlcTxHash.String()
		}
		// First we insert the swap update.
		err := tx.InsertSwapUpdate(ctx, updateParams)
		if err != nil {
			return err
		}

		return nil
	})
}

// BatchInsertUpdate inserts multiple swap updates to the store.
func (s *BaseDB) BatchInsertUpdate(ctx context.Context,
	updateData map[lntypes.Hash][]BatchInsertUpdateData) error {

	writeOpts := &SqliteTxOptions{}
	return s.ExecTx(ctx, writeOpts, func(tx *sqlc.Queries) error {
		for swapHash, updates := range updateData {
			for _, update := range updates {
				updateParams := sqlc.InsertSwapUpdateParams{
					SwapHash:        swapHash[:],
					UpdateTimestamp: update.Time.UTC(),
					UpdateState:     int32(update.State.State),
					ServerCost:      int64(update.State.Cost.Server),
					OnchainCost:     int64(update.State.Cost.Onchain),
					OffchainCost:    int64(update.State.Cost.Offchain),
				}

				if update.State.HtlcTxHash != nil {
					updateParams.HtlcTxhash = update.State.HtlcTxHash.String()
				}
				// First we insert the swap update.
				err := tx.InsertSwapUpdate(ctx, updateParams)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// FetchUnconfirmedSweepBatches fetches all the batches from the database that
// are not in a confirmed state.
func (s *BaseDB) FetchUnconfirmedSweepBatches(ctx context.Context) ([]*Batch,
	error) {

	var batches []*Batch

	dbBatches, err := s.GetUnconfirmedBatches(ctx)
	if err != nil {
		return nil, err
	}

	for _, dbBatch := range dbBatches {
		batch := convertBatchRow(dbBatch)
		if err != nil {
			return nil, err
		}

		batches = append(batches, batch)
	}

	return batches, err
}

// InsertSweepBatch inserts a batch into the database, returning the id of the
// inserted batch.
func (s *BaseDB) InsertSweepBatch(ctx context.Context, batch *Batch) (int32,
	error) {

	return s.Queries.InsertBatch(ctx, batchToInsertArgs(*batch))
}

// UpdateSweepBatch updates a batch in the database.
func (s *BaseDB) UpdateSweepBatch(ctx context.Context, batch *Batch) error {
	return s.Queries.UpdateBatch(ctx, batchToUpdateArgs(*batch))
}

// ConfirmBatch confirms a batch by setting the state to confirmed.
func (s *BaseDB) ConfirmBatch(ctx context.Context, id int32) error {
	return s.Queries.ConfirmBatch(ctx, id)
}

// FetchBatchSweeps fetches all the sweeps that are part a batch.
func (s *BaseDB) FetchBatchSweeps(ctx context.Context, id int32) (
	[]*Sweep, error) {

	readOpts := NewSqlReadOpts()
	var sweeps []*Sweep

	err := s.ExecTx(ctx, readOpts, func(tx *sqlc.Queries) error {
		dbSweeps, err := tx.GetBatchSweeps(ctx, id)
		if err != nil {
			return err
		}

		for _, dbSweep := range dbSweeps {
			updates, err := s.Queries.GetSwapUpdates(
				ctx, dbSweep.SwapHash,
			)
			if err != nil {
				return err
			}

			sweep, err := s.convertSweepRow(dbSweep, updates)
			if err != nil {
				return err
			}

			sweeps = append(sweeps, &sweep)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return sweeps, nil
}

// UpsertSweep inserts a sweep into the database, or updates an existing sweep
// if it already exists.
func (s *BaseDB) UpsertSweep(ctx context.Context, sweep *Sweep) error {
	return s.Queries.UpsertSweep(ctx, sweepToUpsertArgs(*sweep))
}

// GetSweepStatus returns true if the sweep has been completed.
func (s *BaseDB) GetSweepStatus(ctx context.Context, swapHash lntypes.Hash) (
	bool, error) {

	return s.Queries.GetSweepStatus(ctx, swapHash[:])
}

// loopToInsertArgs converts a SwapContract struct to the arguments needed to
// insert it into the database.
func loopToInsertArgs(hash lntypes.Hash,
	swap *SwapContract) sqlc.InsertSwapParams {

	return sqlc.InsertSwapParams{
		SwapHash:         hash[:],
		Preimage:         swap.Preimage[:],
		InitiationTime:   swap.InitiationTime.UTC(),
		AmountRequested:  int64(swap.AmountRequested),
		CltvExpiry:       swap.CltvExpiry,
		MaxSwapFee:       int64(swap.MaxSwapFee),
		MaxMinerFee:      int64(swap.MaxMinerFee),
		InitiationHeight: swap.InitiationHeight,
		ProtocolVersion:  int32(swap.ProtocolVersion),
		Label:            swap.Label,
	}
}

// loopOutToInsertArgs converts a LoopOutContract struct to the arguments
// needed to insert it into the database.
func loopOutToInsertArgs(hash lntypes.Hash,
	loopOut *LoopOutContract) sqlc.InsertLoopOutParams {

	return sqlc.InsertLoopOutParams{
		SwapHash:            hash[:],
		DestAddress:         loopOut.DestAddr.String(),
		SingleSweep:         loopOut.IsExternalAddr,
		SwapInvoice:         loopOut.SwapInvoice,
		MaxSwapRoutingFee:   int64(loopOut.MaxSwapRoutingFee),
		SweepConfTarget:     loopOut.SweepConfTarget,
		HtlcConfirmations:   int32(loopOut.HtlcConfirmations),
		OutgoingChanSet:     loopOut.OutgoingChanSet.String(),
		PrepayInvoice:       loopOut.PrepayInvoice,
		MaxPrepayRoutingFee: int64(loopOut.MaxPrepayRoutingFee),
		PublicationDeadline: loopOut.SwapPublicationDeadline.UTC(),
	}
}

// loopInToInsertArgs converts a LoopInContract struct to the arguments needed
// to insert it into the database.
func loopInToInsertArgs(hash lntypes.Hash,
	loopIn *LoopInContract) sqlc.InsertLoopInParams {

	loopInInsertParams := sqlc.InsertLoopInParams{
		SwapHash:       hash[:],
		HtlcConfTarget: loopIn.HtlcConfTarget,
		ExternalHtlc:   loopIn.ExternalHtlc,
	}

	if loopIn.LastHop != nil {
		loopInInsertParams.LastHop = loopIn.LastHop[:]
	}

	return loopInInsertParams
}

// swapToHtlcKeysInsertArgs extracts the htlc keys from a SwapContract struct
// and converts them to the arguments needed to insert them into the database.
func swapToHtlcKeysInsertArgs(hash lntypes.Hash,
	swap *SwapContract) sqlc.InsertHtlcKeysParams {

	return sqlc.InsertHtlcKeysParams{
		SwapHash:               hash[:],
		SenderScriptPubkey:     swap.HtlcKeys.SenderScriptKey[:],
		ReceiverScriptPubkey:   swap.HtlcKeys.ReceiverScriptKey[:],
		SenderInternalPubkey:   swap.HtlcKeys.SenderInternalPubKey[:],
		ReceiverInternalPubkey: swap.HtlcKeys.ReceiverInternalPubKey[:],
		ClientKeyFamily: int32(
			swap.HtlcKeys.ClientScriptKeyLocator.Family,
		),
		ClientKeyIndex: int32(
			swap.HtlcKeys.ClientScriptKeyLocator.Index,
		),
	}
}

// convertLoopOutRow converts a database row containing a loop out swap to a
// LoopOut struct.
func (s *BaseDB) convertLoopOutRow(row sqlc.GetLoopOutSwapRow,
	updates []sqlc.SwapUpdate) (*LoopOut, error) {

	htlcKeys, err := fetchHtlcKeys(
		row.SenderScriptPubkey, row.ReceiverScriptPubkey,
		row.SenderInternalPubkey, row.ReceiverInternalPubkey,
		row.ClientKeyFamily, row.ClientKeyIndex,
	)
	if err != nil {
		return nil, err
	}

	preimage, err := lntypes.MakePreimage(row.Preimage)
	if err != nil {
		return nil, err
	}

	destAddress, err := btcutil.DecodeAddress(row.DestAddress, s.network)
	if err != nil {
		return nil, err
	}

	swapHash, err := lntypes.MakeHash(row.SwapHash)
	if err != nil {
		return nil, err
	}

	loopOut := &LoopOut{
		Contract: &LoopOutContract{
			SwapContract: SwapContract{
				Preimage:         preimage,
				AmountRequested:  btcutil.Amount(row.AmountRequested),
				HtlcKeys:         htlcKeys,
				CltvExpiry:       row.CltvExpiry,
				MaxSwapFee:       btcutil.Amount(row.MaxSwapFee),
				MaxMinerFee:      btcutil.Amount(row.MaxMinerFee),
				InitiationHeight: row.InitiationHeight,
				InitiationTime:   row.InitiationTime,
				Label:            row.Label,
				ProtocolVersion:  ProtocolVersion(row.ProtocolVersion),
			},
			DestAddr:                destAddress,
			IsExternalAddr:          row.SingleSweep,
			SwapInvoice:             row.SwapInvoice,
			MaxSwapRoutingFee:       btcutil.Amount(row.MaxSwapRoutingFee),
			SweepConfTarget:         row.SweepConfTarget,
			HtlcConfirmations:       uint32(row.HtlcConfirmations),
			PrepayInvoice:           row.PrepayInvoice,
			MaxPrepayRoutingFee:     btcutil.Amount(row.MaxPrepayRoutingFee),
			SwapPublicationDeadline: row.PublicationDeadline,
		},
		Loop: Loop{
			Hash: swapHash,
		},
	}

	if row.OutgoingChanSet != "" {
		chanSet, err := convertOutgoingChanSet(row.OutgoingChanSet)
		if err != nil {
			return nil, err
		}

		loopOut.Contract.OutgoingChanSet = chanSet
	}

	// If we don't have any updates yet we can return early
	if len(updates) == 0 {
		return loopOut, nil
	}

	events, err := getSwapEvents(updates)
	if err != nil {
		return nil, err
	}

	loopOut.Events = events

	return loopOut, nil
}

// convertLoopInRow converts a database row containing a loop in swap to a
// LoopIn struct.
func (s *BaseDB) convertLoopInRow(row sqlc.GetLoopInSwapsRow,
	updates []sqlc.SwapUpdate) (*LoopIn, error) {

	htlcKeys, err := fetchHtlcKeys(
		row.SenderScriptPubkey, row.ReceiverScriptPubkey,
		row.SenderInternalPubkey, row.ReceiverInternalPubkey,
		row.ClientKeyFamily, row.ClientKeyIndex,
	)
	if err != nil {
		return nil, err
	}

	preimage, err := lntypes.MakePreimage(row.Preimage)
	if err != nil {
		return nil, err
	}

	swapHash, err := lntypes.MakeHash(row.SwapHash)
	if err != nil {
		return nil, err
	}

	loopIn := &LoopIn{
		Contract: &LoopInContract{
			SwapContract: SwapContract{
				Preimage:         preimage,
				AmountRequested:  btcutil.Amount(row.AmountRequested),
				HtlcKeys:         htlcKeys,
				CltvExpiry:       row.CltvExpiry,
				MaxSwapFee:       btcutil.Amount(row.MaxSwapFee),
				MaxMinerFee:      btcutil.Amount(row.MaxMinerFee),
				InitiationHeight: row.InitiationHeight,
				InitiationTime:   row.InitiationTime,
				Label:            row.Label,
				ProtocolVersion:  ProtocolVersion(row.ProtocolVersion),
			},
			HtlcConfTarget: row.HtlcConfTarget,
			ExternalHtlc:   row.ExternalHtlc,
		},
		Loop: Loop{
			Hash: swapHash,
		},
	}

	if row.LastHop != nil {
		lastHop, err := route.NewVertexFromBytes(row.LastHop)
		if err != nil {
			return nil, err
		}

		loopIn.Contract.LastHop = &lastHop
	}

	// If we don't have any updates yet we can return early
	if len(updates) == 0 {
		return loopIn, nil
	}

	events, err := getSwapEvents(updates)
	if err != nil {
		return nil, err
	}

	loopIn.Events = events

	return loopIn, nil
}

// convertBatchRow converts a batch row from db to a sweepbatcher.Batch struct.
func convertBatchRow(row sqlc.SweepBatch) *Batch {
	batch := Batch{
		ID:    row.ID,
		State: row.State,
	}

	if row.BatchTxID.Valid {
		err := chainhash.Decode(&batch.BatchTxid, row.BatchTxID.String)
		if err != nil {
			return nil
		}
	}

	batch.BatchPkScript = row.BatchPkScript

	if row.LastRbfHeight.Valid {
		batch.LastRbfHeight = row.LastRbfHeight.Int32
	}

	if row.LastRbfSatPerKw.Valid {
		batch.LastRbfSatPerKw = row.LastRbfSatPerKw.Int32
	}

	batch.MaxTimeoutDistance = row.MaxTimeoutDistance

	return &batch
}

// BatchToUpsertArgs converts a Batch struct to the arguments needed to insert
// it into the database.
func batchToInsertArgs(batch Batch) sqlc.InsertBatchParams {
	args := sqlc.InsertBatchParams{
		State: batch.State,
		BatchTxID: sql.NullString{
			Valid:  true,
			String: batch.BatchTxid.String(),
		},
		BatchPkScript: batch.BatchPkScript,
		LastRbfHeight: sql.NullInt32{
			Valid: true,
			Int32: batch.LastRbfHeight,
		},
		LastRbfSatPerKw: sql.NullInt32{
			Valid: true,
			Int32: batch.LastRbfSatPerKw,
		},
		MaxTimeoutDistance: batch.MaxTimeoutDistance,
	}

	return args
}

// BatchToUpsertArgs converts a Batch struct to the arguments needed to insert
// it into the database.
func batchToUpdateArgs(batch Batch) sqlc.UpdateBatchParams {
	args := sqlc.UpdateBatchParams{
		ID:    batch.ID,
		State: batch.State,
		BatchTxID: sql.NullString{
			Valid:  true,
			String: batch.BatchTxid.String(),
		},
		BatchPkScript: batch.BatchPkScript,
		LastRbfHeight: sql.NullInt32{
			Valid: true,
			Int32: batch.LastRbfHeight,
		},
		LastRbfSatPerKw: sql.NullInt32{
			Valid: true,
			Int32: batch.LastRbfSatPerKw,
		},
	}

	return args
}

// convertSweepRow converts a sweep row from db to a sweep struct.
func (s *BaseDB) convertSweepRow(row sqlc.GetBatchSweepsRow,
	updates []sqlc.SwapUpdate) (Sweep, error) {

	sweep := Sweep{
		ID:      row.ID,
		BatchID: row.BatchID,
		Amount:  btcutil.Amount(row.Amt),
	}

	swapHash, err := lntypes.MakeHash(row.SwapHash)
	if err != nil {
		return sweep, err
	}

	sweep.SwapHash = swapHash

	hash, err := chainhash.NewHash(row.OutpointTxid)
	if err != nil {
		return sweep, err
	}

	sweep.Outpoint = wire.OutPoint{
		Hash:  *hash,
		Index: uint32(row.OutpointIndex),
	}

	sweep.LoopOut, err = s.convertLoopOutRow(
		sqlc.GetLoopOutSwapRow{
			ID:                     row.ID,
			SwapHash:               row.SwapHash,
			Preimage:               row.Preimage,
			InitiationTime:         row.InitiationTime,
			AmountRequested:        row.AmountRequested,
			CltvExpiry:             row.CltvExpiry,
			MaxMinerFee:            row.MaxMinerFee,
			MaxSwapFee:             row.MaxSwapFee,
			InitiationHeight:       row.InitiationHeight,
			ProtocolVersion:        row.ProtocolVersion,
			Label:                  row.Label,
			DestAddress:            row.DestAddress,
			SwapInvoice:            row.SwapInvoice,
			MaxSwapRoutingFee:      row.MaxSwapRoutingFee,
			SweepConfTarget:        row.SweepConfTarget,
			HtlcConfirmations:      row.HtlcConfirmations,
			OutgoingChanSet:        row.OutgoingChanSet,
			PrepayInvoice:          row.PrepayInvoice,
			MaxPrepayRoutingFee:    row.MaxPrepayRoutingFee,
			PublicationDeadline:    row.PublicationDeadline,
			SingleSweep:            row.SingleSweep,
			SenderScriptPubkey:     row.SenderScriptPubkey,
			ReceiverScriptPubkey:   row.ReceiverScriptPubkey,
			SenderInternalPubkey:   row.SenderInternalPubkey,
			ReceiverInternalPubkey: row.ReceiverInternalPubkey,
			ClientKeyFamily:        row.ClientKeyFamily,
			ClientKeyIndex:         row.ClientKeyIndex,
		}, updates,
	)

	return sweep, err
}

// sweepToUpsertArgs converts a Sweep struct to the arguments needed to insert.
func sweepToUpsertArgs(sweep Sweep) sqlc.UpsertSweepParams {
	return sqlc.UpsertSweepParams{
		SwapHash:      sweep.SwapHash[:],
		BatchID:       sweep.BatchID,
		OutpointTxid:  sweep.Outpoint.Hash[:],
		OutpointIndex: int32(sweep.Outpoint.Index),
		Amt:           int64(sweep.Amount),
		Completed:     sweep.Completed,
	}
}

// getSwapEvents returns a slice of LoopEvents for the swap.
func getSwapEvents(updates []sqlc.SwapUpdate) ([]*LoopEvent, error) {
	events := make([]*LoopEvent, len(updates))

	for i := 0; i < len(events); i++ {
		events[i] = &LoopEvent{
			SwapStateData: SwapStateData{
				State: SwapState(updates[i].UpdateState),
				Cost: SwapCost{
					Server:   btcutil.Amount(updates[i].ServerCost),
					Onchain:  btcutil.Amount(updates[i].OnchainCost),
					Offchain: btcutil.Amount(updates[i].OffchainCost),
				},
			},
			Time: updates[i].UpdateTimestamp.UTC(),
		}

		if updates[i].HtlcTxhash != "" {
			chainHash, err := chainhash.NewHashFromStr(updates[i].HtlcTxhash)
			if err != nil {
				return nil, err
			}

			events[i].HtlcTxHash = chainHash
		}
	}

	return events, nil
}

// convertOutgoingChanSet converts a comma separated string of channel IDs into
// a ChannelSet.
func convertOutgoingChanSet(outgoingChanSet string) (ChannelSet, error) {
	// Split the string into a slice of strings
	chanStrings := strings.Split(outgoingChanSet, ",")
	channels := make([]uint64, len(chanStrings))

	// Iterate over the chanStrings slice and convert each string to ChannelID
	for i, chanString := range chanStrings {
		chanID, err := strconv.ParseInt(chanString, 10, 64)
		if err != nil {
			return nil, err
		}
		channels[i] = uint64(chanID)
	}

	return NewChannelSet(channels)
}

// fetchHtlcKeys converts the blob encoded htlc keys into a HtlcKeys struct.
func fetchHtlcKeys(senderScriptPubkey, receiverScriptPubkey,
	senderInternalPubkey, receiverInternalPubkey []byte,
	clientKeyFamily, clientKeyIndex int32) (HtlcKeys, error) {

	senderScriptKey, err := blobTo33ByteSlice(senderScriptPubkey)
	if err != nil {
		return HtlcKeys{}, err
	}

	receiverScriptKey, err := blobTo33ByteSlice(receiverScriptPubkey)
	if err != nil {
		return HtlcKeys{}, err
	}

	htlcKeys := HtlcKeys{
		SenderScriptKey:   senderScriptKey,
		ReceiverScriptKey: receiverScriptKey,
		ClientScriptKeyLocator: keychain.KeyLocator{
			Family: keychain.KeyFamily(clientKeyFamily),
			Index:  uint32(clientKeyIndex),
		},
	}

	if senderInternalPubkey != nil {
		senderInternalPubkey, err := blobTo33ByteSlice(
			senderInternalPubkey,
		)
		if err != nil {
			return HtlcKeys{}, err
		}
		htlcKeys.SenderInternalPubKey = senderInternalPubkey
	}

	if receiverInternalPubkey != nil {
		receiverInternalPubkey, err := blobTo33ByteSlice(
			receiverInternalPubkey,
		)
		if err != nil {
			return HtlcKeys{}, err
		}
		htlcKeys.ReceiverInternalPubKey = receiverInternalPubkey
	}

	return htlcKeys, nil
}

// blobTo33ByteSlice converts a blob encoded 33 byte public key into a
// [33]byte.
func blobTo33ByteSlice(blob []byte) ([33]byte, error) {
	if len(blob) != 33 {
		return [33]byte{}, errors.New("blob is not 33 bytes")
	}

	var key [33]byte
	copy(key[:], blob)

	return key, nil
}
