CREATE TABLE IF NOT EXISTS pool_payouts (
    payout_id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    miner_pubkey        TEXT        NOT NULL,
    amount_sats         BIGINT      NOT NULL,
    payment_hash        TEXT,
    payment_preimage    TEXT,
    observer_receipt_id TEXT,
    block_height        BIGINT,
    status              TEXT        NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'paid', 'failed')),
    created_at          TIMESTAMPTZ DEFAULT now(),
    paid_at             TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_pool_payouts_miner  ON pool_payouts(miner_pubkey, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pool_payouts_status ON pool_payouts(status);
