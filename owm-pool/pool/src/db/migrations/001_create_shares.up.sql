CREATE TABLE IF NOT EXISTS shares (
    share_id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    miner_pubkey     TEXT        NOT NULL,
    job_id           TEXT        NOT NULL,
    nonce            BIGINT      NOT NULL,
    ntime            INTEGER     NOT NULL,
    difficulty       NUMERIC     NOT NULL,
    fpps_credit_sats BIGINT      NOT NULL,
    is_block         BOOLEAN     DEFAULT FALSE,
    block_height     INTEGER,
    submitted_at     TIMESTAMPTZ DEFAULT now(),
    CONSTRAINT uq_share UNIQUE (job_id, nonce, ntime, miner_pubkey)
);

CREATE INDEX IF NOT EXISTS idx_shares_miner ON shares(miner_pubkey, submitted_at DESC);
