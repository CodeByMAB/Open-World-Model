-- Observer Protocol v0.1: store receipt ID returned by Observer Registry after
-- successful submission. Used for admin dashboard links and audit.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS observer_receipt_id TEXT;
