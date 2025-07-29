//go:build js && wasm

package swapdk

import (
	"encoding/hex"
	"syscall/js"
)

// globalHandles keeps Context pointers referenced from JavaScript so we don’t
// GC them.  A production build would implement a proper finalizer.
var globalHandles = make(map[int]*Context)
var nextHandle = 0

func retain(ctx *Context) int {
	h := nextHandle
	nextHandle++
	globalHandles[h] = ctx
	return h
}

func get(ctxID int) (*Context, bool) { c, ok := globalHandles[ctxID]; return c, ok }

func wasmGenerate(this js.Value, args []js.Value) interface{} {
	ctx, err := GenerateNewContext()
	if err != nil {
		return err.Error()
	}
	id := retain(ctx)
	return map[string]interface{}{
		"id":        id,
		"mnemonic":  ctx.mnemonic, // return once; caller stores it.
		"pubKeyHex": ctx.PublicKeyHex(),
	}
}

func wasmLoad(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return "mnemonic required"
	}
	ctx, err := NewContextFromMnemonic(args[0].String())
	if err != nil {
		return err.Error()
	}
	id := retain(ctx)
	return map[string]interface{}{
		"id":        id,
		"pubKeyHex": ctx.PublicKeyHex(),
	}
}

func wasmSign(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return "id, hexMessage required"
	}
	id := args[0].Int()
	msgBytes, _ := hex.DecodeString(args[1].String())
	ctx, ok := get(id)
	if !ok {
		return "invalid context id"
	}
	sig, err := ctx.Sign(msgBytes)
	if err != nil {
		return err.Error()
	}
	return hex.EncodeToString(sig)
}

func wasmGetPublicKey(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return "id required"
	}
	id := args[0].Int()
	ctx, ok := get(id)
	if !ok {
		return "invalid context id"
	}
	return ctx.PublicKeyHex()
}

func register() {
	sdkObj := js.Global().Get("Object").New()
	sdkObj.Set("generate", js.FuncOf(wasmGenerate))
	sdkObj.Set("load", js.FuncOf(wasmLoad))
	sdkObj.Set("sign", js.FuncOf(wasmSign))
	sdkObj.Set("getPublicKey", js.FuncOf(wasmGetPublicKey))
	sdkObj.Set("getClient", js.FuncOf(wasmGetClient))
	js.Global().Set("SDK", sdkObj)
}

func wasmGetClient(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return "serverAddr required"
	}
	serverAddr := args[0].String()
	client := NewClient(serverAddr)
	clientObj := js.Global().Get("Object").New()
	clientObj.Set("getEvents", wasmGetEvents(client))
	clientObj.Set("respondToEvent", wasmRespondToEvent(client))
	clientObj.Set("getBalance", wasmGetBalance(client))
	clientObj.Set("getTransactions", wasmGetTransactions(client))
	return clientObj
}

func wasmGetEvents(client *Client) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		events, err := client.GetEvents()
		if err != nil {
			return err.Error()
		}
		return events
	})
}

func wasmRespondToEvent(client *Client) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return "id, response required"
		}
		idStr := args[0].String()
		var id [32]byte
		copy(id[:], []byte(idStr))
		var resp SigningResponse
		// This is a simplified example. In a real implementation, you would
		// need to properly decode the response from the JS object.
		err := client.RespondToEvent(id, &resp)
		if err != nil {
			return err.Error()
		}
		return nil
	})
}

func wasmGetBalance(client *Client) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		balance, err := client.GetBalance()
		if err != nil {
			return err.Error()
		}
		return balance
	})
}

func wasmGetTransactions(client *Client) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		txs, err := client.GetTransactions()
		if err != nil {
			return err.Error()
		}
		return txs
	})
}

func main() { register(); select {} }
