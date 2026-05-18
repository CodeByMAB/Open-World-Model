-- Records every heartbeat received from a node (SRS-NODE-04: 60-second interval).
-- Used by UpdateReliability to compute the rolling 7-day uptime fraction per
-- SRS-SCHED-04: reliability = task_success_fraction × uptime_fraction.
--
-- Retention: rows older than 8 days are safe to prune with a scheduled job:
--   DELETE FROM heartbeat_log WHERE recorded_at < now() - INTERVAL '8 days';
CREATE TABLE IF NOT EXISTS heartbeat_log (
    node_id     UUID        NOT NULL REFERENCES nodes(node_id) ON DELETE CASCADE,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_heartbeat_log_node_at
    ON heartbeat_log (node_id, recorded_at DESC);
