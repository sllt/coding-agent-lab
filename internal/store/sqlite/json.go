package sqlite

import (
	"encoding/json"
	"strings"
)

// The API serves store rows as snake_case JSON. Columns that hold JSON text
// are emitted as nested objects (source_spec, draft, snapshot, ...), never as
// escaped strings, and secrets (attempt tokens, capability hashes, runtime
// identity tokens) are never emitted at all.

// nested returns raw JSON for a valid object or array, the plain string for
// anything else, and nil for empty text.
func nested(text string) any {
	t := strings.TrimSpace(text)
	if t == "" {
		return nil
	}
	if (t[0] == '{' || t[0] == '[') && json.Valid([]byte(t)) {
		return json.RawMessage(t)
	}
	return text
}

// nestedWithout parses a JSON object and drops the given keys.
func nestedWithout(text string, drop ...string) any {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(text), &m); err != nil || m == nil {
		return nested(text)
	}
	for _, k := range drop {
		delete(m, k)
	}
	return m
}

func (p Project) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"id": p.ID, "name": p.Name, "source_spec": nested(p.SourceSpec), "created_at": p.CreatedAt})
}

func (t Task) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id": t.ID, "project_id": t.ProjectID, "name": t.Name, "draft": nested(t.DraftJSON),
		"row_version": t.RowVersion, "created_at": t.CreatedAt, "updated_at": t.UpdatedAt,
	})
}

func (v TaskVersion) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id": v.ID, "task_id": v.TaskID, "version": v.Version, "snapshot": nested(v.SnapshotJSON),
		"digest": v.Digest, "precheck": nestedWithout(v.PrecheckJSON, "Logs", "logs"), "created_at": v.CreatedAt,
	})
}

func (a Account) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id": a.ID, "auth_kind": a.AuthKind, "credential_ref": a.CredentialRef, "concurrency": a.Concurrency,
		"blocked_reason": a.BlockedReason, "blocked_until": a.BlockedUntil, "created_at": a.CreatedAt,
	})
}

func (p Profile) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id": p.ID, "name": p.Name, "account_id": p.AccountID, "draft": nested(p.DraftJSON), "row_version": p.RowVersion,
	})
}

func (v ProfileVersion) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id": v.ID, "profile_id": v.ProfileID, "version": v.Version, "snapshot": nested(v.SnapshotJSON),
		"digest": v.Digest, "doctor": nested(v.DoctorJSON), "created_at": v.CreatedAt,
	})
}

func (e Environment) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id": e.ID, "name": e.Name, "snapshot": nested(e.SnapshotJSON), "digest": e.Digest, "created_at": e.CreatedAt,
	})
}

func (e Experiment) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"id": e.ID, "name": e.Name, "description": e.Description, "mode": e.Mode,
		"protocol": nested(e.ProtocolJSON), "plan": nested(e.PlanJSON), "plan_digest": e.PlanDigest,
		"state": e.State, "created_by": e.CreatedBy, "created_at": e.CreatedAt, "trial_count": e.TrialCount,
	}
	if e.StateCounts != nil {
		m["state_counts"] = e.StateCounts
	}
	if e.VerdictCounts != nil {
		m["verdict_counts"] = e.VerdictCounts
	}
	return json.Marshal(m)
}

// JSONMap is the snake_case view of a trial, for callers that add fields.
func (t Trial) JSONMap() map[string]any {
	return map[string]any{
		"id": t.ID, "experiment_id": t.ExperimentID, "task_version_id": t.TaskVersionID,
		"profile_version_id": t.ProfileVersionID, "repeat_index": t.RepeatIndex,
		"execution_state": t.ExecutionState, "verdict": t.Verdict, "current_attempt_id": t.CurrentAttemptID,
		"row_version": t.RowVersion, "cancel_requested": t.CancelRequested,
		"enqueue_seq": t.EnqueueSeq, "queued_at": t.QueuedAt,
	}
}

func (t Trial) MarshalJSON() ([]byte, error) { return json.Marshal(t.JSONMap()) }

func (a Attempt) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id": a.ID, "trial_id": a.TrialID, "number": a.Number, "account_id": a.AccountID, "fence": a.Fence,
		"runtime": nestedWithout(a.RuntimeJSON, "identity_token"), "state": a.State, "reason": a.Reason,
		"cleanup_state": a.CleanupState, "verdict": a.Verdict, "agent_started": a.AgentStarted,
		"created_at": a.CreatedAt, "finished_at": a.FinishedAt,
	})
}

func (a Artifact) MarshalJSON() ([]byte, error) {
	// storage_key is an absolute server path and is never served.
	return json.Marshal(map[string]any{
		"id": a.ID, "attempt_id": a.AttemptID, "kind": a.Kind, "digest": a.Digest, "bytes": a.Bytes, "status": a.Status,
	})
}
