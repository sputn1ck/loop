// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.17.2

package sqlc

import (
	"context"
)

type Querier interface {
	GetRootKey(ctx context.Context, id []byte) (Macaroon, error)
	InsertRootKey(ctx context.Context, arg InsertRootKeyParams) error
}

var _ Querier = (*Queries)(nil)
