package wasm

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/loop/swapserverrpc/restclient"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/zpay32"
	"gopkg.in/macaroon.v2"
)

const createAuthorizationTable = `
CREATE TABLE IF NOT EXISTS loop_browser_authorization (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    origin TEXT,
    authorization TEXT NOT NULL
)`

const authorizationArtifactVersion uint32 = 1

// AuthorizationStore persists the complete L402 Authorization header in the
// runtime SQLite database. The value is a reusable secret, so recovery callers
// must encrypt and authenticate the outer bundle.
type AuthorizationStore struct {
	db *sql.DB

	mu     sync.RWMutex
	origin string
}

type authorizationArtifact struct {
	Version       uint32 `json:"version"`
	Origin        string `json:"origin"`
	Authorization string `json:"authorization"`
}

// NewAuthorizationStore initializes the authorization table.
func NewAuthorizationStore(ctx context.Context,
	db *sql.DB) (*AuthorizationStore, error) {

	if db == nil {
		return nil, errors.New("authorization database is required")
	}
	if _, err := db.ExecContext(ctx, createAuthorizationTable); err != nil {
		return nil, fmt.Errorf("create authorization table: %w", err)
	}
	if err := ensureAuthorizationOriginColumn(ctx, db); err != nil {
		return nil, err
	}

	return &AuthorizationStore{db: db}, nil
}

// BindOrigin binds this store to the canonical grpc-gateway base URL that is
// allowed to receive its bearer credential. Existing credentials without an
// origin, or credentials issued for another origin, are rejected.
func (s *AuthorizationStore) BindOrigin(ctx context.Context,
	origin string) error {

	if s == nil || s.db == nil {
		return errors.New("authorization store is not initialized")
	}
	canonical, err := restclient.NormalizeBaseURL(origin)
	if err != nil {
		return fmt.Errorf("normalize authorization origin: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.origin != "" && s.origin != canonical {
		return errors.New("authorization store is already bound to another origin")
	}

	storedOrigin, _, exists, err := s.storedAuthorization(ctx)
	if err != nil {
		return err
	}
	if exists {
		if err := validateStoredOrigin(storedOrigin, canonical); err != nil {
			return err
		}
	}

	s.origin = canonical

	return nil
}

// LoadAuthorization returns the persisted Authorization header. An empty
// string means that this browser profile has not acquired an L402 yet.
func (s *AuthorizationStore) LoadAuthorization(
	ctx context.Context) (string, error) {

	if s == nil || s.db == nil {
		return "", errors.New("authorization store is not initialized")
	}

	storedOrigin, authorization, exists, err := s.storedAuthorization(ctx)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	origin, err := s.boundOrigin()
	if err != nil {
		return "", err
	}
	if err := validateStoredOrigin(storedOrigin, origin); err != nil {
		return "", err
	}
	if err := validateAuthorization(authorization); err != nil {
		return "", fmt.Errorf("load authorization: %w", err)
	}

	return authorization, nil
}

// StoreAuthorization validates and durably stores an Authorization header.
func (s *AuthorizationStore) StoreAuthorization(ctx context.Context,
	authorization string) error {

	if s == nil || s.db == nil {
		return errors.New("authorization store is not initialized")
	}
	origin, err := s.boundOrigin()
	if err != nil {
		return err
	}
	authorization = strings.TrimSpace(authorization)
	if err := validateAuthorization(authorization); err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `
	INSERT INTO loop_browser_authorization (id, origin, authorization)
	VALUES (1, ?, ?)
	ON CONFLICT(id) DO UPDATE SET authorization = excluded.authorization
	WHERE loop_browser_authorization.origin = excluded.origin
	`, origin, authorization)
	if err != nil {
		return fmt.Errorf("store authorization: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect stored authorization: %w", err)
	}
	if rowsAffected != 1 {
		return errors.New("stored authorization belongs to another origin")
	}

	return nil
}

// ClearAuthorization removes the current Authorization header.
func (s *AuthorizationStore) ClearAuthorization(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("authorization store is not initialized")
	}
	origin, err := s.boundOrigin()
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
	DELETE FROM loop_browser_authorization
	WHERE id = 1 AND origin = ?
	`, origin)
	if err != nil {
		return fmt.Errorf("clear authorization: %w", err)
	}

	return nil
}

// Dump returns the current Authorization header for a recovery artifact.
func (s *AuthorizationStore) Dump(ctx context.Context) ([]byte, error) {
	authorization, err := s.LoadAuthorization(ctx)
	if err != nil {
		return nil, err
	}
	if authorization == "" {
		return nil, errors.New("no L402 authorization is stored")
	}
	origin, err := s.boundOrigin()
	if err != nil {
		return nil, err
	}

	dump, err := json.Marshal(authorizationArtifact{
		Version:       authorizationArtifactVersion,
		Origin:        origin,
		Authorization: authorization,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal L402 authorization artifact: %w", err)
	}

	return dump, nil
}

// Restore validates and stores an Authorization recovery artifact. Raw legacy
// artifacts are accepted only when the restored database already contains the
// same origin binding.
func (s *AuthorizationStore) Restore(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return errors.New("L402 authorization artifact is empty")
	}

	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		var artifact authorizationArtifact
		if err := decoder.Decode(&artifact); err != nil {
			return fmt.Errorf("decode L402 authorization artifact: %w", err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return err
		}
		if artifact.Version != authorizationArtifactVersion {
			return fmt.Errorf(
				"unsupported L402 authorization artifact version %d",
				artifact.Version,
			)
		}
		if err := s.BindOrigin(ctx, artifact.Origin); err != nil {
			return err
		}

		return s.StoreAuthorization(ctx, artifact.Authorization)
	}

	if _, err := s.boundOrigin(); err != nil {
		storedOrigin, _, exists, loadErr := s.storedAuthorization(ctx)
		if loadErr != nil {
			return loadErr
		}
		if !exists || !storedOrigin.Valid ||
			strings.TrimSpace(storedOrigin.String) == "" {

			return errors.New(
				"legacy L402 authorization has no gateway origin binding",
			)
		}
		if bindErr := s.BindOrigin(ctx, storedOrigin.String); bindErr != nil {
			return bindErr
		}
	}

	return s.StoreAuthorization(ctx, string(data))
}

func (s *AuthorizationStore) boundOrigin() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.origin == "" {
		return "", errors.New("authorization gateway origin is not bound")
	}

	return s.origin, nil
}

func (s *AuthorizationStore) storedAuthorization(ctx context.Context) (
	sql.NullString, string, bool, error) {

	var (
		origin        sql.NullString
		authorization string
	)
	err := s.db.QueryRowContext(ctx, `
	SELECT origin, authorization
	FROM loop_browser_authorization
	WHERE id = 1
	`).Scan(&origin, &authorization)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, "", false, nil
	}
	if err != nil {
		return sql.NullString{}, "", false, fmt.Errorf(
			"load authorization: %w", err,
		)
	}

	return origin, authorization, true, nil
}

func validateStoredOrigin(stored sql.NullString, expected string) error {
	if !stored.Valid || strings.TrimSpace(stored.String) == "" {
		return errors.New(
			"stored L402 authorization has no gateway origin binding",
		)
	}
	canonical, err := restclient.NormalizeBaseURL(stored.String)
	if err != nil || canonical != stored.String {
		return errors.New("stored L402 authorization origin is not canonical")
	}
	if canonical != expected {
		return errors.New("stored L402 authorization belongs to another origin")
	}

	return nil
}

func ensureAuthorizationOriginColumn(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
	PRAGMA table_info(loop_browser_authorization)
	`)
	if err != nil {
		return fmt.Errorf("inspect authorization schema: %w", err)
	}

	hasOrigin := false
	for rows.Next() {
		var (
			columnID     int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		if err := rows.Scan(
			&columnID, &name, &columnType, &notNull, &defaultValue,
			&primaryKey,
		); err != nil {
			_ = rows.Close()

			return fmt.Errorf("inspect authorization column: %w", err)
		}
		if name == "origin" {
			hasOrigin = true
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close authorization schema rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect authorization schema rows: %w", err)
	}
	if hasOrigin {
		return nil
	}

	if _, err := db.ExecContext(ctx, `
	ALTER TABLE loop_browser_authorization ADD COLUMN origin TEXT
	`); err != nil {
		return fmt.Errorf("add authorization origin column: %w", err)
	}

	return nil
}

// L402InvoicePayer pays one L402 invoice and returns its payment preimage.
// Browser integrations normally implement this with WebLN or NWC.
type L402InvoicePayer func(context.Context, string) (lntypes.Preimage, error)

// L402ChallengeConfig configures a persistent REST challenge handler.
type L402ChallengeConfig struct {
	ChainParams *chaincfg.Params
	MaxCost     btcutil.Amount
	Store       *AuthorizationStore
	PayInvoice  L402InvoicePayer
}

// NewL402ChallengeHandler returns a challenge handler that validates the
// invoice, pays it externally, verifies the returned preimage, and persists the
// Authorization header before allowing the REST request to retry.
func NewL402ChallengeHandler(config L402ChallengeConfig) (
	restclient.L402ChallengeHandler, error) {

	switch {
	case config.ChainParams == nil:
		return nil, errors.New("L402 chain parameters are required")

	case config.MaxCost <= 0:
		return nil, errors.New("positive maximum L402 cost is required")

	case config.Store == nil:
		return nil, errors.New("L402 authorization store is required")

	case config.PayInvoice == nil:
		return nil, errors.New("L402 invoice payer is required")
	}

	return func(ctx context.Context,
		challenge restclient.L402Challenge) (string, error) {

		if err := validateChallenge(challenge); err != nil {
			return "", err
		}
		invoice, err := zpay32.Decode(
			challenge.Invoice, config.ChainParams,
		)
		if err != nil {
			return "", fmt.Errorf("decode L402 invoice: %w", err)
		}
		if invoice.MilliSat == nil {
			return "", errors.New("L402 invoice has no amount")
		}
		maxCost := lnwire.NewMSatFromSatoshis(config.MaxCost)
		if *invoice.MilliSat > maxCost {
			return "", fmt.Errorf(
				"L402 invoice cost %d msat exceeds maximum %d msat",
				*invoice.MilliSat, maxCost,
			)
		}

		preimage, err := config.PayInvoice(ctx, challenge.Invoice)
		if err != nil {
			return "", fmt.Errorf("pay L402 invoice: %w", err)
		}
		if invoice.PaymentHash == nil ||
			!preimage.Matches(lntypes.Hash(*invoice.PaymentHash)) {

			return "", errors.New(
				"L402 payer returned a mismatched preimage",
			)
		}

		authorization := fmt.Sprintf(
			"L402 %s:%s", challenge.Macaroon, preimage.String(),
		)
		if err := config.Store.StoreAuthorization(
			ctx, authorization,
		); err != nil {

			return "", fmt.Errorf("persist L402 authorization: %w", err)
		}

		return authorization, nil
	}, nil
}

func validateChallenge(challenge restclient.L402Challenge) error {
	if !strings.EqualFold(challenge.Scheme, "L402") &&
		!strings.EqualFold(challenge.Scheme, "LSAT") {

		return errors.New("unsupported authorization challenge")
	}
	if strings.TrimSpace(challenge.Macaroon) == "" {
		return errors.New("L402 challenge has no macaroon")
	}
	if strings.TrimSpace(challenge.Invoice) == "" {
		return errors.New("L402 challenge has no invoice")
	}

	return validateMacaroon(challenge.Macaroon)
}

func validateAuthorization(authorization string) error {
	if strings.ContainsAny(authorization, "\r\n") {
		return errors.New("authorization contains a newline")
	}

	scheme, credential, ok := strings.Cut(authorization, " ")
	if !ok || (!strings.EqualFold(scheme, "L402") &&
		!strings.EqualFold(scheme, "LSAT")) {

		return errors.New("invalid L402 authorization scheme")
	}
	macaroonValue, preimageValue, ok := strings.Cut(credential, ":")
	if !ok || macaroonValue == "" || preimageValue == "" {
		return errors.New("invalid L402 authorization value")
	}
	if strings.Contains(preimageValue, ":") {
		return errors.New("invalid L402 authorization value")
	}
	if err := validateMacaroon(macaroonValue); err != nil {
		return err
	}
	if _, err := lntypes.MakePreimageFromStr(preimageValue); err != nil {
		return errors.New("invalid L402 authorization preimage")
	}

	return nil
}

func validateMacaroon(encoded string) error {
	macaroonBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return errors.New("invalid L402 macaroon encoding")
	}
	mac := new(macaroon.Macaroon)
	if err := mac.UnmarshalBinary(macaroonBytes); err != nil {
		return errors.New("invalid L402 macaroon")
	}

	return nil
}
