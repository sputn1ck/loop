package restclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lightninglabs/loop/swapserverrpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestGRPCGatewayPaths(t *testing.T) {
	t.Parallel()

	paths := map[string]string{
		"LoopOutTerms":            "/looprpc.SwapServer/LoopOutTerms",
		"LoopOutQuote":            "/looprpc.SwapServer/LoopOutQuote",
		"NewLoopOutSwap":          "/looprpc.SwapServer/NewLoopOutSwap",
		"LoopOutPushPreimage":     "/looprpc.SwapServer/LoopOutPushPreimage",
		"CancelLoopOutSwap":       "/looprpc.SwapServer/CancelLoopOutSwap",
		"MuSig2SignSweep":         "/looprpc.SwapServer/MuSig2SignSweep",
		"PushKey":                 "/looprpc.SwapServer/PushKey",
		"FetchL402":               "/looprpc.SwapServer/FetchL402",
		"SubscribeLoopOutUpdates": "/looprpc.SwapServer/SubscribeLoopOutUpdates",
	}

	requirePath := func(name, got string) {
		t.Helper()

		if got != paths[name] {
			t.Fatalf("unexpected %s path: %s", name, got)
		}
	}

	requirePath("LoopOutTerms", LoopOutTermsPath)
	requirePath("LoopOutQuote", LoopOutQuotePath)
	requirePath("NewLoopOutSwap", NewLoopOutPath)
	requirePath("LoopOutPushPreimage", LoopOutPreimagePath)
	requirePath("CancelLoopOutSwap", CancelLoopOutPath)
	requirePath("MuSig2SignSweep", MuSig2SignSweepPath)
	requirePath("PushKey", PushKeyPath)
	requirePath("FetchL402", FetchL402Path)
	requirePath("SubscribeLoopOutUpdates", LoopOutUpdatesPath)
}

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{
			name:     "canonical HTTPS",
			baseURL:  " HTTPS://LOOP.EXAMPLE:443/gateway/// ",
			expected: "https://loop.example/gateway",
		},
		{
			name:     "canonical HTTP",
			baseURL:  "http://LOOP.EXAMPLE:80",
			expected: "http://loop.example",
		},
		{
			name:     "non-default port",
			baseURL:  "https://LOOP.EXAMPLE:8443/api/",
			expected: "https://loop.example:8443/api",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, err := NormalizeBaseURL(testCase.baseURL)
			if err != nil {
				t.Fatalf("normalize base URL: %v", err)
			}
			if normalized != testCase.expected {
				t.Fatalf(
					"unexpected base URL: got %q, want %q",
					normalized, testCase.expected,
				)
			}
		})
	}
}

func TestNormalizeBaseURLRejectsUnsafeValues(t *testing.T) {
	t.Parallel()

	for _, baseURL := range []string{
		"file:///tmp/gateway",
		"https://user:password@loop.example",
		"https://loop.example?profile=other",
		"https://loop.example#other",
	} {
		t.Run(baseURL, func(t *testing.T) {
			if _, err := NormalizeBaseURL(baseURL); err == nil {
				t.Fatalf("accepted unsafe base URL %q", baseURL)
			}
		})
	}
}

func TestLoopOutQuote(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {

		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %v", r.Method)
		}
		if r.URL.Path != LoopOutQuotePath {
			t.Errorf("unexpected path: %v", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %v",
				r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "L402 cached-token" {
			t.Errorf("unexpected authorization: %v",
				r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Static") != "static-value" {
			t.Errorf("missing static header")
		}
		if r.Header.Get("X-Request") != "request-value" {
			t.Errorf("missing request metadata")
		}

		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		request := new(swapserverrpc.ServerLoopOutQuoteRequest)
		if err := protojson.Unmarshal(requestBody, request); err != nil {
			t.Errorf("unmarshal request: %v", err)
			return
		}
		if request.Amt != 123_000 || request.Expiry != 900_000 {
			t.Errorf("unexpected request: %v", request)
		}

		writeProtoJSON(t, w, &swapserverrpc.ServerLoopOutQuote{
			SwapFee:   1_200,
			PrepayAmt: 300,
		})
	}))
	defer server.Close()

	client, err := New(
		server.URL, WithAuthorization("L402 cached-token"),
		WithHeader("X-Static", "static-value"),
	)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	ctx := metadata.NewOutgoingContext(
		context.Background(), metadata.Pairs(
			"x-request", "request-value", "ignored-bin", "binary",
		),
	)
	quote, err := client.LoopOutQuote(
		ctx, &swapserverrpc.ServerLoopOutQuoteRequest{
			Amt:    123_000,
			Expiry: 900_000,
		},
	)
	if err != nil {
		t.Fatalf("request quote: %v", err)
	}
	if quote.SwapFee != 1_200 || quote.PrepayAmt != 300 {
		t.Fatalf("unexpected quote: %v", quote)
	}
}

func TestBrowserCompatibleJSONContentType(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {

		requestCount.Add(1)
		if r.Header.Get("Content-Type") != browserJSONContentType {
			t.Errorf(
				"unexpected content type: %v",
				r.Header.Get("Content-Type"),
			)
		}

		switch r.URL.Path {
		case LoopOutTermsPath:
			writeProtoJSON(t, w, &swapserverrpc.ServerLoopOutTerms{
				MinSwapAmount: 1_000,
			})

		case LoopOutUpdatesPath:
			w.Header().Set("Content-Type", "application/json")
			writeNDJSON(
				t, w,
				&swapserverrpc.SubscribeLoopOutUpdatesResponse{
					State: swapserverrpc.ServerSwapState_SERVER_SUCCESS,
				},
			)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, WithBrowserCompatibleJSON())
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	terms, err := client.LoopOutTerms(
		t.Context(), &swapserverrpc.ServerLoopOutTermsRequest{},
	)
	if err != nil {
		t.Fatalf("request terms: %v", err)
	}
	if terms.MinSwapAmount != 1_000 {
		t.Fatalf("unexpected terms: %v", terms)
	}

	stream, err := client.SubscribeLoopOutUpdates(
		t.Context(), &swapserverrpc.SubscribeUpdatesRequest{},
	)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() {
		_ = stream.CloseSend()
	}()

	update, err := stream.Recv()
	if err != nil {
		t.Fatalf("receive update: %v", err)
	}
	if update.State != swapserverrpc.ServerSwapState_SERVER_SUCCESS {
		t.Fatalf("unexpected update: %v", update)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("unexpected request count: %v", requestCount.Load())
	}
}

func TestL402ChallengeRetryAndCache(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {

		request := requestCount.Add(1)
		if request == 1 {
			w.Header().Set(
				"WWW-Authenticate",
				`L402 macaroon="macaroon-secret", `+
					`invoice="invoice-secret"`,
			)
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}

		if r.Header.Get("Authorization") != "L402 paid-token" {
			t.Errorf("unexpected authorization: %v",
				r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		writeProtoJSON(t, w, &swapserverrpc.ServerLoopOutTerms{
			MinSwapAmount: 1_000,
		})
	}))
	defer server.Close()

	var challengeCount atomic.Int32
	client, err := New(server.URL, WithL402ChallengeHandler(
		func(_ context.Context, challenge L402Challenge) (string, error) {
			challengeCount.Add(1)
			if challenge.Scheme != "L402" ||
				challenge.Macaroon != "macaroon-secret" ||
				challenge.Invoice != "invoice-secret" {

				return "", fmt.Errorf("unexpected challenge: %+v", challenge)
			}

			return "L402 paid-token", nil
		},
	))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	for i := 0; i < 2; i++ {
		_, err := client.LoopOutTerms(
			context.Background(),
			&swapserverrpc.ServerLoopOutTermsRequest{},
		)
		if err != nil {
			t.Fatalf("request terms %d: %v", i, err)
		}
	}

	if requestCount.Load() != 3 {
		t.Fatalf("unexpected request count: %v", requestCount.Load())
	}
	if challengeCount.Load() != 1 {
		t.Fatalf("unexpected challenge count: %v", challengeCount.Load())
	}
}

func TestConcurrentL402ChallengePaidOnce(t *testing.T) {
	t.Parallel()

	var (
		unauthorizedCount atomic.Int32
		paidCount         atomic.Int32
	)
	bothChallenged := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {

		if r.Header.Get("Authorization") == "L402 paid-token" {
			paidCount.Add(1)
			writeProtoJSON(t, w, &swapserverrpc.ServerLoopOutTerms{
				MinSwapAmount: 1_000,
			})

			return
		}

		if unauthorizedCount.Add(1) == 2 {
			close(bothChallenged)
		}
		w.Header().Set(
			"WWW-Authenticate",
			`L402 macaroon="macaroon-secret", `+
				`invoice="invoice-secret"`,
		)
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()

	var challengeCount atomic.Int32
	firstChallenge := make(chan struct{})
	releasePayment := make(chan struct{})
	client, err := New(server.URL, WithL402ChallengeHandler(
		func(ctx context.Context, _ L402Challenge) (string, error) {
			challengeCount.Add(1)
			close(firstChallenge)

			select {
			case <-releasePayment:
				return "L402 paid-token", nil

			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	request := func(errors chan<- error) {
		_, err := client.LoopOutTerms(
			t.Context(), &swapserverrpc.ServerLoopOutTermsRequest{},
		)
		errors <- err
	}

	errors := make(chan error, 2)
	go request(errors)
	<-firstChallenge
	go request(errors)

	select {
	case <-bothChallenged:

	case <-t.Context().Done():
		t.Fatal("second request did not receive an L402 challenge")
	}
	close(releasePayment)

	for range 2 {
		if err := <-errors; err != nil {
			t.Fatalf("request terms: %v", err)
		}
	}
	if challengeCount.Load() != 1 {
		t.Fatalf("unexpected challenge count: %v", challengeCount.Load())
	}
	if paidCount.Load() != 2 {
		t.Fatalf("unexpected authorized request count: %v", paidCount.Load())
	}
}

func TestPaymentRequiredWithoutHandler(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		_ *http.Request) {

		w.Header().Set(
			"WWW-Authenticate",
			`LSAT macaroon="macaroon-secret", invoice="invoice-secret"`,
		)
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = client.FetchL402(
		context.Background(), &swapserverrpc.FetchL402Request{},
	)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unexpected status: %v", err)
	}

	var paymentError *PaymentRequiredError
	if !errors.As(err, &paymentError) {
		t.Fatalf("unexpected error type: %T", err)
	}
	if paymentError.Challenge.Scheme != "LSAT" {
		t.Fatalf("unexpected challenge: %+v", paymentError.Challenge)
	}
	if strings.Contains(err.Error(), "macaroon-secret") ||
		strings.Contains(err.Error(), "invoice-secret") {

		t.Fatalf("challenge secret leaked: %v", err)
	}
}

func TestRESTErrorMapping(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		_ *http.Request) {

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(
			w, `{"code":3,"message":"bad amount","details":[]}`,
		)
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = client.LoopOutQuote(
		context.Background(),
		&swapserverrpc.ServerLoopOutQuoteRequest{},
	)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unexpected status: %v", err)
	}
	if status.Convert(err).Message() != "bad amount" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestGRPCGatewayErrorEnvelopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode codes.Code
	}{
		{
			name: "unary numeric status",
			body: `{"code":9,"message":"not ready",` +
				`"details":[]}`,
			wantCode: codes.FailedPrecondition,
		},
		{
			name: "nested stream status",
			body: `{"error":{"code":6,"message":"duplicate",` +
				`"details":[]}}`,
			wantCode: codes.AlreadyExists,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = io.WriteString(w, test.body)
				},
			))
			defer server.Close()

			client, err := New(server.URL)
			if err != nil {
				t.Fatalf("create client: %v", err)
			}
			_, err = client.LoopOutQuote(
				context.Background(),
				&swapserverrpc.ServerLoopOutQuoteRequest{},
			)
			if status.Code(err) != test.wantCode {
				t.Fatalf("unexpected status: %v", err)
			}
		})
	}
}

func TestDecodeGRPCGatewayStreamError(t *testing.T) {
	t.Parallel()

	_, err := decodeLoopOutUpdate([]byte(
		`{"error":{"code":10,"message":"aborted","details":[]}}`,
	))
	if status.Code(err) != codes.Aborted {
		t.Fatalf("unexpected stream error: %v", err)
	}
}

func TestLoopOutUpdatesReconnectAndDeduplicate(t *testing.T) {
	t.Parallel()

	initiated := &swapserverrpc.SubscribeLoopOutUpdatesResponse{
		TimestampNs: 1,
		State:       swapserverrpc.ServerSwapState_SERVER_INITIATED,
	}
	success := &swapserverrpc.SubscribeLoopOutUpdatesResponse{
		TimestampNs: 2,
		State:       swapserverrpc.ServerSwapState_SERVER_SUCCESS,
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {

		if r.URL.Path != LoopOutUpdatesPath {
			t.Errorf("unexpected path: %v", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected accept header: %v", r.Header.Get("Accept"))
		}

		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		request := new(swapserverrpc.SubscribeUpdatesRequest)
		if err := protojson.Unmarshal(requestBody, request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if string(request.SwapHash) != "swap-hash" {
			t.Errorf("unexpected swap hash: %x", request.SwapHash)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Stream", "yes")
		writeNDJSON(t, w, initiated)
		if requestCount.Add(1) > 1 {
			writeNDJSON(t, w, success)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, WithReconnectPolicy(ReconnectPolicy{
		MaxAttempts: 2,
	}))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	stream, err := client.SubscribeLoopOutUpdates(
		context.Background(), &swapserverrpc.SubscribeUpdatesRequest{
			SwapHash: []byte("swap-hash"),
		},
	)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() {
		_ = stream.CloseSend()
	}()

	header, err := stream.Header()
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	if got := header.Get("x-stream"); len(got) != 1 || got[0] != "yes" {
		t.Fatalf("unexpected stream header: %v", got)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("receive first update: %v", err)
	}
	if !proto.Equal(first, initiated) {
		t.Fatalf("unexpected first update: %v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("receive second update: %v", err)
	}
	if !proto.Equal(second, success) {
		t.Fatalf("unexpected second update: %v", second)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected terminal EOF, got: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("unexpected request count: %v", requestCount.Load())
	}
}

func TestLoopOutUpdatesReconnectLimit(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		_ *http.Request) {

		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
	}))
	defer server.Close()

	client, err := New(server.URL, WithReconnectPolicy(ReconnectPolicy{
		MaxAttempts: 1,
	}))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	stream, err := client.SubscribeLoopOutUpdates(
		context.Background(), &swapserverrpc.SubscribeUpdatesRequest{},
	)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() {
		_ = stream.CloseSend()
	}()

	_, err = stream.Recv()
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("unexpected reconnect error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("unexpected request count: %v", requestCount.Load())
	}
}

func TestParseL402Challenge(t *testing.T) {
	t.Parallel()

	challenge, ok := ParseL402Challenge(
		`l402 macaroon="mac", invoice="lnbc1\"quoted", extra=value`,
	)
	if !ok {
		t.Fatalf("challenge not recognized")
	}
	if challenge.Macaroon != "mac" ||
		challenge.Invoice != `lnbc1"quoted` ||
		challenge.Params["extra"] != "value" {

		t.Fatalf("unexpected challenge: %+v", challenge)
	}

	if _, ok := ParseL402Challenge(`Basic realm="test"`); ok {
		t.Fatalf("accepted non-L402 challenge")
	}
}

func TestUnsupportedRPC(t *testing.T) {
	t.Parallel()

	client, err := New("https://loop.example.com")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = client.LoopInQuote(
		context.Background(),
		&swapserverrpc.ServerLoopInQuoteRequest{},
	)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("unexpected status: %v", err)
	}
}

func writeProtoJSON(t *testing.T, w http.ResponseWriter,
	message proto.Message) {

	t.Helper()

	body, err := marshalJSON.Marshal(message)
	if err != nil {
		t.Errorf("marshal response: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(body); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func writeNDJSON(t *testing.T, w http.ResponseWriter, message proto.Message) {
	t.Helper()

	body, err := marshalJSON.Marshal(message)
	if err != nil {
		t.Errorf("marshal update: %v", err)
		return
	}
	body = append(body, '\n')
	if _, err := w.Write(body); err != nil {
		t.Errorf("write update: %v", err)
	}
}
