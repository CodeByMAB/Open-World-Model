CREATE TABLE IF NOT EXISTS miner_balances (
    miner_pubkey           TEXT    PRIMARY KEY,
    node_id                UUID,
    ln_node_uri            TEXT    NOT NULL,
    accumulated_sats       BIGINT  NOT NULL DEFAULT 0,
    total_paid_sats        BIGINT  NOT NULL DEFAULT 0,
    last_paid_at           TIMESTAMPTZ,
    coordinator_verified   BOOLEAN NOT NULL DEFAULT TRUE,
    payment_status         TEXT    NOT NULL DEFAULT 'idle'
        CHECK (payment_status IN ('idle', 'paying'))
);
