//go:build !js || wasm

// ^^^ Compiles for all targets; WASM‑specific code lives later.

package swapdk

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/aezeed"
)

// Context hides the private key internals; callers only get a *Context pointer
// or an opaque integer handle (in WASM).  No method exposes the raw key bytes.
type Context struct {
	privKey  *btcec.PrivateKey
	pubKey   *btcec.PublicKey
	mnemonic string
}

// GenerateNewContext creates a fresh private key and mnemonic.
func GenerateNewContext() (*Context, error) {
	// The birthday of the seed is required, but can be set to the current
	// time for new seeds.
	birthday := time.Now()

	// A nil password is used for unencrypted seeds.
	seed, err := aezeed.New(aezeed.CipherSeedVersion, nil, birthday)
	if err != nil {
		return nil, err
	}

	// Get the mnemonic for the seed.
	mnemonic, err := seed.ToMnemonic(nil)
	if err != nil {
		return nil, err
	}

	return NewContextFromMnemonic(strings.Join(mnemonic[:], " "))
}

// NewContextFromMnemonic rebuilds the private key from an existing mnemonic.
func NewContextFromMnemonic(m string) (*Context, error) {
	words := strings.Split(m, " ")
	if len(words) != 24 {
		return nil, fmt.Errorf("mnemonic must have 24 words, has %d", len(words))
	}

	var mnemonic aezeed.Mnemonic
	copy(mnemonic[:], words)

	var password []byte
	cipherSeed, err := mnemonic.ToCipherSeed(password)
	if err != nil {
		return nil, err
	}

	masterKey, err := hdkeychain.NewMaster(cipherSeed.Entropy[:], &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}

	privKey, err := masterKey.ECPrivKey()
	if err != nil {
		return nil, err
	}

	return &Context{
		privKey:  privKey,
		pubKey:   privKey.PubKey(),
		mnemonic: m,
	}, nil
}

// Sign returns a schnorr signature over the message. The message is hashed with
// SHA256 before signing.
func (c *Context) Sign(message []byte) ([]byte, error) {
	hash := sha256.Sum256(message)
	sig, err := schnorr.Sign(c.privKey, hash[:])
	if err != nil {
		return nil, err
	}

	return sig.Serialize(), nil
}

// PublicKeyHex exposes the compressed public key (33 bytes hex‑encoded).
func (c *Context) PublicKeyHex() string {
	return hex.EncodeToString(c.pubKey.SerializeCompressed())
}

// Client is a client for the SwapDK server.
type Client struct {
	serverAddr string
	httpClient *http.Client
}

// NewClient creates a new SwapDK client.
func NewClient(serverAddr string) *Client {
	return &Client{
		serverAddr: serverAddr,
		httpClient: &http.Client{},
	}
}

// GetEvents fetches pending signing events from the server.
func (c *Client) GetEvents() ([]*SigningRequest, error) {
	resp, err := c.httpClient.Get(c.serverAddr + "/v1/swapdk/events")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var requests []*SigningRequest
	err = json.NewDecoder(resp.Body).Decode(&requests)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

// RespondToEvent responds to a signing event.
func (c *Client) RespondToEvent(id [32]byte, response *SigningResponse) error {
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.serverAddr+"/v1/swapdk/events/"+string(id[:])+"/respond", bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// GetBalance fetches the balance from the server.
func (c *Client) GetBalance() (int64, error) {
	resp, err := c.httpClient.Get(c.serverAddr + "/v1/swapdk/balance")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var balance int64
	err = json.NewDecoder(resp.Body).Decode(&balance)
	if err != nil {
		return 0, err
	}

	return balance, nil
}

// GetTransactions fetches the transactions from the server.
func (c *Client) GetTransactions() ([]string, error) {
	resp, err := c.httpClient.Get(c.serverAddr + "/v1/swapdk/transactions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var txs []string
	err = json.NewDecoder(resp.Body).Decode(&txs)
	if err != nil {
		return nil, err
	}

	return txs, nil
}
