package browserwallet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/stretchr/testify/require"
)

const notifierPollInterval = 5 * time.Millisecond

type spendNotifierState struct {
	sync.RWMutex

	confirmed   bool
	blockHeight int32
	blockHash   chainhash.Hash
}

func (s *spendNotifierState) setConfirmation(confirmed bool, height int32,
	blockHash chainhash.Hash) {

	s.Lock()
	defer s.Unlock()

	s.confirmed = confirmed
	s.blockHeight = height
	s.blockHash = blockHash
}

func (s *spendNotifierState) snapshot() (bool, int32, chainhash.Hash) {
	s.RLock()
	defer s.RUnlock()

	return s.confirmed, s.blockHeight, s.blockHash
}

// TestSpendNotifierWaitsForConfirmation verifies that a mempool spend is not
// delivered as a final spend and that notifier-owned channels remain open once
// the one-shot watcher exits.
func TestSpendNotifierWaitsForConfirmation(t *testing.T) {
	t.Parallel()

	state := &spendNotifierState{
		blockHeight: 100,
		blockHash:   chainhash.Hash{2},
	}
	notifier, outpoint, spendTx, polled := newSpendNotifier(
		t, state,
	)

	ctx, cancel := contextWithCancel(t)
	defer cancel()

	spends, errs, err := notifier.RegisterSpendNtfn(
		ctx, &outpoint, nil, 0,
	)
	require.NoError(t, err)

	requireSignal(t, polled)
	requireNoChannelValue(t, spends)

	state.setConfirmation(true, 100, chainhash.Hash{2})
	spend := requireChannelValue(t, spends)
	require.Equal(t, spendTx.TxHash(), *spend.SpenderTxHash)
	require.Equal(t, int32(100), spend.SpendingHeight)

	requireNoChannelValue(t, spends)
	requireNoChannelValue(t, errs)
}

// TestSpendNotifierReportsReorg verifies that a delivered spend is revoked if
// Esplora later reports it unconfirmed, then delivered again when it confirms
// in a new block. Cancellation must not close notifier result channels.
func TestSpendNotifierReportsReorg(t *testing.T) {
	t.Parallel()

	state := &spendNotifierState{
		confirmed:   true,
		blockHeight: 100,
		blockHash:   chainhash.Hash{2},
	}
	notifier, outpoint, _, _ := newSpendNotifier(t, state)

	ctx, cancel := contextWithCancel(t)
	reorgs := make(chan struct{}, 1)
	spends, errs, err := notifier.RegisterSpendNtfn(
		ctx, &outpoint, nil, 0, lndclient.WithReOrgChan(reorgs),
	)
	require.NoError(t, err)

	firstSpend := requireChannelValue(t, spends)
	require.Equal(t, int32(100), firstSpend.SpendingHeight)

	state.setConfirmation(false, 0, chainhash.Hash{})
	requireSignal(t, reorgs)
	requireNoChannelValue(t, spends)

	state.setConfirmation(true, 101, chainhash.Hash{3})
	secondSpend := requireChannelValue(t, spends)
	require.Equal(t, int32(101), secondSpend.SpendingHeight)

	cancel()
	requireNoChannelValue(t, spends)
	requireNoChannelValue(t, errs)
}

// TestConfirmationNotifierRediscoversScriptReplacement verifies that a
// script-based registration does not remain pinned to the first transaction
// it discovers. This is needed when the server replaces an HTLC funding
// transaction while retaining the same output script.
func TestConfirmationNotifierRediscoversScriptReplacement(t *testing.T) {
	t.Parallel()

	pkScript := []byte{0x00, 0x14, 1, 2, 3}
	firstTx := confirmationTestTx(chainhash.Hash{1}, pkScript)
	secondTx := confirmationTestTx(chainhash.Hash{2}, pkScript)
	firstBlock := chainhash.Hash{3}
	secondBlock := chainhash.Hash{4}

	var (
		activeTx    atomic.Int32
		scriptPolls atomic.Int32
	)
	server := newConfirmationNotifierServer(
		t, pkScript, firstTx, secondTx, firstBlock, secondBlock,
		&activeTx, &scriptPolls,
	)
	esplora, err := NewEsploraClient(server.URL, server.Client())
	require.NoError(t, err)
	notifier, err := NewChainNotifier(esplora, notifierPollInterval)
	require.NoError(t, err)

	ctx, cancel := contextWithCancel(t)
	defer cancel()

	reorgs := make(chan struct{}, 1)
	confirmations, errs, err := notifier.RegisterConfirmationsNtfn(
		ctx, nil, pkScript, 1, 0, lndclient.WithReOrgChan(reorgs),
	)
	require.NoError(t, err)

	firstConfirmation := requireChannelValue(t, confirmations)
	require.Equal(t, firstTx.TxHash(), firstConfirmation.Tx.TxHash())

	activeTx.Store(1)
	requireSignal(t, reorgs)
	secondConfirmation := requireChannelValue(t, confirmations)
	require.Equal(t, secondTx.TxHash(), secondConfirmation.Tx.TxHash())
	require.GreaterOrEqual(t, scriptPolls.Load(), int32(2))
	requireNoChannelValue(t, errs)
}

func newSpendNotifier(t *testing.T, state *spendNotifierState) (
	*ChainNotifier, wire.OutPoint, *wire.MsgTx, <-chan struct{}) {

	t.Helper()

	outpoint := wire.OutPoint{
		Hash:  chainhash.Hash{1},
		Index: 2,
	}
	spendTx := wire.NewMsgTx(2)
	spendTx.AddTxIn(&wire.TxIn{PreviousOutPoint: outpoint})
	spendTx.AddTxOut(&wire.TxOut{
		Value:    10_000,
		PkScript: []byte{0x00, 0x14, 1, 2, 3},
	})

	var raw bytes.Buffer
	require.NoError(t, spendTx.Serialize(&raw))
	spendTxHex := hex.EncodeToString(raw.Bytes())
	spendTxID := spendTx.TxHash()
	polled := make(chan struct{}, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(
		fmt.Sprintf("/tx/%s/outspend/%d", outpoint.Hash,
			outpoint.Index),
		func(w http.ResponseWriter, _ *http.Request) {
			confirmed, height, blockHash := state.snapshot()
			_, _ = fmt.Fprintf(w, `{"spent":true,"txid":%q,`+
				`"vin":0,"status":{"confirmed":%t,`+
				`"block_height":%d,"block_hash":%q}}`,
				spendTxID.String(), confirmed, height,
				blockHash.String())

			select {
			case polled <- struct{}{}:
			default:
			}
		},
	)
	mux.HandleFunc(
		"/tx/"+spendTxID.String()+"/hex",
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(spendTxHex))
		},
	)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	esplora, err := NewEsploraClient(server.URL, server.Client())
	require.NoError(t, err)
	notifier, err := NewChainNotifier(esplora, notifierPollInterval)
	require.NoError(t, err)

	return notifier, outpoint, spendTx, polled
}

func confirmationTestTx(prevHash chainhash.Hash,
	pkScript []byte) *wire.MsgTx {

	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Hash: prevHash},
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    10_000,
		PkScript: pkScript,
	})

	return tx
}

func newConfirmationNotifierServer(t *testing.T, pkScript []byte,
	firstTx, secondTx *wire.MsgTx, firstBlock, secondBlock chainhash.Hash,
	activeTx, scriptPolls *atomic.Int32) *httptest.Server {

	t.Helper()

	txHex := func(tx *wire.MsgTx) string {
		var raw bytes.Buffer
		require.NoError(t, tx.Serialize(&raw))

		return hex.EncodeToString(raw.Bytes())
	}
	firstTxHex := txHex(firstTx)
	secondTxHex := txHex(secondTx)

	mux := http.NewServeMux()
	scriptHash := sha256.Sum256(pkScript)
	mux.HandleFunc(
		"/scripthash/"+hex.EncodeToString(scriptHash[:])+"/txs",
		func(w http.ResponseWriter, _ *http.Request) {
			scriptPolls.Add(1)
			tx := firstTx
			block := firstBlock
			height := int32(100)
			if activeTx.Load() == 1 {
				tx = secondTx
				block = secondBlock
				height = 101
			}

			_, _ = fmt.Fprintf(w, `[{"txid":%q,"status":{`+
				`"confirmed":true,"block_height":%d,`+
				`"block_hash":%q},"vout":[{"scriptpubkey":%q}]}]`,
				tx.TxHash().String(), height, block.String(),
				hex.EncodeToString(pkScript))
		},
	)
	mux.HandleFunc("/blocks/tip/height", func(w http.ResponseWriter,
		_ *http.Request) {

		height := 100
		if activeTx.Load() == 1 {
			height = 101
		}
		_, _ = fmt.Fprintf(w, "%d", height)
	})
	mux.HandleFunc(
		"/tx/"+firstTx.TxHash().String()+"/status",
		func(w http.ResponseWriter, _ *http.Request) {
			confirmed := activeTx.Load() == 0
			_, _ = fmt.Fprintf(w, `{"confirmed":%t,`+
				`"block_height":100,"block_hash":%q}`,
				confirmed, firstBlock.String())
		},
	)
	mux.HandleFunc(
		"/tx/"+firstTx.TxHash().String()+"/hex",
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(firstTxHex))
		},
	)
	mux.HandleFunc(
		"/tx/"+secondTx.TxHash().String()+"/hex",
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(secondTxHex))
		},
	)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func contextWithCancel(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()

	return context.WithCancel(t.Context())
}

func requireSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("expected notifier signal")
	}
}

func requireChannelValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()

	select {
	case value, ok := <-values:
		require.True(t, ok, "notifier channel closed")

		return value

	case <-time.After(time.Second):
		var zero T
		t.Fatal("expected notifier value")

		return zero
	}
}

func requireNoChannelValue[T any](t *testing.T, values <-chan T) {
	t.Helper()

	select {
	case _, ok := <-values:
		require.True(t, ok, "notifier channel closed")
		t.Fatal("unexpected notifier value")

	case <-time.After(25 * time.Millisecond):
	}
}
