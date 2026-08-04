package wasm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/loop"
	"github.com/lightninglabs/loop/labels"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/looprpc"
	"github.com/lightninglabs/loop/swap"
	"github.com/lightningnetwork/lnd/lntypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	embeddedInitiator       = "loop-wasm"
	minimumConfTarget       = int32(2)
	monitorSubscriberBuffer = 20
)

// loopOutRPCClient is the Loop client surface exposed by the embedded RPC
// server.
type loopOutRPCClient interface {
	LoopOut(context.Context, *loop.OutRequest) (
		*loop.LoopOutSwapInfo, error)
	LoopOutQuote(context.Context, *loop.LoopOutQuoteRequest) (
		*loop.LoopOutQuote, error)
	LoopOutTerms(context.Context, string) (*loop.LoopOutTerms, error)
	FetchSwaps(context.Context) ([]*loop.SwapInfo, error)
}

// loopOutRPCServer exposes only the external-payment Loop Out milestone. The
// embedded unimplemented server rejects every other Loop RPC.
type loopOutRPCServer struct {
	looprpc.UnimplementedSwapClientServer

	client      loopOutRPCClient
	chainParams *chaincfg.Params
	network     string
	stop        func()

	subscribersMu sync.Mutex
	subscribers   map[uint64]chan loop.SwapInfo
	nextID        uint64
}

func newLoopOutRPCServer(client loopOutRPCClient,
	chainParams *chaincfg.Params, stop func()) (*loopOutRPCServer, error) {

	if client == nil {
		return nil, errors.New("Loop client is required")
	}
	if chainParams == nil {
		return nil, errors.New("chain parameters are required")
	}

	return &loopOutRPCServer{
		client:      client,
		chainParams: chainParams,
		network:     chainParams.Name,
		stop:        stop,
		subscribers: make(map[uint64]chan loop.SwapInfo),
	}, nil
}

// LoopOut starts an externally paid Loop Out to an explicit destination.
func (s *loopOutRPCServer) LoopOut(ctx context.Context,
	request *looprpc.LoopOutRequest) (*looprpc.SwapResponse, error) {

	loopRequest, err := s.loopOutRequest(request)
	if err != nil {
		return nil, err
	}

	info, err := s.client.LoopOut(ctx, loopRequest)
	if err != nil {
		return nil, err
	}

	return marshalLoopOutResponse(info)
}

// LoopOutTerms returns the server's Loop Out limits.
func (s *loopOutRPCServer) LoopOutTerms(ctx context.Context,
	_ *looprpc.TermsRequest) (*looprpc.OutTermsResponse, error) {

	terms, err := s.client.LoopOutTerms(ctx, embeddedInitiator)
	if err != nil {
		return nil, err
	}

	return &looprpc.OutTermsResponse{
		MinSwapAmount: int64(terms.MinSwapAmount),
		MaxSwapAmount: int64(terms.MaxSwapAmount),
		MinCltvDelta:  terms.MinCltvDelta,
		MaxCltvDelta:  terms.MaxCltvDelta,
	}, nil
}

// LoopOutQuote returns the server and Esplora-backed sweep fee quote.
func (s *loopOutRPCServer) LoopOutQuote(ctx context.Context,
	request *looprpc.QuoteRequest) (*looprpc.OutQuoteResponse, error) {

	if request == nil {
		return nil, invalidArgument("quote request is required")
	}
	if request.Amt <= 0 {
		return nil, invalidArgument("amount must be positive")
	}
	if request.ExternalHtlc || len(request.LoopInLastHop) != 0 ||
		len(request.LoopInRouteHints) != 0 || request.Private ||
		len(request.DepositOutpoints) != 0 ||
		request.AutoSelectDeposits || request.Fast {

		return nil, invalidArgument(
			"Loop In quote fields are unavailable in the embedded " +
				"Loop Out runtime",
		)
	}
	if request.AssetInfo != nil {
		return nil, invalidArgument(
			"asset Loop Outs are unavailable in the embedded runtime",
		)
	}

	confTarget, err := validateConfTarget(request.ConfTarget)
	if err != nil {
		return nil, err
	}

	quote, err := s.client.LoopOutQuote(
		ctx, &loop.LoopOutQuoteRequest{
			Amount:          btcutil.Amount(request.Amt),
			SweepConfTarget: confTarget,
			SwapPublicationDeadline: publicationDeadline(
				request.SwapPublicationDeadline,
			),
			Initiator: embeddedInitiator,
		},
	)
	if err != nil {
		return nil, err
	}

	return &looprpc.OutQuoteResponse{
		HtlcSweepFeeSat: int64(quote.MinerFee),
		PrepayAmtSat:    int64(quote.PrepayAmount),
		SwapFeeSat:      int64(quote.SwapFee),
		SwapPaymentDest: quote.SwapPaymentDest[:],
		ConfTarget:      confTarget,
	}, nil
}

// ListSwaps lists stored Loop Outs and applies the standard list filters.
func (s *loopOutRPCServer) ListSwaps(ctx context.Context,
	request *looprpc.ListSwapsRequest) (*looprpc.ListSwapsResponse, error) {

	if request == nil {
		request = &looprpc.ListSwapsRequest{}
	}

	swaps, err := s.fetchLoopOuts(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]*loop.SwapInfo, 0, len(swaps))
	for _, swapInfo := range swaps {
		if matchesLoopOutFilter(swapInfo, request.ListSwapFilter) {
			filtered = append(filtered, swapInfo)
		}
	}
	slices.SortFunc(filtered, func(a, b *loop.SwapInfo) int {
		return a.InitiationTime.Compare(b.InitiationTime)
	})

	canPage := request.MaxSwaps > 0 &&
		uint64(len(filtered)) > request.MaxSwaps
	if canPage {
		filtered = filtered[:request.MaxSwaps]
	}

	response := &looprpc.ListSwapsResponse{
		Swaps: make([]*looprpc.SwapStatus, 0, len(filtered)),
	}
	for _, swapInfo := range filtered {
		rpcSwap, err := marshalLoopOutSwap(swapInfo)
		if err != nil {
			return nil, err
		}
		response.Swaps = append(response.Swaps, rpcSwap)
	}
	if canPage && len(response.Swaps) != 0 {
		last := response.Swaps[len(response.Swaps)-1]
		response.NextStartTime = last.InitiationTime + 1
	}

	return response, nil
}

// SwapInfo returns one stored Loop Out by its binary swap hash.
func (s *loopOutRPCServer) SwapInfo(ctx context.Context,
	request *looprpc.SwapInfoRequest) (*looprpc.SwapStatus, error) {

	if request == nil {
		return nil, invalidArgument("swap info request is required")
	}
	hash, err := lntypes.MakeHash(request.Id)
	if err != nil {
		return nil, invalidArgument("swap ID must be 32 bytes")
	}

	swaps, err := s.fetchLoopOuts(ctx)
	if err != nil {
		return nil, err
	}
	for _, swapInfo := range swaps {
		if swapInfo.SwapHash == hash {
			return marshalLoopOutSwap(swapInfo)
		}
	}

	return nil, status.Errorf(
		codes.NotFound, "Loop Out %x was not found", hash[:],
	)
}

// Monitor sends a snapshot followed by live Loop Out state changes.
func (s *loopOutRPCServer) Monitor(_ *looprpc.MonitorRequest,
	stream looprpc.SwapClient_MonitorServer) error {

	id, updates := s.subscribe()
	defer s.unsubscribe(id)

	swaps, err := s.fetchLoopOuts(stream.Context())
	if err != nil {
		return err
	}
	slices.SortFunc(swaps, func(a, b *loop.SwapInfo) int {
		return a.LastUpdate.Compare(b.LastUpdate)
	})
	for _, swapInfo := range swaps {
		if err := sendLoopOutSwap(stream, swapInfo); err != nil {
			return err
		}
	}

	for {
		select {
		case update := <-updates:
			if err := sendLoopOutSwap(stream, &update); err != nil {
				return err
			}

		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// GetInfo returns version, network, and Loop Out aggregate statistics.
func (s *loopOutRPCServer) GetInfo(ctx context.Context,
	_ *looprpc.GetInfoRequest) (*looprpc.GetInfoResponse, error) {

	swaps, err := s.fetchLoopOuts(ctx)
	if err != nil {
		return nil, err
	}

	stats := &looprpc.LoopStats{}
	for _, swapInfo := range swaps {
		switch swapInfo.State.Type() {
		case loopdb.StateTypePending:
			stats.PendingCount++
			stats.SumPendingAmt += int64(swapInfo.AmountRequested)

		case loopdb.StateTypeSuccess:
			stats.SuccessCount++
			stats.SumSucceededAmt += int64(swapInfo.AmountRequested)

		case loopdb.StateTypeFail:
			stats.FailCount++
		}
	}

	commitHash := loop.CommitHash
	if loop.Dirty != "" {
		commitHash += "-dirty"
	}

	return &looprpc.GetInfoResponse{
		Version:      loop.Version(),
		CommitHash:   commitHash,
		Network:      s.network,
		RpcListen:    "bufconn",
		LoopOutStats: stats,
		LoopInStats:  &looprpc.LoopStats{},
	}, nil
}

// StopDaemon requests a clean shutdown of the embedded runtime.
func (s *loopOutRPCServer) StopDaemon(context.Context,
	*looprpc.StopDaemonRequest) (*looprpc.StopDaemonResponse, error) {

	if s.stop == nil {
		return nil, status.Error(
			codes.Unimplemented, "embedded stop is not configured",
		)
	}

	s.stop()

	return &looprpc.StopDaemonResponse{}, nil
}

// runUpdates forwards core state changes to active monitor streams.
func (s *loopOutRPCServer) runUpdates(ctx context.Context,
	updates <-chan loop.SwapInfo) {

	for {
		select {
		case update, ok := <-updates:
			if !ok {
				return
			}
			if update.SwapType != swap.TypeOut {
				continue
			}
			s.broadcast(update)

		case <-ctx.Done():
			return
		}
	}
}

func (s *loopOutRPCServer) loopOutRequest(
	request *looprpc.LoopOutRequest) (*loop.OutRequest, error) {

	if request == nil {
		return nil, invalidArgument("Loop Out request is required")
	}
	if request.Amt <= 0 {
		return nil, invalidArgument("amount must be positive")
	}
	if !request.ExternalPayments {
		return nil, invalidArgument(
			"external_payments must be enabled in the embedded runtime",
		)
	}
	if strings.TrimSpace(request.Dest) == "" {
		return nil, invalidArgument(
			"an external destination address is required",
		)
	}
	if request.Account != "" ||
		request.AccountAddrType != looprpc.AddressType_ADDRESS_TYPE_UNKNOWN {

		return nil, invalidArgument(
			"wallet accounts are unavailable in the embedded runtime",
		)
	}
	if request.LoopOutChannel != 0 || len(request.OutgoingChanSet) != 0 {
		return nil, invalidArgument(
			"outgoing channel selection is unavailable for external " +
				"payments",
		)
	}
	if len(request.ReservationIds) != 0 {
		return nil, invalidArgument(
			"instant-out reservations are unavailable in the embedded " +
				"runtime",
		)
	}
	if request.AssetInfo != nil || request.AssetRfqInfo != nil {
		return nil, invalidArgument(
			"asset Loop Outs are unavailable in the embedded runtime",
		)
	}
	if request.MaxSwapRoutingFee < 0 ||
		request.MaxPrepayRoutingFee < 0 || request.MaxSwapFee < 0 ||
		request.MaxPrepayAmt < 0 || request.MaxMinerFee < 0 {

		return nil, invalidArgument("fee limits must not be negative")
	}
	if request.HtlcConfirmations < 0 {
		return nil, invalidArgument(
			"HTLC confirmations must not be negative",
		)
	}
	if err := labels.Validate(request.Label); err != nil {
		return nil, invalidArgument(err.Error())
	}

	destination, err := btcutil.DecodeAddress(
		request.Dest, s.chainParams,
	)
	if err != nil {
		return nil, invalidArgument("decode destination address: " +
			err.Error())
	}
	if !destination.IsForNet(s.chainParams) {
		return nil, invalidArgument(
			"destination address is for a different network",
		)
	}
	switch destination.(type) {
	case *btcutil.AddressTaproot,
		*btcutil.AddressWitnessScriptHash,
		*btcutil.AddressWitnessPubKeyHash,
		*btcutil.AddressScriptHash,
		*btcutil.AddressPubKeyHash:

	default:
		return nil, invalidArgument("unsupported destination address")
	}

	confTarget, err := validateConfTarget(request.SweepConfTarget)
	if err != nil {
		return nil, err
	}
	initiator := strings.TrimSpace(request.Initiator)
	if initiator == "" {
		initiator = embeddedInitiator
	}

	return &loop.OutRequest{
		Amount:              btcutil.Amount(request.Amt),
		DestAddr:            destination,
		IsExternalAddr:      true,
		MaxMinerFee:         btcutil.Amount(request.MaxMinerFee),
		MaxPrepayAmount:     btcutil.Amount(request.MaxPrepayAmt),
		MaxPrepayRoutingFee: btcutil.Amount(request.MaxPrepayRoutingFee),
		MaxSwapRoutingFee:   btcutil.Amount(request.MaxSwapRoutingFee),
		MaxSwapFee:          btcutil.Amount(request.MaxSwapFee),
		SweepConfTarget:     confTarget,
		HtlcConfirmations:   request.HtlcConfirmations,
		SwapPublicationDeadline: publicationDeadline(
			request.SwapPublicationDeadline,
		),
		Label:            request.Label,
		Initiator:        initiator,
		PaymentTimeout:   time.Duration(request.PaymentTimeout) * time.Second,
		ExternalPayments: true,
	}, nil
}

func (s *loopOutRPCServer) fetchLoopOuts(ctx context.Context) (
	[]*loop.SwapInfo, error) {

	allSwaps, err := s.client.FetchSwaps(ctx)
	if err != nil {
		return nil, err
	}

	loopOuts := make([]*loop.SwapInfo, 0, len(allSwaps))
	for _, swapInfo := range allSwaps {
		if swapInfo != nil && swapInfo.SwapType == swap.TypeOut {
			loopOuts = append(loopOuts, swapInfo)
		}
	}

	return loopOuts, nil
}

func (s *loopOutRPCServer) subscribe() (uint64, <-chan loop.SwapInfo) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	id := s.nextID
	s.nextID++
	updates := make(chan loop.SwapInfo, monitorSubscriberBuffer)
	s.subscribers[id] = updates

	return id, updates
}

func (s *loopOutRPCServer) unsubscribe(id uint64) {
	s.subscribersMu.Lock()
	delete(s.subscribers, id)
	s.subscribersMu.Unlock()
}

func (s *loopOutRPCServer) broadcast(update loop.SwapInfo) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	for _, updates := range s.subscribers {
		select {
		case updates <- update:
			continue
		default:
		}

		// A slow monitor only loses its oldest intermediate state. It cannot
		// block swap execution or unrelated monitors.
		select {
		case <-updates:
		default:
		}
		select {
		case updates <- update:
		default:
		}
	}
}

func sendLoopOutSwap(stream looprpc.SwapClient_MonitorServer,
	swapInfo *loop.SwapInfo) error {

	rpcSwap, err := marshalLoopOutSwap(swapInfo)
	if err != nil {
		return err
	}

	return stream.Send(rpcSwap)
}

func marshalLoopOutResponse(info *loop.LoopOutSwapInfo) (
	*looprpc.SwapResponse, error) {

	if info == nil || info.HtlcAddress == nil {
		return nil, errors.New("Loop Out response is missing its HTLC address")
	}

	address := info.HtlcAddress.EncodeAddress()
	response := &looprpc.SwapResponse{
		Id:               info.SwapHash.String(),
		IdBytes:          append([]byte(nil), info.SwapHash[:]...),
		HtlcAddress:      address,
		ServerMessage:    info.ServerMessage,
		SwapInvoice:      info.SwapInvoice,
		PrepayInvoice:    info.PrepayInvoice,
		ExternalPayments: info.ExternalPayments,
	}
	switch info.HtlcAddress.(type) {
	case *btcutil.AddressTaproot:
		response.HtlcAddressP2Tr = address

	case *btcutil.AddressWitnessScriptHash:
		response.HtlcAddressP2Wsh = address

	default:
		return nil, fmt.Errorf(
			"unsupported Loop Out HTLC address type %T",
			info.HtlcAddress,
		)
	}

	return response, nil
}

func marshalLoopOutSwap(swapInfo *loop.SwapInfo) (
	*looprpc.SwapStatus, error) {

	if swapInfo == nil || swapInfo.SwapType != swap.TypeOut {
		return nil, errors.New("expected a Loop Out swap")
	}

	state, failureReason, err := marshalSwapState(swapInfo.State)
	if err != nil {
		return nil, err
	}

	var (
		htlcAddress string
		p2wsh       string
		p2tr        string
	)
	switch {
	case swapInfo.HtlcAddressP2WSH != nil:
		p2wsh = swapInfo.HtlcAddressP2WSH.EncodeAddress()
		htlcAddress = p2wsh

	case swapInfo.HtlcAddressP2TR != nil:
		p2tr = swapInfo.HtlcAddressP2TR.EncodeAddress()
		htlcAddress = p2tr

	default:
		return nil, errors.New("Loop Out is missing its HTLC address")
	}

	return &looprpc.SwapStatus{
		Amt:              int64(swapInfo.AmountRequested),
		Id:               swapInfo.SwapHash.String(),
		IdBytes:          append([]byte(nil), swapInfo.SwapHash[:]...),
		Type:             looprpc.SwapType_LOOP_OUT,
		State:            state,
		FailureReason:    failureReason,
		InitiationTime:   swapInfo.InitiationTime.UnixNano(),
		LastUpdateTime:   swapInfo.LastUpdate.UnixNano(),
		HtlcAddress:      htlcAddress,
		HtlcAddressP2Wsh: p2wsh,
		HtlcAddressP2Tr:  p2tr,
		CostServer:       int64(swapInfo.Cost.Server),
		CostOnchain:      int64(swapInfo.Cost.Onchain),
		CostOffchain:     int64(swapInfo.Cost.Offchain),
		OutgoingChanSet: append(
			[]uint64(nil), swapInfo.OutgoingChanSet...,
		),
		Label:            swapInfo.Label,
		SwapInvoice:      swapInfo.SwapInvoice,
		PrepayInvoice:    swapInfo.PrepayInvoice,
		ExternalPayments: swapInfo.ExternalPayments,
	}, nil
}

func marshalSwapState(state loopdb.SwapState) (
	looprpc.SwapState, looprpc.FailureReason, error) {

	failure := looprpc.FailureReason_FAILURE_REASON_NONE
	var rpcState looprpc.SwapState
	switch state {
	case loopdb.StateInitiated:
		rpcState = looprpc.SwapState_INITIATED

	case loopdb.StatePreimageRevealed:
		rpcState = looprpc.SwapState_PREIMAGE_REVEALED

	case loopdb.StateHtlcPublished:
		rpcState = looprpc.SwapState_HTLC_PUBLISHED

	case loopdb.StateInvoiceSettled:
		rpcState = looprpc.SwapState_INVOICE_SETTLED

	case loopdb.StateSuccess:
		rpcState = looprpc.SwapState_SUCCESS

	case loopdb.StateFailOffchainPayments:
		failure = looprpc.FailureReason_FAILURE_REASON_OFFCHAIN

	case loopdb.StateFailTimeout:
		failure = looprpc.FailureReason_FAILURE_REASON_TIMEOUT

	case loopdb.StateFailSweepTimeout:
		failure = looprpc.FailureReason_FAILURE_REASON_SWEEP_TIMEOUT

	case loopdb.StateFailInsufficientValue:
		failure = looprpc.FailureReason_FAILURE_REASON_INSUFFICIENT_VALUE

	case loopdb.StateFailTemporary:
		failure = looprpc.FailureReason_FAILURE_REASON_TEMPORARY

	case loopdb.StateFailIncorrectHtlcAmt:
		failure = looprpc.FailureReason_FAILURE_REASON_INCORRECT_AMOUNT

	case loopdb.StateFailAbandoned:
		failure = looprpc.FailureReason_FAILURE_REASON_ABANDONED

	case loopdb.StateFailInsufficientConfirmedBalance:
		failure = looprpc.
			FailureReason_FAILURE_REASON_INSUFFICIENT_CONFIRMED_BALANCE

	case loopdb.StateFailIncorrectHtlcAmtSwept:
		failure = looprpc.
			FailureReason_FAILURE_REASON_INCORRECT_HTLC_AMT_SWEPT

	default:
		return 0, 0, fmt.Errorf("unknown swap state: %v", state)
	}

	if failure != looprpc.FailureReason_FAILURE_REASON_NONE {
		rpcState = looprpc.SwapState_FAILED
	}

	return rpcState, failure, nil
}

func matchesLoopOutFilter(swapInfo *loop.SwapInfo,
	filter *looprpc.ListSwapsFilter) bool {

	if filter == nil {
		return true
	}
	if filter.SwapType == looprpc.ListSwapsFilter_LOOP_IN {
		return false
	}
	if filter.PendingOnly && !swapInfo.State.IsPending() {
		return false
	}
	if filter.StartTimestampNs > 0 &&
		swapInfo.InitiationTime.UnixNano() < filter.StartTimestampNs {

		return false
	}
	if len(filter.OutgoingChanSet) != 0 &&
		!slices.Equal(swapInfo.OutgoingChanSet, filter.OutgoingChanSet) {

		return false
	}
	if filter.Label != "" && !strings.Contains(
		swapInfo.Label, filter.Label,
	) {

		return false
	}
	if len(filter.LoopInLastHop) != 0 || filter.AssetSwapOnly {
		return false
	}

	return true
}

func validateConfTarget(target int32) (int32, error) {
	if target == 0 {
		return loop.DefaultSweepConfTarget, nil
	}
	if target < minimumConfTarget {
		return 0, invalidArgument(fmt.Sprintf(
			"confirmation target must be at least %d",
			minimumConfTarget,
		))
	}

	return target, nil
}

func publicationDeadline(timestamp uint64) time.Time {
	if timestamp >= 1_000_000_000_000 {
		seconds := timestamp / 1000
		nanoseconds := (timestamp % 1000) * uint64(time.Millisecond)

		return time.Unix(int64(seconds), int64(nanoseconds))
	}

	return time.Unix(int64(timestamp), 0)
}

func invalidArgument(message string) error {
	return status.Error(codes.InvalidArgument, message)
}
