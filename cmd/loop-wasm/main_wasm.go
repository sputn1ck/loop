//go:build js && wasm

// Command loop-wasm exposes the embedded Loop-Out-only runtime to browser
// JavaScript. It uses bufconn for local looprpc calls and HTTPS for the Loop
// server's existing grpc-gateway; it does not open or bridge a TCP socket.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"syscall/js"

	"github.com/lightninglabs/loop/looprpc"
	wasmsdk "github.com/lightninglabs/loop/sdk/wasm"
	"github.com/lightningnetwork/lnd/lntypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const rpcMethodPrefix = "looprpc.SwapClient."

type jsonRPCCallback = func(context.Context, *grpc.ClientConn, string,
	func(string, error))

type rpcResult struct {
	json string
	err  error
}

type activeRuntime struct {
	lifecycle sync.Mutex
	mu        sync.RWMutex
	runtime   *wasmsdk.Runtime
}

var (
	runtimeState activeRuntime
	rpcCallbacks = make(map[string]jsonRPCCallback)

	rpcAliases = map[string]string{
		"loopOut":      rpcMethodPrefix + "LoopOut",
		"loopOutTerms": rpcMethodPrefix + "LoopOutTerms",
		"loopOutQuote": rpcMethodPrefix + "LoopOutQuote",
		"listSwaps":    rpcMethodPrefix + "ListSwaps",
		"swapInfo":     rpcMethodPrefix + "SwapInfo",
		"getInfo":      rpcMethodPrefix + "GetInfo",
		"stopDaemon":   rpcMethodPrefix + "StopDaemon",
		"monitor":      rpcMethodPrefix + "Monitor",
	}
)

func init() {
	looprpc.RegisterSwapClientJSONCallbacks(rpcCallbacks)
}

// main installs one Promise-based browser API and keeps its callbacks live.
func main() {
	js.Global().Set("loopWasmCall", js.FuncOf(loopCall))
	js.Global().Call(
		"dispatchEvent",
		js.Global().Get("Event").New("loop-wasm-ready"),
	)

	select {}
}

// loopCall dispatches lifecycle operations and generated looprpc JSON calls.
func loopCall(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return rejected(errors.New("method is required"))
	}

	method := args[0].String()
	request := js.Undefined()
	if len(args) > 1 {
		request = args[1]
	}

	switch method {
	case "start":
		return promise(func() (any, error) {
			return js.Null(), start(request)
		})

	case "stop":
		return promise(func() (any, error) {
			return js.Null(), stop()
		})

	case "exportRecovery":
		return promise(exportRecovery)

	case "restoreRecovery":
		return promise(func() (any, error) {
			return restoreRecovery(request)
		})

	case "monitor", rpcMethodPrefix + "Monitor":
		return promise(func() (any, error) {
			return newSubscription(request)
		})

	default:
		return promise(func() (any, error) {
			response, err := invokeRPC(method, request)
			if err != nil {
				return nil, err
			}

			return parseJSON(response), nil
		})
	}
}

func start(request js.Value) error {
	config, err := parseStartConfig(jsonBytes(request))
	if err != nil {
		return err
	}

	payer, err := invoicePayer(request, config.L402PayerName)
	if err != nil {
		return err
	}
	config.Runtime.L402InvoicePayer = payer

	runtimeState.lifecycle.Lock()
	defer runtimeState.lifecycle.Unlock()

	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()

	if runtimeState.runtime != nil {
		select {
		case <-runtimeState.runtime.Done():
			runtimeState.runtime = nil

		default:
			return errors.New("browser Loop runtime is already running")
		}
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), config.StartupTimeout,
	)
	defer cancel()

	runtime, err := wasmsdk.Start(ctx, config.Runtime)
	if err != nil {
		return err
	}
	runtimeState.runtime = runtime
	go clearStoppedRuntime(runtime)

	return nil
}

func clearStoppedRuntime(runtime *wasmsdk.Runtime) {
	<-runtime.Done()

	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()

	if runtimeState.runtime == runtime {
		runtimeState.runtime = nil
	}
}

func stop() error {
	runtimeState.lifecycle.Lock()
	defer runtimeState.lifecycle.Unlock()

	runtimeState.mu.RLock()
	runtime := runtimeState.runtime
	runtimeState.mu.RUnlock()
	if runtime == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), defaultStopTimeout,
	)
	defer cancel()

	return runtime.Stop(ctx)
}

func exportRecovery() (any, error) {
	runtimeState.lifecycle.Lock()
	defer runtimeState.lifecycle.Unlock()

	runtimeState.mu.RLock()
	runtime := runtimeState.runtime
	runtimeState.mu.RUnlock()
	if runtime == nil {
		return nil, errors.New("browser Loop runtime is not running")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), defaultStartupTimeout,
	)
	defer cancel()

	bundle, err := runtime.ExportRecoveryBundle(ctx)
	if err != nil {
		return nil, err
	}

	response := js.Global().Get("Object").New()
	response.Set("bundle", string(bundle))
	response.Set("plaintext", true)
	response.Set(
		"warning",
		"MUST encrypt and authenticate this recovery bundle before export",
	)

	return response, nil
}

func restoreRecovery(request js.Value) (any, error) {
	config, err := parseRestoreConfig(jsonBytes(request))
	if err != nil {
		return nil, err
	}

	runtimeState.lifecycle.Lock()
	defer runtimeState.lifecycle.Unlock()

	runtimeState.mu.RLock()
	running := runtimeState.runtime != nil
	runtimeState.mu.RUnlock()
	if running {
		return nil, errors.New(
			"stop the browser Loop runtime before restoring recovery",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), config.Timeout,
	)
	defer cancel()

	restored, err := wasmsdk.RestoreRecoveryBundle(ctx, config.Runtime)
	if err != nil {
		return nil, err
	}

	response := js.Global().Get("Object").New()
	response.Set("seed", hex.EncodeToString(restored.Seed))

	return response, nil
}

func runtimeConnection() (*grpc.ClientConn, error) {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()

	if runtimeState.runtime == nil {
		return nil, errors.New("browser Loop runtime is not running")
	}
	select {
	case <-runtimeState.runtime.Done():
		return nil, errors.New("browser Loop runtime has stopped")

	default:
	}

	connection := runtimeState.runtime.ClientConn()
	if connection == nil {
		return nil, errors.New("embedded looprpc connection is unavailable")
	}

	return connection, nil
}

func invokeRPC(method string, request js.Value) (string, error) {
	fullMethod, callback, err := resolveRPC(method)
	if err != nil {
		return "", err
	}
	if fullMethod == rpcMethodPrefix+"Monitor" {
		return "", errors.New("use monitor as a streaming call")
	}

	connection, err := runtimeConnection()
	if err != nil {
		return "", err
	}

	result := make(chan rpcResult, 1)
	callback(
		context.Background(), connection, requestJSON(request),
		func(response string, err error) {
			result <- rpcResult{json: response, err: err}
		},
	)

	response := <-result
	return response.json, response.err
}

func resolveRPC(method string) (string, jsonRPCCallback, error) {
	fullMethod := method
	if alias, ok := rpcAliases[method]; ok {
		fullMethod = alias
	}

	allowed := false
	for _, candidate := range rpcAliases {
		if fullMethod == candidate {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", nil, fmt.Errorf(
			"unsupported embedded RPC method %q", method,
		)
	}

	callback := rpcCallbacks[fullMethod]
	if callback == nil {
		return "", nil, fmt.Errorf(
			"generated JSON callback is missing for %s", fullMethod,
		)
	}

	return fullMethod, callback, nil
}

func newSubscription(request js.Value) (js.Value, error) {
	_, callback, err := resolveRPC("monitor")
	if err != nil {
		return js.Undefined(), err
	}
	connection, err := runtimeConnection()
	if err != nil {
		return js.Undefined(), err
	}

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan rpcResult, 32)
	callback(ctx, connection, requestJSON(request),
		func(response string, err error) {
			select {
			case events <- rpcResult{json: response, err: err}:

			case <-ctx.Done():
			}
		},
	)

	handle := js.Global().Get("Object").New()
	var nextFunction, closeFunction js.Func
	var closeOnce sync.Once

	nextFunction = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return promise(func() (any, error) {
			select {
			case event := <-events:
				if streamFinished(event.err) {
					return js.Null(), nil
				}
				if event.err != nil {
					return nil, event.err
				}

				return parseJSON(event.json), nil

			case <-ctx.Done():
				return js.Null(), nil
			}
		})
	})

	closeFunction = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		closeOnce.Do(func() {
			cancel()
			nextFunction.Release()
			closeFunction.Release()
		})

		return js.Null()
	})

	handle.Set("next", nextFunction)
	handle.Set("close", closeFunction)

	return handle, nil
}

func streamFinished(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) ||
		status.Code(err) == codes.Canceled
}

func invoicePayer(request js.Value,
	configuredName string) (wasmsdk.L402InvoicePayer, error) {

	callback := js.Undefined()
	if request.Type() == js.TypeObject {
		callback = request.Get("l402_payer")
	}
	if callback.Type() == js.TypeString {
		configuredName = callback.String()
		callback = js.Undefined()
	}
	if callback.Type() != js.TypeFunction && configuredName != "" {
		callback = globalFunction(configuredName)
	}
	if callback.IsUndefined() || callback.IsNull() {
		if configuredName != "" {
			return nil, fmt.Errorf(
				"L402 invoice payer %q is not a global function",
				configuredName,
			)
		}

		return nil, nil
	}
	if callback.Type() != js.TypeFunction {
		return nil, errors.New("l402_payer must be a function or global name")
	}

	return func(ctx context.Context,
		invoice string) (lntypes.Preimage, error) {

		value, err := invokeJS(callback, invoice)
		if err != nil {
			return lntypes.Preimage{}, err
		}
		preimage, err := awaitString(ctx, value)
		if err != nil {
			return lntypes.Preimage{}, err
		}

		return lntypes.MakePreimageFromStr(strings.TrimSpace(preimage))
	}, nil
}

func globalFunction(name string) js.Value {
	value := js.Global()
	for _, part := range strings.Split(name, ".") {
		if part == "" || value.IsUndefined() || value.IsNull() {
			return js.Undefined()
		}
		value = value.Get(part)
	}

	return value
}

func invokeJS(callback js.Value, invoice string) (
	result js.Value, err error) {

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("invoke L402 invoice payer: %v", recovered)
		}
	}()

	return callback.Invoke(invoice), nil
}

func awaitString(ctx context.Context, value js.Value) (string, error) {
	if value.Type() == js.TypeString {
		return value.String(), nil
	}
	if value.Type() != js.TypeObject && value.Type() != js.TypeFunction {
		return "", errors.New(
			"L402 invoice payer must return a hex preimage or Promise",
		)
	}
	if value.Get("then").Type() != js.TypeFunction {
		return "", errors.New(
			"L402 invoice payer must return a hex preimage or Promise",
		)
	}

	type awaitResult struct {
		value string
		err   error
	}
	result := make(chan awaitResult, 1)
	resolve := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 || args[0].Type() != js.TypeString {
			result <- awaitResult{err: errors.New(
				"L402 payer Promise must resolve to a hex preimage",
			)}
		} else {
			result <- awaitResult{value: args[0].String()}
		}

		return nil
	})
	reject := js.FuncOf(func(_ js.Value, args []js.Value) any {
		message := "L402 invoice payer rejected payment"
		if len(args) != 0 {
			message = jsValueError(args[0])
		}
		result <- awaitResult{err: errors.New(message)}

		return nil
	})
	value.Call("then", resolve, reject)

	select {
	case response := <-result:
		resolve.Release()
		reject.Release()

		return response.value, response.err

	case <-ctx.Done():
		// The Promise may still settle and invoke its callbacks, so they cannot
		// safely be released on this path.
		return "", ctx.Err()
	}
}

func promise(fn func() (any, error)) any {
	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve, reject := args[0], args[1]

		go func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					reject.Invoke(jsError(fmt.Errorf(
						"browser Loop callback panic: %v", recovered,
					)))
				}
			}()

			value, err := fn()
			if err != nil {
				reject.Invoke(jsError(err))

				return
			}
			resolve.Invoke(value)
		}()

		return nil
	})

	promise := js.Global().Get("Promise").New(executor)
	executor.Release()

	return promise
}

func rejected(err error) any {
	return js.Global().Get("Promise").Call("reject", jsError(err))
}

func jsError(err error) js.Value {
	return js.Global().Get("Error").New(err.Error())
}

func jsValueError(value js.Value) string {
	if value.Type() == js.TypeObject {
		message := value.Get("message")
		if message.Type() == js.TypeString {
			return message.String()
		}
	}

	return value.String()
}

func requestJSON(value js.Value) string {
	if value.IsUndefined() || value.IsNull() {
		return "{}"
	}

	return js.Global().Get("JSON").Call("stringify", value).String()
}

func jsonBytes(value js.Value) []byte {
	if value.IsUndefined() || value.IsNull() {
		return nil
	}

	return []byte(requestJSON(value))
}

func parseJSON(value string) js.Value {
	if strings.TrimSpace(value) == "" {
		return js.Null()
	}

	return js.Global().Get("JSON").Call("parse", value)
}
