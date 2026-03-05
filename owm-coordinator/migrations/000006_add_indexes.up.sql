CREATE INDEX IF NOT EXISTS idx_nodes_status         ON nodes(status);
CREATE INDEX IF NOT EXISTS idx_nodes_pubkey         ON nodes(public_key);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_node  ON tasks(assigned_node, status);
CREATE INDEX IF NOT EXISTS idx_tasks_submitted      ON tasks(submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_slashing_node        ON slashing_events(node_id);
CREATE INDEX IF NOT EXISTS idx_signals_node         ON misbehavior_signals(node_id, detected_at DESC);
