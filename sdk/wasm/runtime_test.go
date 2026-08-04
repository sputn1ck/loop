package wasm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/aperture/l402"
	"github.com/lightninglabs/loop/looprpc"
	"github.com/lightninglabs/loop/swapserverrpc"
	"github.com/lightninglabs/loop/swapserverrpc/restclient"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type inertSwapServerClient struct {
	swapserverrpc.SwapServerClient
}

func TestRuntimeStartWaitsForInitializationAndStops(t *testing.T) {
	t.Parallel()

	var tipRequests atomic.Int32
	esplora := httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/blocks/tip/height" {
				http.NotFound(response, request)

				return
			}

			tipRequests.Add(1)
			_, err := fmt.Fprint(response, "250")
			require.NoError(t, err)
		},
	))
	defer esplora.Close()

	startupCtx, cancelStartup := contextWithTimeout(
		t, 10*time.Second,
	)
	runtime, err := Start(startupCtx, RuntimeConfig{
		DatabasePath:     filepath.Join(t.TempDir(), "loop.db"),
		Seed:             make([]byte, 32),
		ChainParams:      &chaincfg.RegressionNetParams,
		EsploraURL:       esplora.URL,
		HTTPClient:       esplora.Client(),
		SwapServerClient: &inertSwapServerClient{},
		PollInterval:     10 * time.Millisecond,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, tipRequests.Load(), int32(1))

	stopped := false
	defer func() {
		if !stopped {
			runtime.cancel()
			<-runtime.Done()
		}
	}()

	// Canceling the startup deadline does not own the running daemon's
	// lifetime. The generated client remains usable until an explicit stop.
	cancelStartup()
	infoCtx, cancelInfo := contextWithTimeout(t, 5*time.Second)
	defer cancelInfo()
	info, err := runtime.RPCClient().GetInfo(
		infoCtx, &looprpc.GetInfoRequest{},
	)
	require.NoError(t, err)
	require.Equal(t, "regtest", info.Network)
	require.Equal(t, "bufconn", info.RpcListen)

	_, err = runtime.RPCClient().LoopIn(
		infoCtx, &looprpc.LoopInRequest{},
	)
	require.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = runtime.RPCClient().StopDaemon(
		infoCtx, &looprpc.StopDaemonRequest{},
	)
	require.NoError(t, err)
	select {
	case <-runtime.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("runtime did not stop after StopDaemon")
	}
	stopped = true
	require.NoError(t, runtime.Err())

	// Stop is idempotent after StopDaemon has closed all resources.
	stopCtx, cancelStop := contextWithTimeout(t, 10*time.Second)
	defer cancelStop()
	require.NoError(t, runtime.Stop(stopCtx))
}

func TestRuntimeUsesBrowserCompatibleJSON(t *testing.T) {
	t.Parallel()

	var gatewayRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/blocks/tip/height":
				_, err := fmt.Fprint(response, "250")
				require.NoError(t, err)

			case restclient.LoopOutTermsPath:
				gatewayRequests.Add(1)
				require.Equal(
					t, "text/plain",
					request.Header.Get("Content-Type"),
				)
				response.Header().Set(
					"Content-Type", "application/json",
				)
				_, err := fmt.Fprint(
					response,
					`{"min_swap_amount":"1000",`+
						`"max_swap_amount":"1000000",`+
						`"min_cltv_delta":20,`+
						`"max_cltv_delta":100}`,
				)
				require.NoError(t, err)

			default:
				http.NotFound(response, request)
			}
		},
	))
	defer server.Close()

	startupCtx, cancelStartup := contextWithTimeout(
		t, 10*time.Second,
	)
	defer cancelStartup()
	runtime, err := Start(startupCtx, RuntimeConfig{
		DatabasePath:  filepath.Join(t.TempDir(), "loop.db"),
		Seed:          make([]byte, 32),
		ChainParams:   &chaincfg.RegressionNetParams,
		EsploraURL:    server.URL,
		SwapServerURL: server.URL,
		HTTPClient:    server.Client(),
		PollInterval:  10 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() {
		stopCtx, cancelStop := contextWithTimeout(t, 10*time.Second)
		defer cancelStop()
		require.NoError(t, runtime.Stop(stopCtx))
	}()

	terms, err := runtime.RPCClient().LoopOutTerms(
		t.Context(), &looprpc.TermsRequest{},
	)
	require.NoError(t, err)
	require.Equal(t, int64(1_000), terms.MinSwapAmount)
	require.Equal(t, int32(20), terms.MinCltvDelta)
	require.Equal(t, int32(1), gatewayRequests.Load())
}

func TestRuntimeConfigValidation(t *testing.T) {
	t.Parallel()

	valid := RuntimeConfig{
		DatabasePath:     "loop.db",
		Seed:             make([]byte, 32),
		ChainParams:      &chaincfg.RegressionNetParams,
		EsploraURL:       "https://esplora.example",
		SwapServerClient: &inertSwapServerClient{},
	}
	require.NoError(t, validateRuntimeConfig(valid))

	tests := []struct {
		name     string
		mutate   func(*RuntimeConfig)
		expected string
	}{
		{
			name: "database",
			mutate: func(config *RuntimeConfig) {
				config.DatabasePath = ""
			},
			expected: "database path",
		},
		{
			name: "seed",
			mutate: func(config *RuntimeConfig) {
				config.Seed = nil
			},
			expected: "wallet seed",
		},
		{
			name: "network",
			mutate: func(config *RuntimeConfig) {
				config.ChainParams = nil
			},
			expected: "chain parameters",
		},
		{
			name: "esplora",
			mutate: func(config *RuntimeConfig) {
				config.EsploraURL = ""
			},
			expected: "Esplora URL",
		},
		{
			name: "gateway URL",
			mutate: func(config *RuntimeConfig) {
				config.SwapServerClient = nil
			},
			expected: "grpc-gateway URL",
		},
		{
			name: "L402 payer maximum",
			mutate: func(config *RuntimeConfig) {
				config.L402InvoicePayer = func(
					context.Context, string,
				) (lntypes.Preimage, error) {

					return lntypes.Preimage{}, nil
				}
			},
			expected: "maximum L402 cost",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config := valid
			test.mutate(&config)
			err := validateRuntimeConfig(config)
			require.ErrorContains(t, err, test.expected)
		})
	}
}

func TestRuntimeDefaults(t *testing.T) {
	t.Parallel()

	config := runtimeDefaults(RuntimeConfig{})
	require.Equal(t, defaultLoopOutMaxParts, config.LoopOutMaxParts)
	require.Equal(t, defaultPaymentTimeout, config.TotalPaymentTimeout)
	require.Equal(t, defaultMaxPaymentRetries, config.MaxPaymentRetries)
	require.Equal(t, DefaultBufconnBufferSize, config.BufconnBufferSize)

	_, err := noTokenStore{}.CurrentToken()
	require.ErrorIs(t, err, l402.ErrNoToken)
}

func contextWithTimeout(t *testing.T, timeout time.Duration) (
	context.Context, context.CancelFunc) {

	t.Helper()

	return context.WithTimeout(t.Context(), timeout)
}
