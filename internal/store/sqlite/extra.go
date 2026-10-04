package sqlite

import (
	"context"
	"database/sql"

	"github.com/sllt/agentlab/internal/domain"
)

type AuditEvent struct {
	ID       string `json:"id"`
	ActorID  string `json:"actor_id"`
	Action   string `json:"action"`
	Resource string `json:"resource_id"`
	At       string `json:"at"`
	Detail   string `json:"detail_json"`
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, actor_id, action, resource_id, at, detail_json FROM audit_events ORDER BY at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var ev AuditEvent
		if err := rows.Scan(&ev.ID, &ev.ActorID, &ev.Action, &ev.Resource, &ev.At, &ev.Detail); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if out == nil {
		out = []AuditEvent{}
	}
	return out, rows.Err()
}

func (s *Store) ListQuarantine(ctx context.Context) ([]Attempt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, trial_id, number, account_id, fence, capability_hash, runtime_json, state, reason, cleanup_state, verdict, agent_started, created_at, COALESCE(finished_at, '') FROM attempts WHERE cleanup_state=? ORDER BY created_at`, string(domain.CleanupQuarantined))
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
		a.Token = ""
		out = append(out, a)
	}
	if out == nil {
		out = []Attempt{}
	}
	return out, rows.Err()
}

func (s *Store) MarkCleanup(ctx context.Context, attemptID, state string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE attempts SET cleanup_state=? WHERE id=?`, state, attemptID)
		return err
	})
}

func (s *Store) InsertIntervention(ctx context.Context, attemptID, kind, note string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO interventions(id, attempt_id, kind, note, at) VALUES(?,?,?,?,?)`, domain.NewID("ivn"), attemptID, kind, note, now())
		return err
	})
}

func (s *Store) CountInterventions(ctx context.Context, attemptID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM interventions WHERE attempt_id=?`, attemptID).Scan(&n)
	return n, err
}

type TaskFlag struct {
	TaskVersionID string `json:"task_version_id"`
	Flaky         bool   `json:"flaky"`
	FailRate      string `json:"fail_rate"`
	Note          string `json:"note"`
}

func (s *Store) SetTaskFlag(ctx context.Context, flag TaskFlag) error {
	flaky := 0
	if flag.Flaky {
		flaky = 1
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO task_flags(task_version_id, flaky, fail_rate, note) VALUES(?,?,?,?)
			ON CONFLICT(task_version_id) DO UPDATE SET flaky=excluded.flaky, fail_rate=excluded.fail_rate, note=excluded.note`,
			flag.TaskVersionID, flaky, flag.FailRate, flag.Note)
		return err
	})
}

func (s *Store) GetTaskFlag(ctx context.Context, taskVersionID string) (TaskFlag, error) {
	var flag TaskFlag
	var flaky int
	err := s.db.QueryRowContext(ctx, `SELECT task_version_id, flaky, fail_rate, note FROM task_flags WHERE task_version_id=?`, taskVersionID).Scan(&flag.TaskVersionID, &flaky, &flag.FailRate, &flag.Note)
	if err == sql.ErrNoRows {
		return TaskFlag{TaskVersionID: taskVersionID}, nil
	}
	flag.Flaky = flaky != 0
	return flag, err
}

type Notification struct {
	ID        string
	Kind      string
	Target    string
	Body      string
	Attempts  int
	LastError string
	State     string
	CreatedAt string
}

func (s *Store) InsertNotification(ctx context.Context, kind, target, body string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO notifications(id, kind, target, body, attempts, last_error, state, created_at) VALUES(?,?,?,?,0,'','pending',?)`, domain.NewID("ntf"), kind, target, body, now())
		return err
	})
}

func (s *Store) DueNotifications(ctx context.Context) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, target, body, attempts, last_error, state, created_at FROM notifications WHERE state='pending' AND attempts < 3 ORDER BY created_at LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.Target, &n.Body, &n.Attempts, &n.LastError, &n.State, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) FinishNotification(ctx context.Context, id, state, lastError string, attempts int) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE notifications SET state=?, last_error=?, attempts=? WHERE id=?`, state, lastError, attempts, id)
		return err
	})
}

func (s *Store) ListReviewMeta(ctx context.Context, attemptID string) ([]string, []string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT body_json, rubric_json FROM reviews WHERE attempt_id=? ORDER BY created_at`, attemptID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var bodies, rubrics []string
	for rows.Next() {
		var body, rubric string
		if err := rows.Scan(&body, &rubric); err != nil {
			return nil, nil, err
		}
		bodies = append(bodies, body)
		rubrics = append(rubrics, rubric)
	}
	return bodies, rubrics, rows.Err()
}

func (s *Store) AppliedMigrations(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	if out == nil {
		out = []string{}
	}
	return out, rows.Err()
}
