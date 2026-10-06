package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/sllt/agentlab/internal/domain"
)

func (s *Store) ListTrials(ctx context.Context, experimentID string) ([]Trial, error) {
	q := `SELECT id, experiment_id, task_version_id, profile_version_id, repeat_index, execution_state, verdict, COALESCE(current_attempt_id, ''), row_version, cancel_requested FROM trials`
	args := []any{}
	if experimentID != "" {
		q += ` WHERE experiment_id=?`
		args = append(args, experimentID)
	}
	q += ` ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTrials(rows)
}

func (s *Store) ListQueued(ctx context.Context) ([]Trial, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, experiment_id, task_version_id, profile_version_id, repeat_index, execution_state, verdict, COALESCE(current_attempt_id, ''), row_version, cancel_requested FROM trials WHERE execution_state=? AND cancel_requested=0 ORDER BY id`, string(domain.ExecQueued))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTrials(rows)
}

func (s *Store) GetTrial(ctx context.Context, id string) (Trial, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, experiment_id, task_version_id, profile_version_id, repeat_index, execution_state, verdict, COALESCE(current_attempt_id, ''), row_version, cancel_requested FROM trials WHERE id=?`, id)
	var t Trial
	var cancel int
	err := row.Scan(&t.ID, &t.ExperimentID, &t.TaskVersionID, &t.ProfileVersionID, &t.RepeatIndex, &t.ExecutionState, &t.Verdict, &t.CurrentAttemptID, &t.RowVersion, &cancel)
	if errors.Is(err, sql.ErrNoRows) {
		return Trial{}, ErrNotFound
	}
	t.CancelRequested = cancel != 0
	return t, err
}

func (s *Store) RequestCancel(ctx context.Context, trialID string) (Trial, error) {
	var out Trial
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var state string
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT execution_state, row_version FROM trials WHERE id=?`, trialID).Scan(&state, &version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		next := state
		verdict := ""
		if state == string(domain.ExecQueued) {
			next = string(domain.ExecCancelled)
			verdict = string(domain.VerdictUnverified)
		} else if domain.ExecutionState(state).Terminal() {
			return ErrFinished
		} else {
			next = string(domain.ExecCancelling)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE attempts SET state=? WHERE trial_id=? AND state NOT IN ('completed','cancelled','aborted')`, string(domain.ExecCancelling), trialID); err != nil {
			return err
		}
		q := `UPDATE trials SET cancel_requested=1, execution_state=?, row_version=row_version+1 WHERE id=? AND row_version=?`
		args := []any{next, trialID, version}
		if verdict != "" {
			q = `UPDATE trials SET cancel_requested=1, execution_state=?, verdict=?, row_version=row_version+1 WHERE id=? AND row_version=?`
			args = []any{next, verdict, trialID, version}
		}
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id, actor_id, action, resource_id, at, detail_json) VALUES(?,?,?,?,?,?)`, domain.NewID("aud"), "control", "cancel", trialID, now(), `{"state":"`+next+`"}`)
		if err != nil {
			return err
		}
		row := tx.QueryRowContext(ctx, `SELECT id, experiment_id, task_version_id, profile_version_id, repeat_index, execution_state, verdict, COALESCE(current_attempt_id, ''), row_version, cancel_requested FROM trials WHERE id=?`, trialID)
		var cancel int
		if err := row.Scan(&out.ID, &out.ExperimentID, &out.TaskVersionID, &out.ProfileVersionID, &out.RepeatIndex, &out.ExecutionState, &out.Verdict, &out.CurrentAttemptID, &out.RowVersion, &cancel); err != nil {
			return err
		}
		out.CancelRequested = cancel != 0
		return rollupExperiment(ctx, tx, out.ExperimentID)
	})
	return out, err
}

func (s *Store) FinishAttempt(ctx context.Context, attemptID string, fence int64, state, verdict, cleanup, reason string) error {
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var trialID string
		var gotFence int64
		var current string
		if err := tx.QueryRowContext(ctx, `SELECT trial_id, fence, state FROM attempts WHERE id=?`, attemptID).Scan(&trialID, &gotFence, &current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if gotFence != fence {
			return errStaleFence
		}
		if domain.ExecutionState(current).Terminal() {
			return ErrConflict
		}
		if !domain.CanTransition(domain.ExecutionState(current), domain.ExecutionState(state)) {
			return ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `UPDATE attempts SET state=?, verdict=?, cleanup_state=?, reason=?, finished_at=? WHERE id=? AND fence=?`, state, verdict, cleanup, reason, now(), attemptID, fence); err != nil {
			return err
		}
		var currentAttempt string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(current_attempt_id, '') FROM trials WHERE id=?`, trialID).Scan(&currentAttempt); err != nil {
			return err
		}
		if currentAttempt != attemptID {
			return errStaleAttempt
		}
		if _, err := tx.ExecContext(ctx, `UPDATE trials SET execution_state=?, verdict=? WHERE id=? AND current_attempt_id=?`, state, verdict, trialID, attemptID); err != nil {
			return err
		}
		var experimentID string
		if err := tx.QueryRowContext(ctx, `SELECT experiment_id FROM trials WHERE id=?`, trialID).Scan(&experimentID); err != nil {
			return err
		}
		return rollupExperiment(ctx, tx, experimentID)
	})
	if errors.Is(err, errStaleFence) {
		_ = s.Audit(ctx, "control", "stale_fence", attemptID, `{}`)
		return ErrConflict
	}
	if errors.Is(err, errStaleAttempt) {
		_ = s.Audit(ctx, "control", "stale_attempt", attemptID, `{}`)
		return ErrConflict
	}
	return err
}

func rollupExperiment(ctx context.Context, tx *sql.Tx, experimentID string) error {
	if experimentID == "" {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT execution_state FROM trials WHERE experiment_id=?`, experimentID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var states []domain.ExecutionState
	for rows.Next() {
		var state string
		if err := rows.Scan(&state); err != nil {
			return err
		}
		states = append(states, domain.ExecutionState(state))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE experiments SET state=? WHERE id=?`, string(domain.RollupExecution(states)), experimentID)
	return err
}

var (
	errStaleFence   = errors.New("stale fence")
	errStaleAttempt = errors.New("stale attempt")
)

func (s *Store) InsertCheck(ctx context.Context, attemptID, verifierDigest string, check domain.Check, run int) error {
	body, err := domain.Canonical(check)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO checks(id, attempt_id, verifier_digest, check_id, check_run, result_json) VALUES(?,?,?,?,?,?)`, domain.NewID("chk"), attemptID, verifierDigest, check.ID, run, string(body))
		return err
	})
}

func (s *Store) ListChecks(ctx context.Context, attemptID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT result_json FROM checks WHERE attempt_id=? ORDER BY check_id, check_run`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		out = append(out, body)
	}
	return out, rows.Err()
}

func (s *Store) UpsertUsage(ctx context.Context, attemptID string, usage domain.Usage) error {
	body, err := domain.Canonical(usage)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO usage_records(id, attempt_id, source, source_event_id, measures_json) VALUES(?,?,?,?,?) ON CONFLICT(attempt_id, source, source_event_id) DO NOTHING`, domain.NewID("use"), attemptID, usage.Source, usage.SourceEventID, string(body))
		return err
	})
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO users(id, username, password_hash, created_at) VALUES(?,?,?,?)`, domain.NewID("usr"), username, passwordHash, now())
		return err
	})
}

// CreateAdmin inserts the only administrator. The count and insert share one
// write transaction, so concurrent setup calls cannot each succeed.
func (s *Store) CreateAdmin(ctx context.Context, username, passwordHash string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrAdminExists
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO users(id, username, password_hash, created_at) VALUES(?,?,?,?)`, domain.NewID("usr"), username, passwordHash, now())
		return err
	})
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) UserByName(ctx context.Context, username string) (id, hash string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE username=?`, username).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return id, hash, err
}

const (
	sessionIdle     = 30 * time.Minute
	sessionAbsolute = 12 * time.Hour
)

func (s *Store) CreateSession(ctx context.Context, userID, tokenHash, csrf string) error {
	created := time.Now().UTC()
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO sessions(id, user_id, token_hash, csrf_secret, created_at, expires_at, absolute_expires_at) VALUES(?,?,?,?,?,?,?)`,
			domain.NewID("ses"), userID, tokenHash, csrf, created.Format(time.RFC3339Nano), created.Add(sessionIdle).Format(time.RFC3339Nano), created.Add(sessionAbsolute).Format(time.RFC3339Nano))
		return err
	})
}

func (s *Store) Session(ctx context.Context, tokenHash string) (userID, csrf string, err error) {
	var createdAt, expiresAt, absoluteAt string
	err = s.db.QueryRowContext(ctx, `SELECT user_id, csrf_secret, created_at, expires_at, COALESCE(absolute_expires_at, '') FROM sessions WHERE token_hash=?`, tokenHash).Scan(&userID, &csrf, &createdAt, &expiresAt, &absoluteAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	nowTS := time.Now().UTC()
	expires, expErr := time.Parse(time.RFC3339Nano, expiresAt)
	created, createdErr := time.Parse(time.RFC3339Nano, createdAt)
	if expErr != nil || createdErr != nil || !expires.After(nowTS) || expires.Year() >= 2099 {
		_ = s.DeleteSession(ctx, tokenHash)
		return "", "", ErrNotFound
	}
	absolute := created.Add(sessionAbsolute)
	if absoluteAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, absoluteAt); err == nil {
			absolute = parsed
		}
	}
	if !absolute.After(nowTS) {
		_ = s.DeleteSession(ctx, tokenHash)
		return "", "", ErrNotFound
	}
	next := nowTS.Add(sessionIdle)
	if next.After(absolute) {
		next = absolute
	}
	if err := s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE sessions SET expires_at=? WHERE token_hash=?`, next.Format(time.RFC3339Nano), tokenHash)
		return err
	}); err != nil {
		return "", "", err
	}
	return userID, csrf, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
		return err
	})
}

func (s *Store) GetExperiment(ctx context.Context, id string) (Experiment, error) {
	var exp Experiment
	err := s.db.QueryRowContext(ctx, `SELECT id, mode, protocol_json, plan_json, plan_digest, state, created_by, created_at FROM experiments WHERE id=?`, id).Scan(&exp.ID, &exp.Mode, &exp.ProtocolJSON, &exp.PlanJSON, &exp.PlanDigest, &exp.State, &exp.CreatedBy, &exp.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Experiment{}, ErrNotFound
	}
	if err != nil {
		return Experiment{}, err
	}
	exp.TrialCount, err = s.CountTrials(ctx, id)
	return exp, err
}

func (s *Store) ListExperiments(ctx context.Context) ([]Experiment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM experiments ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Experiment
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		exp, err := s.GetExperiment(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, exp)
	}
	return out, nil
}

func (s *Store) AddReview(ctx context.Context, attemptID, kind, rubric, body string) error {
	if rubric == "" {
		rubric = `{"rubric":"v1"}`
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO reviews(id, attempt_id, kind, rubric_json, body_json, created_at) VALUES(?,?,?,?,?,?)`, domain.NewID("rev"), attemptID, kind, rubric, body, now())
		return err
	})
}

func (s *Store) ProfileAccount(ctx context.Context, profileVersionID string) (accountID string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT p.account_id FROM profile_versions v JOIN profiles p ON p.id=v.profile_id WHERE v.id=?`, profileVersionID).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return accountID, err
}

func (s *Store) ListTasks(ctx context.Context, projectID string) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, name, draft_json, row_version, created_at, updated_at FROM tasks WHERE project_id=? ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.DraftJSON, &t.RowVersion, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) TaskVersions(ctx context.Context, taskID string) ([]TaskVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, version, snapshot_json, digest, precheck_json, created_at FROM task_versions WHERE task_id=? ORDER BY version`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskVersion
	for rows.Next() {
		var v TaskVersion
		if err := rows.Scan(&v.ID, &v.TaskID, &v.Version, &v.SnapshotJSON, &v.Digest, &v.PrecheckJSON, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) OpenAttempts(ctx context.Context) ([]Attempt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, trial_id, number, account_id, fence, capability_hash, runtime_json, state, reason, cleanup_state, verdict, agent_started, created_at, COALESCE(finished_at, '') FROM attempts WHERE state NOT IN ('completed','cancelled','aborted')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attempt
	for rows.Next() {
		var a Attempt
		var started int
		if err := rows.Scan(&a.ID, &a.TrialID, &a.Number, &a.AccountID, &a.Fence, &a.CapabilityHash, &a.RuntimeJSON, &a.State, &a.Reason, &a.CleanupState, &a.Verdict, &started, &a.CreatedAt, &a.FinishedAt); err != nil {
			return nil, err
		}
		a.AgentStarted = started != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) MarkAgentStarted(ctx context.Context, attemptID string, fence int64) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE attempts SET agent_started=1, state=? WHERE id=? AND fence=? AND state=?`, string(domain.ExecRunning), attemptID, fence, string(domain.ExecPreparing))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE trials SET execution_state=? WHERE current_attempt_id=?`, string(domain.ExecRunning), attemptID); err != nil {
			return err
		}
		var experimentID string
		if err := tx.QueryRowContext(ctx, `SELECT experiment_id FROM trials WHERE current_attempt_id=?`, attemptID).Scan(&experimentID); err != nil {
			return err
		}
		return rollupExperiment(ctx, tx, experimentID)
	})
}

func (s *Store) Backup(ctx context.Context, dest string) error {
	if dest == "" || strings.ContainsAny(dest, "'\x00") {
		return errors.New("invalid backup path")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.ExecContext(ctx, `VACUUM INTO '`+dest+`'`)
	return err
}

func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	var t Task
	err := s.db.QueryRowContext(ctx, `SELECT id, project_id, name, draft_json, row_version, created_at, updated_at FROM tasks WHERE id=?`, id).Scan(&t.ID, &t.ProjectID, &t.Name, &t.DraftJSON, &t.RowVersion, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

func (s *Store) GetAttempt(ctx context.Context, id string) (Attempt, error) {
	var a Attempt
	var started int
	err := s.db.QueryRowContext(ctx, `SELECT id, trial_id, number, account_id, fence, capability_hash, runtime_json, state, reason, cleanup_state, verdict, agent_started, created_at, COALESCE(finished_at, '') FROM attempts WHERE id=?`, id).Scan(&a.ID, &a.TrialID, &a.Number, &a.AccountID, &a.Fence, &a.CapabilityHash, &a.RuntimeJSON, &a.State, &a.Reason, &a.CleanupState, &a.Verdict, &started, &a.CreatedAt, &a.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, ErrNotFound
	}
	a.AgentStarted = started != 0
	return a, err
}

func (s *Store) ListAttempts(ctx context.Context, trialID string) ([]Attempt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, trial_id, number, account_id, fence, capability_hash, runtime_json, state, reason, cleanup_state, verdict, agent_started, created_at, COALESCE(finished_at, '') FROM attempts WHERE trial_id=? ORDER BY number`, trialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attempt
	for rows.Next() {
		var a Attempt
		var started int
		if err := rows.Scan(&a.ID, &a.TrialID, &a.Number, &a.AccountID, &a.Fence, &a.CapabilityHash, &a.RuntimeJSON, &a.State, &a.Reason, &a.CleanupState, &a.Verdict, &started, &a.CreatedAt, &a.FinishedAt); err != nil {
			return nil, err
		}
		a.AgentStarted = started != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AdvanceAttempt(ctx context.Context, attemptID string, fence int64, to string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		var current string
		var got int64
		if err := tx.QueryRowContext(ctx, `SELECT state, fence FROM attempts WHERE id=?`, attemptID).Scan(&current, &got); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if got != fence || !domain.CanTransition(domain.ExecutionState(current), domain.ExecutionState(to)) {
			return ErrConflict
		}
		res, err := tx.ExecContext(ctx, `UPDATE attempts SET state=? WHERE id=? AND fence=? AND state=?`, to, attemptID, fence, current)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE trials SET execution_state=? WHERE current_attempt_id=?`, to, attemptID); err != nil {
			return err
		}
		var experimentID string
		if err := tx.QueryRowContext(ctx, `SELECT experiment_id FROM trials WHERE current_attempt_id=?`, attemptID).Scan(&experimentID); err != nil {
			return err
		}
		return rollupExperiment(ctx, tx, experimentID)
	})
}

// RequestRetry records a human retry intent without creating an Attempt.
// The scheduler is the only caller that claims capacity and starts work.
func (s *Store) RequestRetry(ctx context.Context, trialID, reason string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE trials SET cancel_requested=0, execution_state=?, verdict=?, retry_reason=? WHERE id=? AND execution_state IN ('completed','cancelled','aborted')`, string(domain.ExecQueued), string(domain.VerdictUnverified), reason, trialID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
		return nil
	})
}

func (s *Store) PrepareRetry(ctx context.Context, trialID string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE trials SET cancel_requested=0, execution_state=?, verdict=? WHERE id=? AND execution_state IN ('completed','cancelled','aborted')`, string(domain.ExecQueued), string(domain.VerdictUnverified), trialID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
		return nil
	})
}

type Artifact struct {
	ID         string
	AttemptID  string
	Kind       string
	Digest     string
	Bytes      int64
	StorageKey string
	Status     string
}

func (s *Store) InsertArtifact(ctx context.Context, a Artifact) (Artifact, error) {
	if a.ID == "" {
		a.ID = domain.NewID("art")
	}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO artifacts(id, attempt_id, kind, digest, bytes, storage_key, status) VALUES(?,?,?,?,?,?,?)`, a.ID, a.AttemptID, a.Kind, a.Digest, a.Bytes, a.StorageKey, a.Status)
		return err
	})
	return a, err
}

func (s *Store) ListArtifacts(ctx context.Context, attemptID string) ([]Artifact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, attempt_id, kind, digest, bytes, storage_key, status FROM artifacts WHERE attempt_id=? ORDER BY id`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifacts(rows)
}

func (s *Store) GetArtifact(ctx context.Context, id string) (Artifact, error) {
	var a Artifact
	err := s.db.QueryRowContext(ctx, `SELECT id, attempt_id, kind, digest, bytes, storage_key, status FROM artifacts WHERE id=?`, id).Scan(&a.ID, &a.AttemptID, &a.Kind, &a.Digest, &a.Bytes, &a.StorageKey, &a.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Artifact{}, ErrNotFound
	}
	return a, err
}

func scanArtifacts(rows *sql.Rows) ([]Artifact, error) {
	var out []Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(&a.ID, &a.AttemptID, &a.Kind, &a.Digest, &a.Bytes, &a.StorageKey, &a.Status); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListUsage(ctx context.Context, attemptID string) ([]domain.Usage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT measures_json FROM usage_records WHERE attempt_id=? ORDER BY source, CAST(source_event_id AS INTEGER), source_event_id`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Usage
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var u domain.Usage
		if err := jsonUnmarshal(body, &u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) ListReviews(ctx context.Context, attemptID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT body_json FROM reviews WHERE attempt_id=? ORDER BY created_at`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		out = append(out, body)
	}
	return out, rows.Err()
}

func (s *Store) InsertEvent(ctx context.Context, attemptID string, sequence int64, origin, eventType, observedAt string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO event_index(attempt_id, sequence, origin, event_type, observed_at, durable) VALUES(?,?,?,?,?,1) ON CONFLICT(attempt_id, sequence) DO NOTHING`, attemptID, sequence, origin, eventType, observedAt)
		return err
	})
}

func (s *Store) EventBounds(ctx context.Context, attemptID string) (minSeq, maxSeq int64, n int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(sequence), 0), COALESCE(MAX(sequence), 0), COUNT(1) FROM event_index WHERE attempt_id=?`, attemptID).Scan(&minSeq, &maxSeq, &n)
	return minSeq, maxSeq, n, err
}

func (s *Store) AcceptWebhook(ctx context.Context, dedupe, bodyDigest string) (bool, error) {
	duplicate := false
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT body_digest FROM webhook_receipts WHERE dedupe_key=?`, dedupe).Scan(&existing)
		if err == nil {
			if existing != bodyDigest {
				return ErrConflict
			}
			duplicate = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO webhook_receipts(id, dedupe_key, body_digest, received_at) VALUES(?,?,?,?)`, domain.NewID("wh"), dedupe, bodyDigest, now())
		return err
	})
	return duplicate, err
}

func (s *Store) ListProfiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, account_id, draft_json, row_version FROM profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.AccountID, &p.DraftJSON, &p.RowVersion); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetProfile(ctx context.Context, id string) (Profile, error) {
	var p Profile
	err := s.db.QueryRowContext(ctx, `SELECT id, name, account_id, draft_json, row_version FROM profiles WHERE id=?`, id).Scan(&p.ID, &p.Name, &p.AccountID, &p.DraftJSON, &p.RowVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	return p, err
}

func (s *Store) ProfileVersions(ctx context.Context, profileID string) ([]ProfileVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, profile_id, version, snapshot_json, digest, doctor_json, created_at FROM profile_versions WHERE profile_id=? ORDER BY version`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProfileVersion
	for rows.Next() {
		var v ProfileVersion
		if err := rows.Scan(&v.ID, &v.ProfileID, &v.Version, &v.SnapshotJSON, &v.Digest, &v.DoctorJSON, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, auth_kind, credential_ref, concurrency, COALESCE(blocked_reason,''), COALESCE(blocked_until,''), created_at FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.AuthKind, &a.CredentialRef, &a.Concurrency, &a.BlockedReason, &a.BlockedUntil, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListEnvironments(ctx context.Context) ([]Environment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, snapshot_json, digest, created_at FROM environment_versions ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Environment
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.Name, &e.SnapshotJSON, &e.Digest, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, id string) (Project, error) {
	var p Project
	err := s.db.QueryRowContext(ctx, `SELECT id, name, source_spec_json, created_at FROM projects WHERE id=?`, id).Scan(&p.ID, &p.Name, &p.SourceSpec, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

func (s *Store) SetAllowedRoots(ctx context.Context, projectID string, roots []string) error {
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	spec := map[string]any{}
	if strings.TrimSpace(project.SourceSpec) != "" {
		_ = json.Unmarshal([]byte(project.SourceSpec), &spec)
	}
	if spec == nil {
		spec = map[string]any{}
	}
	if spec["kind"] == nil {
		spec["kind"] = "local"
	}
	listed := make([]any, 0, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		listed = append(listed, root)
	}
	spec["allowed_roots"] = listed
	body, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE projects SET source_spec_json=? WHERE id=?`, string(body), projectID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) UpdateRuntime(ctx context.Context, attemptID, runtimeJSON string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE attempts SET runtime_json=? WHERE id=?`, runtimeJSON, attemptID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) CountInFlight(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM attempts WHERE state NOT IN ('completed','cancelled','aborted')`).Scan(&n)
	return n, err
}

func (s *Store) SetExperimentState(ctx context.Context, id, state string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE experiments SET state=? WHERE id=?`, state, id)
		return err
	})
}

func (s *Store) RepairExperimentStates(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM experiments`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.WithTx(ctx, func(tx *sql.Tx) error {
			return rollupExperiment(ctx, tx, id)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CountAudit(ctx context.Context, action, resource string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM audit_events WHERE action=? AND resource_id=?`, action, resource).Scan(&n)
	return n, err
}

func jsonUnmarshal(body string, dest any) error {
	return json.Unmarshal([]byte(body), dest)
}

func scanTrials(rows *sql.Rows) ([]Trial, error) {
	var out []Trial
	for rows.Next() {
		var t Trial
		var cancel int
		if err := rows.Scan(&t.ID, &t.ExperimentID, &t.TaskVersionID, &t.ProfileVersionID, &t.RepeatIndex, &t.ExecutionState, &t.Verdict, &t.CurrentAttemptID, &t.RowVersion, &cancel); err != nil {
			return nil, err
		}
		t.CancelRequested = cancel != 0
		out = append(out, t)
	}
	return out, rows.Err()
}
