package wasm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/aperture/l402"
	"github.com/lightninglabs/loop"
	"github.com/lightninglabs/loop/internal/browserwallet"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/looprpc"
	"github.com/lightninglabs/loop/swapserverrpc"
	"github.com/lightninglabs/loop/swapserverrpc/restclient"
	"github.com/lightninglabs/loop/sweepbatcher"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

const (
	defaultLoopOutMaxParts    = uint32(1)
	defaultPaymentTimeout     = time.Hour
	defaultMaxPaymentRetries  = 3
	defaultRuntimeStatusQueue = 100
	stopDaemonMethod          = "/looprpc.SwapClient/StopDaemon"
)

// RuntimeConfig configures an embedded, Loop-Out-only browser runtime.
type RuntimeConfig struct {
	// DatabasePath is the SQLite database path. The WASM SQLite driver maps
	// this path to durable browser storage.
	DatabasePath string

	// Seed is the deterministic seed for the embedded wallet key ring.
	Seed []byte

	// ChainParams selects the Bitcoin network for addresses and transactions.
	ChainParams *chaincfg.Params

	// EsploraURL is the browser-reachable Esplora base URL.
	EsploraURL string

	// SwapServerURL is the browser-reachable Loop server grpc-gateway base URL.
	// It is ignored when SwapServerClient is supplied.
	SwapServerURL string

	// HTTPClient optionally supplies the browser HTTP transport shared by the
	// wallet and REST swap-server clients.
	HTTPClient *http.Client

	// L402Store optionally supplies Loop core's legacy token-store interface.
	// The grpc-gateway transport authenticates with AuthorizationSource instead,
	// so a no-token compatibility store is used when this field is nil.
	L402Store l402.Store

	// AuthorizationSource reloads the complete grpc-gateway Authorization
	// header from browser persistence. It is required for a runtime-built
	// transport.
	AuthorizationSource restclient.AuthorizationSource

	// L402ChallengeHandler pays or otherwise satisfies a grpc-gateway L402
	// challenge, durably stores the resulting Authorization value, and returns
	// it. When omitted, L402InvoicePayer and L402MaxCost configure a handler
	// backed by the runtime SQLite database.
	L402ChallengeHandler restclient.L402ChallengeHandler

	// L402InvoicePayer pays an L402 invoice with an external Lightning wallet
	// and returns its preimage. Browser hosts normally use WebLN or NWC.
	L402InvoicePayer L402InvoicePayer

	// L402MaxCost is the maximum invoice amount accepted by the built-in L402
	// challenge handler. It must be positive when L402InvoicePayer is set.
	L402MaxCost btcutil.Amount

	// SwapServerClient optionally supplies an already assembled generated
	// transport. It is useful for tests and custom browser hosts.
	SwapServerClient swapserverrpc.SwapServerClient

	// ReconnectPolicy optionally overrides Loop Out update-stream reconnects.
	ReconnectPolicy *restclient.ReconnectPolicy

	// PollInterval controls Esplora chain notification polling.
	PollInterval time.Duration

	// MinRelayFee sets the floor used for sweep transaction publication.
	MinRelayFee chainfee.SatPerKWeight

	// LoopOutMaxParts is retained for core compatibility. External-payment
	// Loop Outs do not dispatch payments from this runtime.
	LoopOutMaxParts uint32

	// TotalPaymentTimeout is retained for core compatibility. It is unused by
	// the external-payment path.
	TotalPaymentTimeout time.Duration

	// MaxPaymentRetries is retained for core compatibility. It is unused by
	// the external-payment path.
	MaxPaymentRetries int

	// BufconnBufferSize overrides the in-process gRPC listener buffer size.
	BufconnBufferSize int
}

// runtimeClient extends the RPC surface with the core lifecycle methods.
type runtimeClient interface {
	loopOutRPCClient

	Run(context.Context, chan<- loop.SwapInfo) error
	WaitForInitialized(context.Context) error
}

// Runtime owns the embedded Loop client, its SQLite store, and the in-process
// gRPC endpoint.
type Runtime struct {
	client  runtimeClient
	rpc     looprpc.SwapClientClient
	conn    *grpc.ClientConn
	server  *BufconnServer
	db      *sql.DB
	seed    []byte
	network *chaincfg.Params

	cancel      context.CancelFunc
	done        chan struct{}
	updatesDone chan struct{}
	cleanup     func()

	errMu sync.RWMutex
	err   error

	recoveryMu sync.Mutex
}

// noTokenStore satisfies the legacy core dependency when transport-level
// grpc-gateway authorization is injected independently.
type noTokenStore struct{}

func (noTokenStore) CurrentToken() (*l402.Token, error) {
	return nil, l402.ErrNoToken
}

func (noTokenStore) AllTokens() (map[string]*l402.Token, error) {
	return map[string]*l402.Token{}, nil
}

func (noTokenStore) StoreToken(*l402.Token) error {
	return errors.New(
		"legacy L402 tokens are unavailable with grpc-gateway authorization",
	)
}

func (noTokenStore) RemovePendingToken() error {
	return l402.ErrNoToken
}

type stopRPCContextKey struct{}

// deferredRPCStop waits until gRPC has finished sending the StopDaemon
// response before canceling the runtime. This prevents shutdown from closing
// bufconn underneath the response that requested it.
type deferredRPCStop struct {
	requested atomic.Bool
	stopOnce  sync.Once
	stop      func()
}

func (s *deferredRPCStop) request() {
	s.requested.Store(true)
}

func (s *deferredRPCStop) TagRPC(ctx context.Context,
	info *stats.RPCTagInfo) context.Context {

	if info.FullMethodName == stopDaemonMethod {
		return context.WithValue(ctx, stopRPCContextKey{}, true)
	}

	return ctx
}

func (s *deferredRPCStop) HandleRPC(ctx context.Context,
	rpcStats stats.RPCStats) {

	_, isStopRPC := ctx.Value(stopRPCContextKey{}).(bool)
	end, isEnd := rpcStats.(*stats.End)
	if !isStopRPC || !isEnd || end.Error != nil || !s.requested.Load() {
		return
	}

	s.stopOnce.Do(s.stop)
}

func (s *deferredRPCStop) TagConn(ctx context.Context,
	_ *stats.ConnTagInfo) context.Context {

	return ctx
}

func (s *deferredRPCStop) HandleConn(context.Context, stats.ConnStats) {}

// Start assembles and starts an embedded Loop Out runtime. It returns only
// after Loop has observed the chain tip and handed all persisted swaps to its
// executor.
func Start(ctx context.Context, config RuntimeConfig) (*Runtime, error) {
	if ctx == nil {
		return nil, errors.New("startup context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateRuntimeConfig(config); err != nil {
		return nil, err
	}
	config = runtimeDefaults(config)

	store, err := loopdb.NewSqliteStore(&loopdb.SqliteConfig{
		DatabaseFileName: config.DatabasePath,
	}, config.ChainParams)
	if err != nil {
		return nil, fmt.Errorf("open Loop SQLite store: %w", err)
	}
	closeStore := func() {
		_ = store.Close()
	}

	if err := validatePersistedSwaps(ctx, store); err != nil {
		closeStore()

		return nil, err
	}

	authorizationStore, err := NewAuthorizationStore(ctx, store.DB)
	if err != nil {
		closeStore()

		return nil, fmt.Errorf("open L402 authorization store: %w", err)
	}
	if config.SwapServerClient == nil {
		if err := authorizationStore.BindOrigin(
			ctx, config.SwapServerURL,
		); err != nil {
			closeStore()

			return nil, fmt.Errorf(
				"bind L402 authorization origin: %w", err,
			)
		}
	}

	keyIndexes, err := browserwallet.NewSQLKeyIndexStore(ctx, store.DB)
	if err != nil {
		closeStore()

		return nil, fmt.Errorf("open wallet key index store: %w", err)
	}
	if err := validatePersistedSwapKeys(
		ctx, store, config.Seed, config.ChainParams, keyIndexes,
	); err != nil {
		closeStore()

		return nil, err
	}
	if err := keyIndexes.BindWallet(
		ctx, config.Seed, config.ChainParams,
	); err != nil {
		closeStore()

		return nil, fmt.Errorf("validate browser wallet identity: %w", err)
	}
	walletServices, err := browserwallet.NewServices(browserwallet.Config{
		Seed:         append([]byte(nil), config.Seed...),
		ChainParams:  config.ChainParams,
		KeyIndexes:   keyIndexes,
		EsploraURL:   config.EsploraURL,
		HTTPClient:   config.HTTPClient,
		PollInterval: config.PollInterval,
		MinRelayFee:  config.MinRelayFee,
	})
	if err != nil {
		closeStore()

		return nil, fmt.Errorf("create browser wallet: %w", err)
	}

	swapServerClient := config.SwapServerClient
	if swapServerClient == nil {
		authorizationSource := config.AuthorizationSource
		if authorizationSource == nil {
			authorizationSource = authorizationStore.LoadAuthorization
		}

		challengeHandler := config.L402ChallengeHandler
		if challengeHandler == nil && config.L402InvoicePayer != nil {
			challengeHandler, err = NewL402ChallengeHandler(
				L402ChallengeConfig{
					ChainParams: config.ChainParams,
					MaxCost:     config.L402MaxCost,
					Store:       authorizationStore,
					PayInvoice:  config.L402InvoicePayer,
				},
			)
			if err != nil {
				closeStore()

				return nil, fmt.Errorf(
					"create L402 challenge handler: %w", err,
				)
			}
		}

		restOptions := []restclient.Option{
			restclient.WithAuthorizationSource(
				authorizationSource,
			),
			restclient.WithBrowserCompatibleJSON(),
		}
		if challengeHandler != nil {
			restOptions = append(
				restOptions,
				restclient.WithL402ChallengeHandler(
					challengeHandler,
				),
			)
		}
		if config.HTTPClient != nil {
			restOptions = append(
				restOptions,
				restclient.WithHTTPClient(config.HTTPClient),
			)
		}
		if config.ReconnectPolicy != nil {
			restOptions = append(
				restOptions,
				restclient.WithReconnectPolicy(
					*config.ReconnectPolicy,
				),
			)
		}

		restClient, err := restclient.New(
			config.SwapServerURL, restOptions...,
		)
		if err != nil {
			closeStore()

			return nil, fmt.Errorf(
				"create Loop server grpc-gateway client: %w", err,
			)
		}
		swapServerClient = restClient
	}

	sweepStore := sweepbatcher.NewSQLStore(
		loopdb.NewTypedStore[sweepbatcher.Querier](store.BaseDB),
		config.ChainParams,
	)
	l402Store := config.L402Store
	if l402Store == nil {
		l402Store = noTokenStore{}
	}
	client, cleanup, err := loop.NewClient(
		filepath.Dir(config.DatabasePath), store, sweepStore,
		&loop.ClientConfig{
			SwapServerClient:    swapServerClient,
			L402Store:           l402Store,
			Lnd:                 walletServices,
			LoopOutMaxParts:     config.LoopOutMaxParts,
			TotalPaymentTimeout: config.TotalPaymentTimeout,
			MaxPaymentRetries:   config.MaxPaymentRetries,
		},
	)
	if err != nil {
		closeStore()

		return nil, fmt.Errorf("create Loop client: %w", err)
	}

	// The startup context limits initialization only. Runtime lifetime is
	// controlled explicitly by StopDaemon or Runtime.Stop.
	runCtx, runCancel := context.WithCancel(context.WithoutCancel(ctx))
	runtime := &Runtime{
		client:      client,
		db:          store.DB,
		seed:        append([]byte(nil), config.Seed...),
		network:     config.ChainParams,
		cancel:      runCancel,
		done:        make(chan struct{}),
		updatesDone: make(chan struct{}),
		cleanup:     cleanup,
	}
	stopHandler := &deferredRPCStop{stop: runCancel}
	rpcServer, err := newLoopOutRPCServer(
		client, config.ChainParams, stopHandler.request,
	)
	if err != nil {
		runCancel()
		cleanup()

		return nil, err
	}

	bufconnServer, err := StartBufconnServer(
		config.BufconnBufferSize, func(server *grpc.Server) error {
			looprpc.RegisterSwapClientServer(server, rpcServer)

			return nil
		}, grpc.StatsHandler(stopHandler),
	)
	if err != nil {
		runCancel()
		cleanup()

		return nil, fmt.Errorf("start embedded gRPC server: %w", err)
	}
	runtime.server = bufconnServer

	connection, err := bufconnServer.ClientConn()
	if err != nil {
		runCancel()
		_ = bufconnServer.Close()
		cleanup()

		return nil, fmt.Errorf("connect to embedded gRPC server: %w", err)
	}
	runtime.conn = connection
	runtime.rpc = looprpc.NewSwapClientClient(connection)

	statusUpdates := make(chan loop.SwapInfo, defaultRuntimeStatusQueue)
	go func() {
		defer close(runtime.updatesDone)
		rpcServer.runUpdates(runCtx, statusUpdates)
	}()

	initCtx, cancelInit := context.WithCancel(ctx)
	go func() {
		runErr := client.Run(runCtx, statusUpdates)
		cancelInit()
		runtime.finish(runErr)
	}()

	if err := client.WaitForInitialized(initCtx); err != nil {
		runCancel()
		<-runtime.done
		if runErr := runtime.Err(); runErr != nil &&
			!errors.Is(runErr, context.Canceled) {

			return nil, errors.Join(
				fmt.Errorf("initialize Loop client: %w", err),
				runErr,
			)
		}

		return nil, fmt.Errorf("initialize Loop client: %w", err)
	}
	cancelInit()

	return runtime, nil
}

// RPCClient returns the generated client connected to the in-process gRPC
// server.
func (r *Runtime) RPCClient() looprpc.SwapClientClient {
	if r == nil {
		return nil
	}

	return r.rpc
}

// ClientConn returns the in-process gRPC connection. Browser adapters can use
// it to construct another generated client without opening a TCP socket.
func (r *Runtime) ClientConn() *grpc.ClientConn {
	if r == nil {
		return nil
	}

	return r.conn
}

// Done is closed after the Loop client and in-process gRPC server stop.
func (r *Runtime) Done() <-chan struct{} {
	if r == nil {
		done := make(chan struct{})
		close(done)

		return done
	}

	return r.done
}

// Err returns the runtime's terminal error, if any.
func (r *Runtime) Err() error {
	if r == nil {
		return nil
	}

	r.errMu.RLock()
	defer r.errMu.RUnlock()

	return r.err
}

// Stop requests a clean runtime shutdown and waits for all owned resources to
// close or for ctx to expire.
func (r *Runtime) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("stop context is required")
	}

	r.cancel()
	select {
	case <-r.done:
		return r.Err()

	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) finish(runErr error) {
	r.cancel()
	<-r.updatesDone
	r.recoveryMu.Lock()
	defer r.recoveryMu.Unlock()

	var shutdownErrors []error
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		shutdownErrors = append(shutdownErrors, runErr)
	}
	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			shutdownErrors = append(shutdownErrors, err)
		}
	}
	if r.server != nil {
		if err := r.server.Close(); err != nil {
			shutdownErrors = append(shutdownErrors, err)
		}
	}
	if r.cleanup != nil {
		r.cleanup()
	}

	r.errMu.Lock()
	r.err = errors.Join(shutdownErrors...)
	r.errMu.Unlock()
	close(r.done)
}

func validateRuntimeConfig(config RuntimeConfig) error {
	switch {
	case strings.TrimSpace(config.DatabasePath) == "":
		return errors.New("SQLite database path is required")

	case len(config.Seed) == 0:
		return errors.New("wallet seed is required")

	case config.ChainParams == nil:
		return errors.New("chain parameters are required")

	case strings.TrimSpace(config.EsploraURL) == "":
		return errors.New("Esplora URL is required")

	}

	if config.SwapServerClient == nil &&
		strings.TrimSpace(config.SwapServerURL) == "" {

		return errors.New("Loop server grpc-gateway URL is required")
	}

	if config.PollInterval < 0 {
		return errors.New("Esplora poll interval must not be negative")
	}
	if config.MinRelayFee < 0 {
		return errors.New("minimum relay fee must not be negative")
	}
	if config.TotalPaymentTimeout < 0 {
		return errors.New("payment timeout must not be negative")
	}
	if config.MaxPaymentRetries < 0 {
		return errors.New("payment retries must not be negative")
	}
	if config.BufconnBufferSize < 0 {
		return errors.New("bufconn buffer size must not be negative")
	}
	if config.L402MaxCost < 0 {
		return errors.New("maximum L402 cost must not be negative")
	}
	if config.L402InvoicePayer != nil && config.L402MaxCost == 0 {
		return errors.New(
			"positive maximum L402 cost is required with an invoice payer",
		)
	}

	return nil
}

func runtimeDefaults(config RuntimeConfig) RuntimeConfig {
	if config.LoopOutMaxParts == 0 {
		config.LoopOutMaxParts = defaultLoopOutMaxParts
	}
	if config.TotalPaymentTimeout == 0 {
		config.TotalPaymentTimeout = defaultPaymentTimeout
	}
	if config.MaxPaymentRetries == 0 {
		config.MaxPaymentRetries = defaultMaxPaymentRetries
	}
	if config.BufconnBufferSize == 0 {
		config.BufconnBufferSize = DefaultBufconnBufferSize
	}

	return config
}

func validatePersistedSwaps(ctx context.Context,
	store *loopdb.SqliteSwapStore) error {

	loopIns, err := store.FetchLoopInSwaps(ctx)
	if err != nil {
		return fmt.Errorf("inspect persisted Loop Ins: %w", err)
	}
	for _, loopIn := range loopIns {
		if loopIn.State().State.IsPending() {
			return fmt.Errorf(
				"pending Loop In %x cannot run in the embedded "+
					"Loop-Out-only runtime",
				loopIn.Hash[:],
			)
		}
	}

	loopOuts, err := store.FetchLoopOutSwaps(ctx)
	if err != nil {
		return fmt.Errorf("inspect persisted Loop Outs: %w", err)
	}
	for _, loopOut := range loopOuts {
		if loopOut.State().State.IsPending() &&
			!loopOut.Contract.ExternalPayments {

			return fmt.Errorf(
				"pending internally paid Loop Out %x cannot run in the "+
					"embedded external-payment runtime",
				loopOut.Hash[:],
			)
		}
	}

	return nil
}
