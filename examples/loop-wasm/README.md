# Browser Loop Out demo

This static page drives the embedded Loop-Out-only WASM runtime. It talks to
the Loop server through its existing gRPC-Gateway, keeps Loop state in an OPFS
SQLite database, and uses Esplora for chain data. It does not open a TCP bridge
or connect to an external `lnd` daemon.

Build the flat runtime asset directory first:

```shell
make wasm-loop
```

Serve the repository root over HTTP with these response headers on every
resource:

```text
Cross-Origin-Opener-Policy: same-origin
Cross-Origin-Embedder-Policy: require-corp
Cross-Origin-Resource-Policy: same-origin
```

Also serve `.wasm` as `application/wasm`. Then open
`/examples/loop-wasm/`. The default worker URL points to
`/bin/wasm/loop-wasm-worker.js`, whose SQLite and Go WASM assets are colocated
in that directory.

The Esplora API and Loop server gRPC-Gateway must be browser reachable and
allow the page origin through CORS. The gateway edge must allow
`Content-Type` and `Authorization`, expose `WWW-Authenticate`, and avoid
buffering streaming responses. The page deliberately leaves both URLs blank
because they are deployment specific.

The initial L402 challenge is separate from the swap and prepay invoices. A
WebLN provider can pay it directly. A manual payer must return the 32-byte
payment preimage so the runtime can construct its reusable authorization.

## Recovery encryption

The runtime produces plaintext recovery JSON in page memory. The demo never
offers that value as a download. It derives an AES-256-GCM key from a
user-supplied passphrase with PBKDF2-SHA256, a random salt, and 600,000
iterations, then downloads a versioned authenticated-encryption envelope. The
envelope contains the deterministic wallet seed, the swap database, and
possibly a reusable L402 credential. Keep the passphrase separately; it cannot
be recovered.

Restore only works while the runtime is stopped and requires a fresh, unused
database name. It authenticates and decrypts the envelope in memory, checks the
inner bundle and Bitcoin network, migrates the restored database, and verifies
pending swap keys before the new profile can start. It never overwrites a
profile containing application state.
