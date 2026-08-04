package browserwallet

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/verrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/lightningnetwork/lnd/routing/route"
)

const defaultMinRelayFee = chainfee.SatPerKWeight(250)

var errNoFeeEstimates = errors.New(
	"Esplora returned no usable fee estimates",
)

// Config defines the in-process wallet and Esplora services used by the
// Loop-out-only browser runtime.
type Config struct {
	Seed         []byte
	ChainParams  *chaincfg.Params
	KeyIndexes   KeyIndexStore
	EsploraURL   string
	HTTPClient   *http.Client
	PollInterval time.Duration
	MinRelayFee  chainfee.SatPerKWeight
}

// NewServices assembles the lndclient-compatible subset needed by Loop Out.
// It removes the external LND daemon while retaining Loop's existing adapter
// boundary during the first browser milestone.
func NewServices(cfg Config) (*lndclient.LndServices, error) {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.MinRelayFee <= 0 {
		cfg.MinRelayFee = defaultMinRelayFee
	}

	keys, err := NewKeyRing(cfg.Seed, cfg.ChainParams, cfg.KeyIndexes)
	if err != nil {
		return nil, err
	}
	esplora, err := NewEsploraClient(cfg.EsploraURL, cfg.HTTPClient)
	if err != nil {
		return nil, err
	}
	notifier, err := NewChainNotifier(esplora, cfg.PollInterval)
	if err != nil {
		return nil, err
	}
	signer, err := NewSigner(keys)
	if err != nil {
		return nil, err
	}

	walletKit := &WalletKit{
		keys:        keys,
		esplora:     esplora,
		minRelayFee: cfg.MinRelayFee,
		allowEmptyFeeEstimate: cfg.ChainParams.Net ==
			chaincfg.RegressionNetParams.Net,
	}
	router := &ExternalPaymentRouter{}
	lightning := &ExternalPaymentLightning{}

	identity, err := keys.DeriveKey(context.Background(),
		&keychain.KeyLocator{
			Family: keychain.KeyFamilyNodeKey,
			Index:  0,
		})
	if err != nil {
		return nil, err
	}

	return &lndclient.LndServices{
		Client:        lightning,
		WalletKit:     walletKit,
		ChainNotifier: notifier,
		Signer:        signer,
		Router:        router,
		ChainParams:   cfg.ChainParams,
		NodeAlias:     "browser wallet",
		NodePubkey:    route.NewVertex(identity.PubKey),
		Version: &verrpc.Version{
			AppMajor:      0,
			AppMinor:      1,
			AppPatch:      0,
			AppPreRelease: "embedded",
		},
	}, nil
}

// WalletKit implements the key, fee, and broadcast calls used by Loop Out.
type WalletKit struct {
	lndclient.WalletKitClient

	keys        *KeyRing
	esplora     *EsploraClient
	minRelayFee chainfee.SatPerKWeight

	// allowEmptyFeeEstimate permits the relay floor on regtest, where an
	// Esplora instance commonly has no fee history. Other networks fail
	// closed instead of silently underpaying a sweep.
	allowEmptyFeeEstimate bool
}

// RawClientWithMacAuth satisfies lndclient's service wrapper.
func (w *WalletKit) RawClientWithMacAuth(ctx context.Context) (
	context.Context, time.Duration, walletrpc.WalletKitClient) {

	return ctx, 0, nil
}

// DeriveNextKey derives and reserves a receiver key.
func (w *WalletKit) DeriveNextKey(ctx context.Context, family int32) (
	*keychain.KeyDescriptor, error) {

	return w.keys.DeriveNextKey(ctx, keychain.KeyFamily(family))
}

// DeriveKey derives a stable key by locator.
func (w *WalletKit) DeriveKey(ctx context.Context,
	locator *keychain.KeyLocator) (*keychain.KeyDescriptor, error) {

	return w.keys.DeriveKey(ctx, locator)
}

// EstimateFeeRate maps Esplora's sat/vbyte estimate to sat/kw.
func (w *WalletKit) EstimateFeeRate(ctx context.Context,
	confTarget int32) (chainfee.SatPerKWeight, error) {

	estimates, err := w.esplora.FeeEstimates(ctx)
	if err != nil {
		return 0, err
	}

	rate, err := feeEstimate(estimates, confTarget)
	if err != nil {
		if errors.Is(err, errNoFeeEstimates) &&
			w.allowEmptyFeeEstimate {

			return w.minRelayFee, nil
		}

		return 0, err
	}

	// A kiloweight contains 250 virtual bytes.
	satPerKWeight := chainfee.SatPerKWeight(math.Ceil(rate * 250))
	if satPerKWeight < w.minRelayFee {
		return w.minRelayFee, nil
	}

	return satPerKWeight, nil
}

// MinRelayFee returns the configured backend relay floor.
func (w *WalletKit) MinRelayFee(context.Context) (
	chainfee.SatPerKWeight, error) {

	return w.minRelayFee, nil
}

// PublishTransaction broadcasts a signed transaction over Esplora.
func (w *WalletKit) PublishTransaction(ctx context.Context,
	tx *wire.MsgTx, _ string) error {

	return w.esplora.Broadcast(ctx, tx)
}

func feeEstimate(estimates map[string]float64, confTarget int32) (
	float64, error) {

	if confTarget <= 0 {
		return 0, errors.New("confirmation target must be positive")
	}

	var (
		bestTarget   int32 = -1
		bestRate     float64
		fallback     int32 = math.MaxInt32
		fallbackRate float64
	)
	for targetText, rate := range estimates {
		if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			continue
		}

		target, err := strconv.ParseInt(targetText, 10, 32)
		if err != nil || target <= 0 {
			continue
		}
		target32 := int32(target)

		// Prefer the largest published target no greater than the
		// requested target. This is conservative when Esplora omits an
		// exact bucket.
		if target32 <= confTarget && target32 > bestTarget {
			bestTarget = target32
			bestRate = rate
		}
		if target32 < fallback {
			fallback = target32
			fallbackRate = rate
		}
	}

	if bestTarget >= 0 {
		return bestRate, nil
	}
	if fallback != math.MaxInt32 {
		return fallbackRate, nil
	}

	return 0, errNoFeeEstimates
}

// ExternalPaymentRouter fails if Loop accidentally attempts to pay or track a
// Lightning invoice in external-payment mode.
type ExternalPaymentRouter struct {
	lndclient.RouterClient
}

// RawClientWithMacAuth satisfies lndclient's service wrapper.
func (r *ExternalPaymentRouter) RawClientWithMacAuth(ctx context.Context) (
	context.Context, time.Duration, routerrpc.RouterClient) {

	return ctx, 0, nil
}

// SendPayment rejects internal payment attempts.
func (r *ExternalPaymentRouter) SendPayment(context.Context,
	lndclient.SendPaymentRequest) (chan lndclient.PaymentStatus, chan error,
	error) {

	return nil, nil, errors.New("embedded client cannot pay invoices")
}

// TrackPayment rejects internal payment tracking attempts.
func (r *ExternalPaymentRouter) TrackPayment(context.Context,
	lntypes.Hash) (chan lndclient.PaymentStatus, chan error, error) {

	return nil, nil, errors.New("embedded client cannot track payments")
}

// ExternalPaymentLightning is a fail-closed Lightning client for the
// externally paid Loop-out path.
type ExternalPaymentLightning struct {
	lndclient.LightningClient
}

// RawClientWithMacAuth satisfies lndclient's service wrapper.
func (l *ExternalPaymentLightning) RawClientWithMacAuth(
	ctx context.Context) (context.Context, time.Duration,
	lnrpc.LightningClient) {

	return ctx, 0, nil
}

// DecodePaymentRequest rejects accidental entry into internal payment resume.
func (l *ExternalPaymentLightning) DecodePaymentRequest(context.Context,
	string) (*lndclient.PaymentRequest, error) {

	return nil, errors.New("embedded client cannot decode Lightning payments")
}

// EstimateFee is not needed for external-address Loop Outs.
func (l *ExternalPaymentLightning) EstimateFee(context.Context,
	btcutil.Address, btcutil.Amount, int32) (btcutil.Amount, error) {

	return 0, fmt.Errorf("embedded client cannot fund transactions")
}

// Ensure the concrete adapters keep satisfying Loop's current boundary.
var (
	_ lndclient.WalletKitClient     = (*WalletKit)(nil)
	_ lndclient.ChainNotifierClient = (*ChainNotifier)(nil)
	_ lndclient.SignerClient        = (*Signer)(nil)
	_ lndclient.RouterClient        = (*ExternalPaymentRouter)(nil)
	_ lndclient.LightningClient     = (*ExternalPaymentLightning)(nil)
)
