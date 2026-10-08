package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sllt/agentlab/internal/domain"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrIdempotent     = errors.New("idempotency conflict")
	ErrCapacity       = errors.New("capacity")
	ErrAccountBlocked = errors.New("account blocked")
	ErrFinished       = errors.New("already finished")
)

type Project struct {
	ID         string
	Name       string
	SourceSpec string
	CreatedAt  string
}

type Task struct {
	ID         string
	ProjectID  string
	Name       string
	DraftJSON  string
	RowVersion int64
	CreatedAt  string
	UpdatedAt  string
}

type TaskVersion struct {
	ID           string
	TaskID       string
	Version      int
	SnapshotJSON string
	Digest       string
	PrecheckJSON string
	CreatedAt    string
}

type Account struct {
	ID            string
	AuthKind      string
	CredentialRef string
	Concurrency   int
	BlockedReason string
	BlockedUntil  string
	CreatedAt     string
}

type Profile struct {
	ID         string
	Name       string
	AccountID  string
	DraftJSON  string
	RowVersion int64
}

type ProfileVersion struct {
	ID           string
	ProfileID    string
	Version      int
	SnapshotJSON string
	Digest       string
	DoctorJSON   string
	CreatedAt    string
}

type Environment struct {
	ID           string
	Name         string
	SnapshotJSON string
	Digest       string
	CreatedAt    string
}

type Experiment struct {
	ID           string
	Mode         string
	ProtocolJSON string
	PlanJSON     string
	PlanDigest   string
	State        string
	CreatedBy    string
	CreatedAt    string
	TrialCount   int
	// Name and Description are display metadata (migration 006).
	Name        string
	Description string
	// StateCounts and VerdictCounts are filled by list and get reads.
	StateCounts   map[string]int
	VerdictCounts map[string]int
}

// ExperimentMeta is optional display metadata for a submitted experiment.
type ExperimentMeta struct {
	Name        string
	Description string
}

type Trial struct {
	ID               string
	ExperimentID     string
	TaskVersionID    string
	ProfileVersionID string
	RepeatIndex      int
	ExecutionState   string
	Verdict          string
	CurrentAttemptID string
	RowVersion       int64
	CancelRequested  bool
	// EnqueueSeq is the FIFO position. QueuedAt is when it last entered the queue.
	EnqueueSeq int64
	QueuedAt   string
}

type Attempt struct {
	ID             string
	TrialID        string
	Number         int
	AccountID      string
	Fence          int64
	CapabilityHash string
	Token          string
	RuntimeJSON    string
	State          string
	Reason         string
	CleanupState   string
	Verdict        string
	AgentStarted   bool
	CreatedAt      string
	FinishedAt     string
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Store) CreateProject(ctx context.Context, name, sourceSpec string) (Project, error) {
	p := Project{ID: domain.NewID("prj"), Name: name, SourceSpec: sourceSpec, CreatedAt: now()}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO projects(id, name, source_spec_json, created_at) VALUES(?,?,?,?)`, p.ID, p.Name, p.SourceSpec, p.CreatedAt)
		return err
	})
	return p, err
}

func (s *Store) ListProjects(ctx context.Context, cursor string, limit int) ([]Project, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// Keyset pagination in creation order. Ids are random, so ordering by id
	// alone would list projects in an arbitrary order.
	q := `SELECT id, name, source_spec_json, created_at FROM projects`
	args := []any{}
	if cursor != "" {
		q += ` WHERE (created_at, id) > (SELECT created_at, id FROM projects WHERE id=?)`
		args = append(args, cursor)
	}
	q += ` ORDER BY created_at, id LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.SourceSpec, &p.CreatedAt); err != nil {
			return nil, "", err
		}
		out = append(out, p)
	}
	next := ""
	if len(out) > limit {
		next = out[limit-1].ID
		out = out[:limit]
	}
	return out, next, rows.Err()
}

func (s *Store) CreateTask(ctx context.Context, projectID, name, draft string) (Task, error) {
	t := Task{ID: domain.NewID("tsk"), ProjectID: projectID, Name: name, DraftJSON: draft, RowVersion: 1, CreatedAt: now(), UpdatedAt: now()}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO tasks(id, project_id, name, draft_json, row_version, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`,
			t.ID, t.ProjectID, t.Name, t.DraftJSON, t.RowVersion, t.CreatedAt, t.UpdatedAt)
		return err
	})
	return t, err
}

func (s *Store) UpdateTaskDraft(ctx context.Context, id string, rowVersion int64, name, draft string) (Task, error) {
	var t Task
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE tasks SET name=?, draft_json=?, row_version=row_version+1, updated_at=? WHERE id=? AND row_version=?`, name, draft, now(), id, rowVersion)
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
		return scanTask(tx.QueryRowContext(ctx, `SELECT id, project_id, name, draft_json, row_version, created_at, updated_at FROM tasks WHERE id=?`, id), &t)
	})
	return t, err
}

func (s *Store) PublishTaskVersion(ctx context.Context, taskID, snapshot, digest, precheck string) (TaskVersion, error) {
	var v TaskVersion
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var next int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0)+1 FROM task_versions WHERE task_id=?`, taskID).Scan(&next); err != nil {
			return err
		}
		v = TaskVersion{ID: domain.NewID("tv"), TaskID: taskID, Version: next, SnapshotJSON: snapshot, Digest: digest, PrecheckJSON: precheck, CreatedAt: now()}
		_, err := tx.ExecContext(ctx, `INSERT INTO task_versions(id, task_id, version, snapshot_json, digest, precheck_json, created_at) VALUES(?,?,?,?,?,?,?)`,
			v.ID, v.TaskID, v.Version, v.SnapshotJSON, v.Digest, v.PrecheckJSON, v.CreatedAt)
		return err
	})
	return v, err
}

func (s *Store) GetTaskVersion(ctx context.Context, id string) (TaskVersion, error) {
	var v TaskVersion
	err := scanTaskVersion(s.db.QueryRowContext(ctx, `SELECT id, task_id, version, snapshot_json, digest, precheck_json, created_at FROM task_versions WHERE id=?`, id), &v)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskVersion{}, ErrNotFound
	}
	return v, err
}

func (s *Store) CreateAccount(ctx context.Context, authKind, credentialRef string, concurrency int) (Account, error) {
	if concurrency <= 0 {
		concurrency = 1
	}
	a := Account{ID: domain.NewID("acc"), AuthKind: authKind, CredentialRef: credentialRef, Concurrency: concurrency, CreatedAt: now()}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO accounts(id, auth_kind, credential_ref, concurrency, created_at) VALUES(?,?,?,?,?)`, a.ID, a.AuthKind, a.CredentialRef, a.Concurrency, a.CreatedAt)
		return err
	})
	return a, err
}

func (s *Store) BlockAccount(ctx context.Context, id, reason string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE accounts SET blocked_reason=? WHERE id=?`, reason, id)
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

func (s *Store) CreateProfile(ctx context.Context, name, accountID, draft string) (Profile, error) {
	p := Profile{ID: domain.NewID("prf"), Name: name, AccountID: accountID, DraftJSON: draft, RowVersion: 1}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO profiles(id, name, account_id, draft_json, row_version, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`,
			p.ID, p.Name, p.AccountID, p.DraftJSON, p.RowVersion, now(), now())
		return err
	})
	return p, err
}

func (s *Store) PublishProfileVersion(ctx context.Context, profileID, snapshot, digest, doctor string) (ProfileVersion, error) {
	var v ProfileVersion
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var next int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0)+1 FROM profile_versions WHERE profile_id=?`, profileID).Scan(&next); err != nil {
			return err
		}
		v = ProfileVersion{ID: domain.NewID("pv"), ProfileID: profileID, Version: next, SnapshotJSON: snapshot, Digest: digest, DoctorJSON: doctor, CreatedAt: now()}
		_, err := tx.ExecContext(ctx, `INSERT INTO profile_versions(id, profile_id, version, snapshot_json, digest, doctor_json, created_at) VALUES(?,?,?,?,?,?,?)`,
			v.ID, v.ProfileID, v.Version, v.SnapshotJSON, v.Digest, v.DoctorJSON, v.CreatedAt)
		return err
	})
	return v, err
}

func (s *Store) GetProfileVersion(ctx context.Context, id string) (ProfileVersion, error) {
	var v ProfileVersion
	err := s.db.QueryRowContext(ctx, `SELECT id, profile_id, version, snapshot_json, digest, doctor_json, created_at FROM profile_versions WHERE id=?`, id).
		Scan(&v.ID, &v.ProfileID, &v.Version, &v.SnapshotJSON, &v.Digest, &v.DoctorJSON, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProfileVersion{}, ErrNotFound
	}
	return v, err
}

func (s *Store) SaveEnvironment(ctx context.Context, name, snapshot, digest string) (Environment, error) {
	e := Environment{ID: domain.NewID("env"), Name: name, SnapshotJSON: snapshot, Digest: digest, CreatedAt: now()}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO environment_versions(id, name, snapshot_json, digest, created_at) VALUES(?,?,?,?,?)`, e.ID, e.Name, e.SnapshotJSON, e.Digest, e.CreatedAt)
		return err
	})
	return e, err
}

type NewTrial struct {
	TaskVersionID    string
	ProfileVersionID string
	RepeatIndex      int
}

// SubmitExperiment inserts one immutable plan and its trials. The same actor,
// operation and key with the same body returns the original id. A different
// body is a conflict and writes nothing.
func (s *Store) SubmitExperiment(ctx context.Context, actor, key, bodyDigest, mode, protocol, plan string, trials []NewTrial, meta ...ExperimentMeta) (Experiment, error) {
	var m ExperimentMeta
	if len(meta) > 0 {
		m = meta[0]
	}
	var exp Experiment
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var existingDigest, existingRef string
		err := tx.QueryRowContext(ctx, `SELECT body_digest, result_ref FROM idempotency_keys WHERE actor_id=? AND operation=? AND key=?`, actor, "submit_experiment", key).Scan(&existingDigest, &existingRef)
		if err == nil {
			if existingDigest != bodyDigest {
				return ErrIdempotent
			}
			return loadExperiment(ctx, tx, existingRef, &exp)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		digest, err := domain.Digest(map[string]any{"plan": plan, "mode": mode, "protocol": protocol})
		if err != nil {
			return err
		}
		exp = Experiment{ID: domain.NewID("exp"), Mode: mode, ProtocolJSON: protocol, PlanJSON: plan, PlanDigest: digest, State: string(domain.ExecQueued), CreatedBy: actor, CreatedAt: now(), TrialCount: len(trials), Name: m.Name, Description: m.Description}
		if _, err := tx.ExecContext(ctx, `INSERT INTO experiments(id, mode, protocol_json, plan_json, plan_digest, state, created_by, created_at, name, description) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			exp.ID, exp.Mode, exp.ProtocolJSON, exp.PlanJSON, exp.PlanDigest, exp.State, exp.CreatedBy, exp.CreatedAt, exp.Name, exp.Description); err != nil {
			return err
		}
		var seq int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(enqueue_seq), 0) FROM trials`).Scan(&seq); err != nil {
			return err
		}
		for _, t := range trials {
			id := domain.NewID("trl")
			seq++
			if _, err := tx.ExecContext(ctx, `INSERT INTO trials(id, experiment_id, task_version_id, profile_version_id, repeat_index, execution_state, verdict, row_version, enqueue_seq, queued_at) VALUES(?,?,?,?,?,?,?,1,?,?)`,
				id, exp.ID, t.TaskVersionID, t.ProfileVersionID, t.RepeatIndex, string(domain.ExecQueued), string(domain.VerdictUnverified), seq, exp.CreatedAt); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO idempotency_keys(actor_id, operation, key, body_digest, result_ref, expires_at) VALUES(?,?,?,?,?,?)`,
			actor, "submit_experiment", key, bodyDigest, exp.ID, time.Now().UTC().Add(48*time.Hour).Format(time.RFC3339Nano))
		return err
	})
	return exp, err
}

func (s *Store) CountTrials(ctx context.Context, experimentID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM trials WHERE experiment_id=?`, experimentID).Scan(&n)
	return n, err
}

func (s *Store) ActiveAttempts(ctx context.Context, accountID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM attempts WHERE account_id=? AND cleanup_state != ?`, accountID, string(domain.CleanupClean)).Scan(&n)
	return n, err
}

func (s *Store) GlobalActiveAttempts(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM attempts WHERE cleanup_state != ?`, string(domain.CleanupClean)).Scan(&n)
	return n, err
}

// CreateAttempt claims capacity and stores a new physical execution. It does
// not start a process. cleanup stays pending, so the slot is held.
func (s *Store) CreateAttempt(ctx context.Context, trialID, accountID, runtimeJSON, reason string, globalLimit, accountLimit int) (Attempt, error) {
	var a Attempt
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var state, blocked string
		var rowVersion int64
		var cancelRequested int
		if err := tx.QueryRowContext(ctx, `SELECT execution_state, row_version, cancel_requested FROM trials WHERE id=?`, trialID).Scan(&state, &rowVersion, &cancelRequested); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if cancelRequested != 0 {
			return ErrConflict
		}
		if state != string(domain.ExecQueued) && reason == "" {
			return ErrConflict
		}
		var storedReason string
		if err := tx.QueryRowContext(ctx, `SELECT retry_reason FROM trials WHERE id=?`, trialID).Scan(&storedReason); err != nil {
			return err
		}
		if reason == "" {
			reason = storedReason
		}
		var globalN, accountN, accountCap int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM attempts WHERE cleanup_state != ?`, string(domain.CleanupClean)).Scan(&globalN); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM attempts WHERE account_id=? AND cleanup_state != ?`, accountID, string(domain.CleanupClean)).Scan(&accountN); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT concurrency, COALESCE(blocked_reason, '') FROM accounts WHERE id=?`, accountID).Scan(&accountCap, &blocked); err != nil {
			return err
		}
		if blocked != "" {
			return ErrAccountBlocked
		}
		if accountLimit <= 0 {
			accountLimit = accountCap
		}
		if globalN >= globalLimit || accountN >= accountLimit {
			return ErrCapacity
		}
		var number int
		var fence int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(number), 0)+1, COALESCE(MAX(fence), 0)+1 FROM attempts WHERE trial_id=?`, trialID).Scan(&number, &fence); err != nil {
			return err
		}
		token := domain.NewID("cap")
		a = Attempt{
			ID: domain.NewID("att"), TrialID: trialID, Number: number, AccountID: accountID, Fence: fence,
			Token: token, CapabilityHash: domain.MustDigest(token),
			RuntimeJSON: runtimeJSON, State: string(domain.ExecPreparing), Reason: reason,
			CleanupState: string(domain.CleanupPending), Verdict: string(domain.VerdictUnverified), CreatedAt: now(),
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO attempts(id, trial_id, number, account_id, fence, capability_hash, runtime_json, state, reason, cleanup_state, verdict, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.TrialID, a.Number, a.AccountID, a.Fence, a.CapabilityHash, a.RuntimeJSON, a.State, a.Reason, a.CleanupState, a.Verdict, a.CreatedAt); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE trials SET execution_state=?, current_attempt_id=?, row_version=row_version+1 WHERE id=? AND row_version=?`,
			string(domain.ExecPreparing), a.ID, trialID, rowVersion)
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
		if _, err := tx.ExecContext(ctx, `UPDATE trials SET retry_reason='' WHERE id=?`, trialID); err != nil {
			return err
		}
		var experimentID string
		if err := tx.QueryRowContext(ctx, `SELECT experiment_id FROM trials WHERE id=?`, trialID).Scan(&experimentID); err != nil {
			return err
		}
		return rollupExperiment(ctx, tx, experimentID)
	})
	return a, err
}

func (s *Store) Audit(ctx context.Context, actor, action, resource, detail string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, actor_id, action, resource_id, at, detail_json) VALUES(?,?,?,?,?,?)`,
			domain.NewID("aud"), actor, action, resource, now(), detail)
		return err
	})
}

func scanTask(row *sql.Row, t *Task) error {
	return row.Scan(&t.ID, &t.ProjectID, &t.Name, &t.DraftJSON, &t.RowVersion, &t.CreatedAt, &t.UpdatedAt)
}

func scanTaskVersion(row *sql.Row, v *TaskVersion) error {
	return row.Scan(&v.ID, &v.TaskID, &v.Version, &v.SnapshotJSON, &v.Digest, &v.PrecheckJSON, &v.CreatedAt)
}

func loadExperiment(ctx context.Context, tx *sql.Tx, id string, exp *Experiment) error {
	if err := tx.QueryRowContext(ctx, `SELECT `+experimentCols+` FROM experiments WHERE id=?`, id).
		Scan(&exp.ID, &exp.Mode, &exp.ProtocolJSON, &exp.PlanJSON, &exp.PlanDigest, &exp.State, &exp.CreatedBy, &exp.CreatedAt, &exp.Name, &exp.Description); err != nil {
		return err
	}
	return tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM trials WHERE experiment_id=?`, id).Scan(&exp.TrialCount)
}
