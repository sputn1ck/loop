package restclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lightninglabs/loop/swapserverrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	defaultJSONContentType = "application/json"
	browserJSONContentType = "text/plain"

	// LoopOutTermsPath is the grpc-gateway endpoint for Loop Out terms.
	LoopOutTermsPath = "/looprpc.SwapServer/LoopOutTerms"

	// LoopOutQuotePath is the grpc-gateway endpoint for Loop Out quotes.
	LoopOutQuotePath = "/looprpc.SwapServer/LoopOutQuote"

	// NewLoopOutPath is the grpc-gateway endpoint for creating a Loop Out.
	NewLoopOutPath = "/looprpc.SwapServer/NewLoopOutSwap"

	// LoopOutPreimagePath is the grpc-gateway endpoint for pushing a
	// preimage.
	LoopOutPreimagePath = "/looprpc.SwapServer/LoopOutPushPreimage"

	// CancelLoopOutPath is the grpc-gateway endpoint for canceling a Loop
	// Out.
	CancelLoopOutPath = "/looprpc.SwapServer/CancelLoopOutSwap"

	// MuSig2SignSweepPath is the grpc-gateway endpoint for signing a sweep.
	MuSig2SignSweepPath = "/looprpc.SwapServer/MuSig2SignSweep"

	// PushKeyPath is the grpc-gateway endpoint for pushing a swap key.
	PushKeyPath = "/looprpc.SwapServer/PushKey"

	// FetchL402Path is the grpc-gateway endpoint that triggers an L402
	// challenge.
	FetchL402Path = "/looprpc.SwapServer/FetchL402"

	// LoopOutUpdatesPath is the grpc-gateway endpoint for Loop Out state
	// updates.
	LoopOutUpdatesPath = "/looprpc.SwapServer/SubscribeLoopOutUpdates"

	maxErrorBodySize = 1 << 20
)

var (
	marshalJSON = protojson.MarshalOptions{
		UseProtoNames: true,
	}
	unmarshalJSON = protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
)

// AuthorizationSource returns the current value for the HTTP Authorization
// header. It can load a previously paid L402 token from browser persistence.
type AuthorizationSource func(context.Context) (string, error)

// L402ChallengeHandler satisfies an L402 challenge and returns the complete
// Authorization header value to use for the retried request.
type L402ChallengeHandler func(context.Context, L402Challenge) (string, error)

// ReconnectPolicy controls consecutive Loop Out update stream reconnects.
type ReconnectPolicy struct {
	// MaxAttempts is the number of reconnects allowed after the initial
	// connection. A negative value retries until the context is canceled.
	MaxAttempts int

	// InitialBackoff is the delay before the first reconnect.
	InitialBackoff time.Duration

	// MaxBackoff caps exponential reconnect delays.
	MaxBackoff time.Duration
}

// Option configures a Client.
type Option func(*Client)

// Client implements swapserverrpc.SwapServerClient over browser-safe HTTP.
// The initial implementation intentionally supports the Loop Out RPC surface;
// unrelated RPCs return codes.Unimplemented until the server exposes matching
// browser endpoints.
type Client struct {
	baseURL            string
	httpClient         *http.Client
	headers            http.Header
	requestContentType string

	authMu              sync.RWMutex
	challengeMu         sync.Mutex
	authorization       string
	authorizationSource AuthorizationSource
	challengeHandler    L402ChallengeHandler

	reconnect ReconnectPolicy
}

var _ swapserverrpc.SwapServerClient = (*Client)(nil)

// New creates a browser-safe REST client for the Loop server.
func New(baseURL string, opts ...Option) (*Client, error) {
	normalizedURL, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	client := &Client{
		baseURL:            normalizedURL,
		httpClient:         http.DefaultClient,
		headers:            make(http.Header),
		requestContentType: defaultJSONContentType,
		reconnect: ReconnectPolicy{
			MaxAttempts:    8,
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     2 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(client)
	}

	if client.httpClient == nil {
		return nil, fmt.Errorf("HTTP client must not be nil")
	}
	if client.reconnect.MaxBackoff < 0 ||
		client.reconnect.InitialBackoff < 0 {

		return nil, fmt.Errorf("reconnect backoff must not be negative")
	}
	if client.reconnect.MaxBackoff > 0 &&
		client.reconnect.InitialBackoff > client.reconnect.MaxBackoff {

		return nil, fmt.Errorf("initial reconnect backoff exceeds maximum")
	}

	return client, nil
}

// WithHTTPClient makes the REST client use httpClient.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(client *Client) {
		client.httpClient = httpClient
	}
}

// WithHeader adds a static HTTP header to every request.
func WithHeader(key, value string) Option {
	return func(client *Client) {
		client.headers.Add(key, value)
	}
}

// WithBrowserCompatibleJSON sends proto JSON request bodies with the
// CORS-safelisted text/plain content type. This avoids a Content-Type preflight
// in browsers while retaining the default application/json behavior for all
// other clients. The target gRPC-Gateway must accept JSON for text/plain.
func WithBrowserCompatibleJSON() Option {
	return func(client *Client) {
		client.requestContentType = browserJSONContentType
	}
}

// WithAuthorization sets an initial Authorization header value. A successful
// challenge handler replaces this cached value.
func WithAuthorization(authorization string) Option {
	return func(client *Client) {
		client.authorization = authorization
	}
}

// WithAuthorizationSource configures a dynamic Authorization header source.
func WithAuthorizationSource(source AuthorizationSource) Option {
	return func(client *Client) {
		client.authorizationSource = source
	}
}

// WithL402ChallengeHandler configures challenge handling and one-shot retry.
func WithL402ChallengeHandler(handler L402ChallengeHandler) Option {
	return func(client *Client) {
		client.challengeHandler = handler
	}
}

// WithReconnectPolicy configures Loop Out update stream reconnects.
func WithReconnectPolicy(policy ReconnectPolicy) Option {
	return func(client *Client) {
		client.reconnect = policy
	}
}

// NormalizeBaseURL validates and canonicalizes a grpc-gateway base URL. The
// result is suitable both for request construction and for binding persisted
// bearer credentials to the gateway that issued them.
func NormalizeBaseURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("base URL must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("base URL must include a host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("base URL must not include user information")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("base URL must not include query or fragment")
	}

	parsed.Host = strings.ToLower(parsed.Host)
	hostname := parsed.Hostname()
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") ||
		(parsed.Scheme == "https" && port == "443") {

		if strings.Contains(hostname, ":") {
			parsed.Host = "[" + hostname + "]"
		} else {
			parsed.Host = hostname
		}
	}

	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *Client) currentAuthorization(ctx context.Context) (string, error) {
	if c.authorizationSource != nil {
		authorization, err := c.authorizationSource(ctx)
		if err != nil {
			return "", fmt.Errorf("load authorization: %w", err)
		}
		if authorization != "" {
			return authorization, nil
		}
	}

	c.authMu.RLock()
	defer c.authMu.RUnlock()

	return c.authorization, nil
}

func (c *Client) cacheAuthorization(authorization string) {
	c.authMu.Lock()
	c.authorization = authorization
	c.authMu.Unlock()
}

func (c *Client) post(ctx context.Context, path string, in,
	out proto.Message) error {

	body, err := marshalJSON.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.do(ctx, path, "application/json", body)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices {

		return responseError(resp)
	}

	responseBody, err := readBounded(resp.Body, maxErrorBodySize)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}

	if err := unmarshalJSON.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}

func (c *Client) do(ctx context.Context, path, accept string,
	body []byte) (*http.Response, error) {

	authorization, err := c.currentAuthorization(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.doOnce(ctx, path, accept, body, authorization)
	if err != nil {
		return nil, err
	}

	challenge, paymentRequired := responseL402Challenge(resp)
	if !paymentRequired || c.challengeHandler == nil {
		return resp, nil
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodySize))
	_ = resp.Body.Close()

	// Serialize challenge payment and re-check persisted authorization after
	// taking the lock. Concurrent browser requests can all observe the same
	// unpaid challenge, but only the first one should pay it.
	c.challengeMu.Lock()
	defer c.challengeMu.Unlock()

	latestAuthorization, err := c.currentAuthorization(ctx)
	if err != nil {
		return nil, err
	}
	if latestAuthorization != "" && latestAuthorization != authorization {
		return c.doOnce(ctx, path, accept, body, latestAuthorization)
	}

	authorization, err = c.challengeHandler(ctx, challenge)
	if err != nil {
		return nil, fmt.Errorf("handle L402 challenge: %w", err)
	}
	if strings.TrimSpace(authorization) == "" {
		return nil, fmt.Errorf("handle L402 challenge: empty authorization")
	}
	c.cacheAuthorization(authorization)

	return c.doOnce(ctx, path, accept, body, authorization)
}

func (c *Client) doOnce(ctx context.Context, path, accept string, body []byte,
	authorization string) (*http.Response, error) {

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", c.requestContentType)
	req.Header.Set("Accept", accept)
	for key, values := range c.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if outgoing, ok := metadata.FromOutgoingContext(ctx); ok {
		for key, values := range outgoing {
			if strings.HasSuffix(strings.ToLower(key), "-bin") {
				continue
			}
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // Caller supplies URL.
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	return resp, nil
}

func responseError(resp *http.Response) error {
	body, err := readBounded(resp.Body, maxErrorBodySize)
	if err != nil {
		return fmt.Errorf("read error response: %w", err)
	}

	if challenge, ok := responseL402Challenge(resp); ok {
		return &PaymentRequiredError{
			StatusCode: resp.StatusCode,
			Challenge:  challenge,
		}
	}

	gatewayStatus, _ := decodeGatewayStatus(body)
	if gatewayStatus.Message == "" {
		gatewayStatus.Message = strings.TrimSpace(string(body))
	}
	if gatewayStatus.Message == "" {
		gatewayStatus.Message = http.StatusText(resp.StatusCode)
	}

	code := codeFromHTTPStatus(resp.StatusCode)
	if parsedCode, ok := parseGatewayCode(gatewayStatus.Code); ok {
		code = parsedCode
	}

	return status.Error(code, gatewayStatus.Message)
}

// gatewayStatus is the google.rpc.Status JSON shape emitted by grpc-gateway.
// Code is kept raw so legacy string values and standard numeric values can
// both be accepted.
type gatewayStatus struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

func decodeGatewayStatus(body []byte) (gatewayStatus, bool) {
	var wrapped struct {
		Error *gatewayStatus `json:"error"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return gatewayStatus{}, false
	}
	if wrapped.Error != nil {
		return *wrapped.Error, true
	}

	var direct gatewayStatus
	if err := json.Unmarshal(body, &direct); err != nil {
		return gatewayStatus{}, false
	}
	if len(direct.Code) == 0 && direct.Message == "" {
		return gatewayStatus{}, false
	}

	return direct, true
}

func parseGatewayCode(raw json.RawMessage) (codes.Code, bool) {
	if len(raw) == 0 {
		return codes.Unknown, false
	}

	var numeric int32
	if err := json.Unmarshal(raw, &numeric); err == nil {
		code := codes.Code(numeric)
		if code >= codes.OK && code <= codes.Unauthenticated {
			return code, true
		}

		return codes.Unknown, false
	}

	var textual string
	if err := json.Unmarshal(raw, &textual); err != nil {
		return codes.Unknown, false
	}
	if numeric, err := strconv.ParseInt(textual, 10, 32); err == nil {
		return parseGatewayCode(json.RawMessage(strconv.FormatInt(numeric, 10)))
	}

	return parseCode(textual)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}

	return body, nil
}

func codeFromHTTPStatus(httpStatus int) codes.Code {
	switch httpStatus {
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.Aborted
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case 499:
		return codes.Canceled
	case http.StatusNotImplemented:
		return codes.Unimplemented
	case http.StatusServiceUnavailable, http.StatusBadGateway:

		return codes.Unavailable
	case http.StatusGatewayTimeout:
		return codes.DeadlineExceeded
	default:
		if httpStatus >= http.StatusInternalServerError {
			return codes.Internal
		}

		return codes.Unknown
	}
}

func parseCode(value string) (codes.Code, bool) {
	normalized := strings.NewReplacer("_", "", "-", "", " ", "").
		Replace(strings.ToLower(value))
	for code := codes.OK; code <= codes.Unauthenticated; code++ {
		candidate := strings.NewReplacer("_", "", "-", "", " ", "").
			Replace(strings.ToLower(code.String()))
		if candidate == normalized {
			return code, true
		}
	}

	return codes.Unknown, false
}

func unsupported(method string) error {
	return status.Errorf(
		codes.Unimplemented, "%s is not available over browser REST", method,
	)
}

// LoopOutTerms returns the server's Loop Out terms.
func (c *Client) LoopOutTerms(ctx context.Context,
	in *swapserverrpc.ServerLoopOutTermsRequest, _ ...grpc.CallOption) (
	*swapserverrpc.ServerLoopOutTerms, error) {

	out := new(swapserverrpc.ServerLoopOutTerms)
	if err := c.post(ctx, LoopOutTermsPath, in, out); err != nil {
		return nil, err
	}

	return out, nil
}

// NewLoopOutSwap creates a Loop Out swap.
func (c *Client) NewLoopOutSwap(ctx context.Context,
	in *swapserverrpc.ServerLoopOutRequest, _ ...grpc.CallOption) (
	*swapserverrpc.ServerLoopOutResponse, error) {

	out := new(swapserverrpc.ServerLoopOutResponse)
	if err := c.post(ctx, NewLoopOutPath, in, out); err != nil {
		return nil, err
	}

	return out, nil
}

// LoopOutPushPreimage pushes a Loop Out preimage.
func (c *Client) LoopOutPushPreimage(ctx context.Context,
	in *swapserverrpc.ServerLoopOutPushPreimageRequest,
	_ ...grpc.CallOption) (*swapserverrpc.ServerLoopOutPushPreimageResponse,
	error) {

	out := new(swapserverrpc.ServerLoopOutPushPreimageResponse)
	if err := c.post(ctx, LoopOutPreimagePath, in, out); err != nil {
		return nil, err
	}

	return out, nil
}

// LoopOutQuote returns a Loop Out quote.
func (c *Client) LoopOutQuote(ctx context.Context,
	in *swapserverrpc.ServerLoopOutQuoteRequest, _ ...grpc.CallOption) (
	*swapserverrpc.ServerLoopOutQuote, error) {

	out := new(swapserverrpc.ServerLoopOutQuote)
	if err := c.post(ctx, LoopOutQuotePath, in, out); err != nil {
		return nil, err
	}

	return out, nil
}

// LoopInTerms is not yet exposed by the browser REST API.
func (c *Client) LoopInTerms(context.Context,
	*swapserverrpc.ServerLoopInTermsRequest, ...grpc.CallOption) (
	*swapserverrpc.ServerLoopInTerms, error) {

	return nil, unsupported("LoopInTerms")
}

// NewLoopInSwap is not yet exposed by the browser REST API.
func (c *Client) NewLoopInSwap(context.Context,
	*swapserverrpc.ServerLoopInRequest, ...grpc.CallOption) (
	*swapserverrpc.ServerLoopInResponse, error) {

	return nil, unsupported("NewLoopInSwap")
}

// LoopInQuote is not yet exposed by the browser REST API.
func (c *Client) LoopInQuote(context.Context,
	*swapserverrpc.ServerLoopInQuoteRequest, ...grpc.CallOption) (
	*swapserverrpc.ServerLoopInQuoteResponse, error) {

	return nil, unsupported("LoopInQuote")
}

// SubscribeLoopInUpdates is not yet exposed by the browser REST API.
func (c *Client) SubscribeLoopInUpdates(context.Context,
	*swapserverrpc.SubscribeUpdatesRequest, ...grpc.CallOption) (
	swapserverrpc.SwapServer_SubscribeLoopInUpdatesClient, error) {

	return nil, unsupported("SubscribeLoopInUpdates")
}

// CancelLoopOutSwap cancels a Loop Out swap.
func (c *Client) CancelLoopOutSwap(ctx context.Context,
	in *swapserverrpc.CancelLoopOutSwapRequest, _ ...grpc.CallOption) (
	*swapserverrpc.CancelLoopOutSwapResponse, error) {

	out := new(swapserverrpc.CancelLoopOutSwapResponse)
	if err := c.post(ctx, CancelLoopOutPath, in, out); err != nil {
		return nil, err
	}

	return out, nil
}

// Probe is not yet exposed by the browser REST API.
func (c *Client) Probe(context.Context, *swapserverrpc.ServerProbeRequest,
	...grpc.CallOption) (*swapserverrpc.ServerProbeResponse, error) {

	return nil, unsupported("Probe")
}

// RecommendRoutingPlugin is not yet exposed by the browser REST API.
func (c *Client) RecommendRoutingPlugin(context.Context,
	*swapserverrpc.RecommendRoutingPluginReq, ...grpc.CallOption) (
	*swapserverrpc.RecommendRoutingPluginRes, error) {

	return nil, unsupported("RecommendRoutingPlugin")
}

// ReportRoutingResult is not yet exposed by the browser REST API.
func (c *Client) ReportRoutingResult(context.Context,
	*swapserverrpc.ReportRoutingResultReq, ...grpc.CallOption) (
	*swapserverrpc.ReportRoutingResultRes, error) {

	return nil, unsupported("ReportRoutingResult")
}

// MuSig2SignSweep requests a MuSig2 Loop Out sweep signature.
func (c *Client) MuSig2SignSweep(ctx context.Context,
	in *swapserverrpc.MuSig2SignSweepReq, _ ...grpc.CallOption) (
	*swapserverrpc.MuSig2SignSweepRes, error) {

	out := new(swapserverrpc.MuSig2SignSweepRes)
	if err := c.post(ctx, MuSig2SignSweepPath, in, out); err != nil {
		return nil, err
	}

	return out, nil
}

// PushKey pushes a client swap key to the server.
func (c *Client) PushKey(ctx context.Context,
	in *swapserverrpc.ServerPushKeyReq, _ ...grpc.CallOption) (
	*swapserverrpc.ServerPushKeyRes, error) {

	out := new(swapserverrpc.ServerPushKeyRes)
	if err := c.post(ctx, PushKeyPath, in, out); err != nil {
		return nil, err
	}

	return out, nil
}

// FetchL402 triggers L402 challenge creation.
func (c *Client) FetchL402(ctx context.Context,
	in *swapserverrpc.FetchL402Request, _ ...grpc.CallOption) (
	*swapserverrpc.FetchL402Response, error) {

	out := new(swapserverrpc.FetchL402Response)
	if err := c.post(ctx, FetchL402Path, in, out); err != nil {
		return nil, err
	}

	return out, nil
}

// SubscribeNotifications is not yet exposed by the browser REST API.
func (c *Client) SubscribeNotifications(context.Context,
	*swapserverrpc.SubscribeNotificationsRequest, ...grpc.CallOption) (
	swapserverrpc.SwapServer_SubscribeNotificationsClient, error) {

	return nil, unsupported("SubscribeNotifications")
}
