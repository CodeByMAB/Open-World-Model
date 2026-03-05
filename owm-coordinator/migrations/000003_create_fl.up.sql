-- fl_rounds
CREATE TABLE IF NOT EXISTS fl_rounds (
    round_id        SERIAL PRIMARY KEY,
    round_number    INTEGER NOT NULL UNIQUE,
    status          TEXT NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open','aggregating','complete','failed')),
    model_version   TEXT,                          -- output version
    checkpoint_hash TEXT,
    ots_proof_path  TEXT,
    btc_block       INTEGER,
    started_at      TIMESTAMPTZ DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    participant_count INTEGER
);

-- fl_participants
CREATE TABLE IF NOT EXISTS fl_participants (
    round_id        INTEGER REFERENCES fl_rounds(round_id),
    node_id         UUID REFERENCES nodes(node_id),
    gradient_hash   TEXT,
    anomaly_flagged BOOLEAN DEFAULT FALSE,
    reward_sats     INTEGER,
    s3_url          TEXT,
    PRIMARY KEY (round_id, node_id)
);

-- model_versions
CREATE TABLE IF NOT EXISTS model_versions (
    version_id          TEXT PRIMARY KEY,          -- e.g. "owm-v0.4.2"
    round_number        INTEGER REFERENCES fl_rounds(round_number),
    created_at          TIMESTAMPTZ DEFAULT now(),
    sub_model_hashes    JSONB,                     -- {sub_model_id: sha256}
    ots_proof_paths     JSONB,                     -- {sub_model_id: path}
    btc_block           INTEGER,
    coordinator_sig     TEXT,
    is_current          BOOLEAN DEFAULT FALSE
);
