"use strict";

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => [...document.querySelectorAll(selector)];

const recoveryFormat = "loop-wasm-recovery-encrypted";
const recoveryVersion = 1;
const recoveryIterations = 600000;
const recoveryAAD = new TextEncoder().encode(
  `${recoveryFormat}:v${recoveryVersion}`,
);

class LoopWorkerClient {
  constructor(url, handlers = {}) {
    this.worker = new Worker(url);
    this.handlers = handlers;
    this.pending = new Map();
    this.nextID = 1;
    this.worker.addEventListener("message", (event) => this.onMessage(event));
    this.worker.addEventListener("error", (event) => {
      this.failAll(new Error(event.message || "Loop worker failed"));
    });
  }

  request(method, request = {}) {
    const id = `call-${this.nextID++}`;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { type: "unary", resolve, reject });
      this.worker.postMessage({ id, method, request });
    });
  }

  monitor(onEvent) {
    const id = `stream-${this.nextID++}`;
    const started = new Promise((resolve, reject) => {
      this.pending.set(id, {
        type: "stream", resolve, reject, onEvent,
      });
      this.worker.postMessage({ id, method: "monitor", request: {} });
    });
    return {
      started,
      close: () => this.worker.postMessage({ id, cancel: true }),
    };
  }

  completeL402(paymentID, preimage, error) {
    this.worker.postMessage({
      type: "l402_payment",
      payment_id: paymentID,
      preimage,
      error,
    });
  }

  onMessage(event) {
    const message = event.data || {};
    if (message.type === "l402_invoice") {
      this.handlers.onL402?.(message);
      return;
    }
    if (message.type === "runtime_error") {
      const error = new Error(message.error || "Loop runtime exited");
      this.handlers.onError?.(error);
      this.failAll(error);
      return;
    }

    const pending = this.pending.get(message.id);
    if (!pending) return;
    if (!message.ok) {
      const error = new Error(message.error || "Loop worker call failed");
      pending.reject(error);
      this.pending.delete(message.id);
      this.handlers.onError?.(error);
      return;
    }
    if (pending.type === "unary") {
      pending.resolve(message.result);
      this.pending.delete(message.id);
      return;
    }
    if (message.started) pending.resolve();
    if (message.event) pending.onEvent(message.event);
    if (message.done) this.pending.delete(message.id);
  }

  failAll(error) {
    for (const pending of this.pending.values()) pending.reject(error);
    this.pending.clear();
  }
}

const state = {
  client: null,
  running: false,
  busy: false,
  quote: null,
  quoteKey: "",
  deadline: 0,
  monitor: null,
  currentSwapID: "",
  invoices: { prepay: "", swap: "" },
  l402: null,
  recoveryBundle: "",
  seenUpdates: new Set(),
};

function value(id) {
  return $(id).value.trim();
}

function integerValue(id, name) {
  const number = Number(value(id));
  if (!Number.isSafeInteger(number)) throw new Error(`${name} must be an integer`);
  return number;
}

function formatSat(raw) {
  const number = Number(raw || 0);
  return `${new Intl.NumberFormat().format(number)} sat`;
}

function hex(bytes) {
  return [...bytes].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

function bytesToBase64(bytes) {
  let binary = "";
  const chunkSize = 0x8000;
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
  }
  return btoa(binary);
}

function base64ToBytes(encoded) {
  const binary = atob(encoded);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

async function recoveryKey(passphrase, salt, usage) {
  const material = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(passphrase),
    "PBKDF2",
    false,
    ["deriveKey"],
  );
  return crypto.subtle.deriveKey({
    name: "PBKDF2",
    hash: "SHA-256",
    salt,
    iterations: recoveryIterations,
  }, material, { name: "AES-GCM", length: 256 }, false, [usage]);
}

async function encryptRecovery(bundle, passphrase) {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const key = await recoveryKey(passphrase, salt, "encrypt");
  const ciphertext = await crypto.subtle.encrypt({
    name: "AES-GCM",
    iv,
    additionalData: recoveryAAD,
    tagLength: 128,
  }, key, new TextEncoder().encode(bundle));

  return JSON.stringify({
    format: recoveryFormat,
    version: recoveryVersion,
    kdf: {
      name: "PBKDF2",
      hash: "SHA-256",
      iterations: recoveryIterations,
      salt: bytesToBase64(salt),
    },
    cipher: {
      name: "AES-GCM",
      tag_length: 128,
      iv: bytesToBase64(iv),
    },
    ciphertext: bytesToBase64(new Uint8Array(ciphertext)),
  }, null, 2);
}

async function decryptRecovery(encrypted, passphrase) {
  let envelope;
  try {
    envelope = JSON.parse(encrypted);
  } catch (error) {
    throw new Error(`Encrypted recovery is not valid JSON: ${error.message}`);
  }
  if (envelope.format !== recoveryFormat ||
      envelope.version !== recoveryVersion ||
      envelope.kdf?.name !== "PBKDF2" ||
      envelope.kdf?.hash !== "SHA-256" ||
      envelope.kdf?.iterations !== recoveryIterations ||
      envelope.cipher?.name !== "AES-GCM" ||
      envelope.cipher?.tag_length !== 128) {
    throw new Error("Unsupported encrypted recovery envelope");
  }

  try {
    const salt = base64ToBytes(envelope.kdf.salt);
    const iv = base64ToBytes(envelope.cipher.iv);
    if (salt.length !== 16 || iv.length !== 12) {
      throw new Error("invalid salt or IV length");
    }
    const key = await recoveryKey(passphrase, salt, "decrypt");
    const plaintext = await crypto.subtle.decrypt({
      name: "AES-GCM",
      iv,
      additionalData: recoveryAAD,
      tagLength: 128,
    }, key, base64ToBytes(envelope.ciphertext));
    return new TextDecoder().decode(plaintext);
  } catch (error) {
    throw new Error(
      `Recovery decryption failed; check the passphrase and file: ${
        error.message}`,
    );
  }
}

function generateSeed() {
  const seed = new Uint8Array(32);
  crypto.getRandomValues(seed);
  $("#seed").value = hex(seed);
  invalidateQuote();
  toast("Generated a new seed. Back it up before starting a swap.");
}

function log(message, level = "info") {
  const item = document.createElement("li");
  const time = document.createElement("time");
  const body = document.createElement("span");
  time.dateTime = new Date().toISOString();
  time.textContent = new Date().toLocaleTimeString();
  body.textContent = message;
  body.className = level;
  item.append(time, body);
  $("#activity-log").prepend(item);
}

let toastTimer;
function toast(message) {
  const element = $("#toast");
  element.textContent = message;
  element.classList.add("visible");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => element.classList.remove("visible"), 2600);
}

function setRuntimeStatus(kind, text) {
  $("#runtime-dot").className = `status-dot ${kind}`;
  $("#runtime-status").textContent = text;
}

function refreshControls() {
  const running = state.running;
  $("#start-button").disabled = running || state.busy;
  $("#stop-button").disabled = !running || state.busy;
  $("#terms-button").disabled = !running || state.busy;
  $("#quote-button").disabled = !running || state.busy;
  $("#loop-out-button").disabled =
    !running || state.busy || !state.quote || state.quoteKey !== quoteKey();
  $("#export-button").disabled = !running || state.busy;
  $("#restore-button").disabled =
    running || state.busy || !state.recoveryBundle;
  for (const control of $("#config-form").elements) {
    if (control.id !== "worker-url") control.disabled = running || state.busy;
  }
}

async function operation(name, fn) {
  if (state.busy) return;
  state.busy = true;
  refreshControls();
  try {
    return await fn();
  } catch (error) {
    log(`${name}: ${error.message}`, "error");
    toast(error.message);
    throw error;
  } finally {
    state.busy = false;
    refreshControls();
  }
}

function ensureBrowserReady() {
  if (!window.crossOriginIsolated || typeof SharedArrayBuffer === "undefined") {
    throw new Error(
      "Cross-origin isolation is required. Serve with COOP and COEP headers.",
    );
  }
  if (!navigator.storage?.getDirectory) {
    throw new Error("This browser does not expose OPFS storage");
  }
}

function ensureClient() {
  if (state.client) return state.client;
  state.client = new LoopWorkerClient(value("#worker-url"), {
    onL402: showL402,
    onError: (error) => {
      state.running = false;
      setRuntimeStatus("error", "Runtime error");
      log(error.message, "error");
      refreshControls();
    },
  });
  return state.client;
}

function startConfig() {
  return {
    database_path: value("#database-path"),
    seed: value("#seed"),
    network: value("#network"),
    esplora_url: value("#esplora-url"),
    loop_server_url: value("#server-url"),
    l402_payer: "loopWasmPayInvoice",
    l402_max_cost_sat: integerValue("#l402-max", "L402 maximum"),
    poll_interval_ms: integerValue("#poll-interval", "Poll interval"),
    min_relay_fee_sat_per_kw: 253,
    startup_timeout_ms: 120000,
  };
}

async function startRuntime() {
  if (!$("#config-form").reportValidity()) return;
  await operation("Start runtime", async () => {
    ensureBrowserReady();
    setRuntimeStatus("loading", "Starting runtime…");
    log("Loading WASM and opening the OPFS database.");
    await ensureClient().request("start", startConfig());
    state.running = true;
    setRuntimeStatus("running", "Runtime running");
    log("Embedded Loop runtime started over bufconn.");
    await startMonitor();
    try {
      await refreshTerms();
    } catch (error) {
      log(`Load terms: ${error.message}`, "error");
    }
    const info = await state.client.request("getInfo", {});
    log(`Loop ${info.version || "runtime"} on ${info.network || value("#network")}.`);
  }).catch(() => setRuntimeStatus("error", "Startup failed"));
}

async function stopRuntime() {
  await operation("Stop runtime", async () => {
    state.monitor?.close();
    state.monitor = null;
    await state.client.request("stop", {});
    state.running = false;
    setRuntimeStatus("", "Runtime stopped");
    log("Runtime stopped cleanly.");
  }).catch(() => {});
}

async function startMonitor() {
  state.monitor?.close();
  state.monitor = state.client.monitor(handleSwapUpdate);
  await state.monitor.started;
  log("Swap monitor connected.");
}

async function refreshTerms() {
  const terms = await state.client.request("loopOutTerms", {});
  const min = Number(terms.min_swap_amount);
  const max = Number(terms.max_swap_amount);
  $("#amount").min = String(min);
  $("#amount").max = String(max);
  if (!value("#amount")) $("#amount").value = String(min);
  $("#terms-summary").textContent =
    `Current range: ${formatSat(min)} to ${formatSat(max)}. ` +
    `CLTV delta: ${terms.min_cltv_delta}–${terms.max_cltv_delta} blocks.`;
  log("Loaded current Loop Out terms.");
}

function quoteKey() {
  return [value("#amount"), value("#conf-target"),
    value("#publication-window")].join(":");
}

function invalidateQuote() {
  state.quote = null;
  state.quoteKey = "";
  state.deadline = 0;
  $("#quote-panel").hidden = true;
  $("#quote-hint").textContent = "A fresh quote is required before initiation.";
  refreshControls();
}

async function getQuote() {
  if (!$("#swap-form").reportValidity()) return;
  await operation("Get quote", async () => {
    const amount = integerValue("#amount", "Amount");
    const confTarget = integerValue("#conf-target", "Confirmation target");
    const windowMinutes = integerValue(
      "#publication-window", "Publication window",
    );
    state.deadline = Math.floor(Date.now() / 1000) + windowMinutes * 60;
    state.quote = await state.client.request("loopOutQuote", {
      amt: String(amount),
      conf_target: confTarget,
      swap_publication_deadline: String(state.deadline),
    });
    state.quoteKey = quoteKey();
    $("#quote-swap-fee").textContent = formatSat(state.quote.swap_fee_sat);
    $("#quote-prepay").textContent = formatSat(state.quote.prepay_amt_sat);
    $("#quote-sweep-fee").textContent =
      formatSat(state.quote.htlc_sweep_fee_sat);
    $("#quote-deadline").textContent =
      new Date(state.deadline * 1000).toLocaleString();
    $("#quote-panel").hidden = false;
    $("#quote-hint").textContent =
      "Fee limits are fixed to this quote; changed fields require a new quote.";
    log(`Received quote for ${formatSat(amount)}.`);
  }).catch(() => {});
}

async function initiateLoopOut() {
  if (!state.quote || state.quoteKey !== quoteKey()) {
    invalidateQuote();
    toast("Get a fresh quote first.");
    return;
  }
  if (!$("#swap-form").reportValidity()) return;
  await operation("Initiate Loop Out", async () => {
    const response = await state.client.request("loopOut", {
      amt: String(integerValue("#amount", "Amount")),
      dest: value("#destination"),
      max_swap_routing_fee: "0",
      max_prepay_routing_fee: "0",
      max_swap_fee: String(state.quote.swap_fee_sat),
      max_prepay_amt: String(state.quote.prepay_amt_sat),
      max_miner_fee: String(state.quote.htlc_sweep_fee_sat),
      sweep_conf_target: integerValue(
        "#conf-target", "Confirmation target",
      ),
      htlc_confirmations: 1,
      swap_publication_deadline: String(state.deadline),
      label: value("#swap-label"),
      initiator: "browser-demo",
      is_external_addr: true,
      external_payments: true,
    });
    if (!response.external_payments || !response.swap_invoice ||
        !response.prepay_invoice) {
      throw new Error("Server response did not include both external invoices");
    }
    state.currentSwapID = response.id;
    displaySwapIdentity(response);
    displayInvoices(response.prepay_invoice, response.swap_invoice);
    $("#swap-state").textContent = "INITIATED";
    $("#swap-state").className = "state-badge active";
    log(`Loop Out ${shortID(response.id)} initiated. Pay both invoices.`);
    $("#invoices-section").scrollIntoView({ behavior: "smooth" });
  }).catch(() => {});
}

function displayInvoices(prepay, swap) {
  state.invoices = { prepay, swap };
  $("#prepay-invoice").textContent = prepay;
  $("#swap-invoice").textContent = swap;
  for (const link of $$(".invoice-open")) {
    link.href = `lightning:${state.invoices[link.dataset.invoice]}`;
  }
  $("#invoices-section").hidden = false;
}

function displaySwapIdentity(swap) {
  $("#swap-id").textContent = swap.id || "—";
  $("#htlc-address").textContent =
    swap.htlc_address_p2tr || swap.htlc_address_p2wsh ||
    swap.htlc_address || "—";
}

function handleSwapUpdate(update) {
  const signature = `${update.id}:${update.state}:${update.last_update_time}`;
  if (state.seenUpdates.has(signature)) return;
  state.seenUpdates.add(signature);
  if (!state.currentSwapID && update.external_payments &&
      update.state !== "SUCCESS" && update.state !== "FAILED") {
    state.currentSwapID = update.id;
    displayInvoices(update.prepay_invoice, update.swap_invoice);
    log(`Recovered pending Loop Out ${shortID(update.id)} from SQLite.`);
  }
  if (update.id === state.currentSwapID) {
    displaySwapIdentity(update);
    $("#swap-state").textContent = update.state;
    $("#swap-state").className = `state-badge ${
      update.state === "SUCCESS" ? "success" :
      update.state === "FAILED" ? "failed" : "active"
    }`;
    $("#failure-reason").textContent =
      update.failure_reason === "FAILURE_REASON_NONE" ? "—" :
        update.failure_reason;
  }
  const timeline = $("#timeline");
  timeline.querySelector(".empty")?.remove();
  const item = document.createElement("li");
  const detail = document.createElement("span");
  const time = document.createElement("time");
  detail.textContent = `${shortID(update.id)} · ${update.state}`;
  const timestamp = Number(update.last_update_time) / 1e6;
  const date = Number.isFinite(timestamp) ? new Date(timestamp) : new Date();
  time.dateTime = date.toISOString();
  time.textContent = date.toLocaleString();
  item.append(detail, time);
  timeline.prepend(item);
}

function shortID(id = "") {
  return id.length > 14 ? `${id.slice(0, 8)}…${id.slice(-6)}` : id;
}

async function copyText(text) {
  await navigator.clipboard.writeText(text);
  toast("Copied to clipboard.");
}

async function payWithWebLN(invoice) {
  if (!window.webln) throw new Error("No WebLN provider is available");
  await window.webln.enable();
  return window.webln.sendPayment(invoice);
}

function showL402(message) {
  state.l402 = message;
  $("#l402-invoice").textContent = message.invoice;
  $("#l402-preimage").value = "";
  $("#l402-panel").hidden = false;
  $("#l402-preimage").focus();
  log("Loop server requested an L402 authentication payment.");
}

function completeL402(preimage, error = "") {
  if (!state.l402) return;
  state.client.completeL402(state.l402.payment_id, preimage, error);
  state.l402 = null;
  $("#l402-panel").hidden = true;
}

async function exportRecovery() {
  const passphrase = value("#recovery-passphrase");
  const confirmation = value("#recovery-passphrase-confirm");
  if (passphrase.length < 12) {
    toast("Use a recovery passphrase of at least 12 characters.");
    return;
  }
  if (passphrase !== confirmation) {
    toast("Recovery passphrases do not match.");
    return;
  }
  await operation("Export recovery", async () => {
    const result = await state.client.request("exportRecovery", {});
    const encrypted = await encryptRecovery(result.bundle, passphrase);
    const blob = new Blob([encrypted], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    const stamp = new Date().toISOString().replaceAll(":", "-");
    link.href = url;
    link.download =
      `loop-recovery-${value("#network")}-${stamp}.encrypted.json`;
    link.click();
    URL.revokeObjectURL(url);
    $("#recovery-passphrase").value = "";
    $("#recovery-passphrase-confirm").value = "";
    log("Exported an authenticated, encrypted recovery bundle.");
  }).catch(() => {});
}

async function restoreRecovery() {
  const accepted = window.confirm(
    "Restore requires a fresh, unused database name. Continue?",
  );
  if (!accepted) return;
  const passphrase = value("#restore-passphrase");
  if (passphrase.length < 12) {
    toast("Enter the recovery bundle passphrase.");
    return;
  }
  await operation("Restore recovery", async () => {
    const bundle = await decryptRecovery(state.recoveryBundle, passphrase);
    const result = await ensureClient().request("restoreRecovery", {
      database_path: value("#database-path"),
      network: value("#network"),
      bundle,
      fresh_target: true,
      timeout_ms: 120000,
    });
    $("#seed").value = result.seed;
    $("#restore-passphrase").value = "";
    log("Recovery bundle restored. Start the runtime to resume pending swaps.");
    toast("Recovery restored; the recovered seed is loaded.");
  }).catch(() => {});
}

function renderPreflight() {
  const checks = [
    ["WebAssembly", typeof WebAssembly !== "undefined"],
    ["Worker", typeof Worker !== "undefined"],
    ["WebCrypto", Boolean(window.crypto?.subtle)],
    ["OPFS", Boolean(navigator.storage?.getDirectory)],
    ["Cross-origin isolated", window.crossOriginIsolated],
  ];
  const list = $("#preflight-list");
  list.replaceChildren(...checks.map(([label, pass]) => {
    const item = document.createElement("li");
    item.className = pass ? "pass" : "fail";
    item.textContent = `${pass ? "Ready" : "Missing"}: ${label}`;
    return item;
  }));
}

$("#start-button").addEventListener("click", startRuntime);
$("#stop-button").addEventListener("click", stopRuntime);
$("#terms-button").addEventListener("click", () =>
  operation("Refresh terms", refreshTerms).catch(() => {}));
$("#quote-button").addEventListener("click", getQuote);
$("#loop-out-button").addEventListener("click", initiateLoopOut);
$("#generate-seed").addEventListener("click", generateSeed);
$("#reveal-seed").addEventListener("click", (event) => {
  const visible = $("#seed").type === "text";
  $("#seed").type = visible ? "password" : "text";
  event.currentTarget.textContent = visible ? "Show" : "Hide";
  event.currentTarget.setAttribute("aria-pressed", String(!visible));
});
for (const input of ["#amount", "#conf-target", "#publication-window"]) {
  $(input).addEventListener("input", invalidateQuote);
}
for (const button of $$(".invoice-copy")) {
  button.addEventListener("click", () =>
    copyText(state.invoices[button.dataset.invoice]).catch((error) =>
      toast(error.message)));
}
for (const button of $$(".invoice-webln")) {
  button.addEventListener("click", async () => {
    const kind = button.dataset.invoice;
    try {
      button.disabled = true;
      await payWithWebLN(state.invoices[kind]);
      $(`#${kind}-payment-note`).textContent = "WebLN reported payment success.";
      $(`#${kind}-payment-note`).classList.add("paid");
      log(`${kind === "prepay" ? "Prepay" : "Swap"} invoice paid with WebLN.`);
    } catch (error) {
      log(`WebLN payment: ${error.message}`, "error");
    } finally {
      button.disabled = false;
    }
  });
}
$("#copy-l402").addEventListener("click", () =>
  copyText(state.l402?.invoice || "").catch((error) => toast(error.message)));
$("#pay-l402-webln").addEventListener("click", async () => {
  try {
    const result = await payWithWebLN(state.l402.invoice);
    const preimage = result?.preimage || result?.payment_preimage || result;
    if (typeof preimage !== "string") {
      throw new Error("WebLN did not return a payment preimage");
    }
    completeL402(preimage);
  } catch (error) {
    log(`L402 payment: ${error.message}`, "error");
  }
});
$("#submit-l402").addEventListener("click", () => {
  const preimage = value("#l402-preimage");
  if (!/^[0-9a-fA-F]{64}$/.test(preimage)) {
    toast("Enter a 32-byte preimage as 64 hex characters.");
    return;
  }
  completeL402(preimage);
});
$("#reject-l402").addEventListener("click", () =>
  completeL402("", "L402 payment canceled by user"));
$("#export-button").addEventListener("click", exportRecovery);
$("#restore-button").addEventListener("click", restoreRecovery);
$("#recovery-file").addEventListener("change", async (event) => {
  const file = event.target.files[0];
  state.recoveryBundle = file ? await file.text() : "";
  $("#recovery-file-name").textContent = file ? file.name :
    "No recovery file selected.";
  refreshControls();
});
$("#clear-log").addEventListener("click", () =>
  $("#activity-log").replaceChildren());
window.addEventListener("beforeunload", (event) => {
  if (!state.running) return;
  event.preventDefault();
  event.returnValue = "";
});

renderPreflight();
generateSeed();
refreshControls();
log("Demo loaded. Configure browser-reachable Esplora and gateway URLs.");
