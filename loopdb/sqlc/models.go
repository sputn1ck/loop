// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.17.2

package sqlc

import (
	"database/sql"
	"time"
)

type HtlcKey struct {
	SwapHash               []byte
	SenderScriptPubkey     []byte
	ReceiverScriptPubkey   []byte
	SenderInternalPubkey   []byte
	ReceiverInternalPubkey []byte
	ClientKeyFamily        int32
	ClientKeyIndex         int32
}

type LiquidityParam struct {
	ID     int32
	Params []byte
}

type LoopinSwap struct {
	SwapHash       []byte
	HtlcConfTarget int32
	LastHop        []byte
	ExternalHtlc   bool
}

type LoopoutSwap struct {
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

type Swap struct {
	ID               int32
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

type SwapUpdate struct {
	ID              int32
	SwapHash        []byte
	UpdateTimestamp time.Time
	UpdateState     int32
	HtlcTxhash      string
	ServerCost      int64
	OnchainCost     int64
	OffchainCost    int64
}

type Sweep struct {
	ID            int32
	SwapHash      []byte
	BatchID       int32
	OutpointTxid  []byte
	OutpointIndex int32
	Amt           int64
	Completed     bool
}

type SweepBatch struct {
	ID                 int32
	State              string
	BatchTxID          sql.NullString
	BatchPkScript      []byte
	LastRbfHeight      sql.NullInt32
	LastRbfSatPerKw    sql.NullInt32
	MaxTimeoutDistance int32
}
