-- Persist the task types each node supports so the scheduler can enforce
-- eligibility at assignment time (SRS-SCHED-01).
ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS supported_task_types TEXT[] NOT NULL DEFAULT '{}';
