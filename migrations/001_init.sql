CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL UNIQUE,
    csrf_secret TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    source_spec_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    draft_json TEXT NOT NULL,
    row_version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE task_versions (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    version INTEGER NOT NULL,
    snapshot_json TEXT NOT NULL,
    digest TEXT NOT NULL,
    precheck_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    UNIQUE (task_id, version)
);

CREATE TRIGGER task_versions_no_update
BEFORE UPDATE ON task_versions
BEGIN
    SELECT RAISE(ABORT, 'task version is immutable');
END;

CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    auth_kind TEXT NOT NULL,
    credential_ref TEXT NOT NULL,
    concurrency INTEGER NOT NULL,
    blocked_reason TEXT,
    blocked_until TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    draft_json TEXT NOT NULL,
    row_version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE profile_versions (
    id TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL REFERENCES profiles(id),
    version INTEGER NOT NULL,
    snapshot_json TEXT NOT NULL,
    digest TEXT NOT NULL,
    doctor_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (profile_id, version)
);

CREATE TRIGGER profile_versions_no_update
BEFORE UPDATE ON profile_versions
BEGIN
    SELECT RAISE(ABORT, 'profile version is immutable');
END;

CREATE TABLE environment_versions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    digest TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TRIGGER environment_versions_no_update
BEFORE UPDATE ON environment_versions
BEGIN
    SELECT RAISE(ABORT, 'environment version is immutable');
END;

CREATE TABLE experiments (
    id TEXT PRIMARY KEY,
    mode TEXT NOT NULL,
    protocol_json TEXT NOT NULL,
    plan_json TEXT NOT NULL,
    plan_digest TEXT NOT NULL,
    state TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TRIGGER experiments_plan_immutable
BEFORE UPDATE OF plan_json, plan_digest, mode, protocol_json ON experiments
BEGIN
    SELECT RAISE(ABORT, 'experiment plan is immutable');
END;

CREATE TABLE trials (
    id TEXT PRIMARY KEY,
    experiment_id TEXT NOT NULL REFERENCES experiments(id),
    task_version_id TEXT NOT NULL,
    profile_version_id TEXT NOT NULL,
    repeat_index INTEGER NOT NULL,
    execution_state TEXT NOT NULL,
    verdict TEXT NOT NULL,
    current_attempt_id TEXT,
    row_version INTEGER NOT NULL,
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    UNIQUE (experiment_id, task_version_id, profile_version_id, repeat_index)
);

CREATE TABLE attempts (
    id TEXT PRIMARY KEY,
    trial_id TEXT NOT NULL REFERENCES trials(id),
    number INTEGER NOT NULL,
    account_id TEXT NOT NULL,
    fence INTEGER NOT NULL,
    capability_hash TEXT NOT NULL,
    runtime_json TEXT NOT NULL,
    state TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    cleanup_state TEXT NOT NULL,
    verdict TEXT NOT NULL,
    heartbeat_at TEXT,
    agent_started INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    finished_at TEXT,
    UNIQUE (trial_id, number)
);

CREATE TABLE checks (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL REFERENCES attempts(id),
    verifier_digest TEXT NOT NULL,
    check_id TEXT NOT NULL,
    check_run INTEGER NOT NULL,
    result_json TEXT NOT NULL,
    evidence_ref TEXT,
    UNIQUE (attempt_id, check_id, check_run)
);

CREATE TABLE artifacts (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    digest TEXT NOT NULL,
    bytes INTEGER NOT NULL,
    storage_key TEXT NOT NULL,
    status TEXT NOT NULL
);

CREATE TABLE usage_records (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL,
    source TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    measures_json TEXT NOT NULL,
    UNIQUE (attempt_id, source, source_event_id)
);

CREATE TABLE idempotency_keys (
    actor_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    key TEXT NOT NULL,
    body_digest TEXT NOT NULL,
    result_ref TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (actor_id, operation, key)
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    at TEXT NOT NULL,
    detail_json TEXT NOT NULL
);

CREATE TABLE event_index (
    attempt_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    origin TEXT NOT NULL,
    event_type TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    durable INTEGER NOT NULL,
    PRIMARY KEY (attempt_id, sequence)
);

CREATE TABLE reviews (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    rubric_json TEXT NOT NULL,
    body_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE interventions (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    note TEXT NOT NULL,
    at TEXT NOT NULL
);

CREATE TABLE webhook_receipts (
    id TEXT PRIMARY KEY,
    dedupe_key TEXT NOT NULL UNIQUE,
    body_digest TEXT NOT NULL,
    received_at TEXT NOT NULL
);

CREATE INDEX ix_trial_queue ON trials(execution_state, experiment_id);
CREATE INDEX ix_attempt_cleanup ON attempts(cleanup_state, heartbeat_at);
