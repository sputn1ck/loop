package browserwallet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestEsploraClient exercises the exact REST encodings used by a browser Loop
// Out, including natural-order script hashes and broadcast verification.
func TestEsploraClient(t *testing.T) {
	t.Parallel()

	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1},
			Index: 2,
		},
	})
	pkScript := []byte{0x00, 0x14, 1, 2, 3}
	tx.AddTxOut(&wire.TxOut{Value: 10_000, PkScript: pkScript})

	var raw bytes.Buffer
	require.NoError(t, tx.Serialize(&raw))
	txHex := hex.EncodeToString(raw.Bytes())
	txid := tx.TxHash()
	blockHash := chainhash.Hash{2}

	mux := http.NewServeMux()
	mux.HandleFunc("/blocks/tip/height", func(w http.ResponseWriter,
		_ *http.Request) {

		_, _ = w.Write([]byte("101\n"))
	})
	mux.HandleFunc("/blocks/tip/hash", func(w http.ResponseWriter,
		_ *http.Request) {

		_, _ = w.Write([]byte(blockHash.String()))
	})
	mux.HandleFunc("/tx/"+txid.String()+"/hex", func(w http.ResponseWriter,
		_ *http.Request) {

		_, _ = w.Write([]byte(txHex))
	})
	mux.HandleFunc("/tx/"+txid.String()+"/status", func(w http.ResponseWriter,
		_ *http.Request) {

		_, _ = fmt.Fprintf(w, `{"confirmed":true,"block_height":100,`+
			`"block_hash":%q}`, blockHash.String())
	})
	mux.HandleFunc("/fee-estimates", func(w http.ResponseWriter,
		_ *http.Request) {

		_, _ = w.Write([]byte(`{"1":8.5,"6":2.1}`))
	})
	scriptHash := sha256.Sum256(pkScript)
	mux.HandleFunc(
		"/scripthash/"+hex.EncodeToString(scriptHash[:])+"/txs",
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprintf(w, `[{"txid":%q,"status":{`+
				`"confirmed":true,"block_height":100,`+
				`"block_hash":%q},"vout":[{"scriptpubkey":%q}]}]`,
				txid.String(), blockHash.String(),
				hex.EncodeToString(pkScript))
		},
	)
	mux.HandleFunc("/tx", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		_, _ = w.Write([]byte(txid.String()))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := NewEsploraClient(server.URL, server.Client())
	require.NoError(t, err)

	height, err := client.TipHeight(t.Context())
	require.NoError(t, err)
	require.Equal(t, int32(101), height)

	gotHash, err := client.TipHash(t.Context())
	require.NoError(t, err)
	require.Equal(t, blockHash, gotHash)

	gotTx, err := client.RawTransaction(t.Context(), txid)
	require.NoError(t, err)
	require.Equal(t, txid, gotTx.TxHash())

	status, err := client.TransactionStatus(t.Context(), txid)
	require.NoError(t, err)
	require.True(t, status.Confirmed)
	require.Equal(t, int32(100), status.BlockHeight)

	estimates, err := client.FeeEstimates(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2.1, estimates["6"])
	foundTxID, foundStatus, err := client.FindTransactionByScript(
		t.Context(), pkScript,
	)
	require.NoError(t, err)
	require.Equal(t, txid, *foundTxID)
	require.True(t, foundStatus.Confirmed)
	require.NoError(t, client.Broadcast(t.Context(), tx))
}
