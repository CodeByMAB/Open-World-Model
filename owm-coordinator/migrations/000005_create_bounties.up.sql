CREATE TABLE IF NOT EXISTS bounties (
    bounty_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    github_issue    TEXT NOT NULL UNIQUE,
    github_repo     TEXT NOT NULL,
    amount_sats     BIGINT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open','claimed','paid','expired')),
    claimed_by      TEXT,                          -- GitHub login
    merged_pr_url   TEXT,
    created_at      TIMESTAMPTZ DEFAULT now(),
    paid_at         TIMESTAMPTZ,
    payment_preimage TEXT
);
