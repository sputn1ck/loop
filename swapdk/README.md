# SwapDK

SwapDK is a portable SDK for mobile and web wallets that enables self-custodial swaps through a central `loop` daemon.

## Overview

The SwapDK consists of two main components:

*   **A client-side SDK:** This is a Go library that can be compiled for mobile (gomobile) and web (WASM). It provides a simple API for wallets to interact with the `loop` daemon.
*   **A server-side service:** This is an HTTP server that runs as part of the `loop` daemon. It manages clients, handles swap requests, and queues signing requests for the clients.

## Getting Started

### Client-Side

To use the SwapDK in your wallet, you will need to import the `github.com/lightninglabs/loop/swapdk` package.

#### Creating a new context

A new context can be created by calling the `GenerateNewContext` function. This will create a new private key and mnemonic.

```go
ctx, err := swapdk.GenerateNewContext()
if err != nil {
    // Handle error
}
```

#### Restoring a context from a mnemonic

A context can be restored from a mnemonic by calling the `NewContextFromMnemonic` function.

```go
ctx, err := swapdk.NewContextFromMnemonic(mnemonic)
if err != nil {
    // Handle error
}
```

#### Creating a new client

A new client can be created by calling the `NewClient` function.

```go
client := swapdk.NewClient("http://localhost:8081")
```

#### Fetching signing events

Pending signing events can be fetched by calling the `GetEvents` method on the client.

```go
events, err := client.GetEvents()
if err != nil {
    // Handle error
}
```

#### Responding to a signing event

A signing event can be responded to by calling the `RespondToEvent` method on the client.

```go
err := client.RespondToEvent(event.Id, &swapdk.SigningResponse{
    Response: signature,
})
if err != nil {
    // Handle error
}
```

### Server-Side

The SwapDK server is started as part of the `loopd` daemon. To enable the SwapDK server, you will need to add the following to your `loopd.conf`:

```
swapdk=true
```

## API Reference

### Client-Side

#### `GenerateNewContext() (*Context, error)`

Creates a new context.

#### `NewContextFromMnemonic(m string) (*Context, error)`

Restores a context from a mnemonic.

#### `(c *Context) Sign(message []byte) ([]byte, error)`

Signs a message.

#### `(c *Context) PublicKeyHex() string`

Returns the public key as a hex string.

#### `NewClient(serverAddr string) *Client`

Creates a new client.

#### `(c *Client) GetEvents() ([]*SigningRequest, error)`

Fetches pending signing events.

#### `(c *Client) RespondToEvent(id [32]byte, response *SigningResponse) error`

Responds to a signing event.

#### `(c *Client) GetBalance() (int64, error)`

Fetches the balance.

#### `(c *Client) GetTransactions() ([]string, error)`

Fetches the transactions.

### Server-Side

#### `POST /v1/swapdk/register`

Registers a new client.

#### `GET /v1/swapdk/events`

Fetches pending signing events.

#### `POST /v1/swapdk/events/{id}/respond`

Responds to a signing event.

#### `GET /v1/swapdk/balance`

Fetches the balance.

#### `GET /v1/swapdk/transactions`

Fetches the transactions.