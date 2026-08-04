package browserwallet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/keychain"
)

const (
	minSeedLen = 16
	maxSeedLen = 64

	walletCommitmentDomain = "loop-browser-wallet-seed-v1"
)

// KeyIndexStore persists the next unused index for each LND key family.
// Keeping this state outside memory prevents a browser reload from reusing a
// receiver key while an externally paid swap is still pending.
type KeyIndexStore interface {
	NextIndex(context.Context, keychain.KeyFamily) (uint32, error)
	SetNextIndex(context.Context, keychain.KeyFamily, uint32) error
}

// SQLKeyIndexStore stores key indexes in the same SQLite database as the
// browser wallet metadata.
type SQLKeyIndexStore struct {
	db *sql.DB
}

// NewSQLKeyIndexStore initializes a SQL-backed key index store.
func NewSQLKeyIndexStore(ctx context.Context, db *sql.DB) (
	*SQLKeyIndexStore, error) {

	if db == nil {
		return nil, errors.New("database is required")
	}

	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS browser_wallet_key_index (
			key_family INTEGER PRIMARY KEY,
			next_index INTEGER NOT NULL CHECK(next_index >= 0)
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create key index table: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS browser_wallet_metadata (
			id INTEGER PRIMARY KEY CHECK(id = 1),
			genesis_hash BLOB NOT NULL,
			seed_commitment BLOB NOT NULL
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create wallet metadata table: %w", err)
	}

	return &SQLKeyIndexStore{db: db}, nil
}

// BindWallet records the wallet seed and network identity on first use and
// rejects later attempts to open the database with different startup
// material. The persisted value is a one-way commitment rather than the seed.
func (s *SQLKeyIndexStore) BindWallet(ctx context.Context, seed []byte,
	params *chaincfg.Params) error {

	if len(seed) < minSeedLen || len(seed) > maxSeedLen {
		return fmt.Errorf("seed must contain between %d and %d bytes",
			minSeedLen, maxSeedLen)
	}
	if params == nil || params.GenesisHash == nil {
		return errors.New("complete chain parameters are required")
	}

	genesisHash := params.GenesisHash[:]
	commitment := seedCommitment(seed, genesisHash)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin wallet identity transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var (
		storedGenesis    []byte
		storedCommitment []byte
	)
	err = tx.QueryRowContext(ctx, `
		SELECT genesis_hash, seed_commitment
		FROM browser_wallet_metadata
		WHERE id = 1
	`).Scan(&storedGenesis, &storedCommitment)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.ExecContext(ctx, `
			INSERT INTO browser_wallet_metadata(
				id, genesis_hash, seed_commitment
			) VALUES (1, ?, ?)
		`, genesisHash, commitment)
		if err != nil {
			return fmt.Errorf("persist wallet identity: %w", err)
		}

	case err != nil:
		return fmt.Errorf("read wallet identity: %w", err)

	default:
		if subtle.ConstantTimeCompare(
			storedGenesis, genesisHash,
		) != 1 {

			return errors.New(
				"wallet database belongs to a different Bitcoin network",
			)
		}
		if subtle.ConstantTimeCompare(
			storedCommitment, commitment,
		) != 1 {

			return errors.New(
				"wallet database belongs to a different seed",
			)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit wallet identity: %w", err)
	}

	return nil
}

func seedCommitment(seed, genesisHash []byte) []byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte(walletCommitmentDomain))
	_, _ = hash.Write(genesisHash)
	_, _ = hash.Write(seed)

	return hash.Sum(nil)
}

// NextIndex returns the next unused index for a key family.
func (s *SQLKeyIndexStore) NextIndex(ctx context.Context,
	family keychain.KeyFamily) (uint32, error) {

	var index uint32
	err := s.db.QueryRowContext(ctx, `
		SELECT next_index
		FROM browser_wallet_key_index
		WHERE key_family = ?
	`, int64(family)).Scan(&index)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read key index: %w", err)
	}

	return index, nil
}

// SetNextIndex records the next unused index for a key family.
func (s *SQLKeyIndexStore) SetNextIndex(ctx context.Context,
	family keychain.KeyFamily, index uint32) error {

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO browser_wallet_key_index(key_family, next_index)
		VALUES (?, ?)
		ON CONFLICT(key_family) DO UPDATE SET next_index = excluded.next_index
	`, int64(family), int64(index))
	if err != nil {
		return fmt.Errorf("write key index: %w", err)
	}

	return nil
}

// MemoryKeyIndexStore is an in-memory key index store for tests and ephemeral
// runtimes.
type MemoryKeyIndexStore struct {
	mu      sync.Mutex
	indexes map[keychain.KeyFamily]uint32
}

// NewMemoryKeyIndexStore returns an empty in-memory key index store.
func NewMemoryKeyIndexStore() *MemoryKeyIndexStore {
	return &MemoryKeyIndexStore{
		indexes: make(map[keychain.KeyFamily]uint32),
	}
}

// NextIndex returns the next unused index for a key family.
func (s *MemoryKeyIndexStore) NextIndex(_ context.Context,
	family keychain.KeyFamily) (uint32, error) {

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.indexes[family], nil
}

// SetNextIndex records the next unused index for a key family.
func (s *MemoryKeyIndexStore) SetNextIndex(_ context.Context,
	family keychain.KeyFamily, index uint32) error {

	s.mu.Lock()
	defer s.mu.Unlock()

	s.indexes[family] = index

	return nil
}

// KeyRing derives the same m/1017'/coin_type'/family'/0/index hierarchy used
// by LND without running an LND daemon.
type KeyRing struct {
	mu       sync.Mutex
	master   *hdkeychain.ExtendedKey
	coinType uint32
	indexes  KeyIndexStore
}

// NewKeyRing creates a deterministic browser key ring from a wallet seed.
func NewKeyRing(seed []byte, params *chaincfg.Params,
	indexes KeyIndexStore) (*KeyRing, error) {

	if len(seed) < minSeedLen || len(seed) > maxSeedLen {
		return nil, fmt.Errorf("seed must contain between %d and %d bytes",
			minSeedLen, maxSeedLen)
	}
	if params == nil {
		return nil, errors.New("chain parameters are required")
	}
	if indexes == nil {
		return nil, errors.New("key index store is required")
	}

	master, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		return nil, fmt.Errorf("create master key: %w", err)
	}

	coinType := uint32(keychain.CoinTypeTestnet)
	if params.Net == chaincfg.MainNetParams.Net {
		coinType = keychain.CoinTypeBitcoin
	}

	return &KeyRing{
		master:   master,
		coinType: coinType,
		indexes:  indexes,
	}, nil
}

// DeriveNextKey derives and durably reserves the next key in a family.
func (k *KeyRing) DeriveNextKey(ctx context.Context,
	family keychain.KeyFamily) (*keychain.KeyDescriptor, error) {

	k.mu.Lock()
	defer k.mu.Unlock()

	index, err := k.indexes.NextIndex(ctx, family)
	if err != nil {
		return nil, err
	}

	desc, err := k.derive(family, index)
	if err != nil {
		return nil, err
	}

	if err := k.indexes.SetNextIndex(ctx, family, index+1); err != nil {
		return nil, err
	}

	return desc, nil
}

// DeriveKey derives a key at a stable key locator.
func (k *KeyRing) DeriveKey(_ context.Context,
	locator *keychain.KeyLocator) (*keychain.KeyDescriptor, error) {

	if locator == nil {
		return nil, errors.New("key locator is required")
	}

	return k.derive(locator.Family, locator.Index)
}

// PrivateKey resolves a descriptor to its deterministic private key. Loop's
// legacy signing call sometimes supplies only a public key, so the key ring
// scans the already-reserved portion of the Loop key family when no locator is
// present.
func (k *KeyRing) PrivateKey(ctx context.Context,
	desc *keychain.KeyDescriptor) (*btcec.PrivateKey, error) {

	if desc == nil {
		return nil, errors.New("key descriptor is required")
	}

	if desc.KeyLocator.Family != 0 || desc.KeyLocator.Index != 0 {
		privKey, err := k.derivePrivate(
			desc.KeyLocator.Family, desc.KeyLocator.Index,
		)
		if err != nil {
			return nil, err
		}
		if desc.PubKey != nil && !bytes.Equal(
			desc.PubKey.SerializeCompressed(),
			privKey.PubKey().SerializeCompressed(),
		) {

			return nil, errors.New(
				"key locator does not match public key",
			)
		}

		return privKey, nil
	}

	if desc.PubKey == nil {
		return nil, errors.New("key descriptor has no locator or public key")
	}

	// Loop currently uses key family 99 for its HTLC receiver keys. Scan
	// only keys that have already been durably reserved.
	family := keychain.KeyFamily(99)
	limit, err := k.indexes.NextIndex(ctx, family)
	if err != nil {
		return nil, err
	}

	want := desc.PubKey.SerializeCompressed()
	for index := uint32(0); index < limit; index++ {
		privKey, err := k.derivePrivate(family, index)
		if err != nil {
			return nil, err
		}

		if bytes.Equal(privKey.PubKey().SerializeCompressed(), want) {
			return privKey, nil
		}
	}

	return nil, errors.New("private key not found")
}

func (k *KeyRing) derive(family keychain.KeyFamily,
	index uint32) (*keychain.KeyDescriptor, error) {

	privKey, err := k.derivePrivate(family, index)
	if err != nil {
		return nil, err
	}

	return &keychain.KeyDescriptor{
		KeyLocator: keychain.KeyLocator{
			Family: family,
			Index:  index,
		},
		PubKey: privKey.PubKey(),
	}, nil
}

func (k *KeyRing) derivePrivate(family keychain.KeyFamily,
	index uint32) (*btcec.PrivateKey, error) {

	path := []uint32{
		keychain.BIP0043Purpose + hdkeychain.HardenedKeyStart,
		k.coinType + hdkeychain.HardenedKeyStart,
		uint32(family) + hdkeychain.HardenedKeyStart,
		0,
		index,
	}

	key := k.master
	var err error
	for _, child := range path {
		key, err = key.Derive(child)
		if err != nil {
			return nil, fmt.Errorf("derive key path: %w", err)
		}
	}

	privKey, err := key.ECPrivKey()
	if err != nil {
		return nil, fmt.Errorf("extract private key: %w", err)
	}

	return privKey, nil
}
