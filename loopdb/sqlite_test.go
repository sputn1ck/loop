package loopdb

import (
	"context"
	"crypto/sha256"
	"errors"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightninglabs/loop/loopdb/sqlc"
	"github.com/lightninglabs/loop/test"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

var (
	testTime1 = time.Date(2018, time.January, 9, 14, 54, 32, 3, time.UTC)
	testTime2 = time.Date(2018, time.January, 9, 15, 02, 03, 5, time.UTC)
)

// TestSqliteLoopOutStore tests all the basic functionality of the current
// sqlite swap store.
func TestSqliteLoopOutStore(t *testing.T) {
	destAddr := test.GetDestAddr(t, 0)
	initiationTime := time.Date(2018, 11, 1, 0, 0, 0, 0, time.UTC)

	// Next, we'll make a new pending swap that we'll insert into the
	// database shortly.
	unrestrictedSwap := LoopOutContract{
		SwapContract: SwapContract{
			AmountRequested: 100,
			Preimage:        testPreimage,
			CltvExpiry:      144,
			HtlcKeys: HtlcKeys{
				SenderScriptKey:        senderKey,
				ReceiverScriptKey:      receiverKey,
				SenderInternalPubKey:   senderInternalKey,
				ReceiverInternalPubKey: receiverInternalKey,
				ClientScriptKeyLocator: keychain.KeyLocator{
					Family: 1,
					Index:  2,
				},
			},
			MaxMinerFee: 10,
			MaxSwapFee:  20,

			InitiationHeight: 99,

			InitiationTime:  initiationTime,
			ProtocolVersion: ProtocolVersionMuSig2,
		},
		MaxPrepayRoutingFee:     40,
		PrepayInvoice:           "prepayinvoice",
		DestAddr:                destAddr,
		SwapInvoice:             "swapinvoice",
		MaxSwapRoutingFee:       30,
		SweepConfTarget:         2,
		HtlcConfirmations:       2,
		SwapPublicationDeadline: initiationTime,
	}

	t.Run("no outgoing set", func(t *testing.T) {
		testSqliteLoopOutStore(t, &unrestrictedSwap)
	})

	restrictedSwap := unrestrictedSwap
	restrictedSwap.OutgoingChanSet = ChannelSet{1, 2}

	t.Run("two channel outgoing set", func(t *testing.T) {
		testSqliteLoopOutStore(t, &restrictedSwap)
	})

	labelledSwap := unrestrictedSwap
	labelledSwap.Label = "test label"
	t.Run("labelled swap", func(t *testing.T) {
		testSqliteLoopOutStore(t, &labelledSwap)
	})
}

// testSqliteLoopOutStore tests the basic functionality of the current sqlite
// swap store for specific swap parameters.
func testSqliteLoopOutStore(t *testing.T, pendingSwap *LoopOutContract) {
	store := NewTestDB(t)

	ctxb := context.Background()

	// First, verify that an empty database has no active swaps.
	swaps, err := store.FetchLoopOutSwaps(ctxb)

	require.NoError(t, err)
	require.Empty(t, swaps)

	hash := pendingSwap.Preimage.Hash()

	// checkSwap is a test helper function that'll assert the state of a
	// swap.
	checkSwap := func(expectedState SwapState) {
		t.Helper()

		swaps, err := store.FetchLoopOutSwaps(ctxb)
		require.NoError(t, err)

		require.Len(t, swaps, 1)

		swap, err := store.FetchLoopOutSwap(ctxb, hash)
		require.NoError(t, err)

		require.Equal(t, hash, swap.Hash)
		require.Equal(t, hash, swaps[0].Hash)

		swapContract := swap.Contract

		require.Equal(t, swapContract, pendingSwap)

		require.Equal(t, expectedState, swap.State().State)

		if expectedState == StatePreimageRevealed {
			require.NotNil(t, swap.State().HtlcTxHash)
		}
	}

	// If we create a new swap, then it should show up as being initialized
	// right after.
	err = store.CreateLoopOut(ctxb, hash, pendingSwap)
	require.NoError(t, err)

	checkSwap(StateInitiated)

	// Trying to make the same swap again should result in an error.
	err = store.CreateLoopOut(ctxb, hash, pendingSwap)
	require.Error(t, err)
	checkSwap(StateInitiated)

	// Next, we'll update to the next state of the pre-image being
	// revealed. The state should be reflected here again.
	err = store.UpdateLoopOut(
		ctxb, hash, testTime,
		SwapStateData{
			State:      StatePreimageRevealed,
			HtlcTxHash: &chainhash.Hash{1, 6, 2},
		},
	)
	require.NoError(t, err)

	checkSwap(StatePreimageRevealed)

	// Next, we'll update to the final state to ensure that the state is
	// properly updated.
	err = store.UpdateLoopOut(
		ctxb, hash, testTime,
		SwapStateData{
			State: StateFailInsufficientValue,
		},
	)
	require.NoError(t, err)
	checkSwap(StateFailInsufficientValue)

	err = store.Close()
	require.NoError(t, err)
}

// TestSQLliteLoopInStore tests all the basic functionality of the current
// sqlite swap store.
func TestSQLliteLoopInStore(t *testing.T) {
	initiationTime := time.Date(2018, 11, 1, 0, 0, 0, 0, time.UTC)

	// Next, we'll make a new pending swap that we'll insert into the
	// database shortly.
	lastHop := route.Vertex{1, 2, 3}

	pendingSwap := LoopInContract{
		SwapContract: SwapContract{
			AmountRequested: 100,
			Preimage:        testPreimage,
			CltvExpiry:      144,
			HtlcKeys: HtlcKeys{
				SenderScriptKey:        senderKey,
				ReceiverScriptKey:      receiverKey,
				SenderInternalPubKey:   senderInternalKey,
				ReceiverInternalPubKey: receiverInternalKey,
				ClientScriptKeyLocator: keychain.KeyLocator{
					Family: 1,
					Index:  2,
				},
			},
			MaxMinerFee:      10,
			MaxSwapFee:       20,
			InitiationHeight: 99,

			// Convert to/from unix to remove timezone, so that it
			// doesn't interfere with DeepEqual.
			InitiationTime:  initiationTime,
			ProtocolVersion: ProtocolVersionMuSig2,
		},
		HtlcConfTarget: 2,
		LastHop:        &lastHop,
		ExternalHtlc:   true,
	}

	t.Run("loop in", func(t *testing.T) {
		testSqliteLoopInStore(t, pendingSwap)
	})

	labelledSwap := pendingSwap
	labelledSwap.Label = "test label"
	t.Run("loop in with label", func(t *testing.T) {
		testSqliteLoopInStore(t, labelledSwap)
	})
}

func testSqliteLoopInStore(t *testing.T, pendingSwap LoopInContract) {
	store := NewTestDB(t)

	ctxb := context.Background()

	// First, verify that an empty database has no active swaps.
	swaps, err := store.FetchLoopInSwaps(ctxb)
	require.NoError(t, err)
	require.Empty(t, swaps)

	hash := sha256.Sum256(testPreimage[:])

	// checkSwap is a test helper function that'll assert the state of a
	// swap.
	checkSwap := func(expectedState SwapState) {
		t.Helper()

		swaps, err := store.FetchLoopInSwaps(ctxb)
		require.NoError(t, err)
		require.Len(t, swaps, 1)

		swap := swaps[0].Contract

		require.Equal(t, swap, &pendingSwap)

		require.Equal(t, swaps[0].State().State, expectedState)
	}

	// If we create a new swap, then it should show up as being initialized
	// right after.
	err = store.CreateLoopIn(ctxb, hash, &pendingSwap)
	require.NoError(t, err)

	checkSwap(StateInitiated)

	// Trying to make the same swap again should result in an error.
	err = store.CreateLoopIn(ctxb, hash, &pendingSwap)
	require.Error(t, err)

	checkSwap(StateInitiated)

	// Next, we'll update to the next state of the pre-image being
	// revealed. The state should be reflected here again.
	err = store.UpdateLoopIn(
		ctxb, hash, testTime,
		SwapStateData{
			State: StatePreimageRevealed,
		},
	)
	require.NoError(t, err)

	checkSwap(StatePreimageRevealed)

	// Next, we'll update to the final state to ensure that the state is
	// properly updated.
	err = store.UpdateLoopIn(
		ctxb, hash, testTime,
		SwapStateData{
			State: StateFailInsufficientValue,
		},
	)
	require.NoError(t, err)
	checkSwap(StateFailInsufficientValue)

	err = store.Close()
	require.NoError(t, err)
}

// TestLiquidityParams checks that reading and writing to liquidty bucket are
// as expected.
func TestSqliteLiquidityParams(t *testing.T) {

	ctxb := context.Background()

	store := NewTestDB(t)

	// Test when there's no params saved before, an empty bytes is
	// returned.
	params, err := store.FetchLiquidityParams(ctxb)
	require.NoError(t, err, "failed to fetch params")
	require.Empty(t, params, "expect empty bytes")
	require.Nil(t, params, "expected nil byte array")

	params = []byte("test")

	// Test we can save the params.
	err = store.PutLiquidityParams(ctxb, params)
	require.NoError(t, err, "failed to put params")

	// Now fetch the db again should return the above saved bytes.
	paramsRead, err := store.FetchLiquidityParams(ctxb)
	require.NoError(t, err, "failed to fetch params")
	require.Equal(t, params, paramsRead, "unexpected return value")
}

// TestSqliteTypeConversion is a small test that checks that we can safely
// convert between the :one and :many types from sqlc.
func TestSqliteTypeConversion(t *testing.T) {
	loopOutSwapRow := sqlc.GetLoopOutSwapRow{}
	randomStruct(&loopOutSwapRow)
	require.NotNil(t, loopOutSwapRow.DestAddress)

	loopOutSwapsRow := sqlc.GetLoopOutSwapsRow(loopOutSwapRow)
	require.EqualValues(t, loopOutSwapRow, loopOutSwapsRow)

}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func randomBytes(length int) []byte {
	b := make([]byte, length)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	return b
}

func randomStruct(v interface{}) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return errors.New("Input should be a pointer to a struct type")
	}

	val = val.Elem()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		switch field.Kind() {
		case reflect.Int64:
			if field.CanSet() {
				field.SetInt(rand.Int63())
			}
		case reflect.String:
			if field.CanSet() {
				field.SetString(randomString(10))
			}
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.Uint8 {
				if field.CanSet() {
					field.SetBytes(randomBytes(32))
				}
			}
		case reflect.Struct:
			if field.Type() == reflect.TypeOf(time.Time{}) {
				if field.CanSet() {
					field.Set(reflect.ValueOf(time.Now()))
				}
			}
			if field.Type() == reflect.TypeOf(route.Vertex{}) {
				if field.CanSet() {
					field.Set(reflect.ValueOf(route.Vertex{1, 2, 3}))
				}
			}

		}
	}

	return nil
}
