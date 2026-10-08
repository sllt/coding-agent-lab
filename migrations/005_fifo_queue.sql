-- FIFO admission: trial ids are random, so ORDER BY id is not arrival order.
-- enqueue_seq is a monotonic counter assigned on submit and on every re-queue.
ALTER TABLE trials ADD COLUMN enqueue_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE trials ADD COLUMN queued_at TEXT NOT NULL DEFAULT '';
-- rowid is insertion order for rows written before this migration.
UPDATE trials SET enqueue_seq = rowid,
    queued_at = COALESCE((SELECT e.created_at FROM experiments e WHERE e.id = trials.experiment_id), '');
CREATE INDEX ix_trial_fifo ON trials(execution_state, enqueue_seq);
CREATE INDEX ix_trial_experiment ON trials(experiment_id, enqueue_seq);
CREATE INDEX ix_attempt_trial ON attempts(trial_id, number);
