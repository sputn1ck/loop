package loopdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"io"

	"github.com/lightninglabs/loop/loopdb/sqlc"
	"github.com/lightningnetwork/lnd/macaroons"
	"gopkg.in/macaroon-bakery.v2/bakery"
)

// MacaroonRootKey is a tuple of (id, rootKey) that is used to validate +
// create macaroons.
type MacaroonRootKey = sqlc.Macaroon

// MacaroonID is used to insert new (id, rootKey) into the database.
type MacaroonID = sqlc.InsertRootKeyParams

// RootKeyStore is an implementation of the bakery.RootKeyStore interface
// that'll be used to store macaroons for the project. This uses the
// sql.Querier interface to have access to the set of storage routines we need
// to implement the interface.
type RootKeyStore struct {
	db *BaseDB
}

// NewRootKeyStore creates a new RKS from the passed querier interface.
func NewRootKeyStore(db *BaseDB) *RootKeyStore {
	return &RootKeyStore{
		db: db,
	}
}

// Get returns the root key for the given id.
// If the item is not there, it returns ErrNotFound.
//
// NOTE: This implements the bakery.RootKeyStore interface.
func (r *RootKeyStore) Get(ctx context.Context,
	id []byte) ([]byte, error) {

	mac, err := r.db.GetRootKey(ctx, id)
	if err != nil {
		return nil, err
	}

	return mac.RootKey, nil
}

// RootKey returns the root key to be used for making a new macaroon, and an id
// that can be used to look it up later with the Get method.
//
// NOTE: This implements the bakery.RootKeyStore interface.
func (r *RootKeyStore) RootKey(ctx context.Context) ([]byte, []byte, error) {
	var (
		rootKey, id []byte
		err         error
	)

	// Create pass in the set of options to create a read/write
	// transaction, which is the default.
	var writeTxOpts SqliteTxOptions
	dbErr := r.db.ExecTx(ctx, &writeTxOpts, func(q *sqlc.Queries) error {
		// Read the root key ID from the context. If no key is
		// specified in the context, an error will be returned.
		id, err = macaroons.RootKeyIDFromContext(ctx)
		if err != nil {
			return err
		}

		// Check to see if there's a root key already stored for this
		// ID.
		mac, err := r.db.GetRootKey(ctx, id)
		switch err {
		case nil:
			rootKey = mac.RootKey
			return nil

		case sql.ErrNoRows:

		default:
			return err
		}

		// Otherwise, we'll create a new root key for this ID.
		rootKey = make([]byte, macaroons.RootKeyLen)
		if _, err := io.ReadFull(rand.Reader, rootKey); err != nil {
			return err
		}

		// Insert this new root key into the database.
		return r.db.InsertRootKey(ctx, sqlc.InsertRootKeyParams{
			ID:      id,
			RootKey: rootKey,
		})
	})
	if dbErr != nil {
		return nil, nil, dbErr
	}

	return rootKey, id, nil
}

// A compile time assertion to ensure that RootKeyStore satisfies the
// bakery.RootKeyStorage interface.
var _ bakery.RootKeyStore = (*RootKeyStore)(nil)
