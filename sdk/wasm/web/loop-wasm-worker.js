"use strict";

// Keep all WASM, SQLite, and chain polling work off the page's main thread.
// This is a classic worker because go-wasmsqlite and wasm_exec.js are classic
// scripts that use importScripts and worker-global configuration variables.
const assetBase = new URL(".", self.location.href);
self.sqliteBridgeWorkerURL = new URL(
  "sqlite-worker.js",
  assetBase,
).href;
self.sqliteBridgeSQLiteJSURL = new URL("sqlite3.js", assetBase).href;

importScripts(
  new URL("sqlite-bridge.js", assetBase).href,
  new URL("wasm_exec.js", assetBase).href,
);

let resolveReady;
let rejectReady;
const ready = new Promise((resolve, reject) => {
  resolveReady = resolve;
  rejectReady = reject;
});

const streams = new Map();
const payments = new Map();
let nextPaymentID = 1;

self.addEventListener("loop-wasm-ready", () => resolveReady());

function errorMessage(error) {
  if (!error) return "unknown browser Loop error";
  if (error.message) return error.message;
  return String(error);
}

function reply(id, body) {
  self.postMessage({ id, ...body });
}

// loopWasmPayInvoice is an optional bridge for the L402 bootstrap payment.
// Configure start with l402_payer: "loopWasmPayInvoice". The page receives
// the invoice and must answer with a 32-byte hex payment preimage.
self.loopWasmPayInvoice = (invoice) => {
  const paymentID = nextPaymentID++;
  self.postMessage({
    type: "l402_invoice",
    payment_id: paymentID,
    invoice,
  });

  return new Promise((resolve, reject) => {
    payments.set(paymentID, { resolve, reject });
  });
};

async function call(message) {
  await ready;
  const result = await self.loopWasmCall(
    message.method,
    message.request ?? null,
  );
  reply(message.id, { ok: true, result });
}

async function monitor(message) {
  await ready;
  const handle = await self.loopWasmCall(
    "monitor",
    message.request ?? null,
  );
  streams.set(message.id, handle);
  reply(message.id, { ok: true, started: true, stream: true });

  try {
    while (streams.has(message.id)) {
      const event = await handle.next();
      if (event === null) break;
      reply(message.id, { ok: true, event, stream: true });
    }
    reply(message.id, { ok: true, done: true, stream: true });
  } finally {
    if (streams.delete(message.id)) handle.close();
  }
}

function cancelStream(id) {
  const handle = streams.get(id);
  if (!handle) return;

  streams.delete(id);
  handle.close();
}

function completePayment(message) {
  const waiter = payments.get(message.payment_id);
  if (!waiter) return;

  payments.delete(message.payment_id);
  if (message.error) {
    waiter.reject(new Error(message.error));
  } else {
    waiter.resolve(message.preimage);
  }
}

self.addEventListener("message", (event) => {
  const message = event.data || {};

  if (message.type === "l402_payment") {
    completePayment(message);
    return;
  }
  if (message.cancel) {
    cancelStream(message.id);
    return;
  }

  const operation = message.method === "monitor" ? monitor : call;
  operation(message).catch((error) => {
    reply(message.id, { ok: false, error: errorMessage(error) });
  });
});

async function instantiateGo() {
  if (!self.crossOriginIsolated ||
      typeof self.SharedArrayBuffer === "undefined") {
    throw new Error(
      "browser Loop requires cross-origin isolation and SharedArrayBuffer",
    );
  }
  if (!self.sqliteBridge) {
    throw new Error("go-wasmsqlite bridge did not load");
  }

  const go = new Go();
  const wasmURL = new URL("loop-wasm.wasm", assetBase);

  let instance;
  try {
    const response = await fetch(wasmURL);
    const result = await WebAssembly.instantiateStreaming(
      response,
      go.importObject,
    );
    instance = result.instance;
  } catch (streamingError) {
    // Some local static servers do not send application/wasm yet. Refetch the
    // consumed response and use the ArrayBuffer fallback for development.
    const response = await fetch(wasmURL);
    if (!response.ok) {
      throw streamingError;
    }
    const result = await WebAssembly.instantiate(
      await response.arrayBuffer(),
      go.importObject,
    );
    instance = result.instance;
  }

  await go.run(instance);
  throw new Error("browser Loop runtime exited");
}

instantiateGo().catch((error) => {
  rejectReady(error);
  self.postMessage({
    type: "runtime_error",
    error: errorMessage(error),
  });
});
