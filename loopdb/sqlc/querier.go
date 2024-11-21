// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.25.0

package sqlc

import (
	"context"
)

type Querier interface {
	ConfirmBatch(ctx context.Context, id int32) error
	CreateAssetOutSwap(ctx context.Context, swapHash []byte) error
	CreateAssetSwap(ctx context.Context, arg CreateAssetSwapParams) error
	CreateReservation(ctx context.Context, arg CreateReservationParams) error
	DropBatch(ctx context.Context, id int32) error
	FetchLiquidityParams(ctx context.Context) ([]byte, error)
	GetAllAssetOutSwaps(ctx context.Context) ([]GetAllAssetOutSwapsRow, error)
	GetAssetOutSwap(ctx context.Context, swapHash []byte) (GetAssetOutSwapRow, error)
	GetBatchSweeps(ctx context.Context, batchID int32) ([]Sweep, error)
	GetBatchSweptAmount(ctx context.Context, batchID int32) (int64, error)
	GetInstantOutSwap(ctx context.Context, swapHash []byte) (GetInstantOutSwapRow, error)
	GetInstantOutSwapUpdates(ctx context.Context, swapHash []byte) ([]InstantoutUpdate, error)
	GetInstantOutSwaps(ctx context.Context) ([]GetInstantOutSwapsRow, error)
	GetLastUpdateID(ctx context.Context, swapHash []byte) (int32, error)
	GetLoopInSwap(ctx context.Context, swapHash []byte) (GetLoopInSwapRow, error)
	GetLoopInSwaps(ctx context.Context) ([]GetLoopInSwapsRow, error)
	GetLoopOutSwap(ctx context.Context, swapHash []byte) (GetLoopOutSwapRow, error)
	GetLoopOutSwaps(ctx context.Context) ([]GetLoopOutSwapsRow, error)
	GetMigration(ctx context.Context, migrationID string) (MigrationTracker, error)
	GetParentBatch(ctx context.Context, swapHash []byte) (SweepBatch, error)
	GetReservation(ctx context.Context, reservationID []byte) (Reservation, error)
	GetReservationUpdates(ctx context.Context, reservationID []byte) ([]ReservationUpdate, error)
	GetReservations(ctx context.Context) ([]Reservation, error)
	GetSwapUpdates(ctx context.Context, swapHash []byte) ([]SwapUpdate, error)
	GetSweepStatus(ctx context.Context, swapHash []byte) (bool, error)
	GetUnconfirmedBatches(ctx context.Context) ([]SweepBatch, error)
	InsertAssetSwapUpdate(ctx context.Context, arg InsertAssetSwapUpdateParams) error
	InsertBatch(ctx context.Context, arg InsertBatchParams) (int32, error)
	InsertHtlcKeys(ctx context.Context, arg InsertHtlcKeysParams) error
	InsertInstantOut(ctx context.Context, arg InsertInstantOutParams) error
	InsertInstantOutUpdate(ctx context.Context, arg InsertInstantOutUpdateParams) error
	InsertLoopIn(ctx context.Context, arg InsertLoopInParams) error
	InsertLoopOut(ctx context.Context, arg InsertLoopOutParams) error
	InsertMigration(ctx context.Context, arg InsertMigrationParams) error
	InsertReservationUpdate(ctx context.Context, arg InsertReservationUpdateParams) error
	InsertSwap(ctx context.Context, arg InsertSwapParams) error
	InsertSwapUpdate(ctx context.Context, arg InsertSwapUpdateParams) error
	OverrideSwapCosts(ctx context.Context, arg OverrideSwapCostsParams) error
	UpdateAssetSwapHtlcTx(ctx context.Context, arg UpdateAssetSwapHtlcTxParams) error
	UpdateAssetSwapOutPreimage(ctx context.Context, arg UpdateAssetSwapOutPreimageParams) error
	UpdateAssetSwapOutProof(ctx context.Context, arg UpdateAssetSwapOutProofParams) error
	UpdateAssetSwapSweepTx(ctx context.Context, arg UpdateAssetSwapSweepTxParams) error
	UpdateBatch(ctx context.Context, arg UpdateBatchParams) error
	UpdateInstantOut(ctx context.Context, arg UpdateInstantOutParams) error
	UpdateReservation(ctx context.Context, arg UpdateReservationParams) error
	UpsertLiquidityParams(ctx context.Context, params []byte) error
	UpsertSweep(ctx context.Context, arg UpsertSweepParams) error
}

var _ Querier = (*Queries)(nil)
