package wasm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/swapserverrpc/restclient"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/zpay32"
	"github.com/stretchr/testify/require"
	"gopkg.in/macaroon.v2"
)

const testGatewayOrigin = "https://loop.example/gateway"

func TestAuthorizationStore(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	db := loopdb.NewTestSqliteDB(t)
	_, err := db.DB.ExecContext(ctx, `
		CREATE TABLE loop_browser_authorization (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			authorization TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	store, err := NewAuthorizationStore(ctx, db.DB)
	require.NoError(t, err)

	authorization, err := store.LoadAuthorization(ctx)
	require.NoError(t, err)
	require.Empty(t, authorization)

	require.NoError(t, store.BindOrigin(ctx, testGatewayOrigin))

	authorization, err = store.LoadAuthorization(ctx)
	require.NoError(t, err)
	require.Empty(t, authorization)

	expected := testAuthorization(t, lntypes.Preimage{1, 2, 3})
	require.NoError(t, store.StoreAuthorization(ctx, expected))

	authorization, err = store.LoadAuthorization(ctx)
	require.NoError(t, err)
	require.Equal(t, expected, authorization)

	dump, err := store.Dump(ctx)
	require.NoError(t, err)
	require.Contains(t, string(dump), testGatewayOrigin)

	require.NoError(t, store.ClearAuthorization(ctx))
	authorization, err = store.LoadAuthorization(ctx)
	require.NoError(t, err)
	require.Empty(t, authorization)
	require.NoError(t, store.Restore(ctx, dump))

	authorization, err = store.LoadAuthorization(ctx)
	require.NoError(t, err)
	require.Equal(t, expected, authorization)
}

func TestAuthorizationStoreRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	db := loopdb.NewTestSqliteDB(t)
	store, err := NewAuthorizationStore(t.Context(), db.DB)
	require.NoError(t, err)
	require.NoError(t, store.BindOrigin(
		t.Context(), testGatewayOrigin,
	))

	require.Error(t, store.StoreAuthorization(
		t.Context(), "L402 not-a-macaroon:not-a-preimage",
	))
	require.Error(t, store.StoreAuthorization(
		t.Context(), testAuthorization(
			t, lntypes.Preimage{1},
		)+"\r\ninjected: value",
	))
}

func TestL402ChallengeHandler(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	db := loopdb.NewTestSqliteDB(t)
	store, err := NewAuthorizationStore(ctx, db.DB)
	require.NoError(t, err)
	require.NoError(t, store.BindOrigin(ctx, testGatewayOrigin))

	preimage := lntypes.Preimage{1, 2, 3, 4}
	challenge := testChallenge(t, preimage, 1_000)
	var paidInvoice string
	handler, err := NewL402ChallengeHandler(L402ChallengeConfig{
		ChainParams: &chaincfg.TestNet3Params,
		MaxCost:     2_000,
		Store:       store,
		PayInvoice: func(_ context.Context, invoice string) (
			lntypes.Preimage, error) {

			paidInvoice = invoice

			return preimage, nil
		},
	})
	require.NoError(t, err)

	authorization, err := handler(ctx, challenge)
	require.NoError(t, err)
	require.Equal(t, challenge.Invoice, paidInvoice)
	require.Equal(t,
		"L402 "+challenge.Macaroon+":"+preimage.String(),
		authorization,
	)

	persisted, err := store.LoadAuthorization(ctx)
	require.NoError(t, err)
	require.Equal(t, authorization, persisted)
}

func TestL402ChallengeHandlerRejectsPayment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxCost    btcutil.Amount
		payerValue lntypes.Preimage
	}{
		{
			name:       "cost exceeds maximum",
			maxCost:    999,
			payerValue: lntypes.Preimage{1},
		},
		{
			name:       "preimage mismatch",
			maxCost:    2_000,
			payerValue: lntypes.Preimage{9},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := loopdb.NewTestSqliteDB(t)
			store, err := NewAuthorizationStore(t.Context(), db.DB)
			require.NoError(t, err)
			require.NoError(t, store.BindOrigin(
				t.Context(), testGatewayOrigin,
			))

			challenge := testChallenge(
				t, lntypes.Preimage{1}, 1_000,
			)
			handler, err := NewL402ChallengeHandler(
				L402ChallengeConfig{
					ChainParams: &chaincfg.TestNet3Params,
					MaxCost:     testCase.maxCost,
					Store:       store,
					PayInvoice: func(context.Context, string) (
						lntypes.Preimage, error) {

						return testCase.payerValue, nil
					},
				},
			)
			require.NoError(t, err)

			_, err = handler(t.Context(), challenge)
			require.Error(t, err)
			persisted, loadErr := store.LoadAuthorization(t.Context())
			require.NoError(t, loadErr)
			require.Empty(t, persisted)
		})
	}
}

func TestAuthorizationStoreOriginBinding(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	db := loopdb.NewTestSqliteDB(t)
	store, err := NewAuthorizationStore(ctx, db.DB)
	require.NoError(t, err)
	require.NoError(t, store.BindOrigin(
		ctx, "HTTPS://LOOP.EXAMPLE:443/gateway/",
	))

	expected := testAuthorization(t, lntypes.Preimage{1, 2, 3})
	require.NoError(t, store.StoreAuthorization(ctx, expected))

	reopened, err := NewAuthorizationStore(ctx, db.DB)
	require.NoError(t, err)
	require.NoError(t, reopened.BindOrigin(ctx, testGatewayOrigin))
	authorization, err := reopened.LoadAuthorization(ctx)
	require.NoError(t, err)
	require.Equal(t, expected, authorization)

	rejected, err := NewAuthorizationStore(ctx, db.DB)
	require.NoError(t, err)
	err = rejected.BindOrigin(ctx, "https://other.example/gateway")
	require.ErrorContains(t, err, "another origin")
}

func TestAuthorizationStoreRejectsUnboundLegacyCredential(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	db := loopdb.NewTestSqliteDB(t)
	store, err := NewAuthorizationStore(ctx, db.DB)
	require.NoError(t, err)

	authorization := testAuthorization(t, lntypes.Preimage{1, 2, 3})
	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO loop_browser_authorization(id, authorization)
		VALUES (1, ?)
	`, authorization)
	require.NoError(t, err)

	err = store.BindOrigin(ctx, testGatewayOrigin)
	require.ErrorContains(t, err, "no gateway origin binding")

	freshDB := loopdb.NewTestSqliteDB(t)
	fresh, err := NewAuthorizationStore(ctx, freshDB.DB)
	require.NoError(t, err)
	err = fresh.Restore(ctx, []byte(authorization))
	require.ErrorContains(t, err, "no gateway origin binding")
}

func TestAuthorizationStoreRawRestoreUsesDatabaseOrigin(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	db := loopdb.NewTestSqliteDB(t)
	store, err := NewAuthorizationStore(ctx, db.DB)
	require.NoError(t, err)
	require.NoError(t, store.BindOrigin(ctx, testGatewayOrigin))

	authorization := testAuthorization(t, lntypes.Preimage{1, 2, 3})
	require.NoError(t, store.StoreAuthorization(ctx, authorization))

	restored, err := NewAuthorizationStore(ctx, db.DB)
	require.NoError(t, err)
	require.NoError(t, restored.Restore(ctx, []byte(authorization)))
	persisted, err := restored.LoadAuthorization(ctx)
	require.NoError(t, err)
	require.Equal(t, authorization, persisted)
}

func testChallenge(t *testing.T, preimage lntypes.Preimage,
	cost btcutil.Amount) restclient.L402Challenge {

	t.Helper()

	hash := preimage.Hash()
	invoice, err := zpay32.NewInvoice(
		&chaincfg.TestNet3Params, hash, time.Unix(1_700_000_000, 0),
		zpay32.Description("browser authorization"),
		zpay32.Amount(lnwire.NewMSatFromSatoshis(cost)),
	)
	require.NoError(t, err)
	privateKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	encodedInvoice, err := invoice.Encode(zpay32.MessageSigner{
		SignCompact: func(message []byte) ([]byte, error) {
			digest := sha256.Sum256(message)

			return ecdsa.SignCompact(privateKey, digest[:], true), nil
		},
	})
	require.NoError(t, err)

	return restclient.L402Challenge{
		Scheme:   "L402",
		Macaroon: testMacaroon(t),
		Invoice:  encodedInvoice,
	}
}

func testAuthorization(t *testing.T,
	preimage lntypes.Preimage) string {

	t.Helper()

	return "L402 " + testMacaroon(t) + ":" + preimage.String()
}

func testMacaroon(t *testing.T) string {
	t.Helper()

	mac, err := macaroon.New(
		[]byte("test root key"), []byte("test id"), "test",
		macaroon.LatestVersion,
	)
	require.NoError(t, err)
	encoded, err := mac.MarshalBinary()
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(encoded)
}
