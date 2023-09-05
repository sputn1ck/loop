// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.17.2
// source: swaps.sql

package sqlc

import (
	"context"
	"time"
)

const getLoopInSwap = `-- name: GetLoopInSwap :one
SELECT 
    swaps.id, swaps.swap_hash, swaps.preimage, swaps.initiation_time, swaps.amount_requested, swaps.cltv_expiry, swaps.max_miner_fee, swaps.max_swap_fee, swaps.initiation_height, swaps.protocol_version, swaps.label,
    loopin_swaps.swap_hash, loopin_swaps.htlc_conf_target, loopin_swaps.last_hop, loopin_swaps.external_htlc,
    htlc_keys.swap_hash, htlc_keys.sender_script_pubkey, htlc_keys.receiver_script_pubkey, htlc_keys.sender_internal_pubkey, htlc_keys.receiver_internal_pubkey, htlc_keys.client_key_family, htlc_keys.client_key_index
FROM
    swaps
JOIN
    loopin_swaps ON swaps.swap_hash = loopin_swaps.swap_hash
JOIN
    htlc_keys ON swaps.swap_hash = htlc_keys.swap_hash
WHERE
    swaps.swap_hash = $1
`

type GetLoopInSwapRow struct {
	ID                     int32
	SwapHash               []byte
	Preimage               []byte
	InitiationTime         time.Time
	AmountRequested        int64
	CltvExpiry             int32
	MaxMinerFee            int64
	MaxSwapFee             int64
	InitiationHeight       int32
	ProtocolVersion        int32
	Label                  string
	SwapHash_2             []byte
	HtlcConfTarget         int32
	LastHop                []byte
	ExternalHtlc           bool
	SwapHash_3             []byte
	SenderScriptPubkey     []byte
	ReceiverScriptPubkey   []byte
	SenderInternalPubkey   []byte
	ReceiverInternalPubkey []byte
	ClientKeyFamily        int32
	ClientKeyIndex         int32
}

func (q *Queries) GetLoopInSwap(ctx context.Context, swapHash []byte) (GetLoopInSwapRow, error) {
	row := q.db.QueryRowContext(ctx, getLoopInSwap, swapHash)
	var i GetLoopInSwapRow
	err := row.Scan(
		&i.ID,
		&i.SwapHash,
		&i.Preimage,
		&i.InitiationTime,
		&i.AmountRequested,
		&i.CltvExpiry,
		&i.MaxMinerFee,
		&i.MaxSwapFee,
		&i.InitiationHeight,
		&i.ProtocolVersion,
		&i.Label,
		&i.SwapHash_2,
		&i.HtlcConfTarget,
		&i.LastHop,
		&i.ExternalHtlc,
		&i.SwapHash_3,
		&i.SenderScriptPubkey,
		&i.ReceiverScriptPubkey,
		&i.SenderInternalPubkey,
		&i.ReceiverInternalPubkey,
		&i.ClientKeyFamily,
		&i.ClientKeyIndex,
	)
	return i, err
}

const getLoopInSwaps = `-- name: GetLoopInSwaps :many
SELECT 
    swaps.id, swaps.swap_hash, swaps.preimage, swaps.initiation_time, swaps.amount_requested, swaps.cltv_expiry, swaps.max_miner_fee, swaps.max_swap_fee, swaps.initiation_height, swaps.protocol_version, swaps.label,
    loopin_swaps.swap_hash, loopin_swaps.htlc_conf_target, loopin_swaps.last_hop, loopin_swaps.external_htlc,
    htlc_keys.swap_hash, htlc_keys.sender_script_pubkey, htlc_keys.receiver_script_pubkey, htlc_keys.sender_internal_pubkey, htlc_keys.receiver_internal_pubkey, htlc_keys.client_key_family, htlc_keys.client_key_index
FROM
    swaps
JOIN
    loopin_swaps ON swaps.swap_hash = loopin_swaps.swap_hash
JOIN
    htlc_keys ON swaps.swap_hash = htlc_keys.swap_hash
ORDER BY
    swaps.id
`

type GetLoopInSwapsRow struct {
	ID                     int32
	SwapHash               []byte
	Preimage               []byte
	InitiationTime         time.Time
	AmountRequested        int64
	CltvExpiry             int32
	MaxMinerFee            int64
	MaxSwapFee             int64
	InitiationHeight       int32
	ProtocolVersion        int32
	Label                  string
	SwapHash_2             []byte
	HtlcConfTarget         int32
	LastHop                []byte
	ExternalHtlc           bool
	SwapHash_3             []byte
	SenderScriptPubkey     []byte
	ReceiverScriptPubkey   []byte
	SenderInternalPubkey   []byte
	ReceiverInternalPubkey []byte
	ClientKeyFamily        int32
	ClientKeyIndex         int32
}

func (q *Queries) GetLoopInSwaps(ctx context.Context) ([]GetLoopInSwapsRow, error) {
	rows, err := q.db.QueryContext(ctx, getLoopInSwaps)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetLoopInSwapsRow
	for rows.Next() {
		var i GetLoopInSwapsRow
		if err := rows.Scan(
			&i.ID,
			&i.SwapHash,
			&i.Preimage,
			&i.InitiationTime,
			&i.AmountRequested,
			&i.CltvExpiry,
			&i.MaxMinerFee,
			&i.MaxSwapFee,
			&i.InitiationHeight,
			&i.ProtocolVersion,
			&i.Label,
			&i.SwapHash_2,
			&i.HtlcConfTarget,
			&i.LastHop,
			&i.ExternalHtlc,
			&i.SwapHash_3,
			&i.SenderScriptPubkey,
			&i.ReceiverScriptPubkey,
			&i.SenderInternalPubkey,
			&i.ReceiverInternalPubkey,
			&i.ClientKeyFamily,
			&i.ClientKeyIndex,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getLoopOutSwap = `-- name: GetLoopOutSwap :one
SELECT 
    swaps.id, swaps.swap_hash, swaps.preimage, swaps.initiation_time, swaps.amount_requested, swaps.cltv_expiry, swaps.max_miner_fee, swaps.max_swap_fee, swaps.initiation_height, swaps.protocol_version, swaps.label,
    loopout_swaps.swap_hash, loopout_swaps.dest_address, loopout_swaps.swap_invoice, loopout_swaps.max_swap_routing_fee, loopout_swaps.sweep_conf_target, loopout_swaps.htlc_confirmations, loopout_swaps.outgoing_chan_set, loopout_swaps.prepay_invoice, loopout_swaps.max_prepay_routing_fee, loopout_swaps.publication_deadline, loopout_swaps.single_sweep,
    htlc_keys.swap_hash, htlc_keys.sender_script_pubkey, htlc_keys.receiver_script_pubkey, htlc_keys.sender_internal_pubkey, htlc_keys.receiver_internal_pubkey, htlc_keys.client_key_family, htlc_keys.client_key_index
FROM
    swaps
JOIN
    loopout_swaps ON swaps.swap_hash = loopout_swaps.swap_hash
JOIN
    htlc_keys ON swaps.swap_hash = htlc_keys.swap_hash
WHERE
    swaps.swap_hash = $1
`

type GetLoopOutSwapRow struct {
	ID                     int32
	SwapHash               []byte
	Preimage               []byte
	InitiationTime         time.Time
	AmountRequested        int64
	CltvExpiry             int32
	MaxMinerFee            int64
	MaxSwapFee             int64
	InitiationHeight       int32
	ProtocolVersion        int32
	Label                  string
	SwapHash_2             []byte
	DestAddress            string
	SwapInvoice            string
	MaxSwapRoutingFee      int64
	SweepConfTarget        int32
	HtlcConfirmations      int32
	OutgoingChanSet        string
	PrepayInvoice          string
	MaxPrepayRoutingFee    int64
	PublicationDeadline    time.Time
	SingleSweep            bool
	SwapHash_3             []byte
	SenderScriptPubkey     []byte
	ReceiverScriptPubkey   []byte
	SenderInternalPubkey   []byte
	ReceiverInternalPubkey []byte
	ClientKeyFamily        int32
	ClientKeyIndex         int32
}

func (q *Queries) GetLoopOutSwap(ctx context.Context, swapHash []byte) (GetLoopOutSwapRow, error) {
	row := q.db.QueryRowContext(ctx, getLoopOutSwap, swapHash)
	var i GetLoopOutSwapRow
	err := row.Scan(
		&i.ID,
		&i.SwapHash,
		&i.Preimage,
		&i.InitiationTime,
		&i.AmountRequested,
		&i.CltvExpiry,
		&i.MaxMinerFee,
		&i.MaxSwapFee,
		&i.InitiationHeight,
		&i.ProtocolVersion,
		&i.Label,
		&i.SwapHash_2,
		&i.DestAddress,
		&i.SwapInvoice,
		&i.MaxSwapRoutingFee,
		&i.SweepConfTarget,
		&i.HtlcConfirmations,
		&i.OutgoingChanSet,
		&i.PrepayInvoice,
		&i.MaxPrepayRoutingFee,
		&i.PublicationDeadline,
		&i.SingleSweep,
		&i.SwapHash_3,
		&i.SenderScriptPubkey,
		&i.ReceiverScriptPubkey,
		&i.SenderInternalPubkey,
		&i.ReceiverInternalPubkey,
		&i.ClientKeyFamily,
		&i.ClientKeyIndex,
	)
	return i, err
}

const getLoopOutSwaps = `-- name: GetLoopOutSwaps :many
SELECT 
    swaps.id, swaps.swap_hash, swaps.preimage, swaps.initiation_time, swaps.amount_requested, swaps.cltv_expiry, swaps.max_miner_fee, swaps.max_swap_fee, swaps.initiation_height, swaps.protocol_version, swaps.label,
    loopout_swaps.swap_hash, loopout_swaps.dest_address, loopout_swaps.swap_invoice, loopout_swaps.max_swap_routing_fee, loopout_swaps.sweep_conf_target, loopout_swaps.htlc_confirmations, loopout_swaps.outgoing_chan_set, loopout_swaps.prepay_invoice, loopout_swaps.max_prepay_routing_fee, loopout_swaps.publication_deadline, loopout_swaps.single_sweep,
    htlc_keys.swap_hash, htlc_keys.sender_script_pubkey, htlc_keys.receiver_script_pubkey, htlc_keys.sender_internal_pubkey, htlc_keys.receiver_internal_pubkey, htlc_keys.client_key_family, htlc_keys.client_key_index
FROM 
    swaps
JOIN
    loopout_swaps ON swaps.swap_hash = loopout_swaps.swap_hash
JOIN
    htlc_keys ON swaps.swap_hash = htlc_keys.swap_hash
ORDER BY
    swaps.id
`

type GetLoopOutSwapsRow struct {
	ID                     int32
	SwapHash               []byte
	Preimage               []byte
	InitiationTime         time.Time
	AmountRequested        int64
	CltvExpiry             int32
	MaxMinerFee            int64
	MaxSwapFee             int64
	InitiationHeight       int32
	ProtocolVersion        int32
	Label                  string
	SwapHash_2             []byte
	DestAddress            string
	SwapInvoice            string
	MaxSwapRoutingFee      int64
	SweepConfTarget        int32
	HtlcConfirmations      int32
	OutgoingChanSet        string
	PrepayInvoice          string
	MaxPrepayRoutingFee    int64
	PublicationDeadline    time.Time
	SingleSweep            bool
	SwapHash_3             []byte
	SenderScriptPubkey     []byte
	ReceiverScriptPubkey   []byte
	SenderInternalPubkey   []byte
	ReceiverInternalPubkey []byte
	ClientKeyFamily        int32
	ClientKeyIndex         int32
}

func (q *Queries) GetLoopOutSwaps(ctx context.Context) ([]GetLoopOutSwapsRow, error) {
	rows, err := q.db.QueryContext(ctx, getLoopOutSwaps)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetLoopOutSwapsRow
	for rows.Next() {
		var i GetLoopOutSwapsRow
		if err := rows.Scan(
			&i.ID,
			&i.SwapHash,
			&i.Preimage,
			&i.InitiationTime,
			&i.AmountRequested,
			&i.CltvExpiry,
			&i.MaxMinerFee,
			&i.MaxSwapFee,
			&i.InitiationHeight,
			&i.ProtocolVersion,
			&i.Label,
			&i.SwapHash_2,
			&i.DestAddress,
			&i.SwapInvoice,
			&i.MaxSwapRoutingFee,
			&i.SweepConfTarget,
			&i.HtlcConfirmations,
			&i.OutgoingChanSet,
			&i.PrepayInvoice,
			&i.MaxPrepayRoutingFee,
			&i.PublicationDeadline,
			&i.SingleSweep,
			&i.SwapHash_3,
			&i.SenderScriptPubkey,
			&i.ReceiverScriptPubkey,
			&i.SenderInternalPubkey,
			&i.ReceiverInternalPubkey,
			&i.ClientKeyFamily,
			&i.ClientKeyIndex,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getSwapUpdates = `-- name: GetSwapUpdates :many
SELECT 
    id, swap_hash, update_timestamp, update_state, htlc_txhash, server_cost, onchain_cost, offchain_cost
FROM
    swap_updates
WHERE
    swap_hash = $1
ORDER BY
    id ASC
`

func (q *Queries) GetSwapUpdates(ctx context.Context, swapHash []byte) ([]SwapUpdate, error) {
	rows, err := q.db.QueryContext(ctx, getSwapUpdates, swapHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SwapUpdate
	for rows.Next() {
		var i SwapUpdate
		if err := rows.Scan(
			&i.ID,
			&i.SwapHash,
			&i.UpdateTimestamp,
			&i.UpdateState,
			&i.HtlcTxhash,
			&i.ServerCost,
			&i.OnchainCost,
			&i.OffchainCost,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const insertHtlcKeys = `-- name: InsertHtlcKeys :exec
INSERT INTO htlc_keys(
    swap_hash,
    sender_script_pubkey,
    receiver_script_pubkey,
    sender_internal_pubkey,
    receiver_internal_pubkey,
    client_key_family,
    client_key_index
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
`

type InsertHtlcKeysParams struct {
	SwapHash               []byte
	SenderScriptPubkey     []byte
	ReceiverScriptPubkey   []byte
	SenderInternalPubkey   []byte
	ReceiverInternalPubkey []byte
	ClientKeyFamily        int32
	ClientKeyIndex         int32
}

func (q *Queries) InsertHtlcKeys(ctx context.Context, arg InsertHtlcKeysParams) error {
	_, err := q.db.ExecContext(ctx, insertHtlcKeys,
		arg.SwapHash,
		arg.SenderScriptPubkey,
		arg.ReceiverScriptPubkey,
		arg.SenderInternalPubkey,
		arg.ReceiverInternalPubkey,
		arg.ClientKeyFamily,
		arg.ClientKeyIndex,
	)
	return err
}

const insertLoopIn = `-- name: InsertLoopIn :exec
INSERT INTO loopin_swaps (
    swap_hash,
    htlc_conf_target,
    last_hop,
    external_htlc
) VALUES (
    $1, $2, $3, $4
)
`

type InsertLoopInParams struct {
	SwapHash       []byte
	HtlcConfTarget int32
	LastHop        []byte
	ExternalHtlc   bool
}

func (q *Queries) InsertLoopIn(ctx context.Context, arg InsertLoopInParams) error {
	_, err := q.db.ExecContext(ctx, insertLoopIn,
		arg.SwapHash,
		arg.HtlcConfTarget,
		arg.LastHop,
		arg.ExternalHtlc,
	)
	return err
}

const insertLoopOut = `-- name: InsertLoopOut :exec
INSERT INTO loopout_swaps (
    swap_hash,
    dest_address,
    swap_invoice,
    max_swap_routing_fee,
    sweep_conf_target,
    htlc_confirmations,
    outgoing_chan_set,
    prepay_invoice,
    max_prepay_routing_fee,
    publication_deadline,
    single_sweep
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
`

type InsertLoopOutParams struct {
	SwapHash            []byte
	DestAddress         string
	SwapInvoice         string
	MaxSwapRoutingFee   int64
	SweepConfTarget     int32
	HtlcConfirmations   int32
	OutgoingChanSet     string
	PrepayInvoice       string
	MaxPrepayRoutingFee int64
	PublicationDeadline time.Time
	SingleSweep         bool
}

func (q *Queries) InsertLoopOut(ctx context.Context, arg InsertLoopOutParams) error {
	_, err := q.db.ExecContext(ctx, insertLoopOut,
		arg.SwapHash,
		arg.DestAddress,
		arg.SwapInvoice,
		arg.MaxSwapRoutingFee,
		arg.SweepConfTarget,
		arg.HtlcConfirmations,
		arg.OutgoingChanSet,
		arg.PrepayInvoice,
		arg.MaxPrepayRoutingFee,
		arg.PublicationDeadline,
		arg.SingleSweep,
	)
	return err
}

const insertSwap = `-- name: InsertSwap :exec
INSERT INTO swaps (
    swap_hash,
    preimage,
    initiation_time,
    amount_requested,
    cltv_expiry,
    max_miner_fee,
    max_swap_fee,
    initiation_height,
    protocol_version,
    label
) VALUES (
     $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
`

type InsertSwapParams struct {
	SwapHash         []byte
	Preimage         []byte
	InitiationTime   time.Time
	AmountRequested  int64
	CltvExpiry       int32
	MaxMinerFee      int64
	MaxSwapFee       int64
	InitiationHeight int32
	ProtocolVersion  int32
	Label            string
}

func (q *Queries) InsertSwap(ctx context.Context, arg InsertSwapParams) error {
	_, err := q.db.ExecContext(ctx, insertSwap,
		arg.SwapHash,
		arg.Preimage,
		arg.InitiationTime,
		arg.AmountRequested,
		arg.CltvExpiry,
		arg.MaxMinerFee,
		arg.MaxSwapFee,
		arg.InitiationHeight,
		arg.ProtocolVersion,
		arg.Label,
	)
	return err
}

const insertSwapUpdate = `-- name: InsertSwapUpdate :exec
INSERT INTO swap_updates (
    swap_hash,
    update_timestamp,
    update_state,
    htlc_txhash,
    server_cost,
    onchain_cost,
    offchain_cost
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
`

type InsertSwapUpdateParams struct {
	SwapHash        []byte
	UpdateTimestamp time.Time
	UpdateState     int32
	HtlcTxhash      string
	ServerCost      int64
	OnchainCost     int64
	OffchainCost    int64
}

func (q *Queries) InsertSwapUpdate(ctx context.Context, arg InsertSwapUpdateParams) error {
	_, err := q.db.ExecContext(ctx, insertSwapUpdate,
		arg.SwapHash,
		arg.UpdateTimestamp,
		arg.UpdateState,
		arg.HtlcTxhash,
		arg.ServerCost,
		arg.OnchainCost,
		arg.OffchainCost,
	)
	return err
}
