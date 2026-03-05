-- nodes
CREATE TABLE IF NOT EXISTS nodes (
    node_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    public_key      TEXT NOT NULL UNIQUE,         -- Ed25519 hex
    ln_node_uri     TEXT NOT NULL,                -- pubkey@host:port
    onion_address   TEXT,
    tier            TEXT NOT NULL CHECK (tier IN ('t1','t2','t3')),
    vram_gb         NUMERIC(6,2),
    ram_gb          NUMERIC(6,2),
    bandwidth_mbps  NUMERIC(8,2),
    reliability     NUMERIC(5,4) DEFAULT 1.0,     -- 0.0–1.0 rolling 7d
    total_tasks     INTEGER DEFAULT 0,
    total_sats      BIGINT DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','active','degraded','suspended')),
    registered_at   TIMESTAMPTZ DEFAULT now(),
    last_heartbeat  TIMESTAMPTZ
);

-- node_stakes
CREATE TABLE IF NOT EXISTS node_stakes (
    node_id             UUID PRIMARY KEY REFERENCES nodes(node_id),
    channel_id          TEXT NOT NULL UNIQUE,     -- LN channel ID hex
    channel_capacity    BIGINT NOT NULL,           -- total sats
    local_balance       BIGINT NOT NULL,           -- node's locked sats
    tier_minimum        BIGINT NOT NULL,           -- required for tier
    bonus_multiplier    NUMERIC(4,3) DEFAULT 1.0, -- 1.000–2.000
    stake_status        TEXT NOT NULL DEFAULT 'active'
                        CHECK (stake_status IN ('active','degraded','force_closed')),
    opened_at           TIMESTAMPTZ DEFAULT now(),
    last_verified_at    TIMESTAMPTZ DEFAULT now(),
    degraded_since      TIMESTAMPTZ
);
