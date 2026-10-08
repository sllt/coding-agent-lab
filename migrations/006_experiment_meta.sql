-- Human-readable experiment name and description. Both are display metadata:
-- they are not part of the plan digest or the idempotency body.
ALTER TABLE experiments ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE experiments ADD COLUMN description TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS ix_experiment_created ON experiments(created_at);
