-- slashing_events
CREATE TABLE IF NOT EXISTS slashing_events (
    event_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id             UUID REFERENCES nodes(node_id),
    channel_id          TEXT NOT NULL,
    tier                TEXT NOT NULL,
    reason              TEXT NOT NULL,
    evidence_hash       TEXT NOT NULL,
    signal_count        INTEGER NOT NULL,
    executed_at         TIMESTAMPTZ DEFAULT now(),
    cooldown_expires_at TIMESTAMPTZ NOT NULL,
    coordinator_sig     TEXT NOT NULL
);

-- misbehavior_signals
CREATE TABLE IF NOT EXISTS misbehavior_signals (
    signal_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id         UUID REFERENCES nodes(node_id),
    signal_type     TEXT NOT NULL,
    evidence_hash   TEXT NOT NULL,
    detected_at     TIMESTAMPTZ DEFAULT now(),
    slashing_event  UUID REFERENCES slashing_events(event_id)
);
