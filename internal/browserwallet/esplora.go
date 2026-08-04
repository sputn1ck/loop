package browserwallet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

const maxEsploraResponseBytes = 16 << 20

// EsploraClient provides the browser-safe chain calls needed by a Loop Out.
// Go's JS/WASM HTTP transport maps these calls directly to browser fetch.
type EsploraClient struct {
	baseURL string
	client  *http.Client
}

// TxStatus describes Esplora's confirmation status for a transaction.
type TxStatus struct {
	Confirmed   bool   `json:"confirmed"`
	BlockHeight int32  `json:"block_height"`
	BlockHash   string `json:"block_hash"`
	BlockTime   int64  `json:"block_time"`
}

// Outspend describes the transaction that spends an output.
type Outspend struct {
	Spent  bool      `json:"spent"`
	TxID   string    `json:"txid"`
	Vin    uint32    `json:"vin"`
	Status *TxStatus `json:"status"`
}

type scriptTx struct {
	TxID   string `json:"txid"`
	Status TxStatus
	Vout   []struct {
		ScriptPubKey string `json:"scriptpubkey"`
	} `json:"vout"`
}

// NewEsploraClient creates an Esplora REST client.
func NewEsploraClient(baseURL string, client *http.Client) (
	*EsploraClient, error) {

	baseURL = strings.TrimRight(baseURL, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Esplora URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("Esplora URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("Esplora URL must include a host")
	}
	if client == nil {
		client = http.DefaultClient
	}

	return &EsploraClient{
		baseURL: baseURL,
		client:  client,
	}, nil
}

// TipHeight returns the best chain height.
func (c *EsploraClient) TipHeight(ctx context.Context) (int32, error) {
	body, err := c.get(ctx, "/blocks/tip/height")
	if err != nil {
		return 0, err
	}

	height, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse tip height: %w", err)
	}

	return int32(height), nil
}

// TipHash returns the best block hash.
func (c *EsploraClient) TipHash(ctx context.Context) (chainhash.Hash, error) {
	body, err := c.get(ctx, "/blocks/tip/hash")
	if err != nil {
		return chainhash.Hash{}, err
	}

	hash, err := chainhash.NewHashFromStr(strings.TrimSpace(string(body)))
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("parse tip hash: %w", err)
	}

	return *hash, nil
}

// RawTransaction returns a hash-verified transaction.
func (c *EsploraClient) RawTransaction(ctx context.Context,
	txid chainhash.Hash) (*wire.MsgTx, error) {

	body, err := c.get(ctx, "/tx/"+txid.String()+"/hex")
	if err != nil {
		return nil, err
	}

	raw, err := hex.DecodeString(strings.TrimSpace(string(body)))
	if err != nil {
		return nil, fmt.Errorf("decode transaction: %w", err)
	}

	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("deserialize transaction: %w", err)
	}
	if tx.TxHash() != txid {
		return nil, fmt.Errorf("transaction hash mismatch: got %s, want %s",
			tx.TxHash(), txid)
	}

	return &tx, nil
}

// TransactionStatus returns the current confirmation status for a txid.
func (c *EsploraClient) TransactionStatus(ctx context.Context,
	txid chainhash.Hash) (*TxStatus, error) {

	body, err := c.get(ctx, "/tx/"+txid.String()+"/status")
	if err != nil {
		return nil, err
	}

	var status TxStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("decode transaction status: %w", err)
	}

	return &status, nil
}

// FindTransactionByScript finds a transaction that creates the given script.
// Esplora uses the natural SHA-256 hex string here, not Electrum's reversed
// display convention.
func (c *EsploraClient) FindTransactionByScript(ctx context.Context,
	pkScript []byte) (*chainhash.Hash, *TxStatus, error) {

	scriptHash := sha256.Sum256(pkScript)
	body, err := c.get(
		ctx, "/scripthash/"+hex.EncodeToString(scriptHash[:])+"/txs",
	)
	if err != nil {
		return nil, nil, err
	}

	var txs []scriptTx
	if err := json.Unmarshal(body, &txs); err != nil {
		return nil, nil, fmt.Errorf("decode script transactions: %w", err)
	}

	wantScript := hex.EncodeToString(pkScript)
	for _, tx := range txs {
		for _, output := range tx.Vout {
			if !strings.EqualFold(output.ScriptPubKey, wantScript) {
				continue
			}

			txid, err := chainhash.NewHashFromStr(tx.TxID)
			if err != nil {
				return nil, nil, fmt.Errorf("parse transaction ID: %w", err)
			}

			return txid, &tx.Status, nil
		}
	}

	return nil, nil, nil
}

// OutputSpend returns the current spend status of a transaction output.
func (c *EsploraClient) OutputSpend(ctx context.Context,
	outpoint wire.OutPoint) (*Outspend, error) {

	path := fmt.Sprintf(
		"/tx/%s/outspend/%d", outpoint.Hash, outpoint.Index,
	)
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var outspend Outspend
	if err := json.Unmarshal(body, &outspend); err != nil {
		return nil, fmt.Errorf("decode output spend: %w", err)
	}

	return &outspend, nil
}

// FeeEstimates returns confirmation targets mapped to sat/vbyte rates.
func (c *EsploraClient) FeeEstimates(ctx context.Context) (
	map[string]float64, error) {

	body, err := c.get(ctx, "/fee-estimates")
	if err != nil {
		return nil, err
	}

	var estimates map[string]float64
	if err := json.Unmarshal(body, &estimates); err != nil {
		return nil, fmt.Errorf("decode fee estimates: %w", err)
	}

	return estimates, nil
}

// Broadcast publishes a transaction and verifies the returned transaction ID.
func (c *EsploraClient) Broadcast(ctx context.Context,
	tx *wire.MsgTx) error {

	var raw bytes.Buffer
	if err := tx.Serialize(&raw); err != nil {
		return fmt.Errorf("serialize transaction: %w", err)
	}

	body, err := c.request(
		ctx, http.MethodPost, "/tx",
		strings.NewReader(hex.EncodeToString(raw.Bytes())),
	)
	if err != nil {
		return err
	}

	got := strings.TrimSpace(string(body))
	want := tx.TxHash().String()
	if got != "" && got != want {
		return fmt.Errorf("broadcast transaction ID mismatch: got %s, want %s",
			got, want)
	}

	return nil
}

func (c *EsploraClient) get(ctx context.Context, path string) ([]byte, error) {
	return c.request(ctx, http.MethodGet, path, nil)
}

func (c *EsploraClient) request(ctx context.Context, method, path string,
	body io.Reader) ([]byte, error) {

	req, err := http.NewRequestWithContext(
		ctx, method, c.baseURL+path, body,
	)
	if err != nil {
		return nil, fmt.Errorf("create Esplora request: %w", err)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "text/plain")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send Esplora request: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxEsploraResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read Esplora response: %w", err)
	}
	if len(responseBody) > maxEsploraResponseBytes {
		return nil, errors.New("Esplora response exceeds size limit")
	}
	if resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices {

		return nil, fmt.Errorf("Esplora returned %s: %s", resp.Status,
			strings.TrimSpace(string(responseBody)))
	}

	return responseBody, nil
}
