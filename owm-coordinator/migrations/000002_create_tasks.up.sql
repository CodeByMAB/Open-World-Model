CREATE TABLE IF NOT EXISTS tasks (
    task_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type       TEXT NOT NULL,
    assigned_node   UUID REFERENCES nodes(node_id),
    node_ln_uri     TEXT,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','running','completed','failed','timeout')),
    input_hash      TEXT,
    output_hash     TEXT,
    reward_sats     INTEGER,
    payment_status  TEXT DEFAULT 'pending'
                    CHECK (payment_status IN ('pending','paid','failed')),
    payment_preimage TEXT,
    submitted_at    TIMESTAMPTZ DEFAULT now(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    timeout_seconds INTEGER DEFAULT 30
);
