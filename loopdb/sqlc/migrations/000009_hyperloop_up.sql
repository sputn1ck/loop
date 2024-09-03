CREATE TABLE IF NOT EXISTS hyperloop_swaps (
    -- id is the autoincrementing primary key.
    id INTEGER PRIMARY KEY,

    -- swap_hash points to the parent swap hash.
    swap_hash BLOB PRIMARY KEY,

    -- preimage is the preimage of the swap.
    preimage BLOB NOT NULL,

    -- hyperloop_id is the id of the hyperloop swap, this can be the same between
    -- multiple hyperloop swaps.
    hyperloop_id BLOB NOT NULL,

    -- publish_time is the time where we request the server to publish the swap or
    -- in case of a public hyperloop swap, the time where the server publishes the swap.
    publish_time TIMESTAMP NOT NULL,

    -- sweep_address is the address that the loop server should sweep the funds to.
    sweep_address TEXT NOT NULL,

    -- outgoing_chan_set is the set of short ids of channels that may be used.
    -- If empty, any channel may be used.
    outgoing_chan_set TEXT NOT NULL,

    -- swap_invoice is the invoice that is to be paid by the client to
    -- initiate the loop out swap.
    swap_invoice TEXT NOT NULL,

    -- server_key is the public key of the server that is used to create the swap.
    server_key BLOB NOT NULL,

    -- hyperloop_expiry is the csv expiry of the hyperloop output.
    hyperloop_expiry INTEGER NOT NULL,

    -- confirmed_hyperloop_txid is the txid of the confirmed hyperloop transaction.
    confirmed_hyperloop_txid TEXT,

    -- confirmed_hyperloop_vout is the vout of the confirmed hyperloop transaction.
    confirmed_hyperloop_vout INTEGER,

    -- confirmed_hyperloop_height is the height at which the hyperloop transaction was confirmed.
    confirmed_hyperloop_height INTEGER,

    -- htlc_fee_rate is the fee rate in sat/kw that is used for the htlc transaction.
    htlc_fee_rate BIGINT NOT NULL,

    -- htlc_full_sig is the full signature of the htlc transaction.
    htlc_full_sig BLOB,

    -- sweepless_sweep_fee_rate is the fee rate in sat/kw that is used for the sweepless sweep transaction.
    sweepless_sweep_fee_rate BIGINT NOT NULL
);

CREATE index IF NOT EXISTS hyperloop_swaps_hyperloop_id_idx ON hyperloop_swaps(hyperloop_id);

CREATE TABLE IF NOT EXISTS hyperloop_participants (
    -- id is the autoincrementing primary key.
    id INTEGER PRIMARY KEY,

    -- swap_id is the id of the hyperloop swap that this participant is part of.
    swap_id INTEGER NOT NULL REFERENCES hyperloop_swaps(id),

    -- participant_swap_hash is the swap hash of the participants htlc.
    participant_swap_hash BLOB NOT NULL,

    -- participant_pubkey is the public key of the participant.
    participant_pubkey BLOB NOT NULL,

    -- participant_amt is the amount that the participant contributes to the swap.
    participant_amt BIGINT NOT NULL,

    -- participant_sweep_address is the address that the participant wants to receive the funds on.
    participant_sweep_address TEXT NOT NULL
);

CREATE index IF NOT EXISTS hyperloop_participants_swap_id_idx ON hyperloop_participants(swap_id);

CREATE TABLE IF NOT EXISTS hyperloop_updates (
    -- id is auto incremented for each update.
    id INTEGER PRIMARY KEY,

    -- swap_hash is the hash of the swap that this update is for.
    swap_hash BLOB NOT NULL REFERENCES hyperloop_swaps(swap_hash),

    -- update_state is the state of the swap at the time of the update.
    update_state TEXT NOT NULL,

    -- update_timestamp is the time at which the update was created.
    update_timestamp TIMESTAMP NOT NULL
);