package swapdk

import (
	"context"
)

// SwapDKManager is an interface that defines the methods that the SwapDKService
// needs to access the static address managers.
type SwapDKManager interface {
	// GetBalance returns the balance of the static address.
	GetBalance(ctx context.Context) (int64, error)

	// GetTransactions returns a list of transactions for the static address.
	GetTransactions(ctx context.Context) ([]string, error)
}
