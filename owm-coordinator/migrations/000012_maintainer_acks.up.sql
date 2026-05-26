CREATE TABLE IF NOT EXISTS maintainer_acks (
    ack_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id         UUID NOT NULL REFERENCES nodes(node_id),
    maintainer_key  TEXT NOT NULL,
    signature       TEXT NOT NULL,
    reason_hash     TEXT NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT now(),
    UNIQUE (node_id, maintainer_key)
);
