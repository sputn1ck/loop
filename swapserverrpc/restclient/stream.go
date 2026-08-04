package restclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/lightninglabs/loop/swapserverrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const maxStreamMessageSize = 1 << 20

// SubscribeLoopOutUpdates subscribes to reconnectable NDJSON state updates.
func (c *Client) SubscribeLoopOutUpdates(ctx context.Context,
	in *swapserverrpc.SubscribeUpdatesRequest, _ ...grpc.CallOption) (
	swapserverrpc.SwapServer_SubscribeLoopOutUpdatesClient, error) {
	if in == nil {
		return nil, status.Error(
			codes.InvalidArgument, "subscription request is required",
		)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	stream := &loopOutUpdatesStream{
		client:  c,
		request: proto.Clone(in).(*swapserverrpc.SubscribeUpdatesRequest),
		ctx:     streamCtx,
		cancel:  cancel,
	}
	if err := stream.connect(); err != nil {
		cancel()

		return nil, err
	}

	return stream, nil
}

type loopOutUpdatesStream struct {
	client  *Client
	request *swapserverrpc.SubscribeUpdatesRequest
	ctx     context.Context //nolint:containedctx // Implements grpc.ClientStream.
	cancel  context.CancelFunc

	responseMu sync.Mutex
	response   *http.Response
	scanner    *bufio.Scanner
	header     metadata.MD

	closeOnce sync.Once
	terminal  bool

	haveLast     bool
	lastTime     int64
	lastState    swapserverrpc.ServerSwapState
	reconnects   int
	reconnectErr error
}

func (s *loopOutUpdatesStream) connect() error {
	body, err := marshalJSON.Marshal(s.request)
	if err != nil {
		return fmt.Errorf("marshal stream request: %w", err)
	}

	resp, err := s.client.do(
		s.ctx, LoopOutUpdatesPath, "application/json", body,
	)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices {

		defer func() {
			_ = resp.Body.Close()
		}()

		return responseError(resp)
	}

	s.responseMu.Lock()
	if s.response != nil {
		_ = s.response.Body.Close()
	}
	s.response = resp
	s.scanner = bufio.NewScanner(resp.Body)
	s.scanner.Buffer(make([]byte, 4096), maxStreamMessageSize)
	s.header = httpHeaderMetadata(resp.Header)
	s.responseMu.Unlock()

	return nil
}

// Recv returns the next non-duplicate state update. A transport EOF before a
// terminal state reconnects using the original subscription request.
func (s *loopOutUpdatesStream) Recv() (
	*swapserverrpc.SubscribeLoopOutUpdatesResponse, error) {

	if s.terminal {
		return nil, io.EOF
	}

	for {
		update, err := s.recvOne()
		if err == nil {
			s.reconnects = 0
			s.reconnectErr = nil
			if s.haveLast && update.TimestampNs == s.lastTime &&
				update.State == s.lastState {

				continue
			}

			s.haveLast = true
			s.lastTime = update.TimestampNs
			s.lastState = update.State
			s.terminal = terminalLoopOutState(update.State)
			if s.terminal {
				s.closeResponse()
			}

			return update, nil
		}

		if !retryableStreamError(err) {
			return nil, err
		}
		if err := s.reconnect(err); err != nil {
			return nil, err
		}
	}
}

func (s *loopOutUpdatesStream) recvOne() (
	*swapserverrpc.SubscribeLoopOutUpdatesResponse, error) {

	s.responseMu.Lock()
	scanner := s.scanner
	s.responseMu.Unlock()
	if scanner == nil {
		return nil, io.EOF
	}

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		message, err := decodeLoopOutUpdate(line)
		if err != nil {
			return nil, err
		}

		return message, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, io.EOF
}

func decodeLoopOutUpdate(line []byte) (
	*swapserverrpc.SubscribeLoopOutUpdatesResponse, error) {

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *gatewayStatus  `json:"error"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil, fmt.Errorf("decode NDJSON envelope: %w", err)
	}
	if envelope.Error != nil {
		code := codes.Unknown
		if parsed, ok := parseGatewayCode(envelope.Error.Code); ok {
			code = parsed
		}

		return nil, status.Error(code, envelope.Error.Message)
	}
	if len(envelope.Result) != 0 {
		line = envelope.Result
	}

	message := new(swapserverrpc.SubscribeLoopOutUpdatesResponse)
	if err := unmarshalJSON.Unmarshal(line, message); err != nil {
		return nil, fmt.Errorf("decode Loop Out update: %w", err)
	}

	return message, nil
}

func (s *loopOutUpdatesStream) reconnect(lastErr error) error {
	policy := s.client.reconnect
	for {
		if policy.MaxAttempts >= 0 &&
			s.reconnects >= policy.MaxAttempts {

			return status.Errorf(
				codes.Unavailable,
				"Loop Out update stream reconnect limit: %v",
				lastErr,
			)
		}

		delay := reconnectDelay(policy, s.reconnects)
		s.reconnects++
		s.reconnectErr = lastErr
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-s.ctx.Done():
				timer.Stop()

				return s.ctx.Err()
			}
			timer.Stop()
		}

		if err := s.connect(); err != nil {
			if !retryableStreamError(err) {
				return err
			}

			lastErr = err
			continue
		}

		return nil
	}
}

func reconnectDelay(policy ReconnectPolicy, attempt int) time.Duration {
	if policy.InitialBackoff == 0 {
		return 0
	}

	delay := policy.InitialBackoff
	for i := 0; i < attempt; i++ {
		if policy.MaxBackoff > 0 && delay >= policy.MaxBackoff/2 {
			return policy.MaxBackoff
		}
		delay *= 2
	}
	if policy.MaxBackoff > 0 && delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}

	return delay
}

func retryableStreamError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {

		return false
	}

	var paymentRequired *PaymentRequiredError
	if errors.As(err, &paymentRequired) {
		return false
	}

	switch status.Code(err) {
	case codes.Unavailable, codes.ResourceExhausted, codes.Internal,
		codes.Unknown:

		return true
	default:
		return false
	}
}

func terminalLoopOutState(state swapserverrpc.ServerSwapState) bool {
	switch state {
	case swapserverrpc.ServerSwapState_SERVER_SUCCESS,
		swapserverrpc.ServerSwapState_SERVER_FAILED_UNKNOWN,
		swapserverrpc.ServerSwapState_SERVER_FAILED_NO_HTLC,
		swapserverrpc.ServerSwapState_SERVER_FAILED_INVALID_HTLC_AMOUNT,
		swapserverrpc.ServerSwapState_SERVER_FAILED_OFF_CHAIN_TIMEOUT,
		swapserverrpc.ServerSwapState_SERVER_FAILED_TIMEOUT,
		swapserverrpc.ServerSwapState_SERVER_FAILED_SWAP_DEADLINE,
		swapserverrpc.ServerSwapState_SERVER_FAILED_HTLC_PUBLICATION,
		swapserverrpc.ServerSwapState_SERVER_UNEXPECTED_FAILURE,
		swapserverrpc.ServerSwapState_SERVER_CLIENT_PREPAY_CANCEL,
		swapserverrpc.ServerSwapState_SERVER_CLIENT_INVOICE_CANCEL,
		swapserverrpc.ServerSwapState_SERVER_FAILED_MULTIPLE_SWAP_SCRIPTS,
		swapserverrpc.ServerSwapState_SERVER_FAILED_INITIALIZATION:

		return true
	default:
		return false
	}
}

func httpHeaderMetadata(header http.Header) metadata.MD {
	result := make(metadata.MD, len(header))
	for key, values := range header {
		result.Set(key, values...)
	}

	return result
}

// Header returns the HTTP response headers as gRPC metadata.
func (s *loopOutUpdatesStream) Header() (metadata.MD, error) {
	s.responseMu.Lock()
	defer s.responseMu.Unlock()

	return s.header.Copy(), nil
}

// Trailer returns no trailers because the NDJSON endpoint does not define any.
func (s *loopOutUpdatesStream) Trailer() metadata.MD {
	return metadata.MD{}
}

// CloseSend closes the response body and cancels future reconnects.
func (s *loopOutUpdatesStream) CloseSend() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.cancel()
		closeErr = s.closeResponse()
	})

	return closeErr
}

func (s *loopOutUpdatesStream) closeResponse() error {
	s.responseMu.Lock()
	defer s.responseMu.Unlock()

	if s.response == nil {
		return nil
	}

	err := s.response.Body.Close()
	s.response = nil
	s.scanner = nil

	return err
}

// Context returns the stream context.
func (s *loopOutUpdatesStream) Context() context.Context {
	return s.ctx
}

// SendMsg is unsupported for a REST server stream.
func (s *loopOutUpdatesStream) SendMsg(any) error {
	return status.Error(codes.Unimplemented, "REST server stream is receive-only")
}

// RecvMsg receives the next update into message.
func (s *loopOutUpdatesStream) RecvMsg(message any) error {
	target, ok := message.(*swapserverrpc.SubscribeLoopOutUpdatesResponse)
	if !ok {
		return fmt.Errorf("unexpected Loop Out update target %T", message)
	}

	update, err := s.Recv()
	if err != nil {
		return err
	}
	proto.Reset(target)
	proto.Merge(target, update)

	return nil
}
