package wasm

import (
	"context"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/loop"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/looprpc"
	"github.com/lightninglabs/loop/swap"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeLoopOutRPCClient struct {
	loopOutRequest *loop.OutRequest
	loopOutInfo    *loop.LoopOutSwapInfo
	loopOutErr     error

	quoteRequest *loop.LoopOutQuoteRequest
	quote        *loop.LoopOutQuote
	quoteErr     error

	termsInitiator string
	terms          *loop.LoopOutTerms
	termsErr       error

	swaps    []*loop.SwapInfo
	fetchErr error
}

func (f *fakeLoopOutRPCClient) LoopOut(_ context.Context,
	request *loop.OutRequest) (*loop.LoopOutSwapInfo, error) {

	f.loopOutRequest = request

	return f.loopOutInfo, f.loopOutErr
}

func (f *fakeLoopOutRPCClient) LoopOutQuote(_ context.Context,
	request *loop.LoopOutQuoteRequest) (*loop.LoopOutQuote, error) {

	f.quoteRequest = request

	return f.quote, f.quoteErr
}

func (f *fakeLoopOutRPCClient) LoopOutTerms(_ context.Context,
	initiator string) (*loop.LoopOutTerms, error) {

	f.termsInitiator = initiator

	return f.terms, f.termsErr
}

func (f *fakeLoopOutRPCClient) FetchSwaps(context.Context) (
	[]*loop.SwapInfo, error) {

	return f.swaps, f.fetchErr
}

func TestLoopOutRPCExternalRequest(t *testing.T) {
	t.Parallel()

	params := &chaincfg.RegressionNetParams
	destination := testP2WSHAddress(t, params, 1)
	htlcAddress := testP2WSHAddress(t, params, 2)
	hash := lntypes.Hash{3}
	fake := &fakeLoopOutRPCClient{
		loopOutInfo: &loop.LoopOutSwapInfo{
			SwapHash:         hash,
			HtlcAddress:      htlcAddress,
			ServerMessage:    "pay both invoices",
			SwapInvoice:      "lnbcrt-swap",
			PrepayInvoice:    "lnbcrt-prepay",
			ExternalPayments: true,
		},
	}
	server, err := newLoopOutRPCServer(fake, params, nil)
	require.NoError(t, err)

	response, err := server.LoopOut(t.Context(), &looprpc.LoopOutRequest{
		Amt:                     100_000,
		Dest:                    destination.EncodeAddress(),
		MaxSwapRoutingFee:       10,
		MaxPrepayRoutingFee:     20,
		MaxSwapFee:              3_000,
		MaxPrepayAmt:            1_000,
		MaxMinerFee:             2_000,
		SweepConfTarget:         6,
		HtlcConfirmations:       1,
		SwapPublicationDeadline: 1_786_000_000,
		Label:                   "browser checkout",
		ExternalPayments:        true,
	})
	require.NoError(t, err)
	require.Equal(t, hash[:], response.IdBytes)
	require.Equal(t, "lnbcrt-swap", response.SwapInvoice)
	require.Equal(t, "lnbcrt-prepay", response.PrepayInvoice)
	require.True(t, response.ExternalPayments)
	require.Equal(t, htlcAddress.EncodeAddress(), response.HtlcAddressP2Wsh)

	require.NotNil(t, fake.loopOutRequest)
	require.Equal(t, btcutil.Amount(100_000), fake.loopOutRequest.Amount)
	require.Equal(t, destination, fake.loopOutRequest.DestAddr)
	require.True(t, fake.loopOutRequest.IsExternalAddr)
	require.True(t, fake.loopOutRequest.ExternalPayments)
	require.Equal(t, int32(6), fake.loopOutRequest.SweepConfTarget)
	require.Equal(t, embeddedInitiator, fake.loopOutRequest.Initiator)

	_, err = server.LoopOut(t.Context(), &looprpc.LoopOutRequest{
		Amt:  100_000,
		Dest: destination.EncodeAddress(),
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.ErrorContains(t, err, "external_payments")
}

func TestLoopOutRPCTermsAndQuote(t *testing.T) {
	t.Parallel()

	fake := &fakeLoopOutRPCClient{
		terms: &loop.LoopOutTerms{
			MinSwapAmount: 10_000,
			MaxSwapAmount: 1_000_000,
			MinCltvDelta:  20,
			MaxCltvDelta:  200,
		},
		quote: &loop.LoopOutQuote{
			SwapFee:      500,
			PrepayAmount: 100,
			MinerFee:     700,
			SwapPaymentDest: [33]byte{
				2, 1,
			},
		},
	}
	server, err := newLoopOutRPCServer(
		fake, &chaincfg.RegressionNetParams, nil,
	)
	require.NoError(t, err)

	terms, err := server.LoopOutTerms(
		t.Context(), &looprpc.TermsRequest{},
	)
	require.NoError(t, err)
	require.EqualValues(t, 10_000, terms.MinSwapAmount)
	require.EqualValues(t, 1_000_000, terms.MaxSwapAmount)
	require.Equal(t, embeddedInitiator, fake.termsInitiator)

	quote, err := server.LoopOutQuote(t.Context(), &looprpc.QuoteRequest{
		Amt:        100_000,
		ConfTarget: 0,
	})
	require.NoError(t, err)
	require.EqualValues(t, 500, quote.SwapFeeSat)
	require.EqualValues(t, 100, quote.PrepayAmtSat)
	require.EqualValues(t, 700, quote.HtlcSweepFeeSat)
	require.Equal(t, int32(loop.DefaultSweepConfTarget), quote.ConfTarget)
	require.Equal(t, int32(loop.DefaultSweepConfTarget),
		fake.quoteRequest.SweepConfTarget)

	_, err = server.LoopOutQuote(t.Context(), &looprpc.QuoteRequest{
		Amt:          100_000,
		ConfTarget:   1,
		ExternalHtlc: true,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestLoopOutRPCStoredSwapViews(t *testing.T) {
	t.Parallel()

	pending := testLoopOutSwap(t, loopdb.StateInitiated, 1)
	succeeded := testLoopOutSwap(t, loopdb.StateSuccess, 2)
	succeeded.InitiationTime = pending.InitiationTime.Add(time.Minute)
	succeeded.LastUpdate = succeeded.InitiationTime.Add(time.Minute)
	loopIn := &loop.SwapInfo{SwapType: swap.TypeIn}
	fake := &fakeLoopOutRPCClient{
		swaps: []*loop.SwapInfo{succeeded, loopIn, pending},
	}
	server, err := newLoopOutRPCServer(
		fake, &chaincfg.RegressionNetParams, nil,
	)
	require.NoError(t, err)

	list, err := server.ListSwaps(
		t.Context(), &looprpc.ListSwapsRequest{},
	)
	require.NoError(t, err)
	require.Len(t, list.Swaps, 2)
	require.Equal(t, pending.SwapHash[:], list.Swaps[0].IdBytes)
	require.Equal(t, succeeded.SwapHash[:], list.Swaps[1].IdBytes)
	require.True(t, list.Swaps[0].ExternalPayments)
	require.Equal(t, "swap-invoice", list.Swaps[0].SwapInvoice)

	pendingOnly, err := server.ListSwaps(
		t.Context(), &looprpc.ListSwapsRequest{
			ListSwapFilter: &looprpc.ListSwapsFilter{
				PendingOnly: true,
			},
		},
	)
	require.NoError(t, err)
	require.Len(t, pendingOnly.Swaps, 1)
	require.Equal(t, pending.SwapHash[:], pendingOnly.Swaps[0].IdBytes)

	info, err := server.SwapInfo(t.Context(), &looprpc.SwapInfoRequest{
		Id: succeeded.SwapHash[:],
	})
	require.NoError(t, err)
	require.Equal(t, looprpc.SwapState_SUCCESS, info.State)

	_, err = server.SwapInfo(t.Context(), &looprpc.SwapInfoRequest{
		Id: make([]byte, 32),
	})
	require.Equal(t, codes.NotFound, status.Code(err))

	getInfo, err := server.GetInfo(
		t.Context(), &looprpc.GetInfoRequest{},
	)
	require.NoError(t, err)
	require.Equal(t, "regtest", getInfo.Network)
	require.Equal(t, uint64(1), getInfo.LoopOutStats.PendingCount)
	require.Equal(t, uint64(1), getInfo.LoopOutStats.SuccessCount)
	require.Zero(t, getInfo.LoopInStats.PendingCount)
}

func TestLoopOutRPCMonitorAndUnsupportedMethods(t *testing.T) {
	t.Parallel()

	initial := testLoopOutSwap(t, loopdb.StateInitiated, 1)
	fake := &fakeLoopOutRPCClient{swaps: []*loop.SwapInfo{initial}}
	stopCalled := make(chan struct{}, 1)
	rpcServer, err := newLoopOutRPCServer(
		fake, &chaincfg.RegressionNetParams, func() {
			stopCalled <- struct{}{}
		},
	)
	require.NoError(t, err)

	updates := make(chan loop.SwapInfo, 1)
	updatesCtx, cancelUpdates := context.WithCancel(t.Context())
	defer cancelUpdates()
	go rpcServer.runUpdates(updatesCtx, updates)

	server, err := StartBufconnServer(
		DefaultBufconnBufferSize, func(server *grpc.Server) error {
			looprpc.RegisterSwapClientServer(server, rpcServer)

			return nil
		},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	connection, err := server.ClientConn()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, connection.Close())
	})
	client := looprpc.NewSwapClientClient(connection)

	monitorCtx, cancelMonitor := context.WithTimeout(
		t.Context(), 5*time.Second,
	)
	defer cancelMonitor()
	monitor, err := client.Monitor(
		monitorCtx, &looprpc.MonitorRequest{},
	)
	require.NoError(t, err)
	first, err := monitor.Recv()
	require.NoError(t, err)
	require.Equal(t, initial.SwapHash[:], first.IdBytes)

	live := *initial
	live.State = loopdb.StatePreimageRevealed
	live.LastUpdate = live.LastUpdate.Add(time.Minute)
	updates <- live
	second, err := monitor.Recv()
	require.NoError(t, err)
	require.Equal(t, looprpc.SwapState_PREIMAGE_REVEALED, second.State)

	_, err = client.LoopIn(monitorCtx, &looprpc.LoopInRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = client.StopDaemon(
		monitorCtx, &looprpc.StopDaemonRequest{},
	)
	require.NoError(t, err)
	select {
	case <-stopCalled:
	case <-monitorCtx.Done():
		t.Fatalf("stop callback was not invoked: %v", monitorCtx.Err())
	}
}

func testLoopOutSwap(t *testing.T, state loopdb.SwapState,
	hashByte byte) *loop.SwapInfo {

	t.Helper()
	address := testP2WSHAddress(t, &chaincfg.RegressionNetParams, hashByte)
	initiation := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	hash := lntypes.Hash{hashByte}

	return &loop.SwapInfo{
		SwapStateData: loopdb.SwapStateData{
			State: state,
			Cost: loopdb.SwapCost{
				Server:   100,
				Onchain:  200,
				Offchain: 0,
			},
		},
		SwapContract: loopdb.SwapContract{
			AmountRequested: 100_000,
			InitiationTime:  initiation,
			Label:           "browser checkout",
		},
		LastUpdate:       initiation.Add(time.Second),
		SwapHash:         hash,
		SwapType:         swap.TypeOut,
		HtlcAddressP2WSH: address,
		SwapInvoice:      "swap-invoice",
		PrepayInvoice:    "prepay-invoice",
		ExternalPayments: true,
	}
}

func testP2WSHAddress(t *testing.T, params *chaincfg.Params,
	fill byte) btcutil.Address {

	t.Helper()
	scriptHash := make([]byte, 32)
	for index := range scriptHash {
		scriptHash[index] = fill
	}
	address, err := btcutil.NewAddressWitnessScriptHash(scriptHash, params)
	require.NoError(t, err)

	return address
}

var _ loopOutRPCClient = (*fakeLoopOutRPCClient)(nil)
