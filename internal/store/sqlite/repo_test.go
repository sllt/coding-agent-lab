package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sllt/agentlab/internal/domain"
)

func TestIdempotentSubmitAndVersionImmutability(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	project, err := s.CreateProject(ctx, "orders", `{"root":"/tmp/orders","kind":"local"}`)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(ctx, project.ID, "分页", `{"prompt":"v1"}`)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := s.PublishTaskVersion(ctx, task.ID, `{"prompt":"v1","base":"abc"}`, "digest-1", `{"ok":true}`)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateTaskDraft(ctx, task.ID, task.RowVersion, "分页", `{"prompt":"v2"}`)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := s.PublishTaskVersion(ctx, task.ID, `{"prompt":"v2","base":"abc"}`, "digest-2", `{"ok":true}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTaskVersion(ctx, v1.ID)
	if err != nil || got.SnapshotJSON != v1.SnapshotJSON || got.Digest == v2.Digest {
		t.Fatalf("old version changed: %+v", got)
	}
	if updated.RowVersion != 2 {
		t.Fatalf("row version %d", updated.RowVersion)
	}
	_, err = s.UpdateTaskDraft(ctx, task.ID, 1, "分页", `{"prompt":"lost"}`)
	if err != ErrConflict {
		t.Fatalf("stale draft err=%v", err)
	}

	account, err := s.CreateAccount(ctx, "official_login", "cred-ref", 1)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := s.CreateProfile(ctx, "fixture", account.ID, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	pv, err := s.PublishProfileVersion(ctx, profile.ID, `{"adapter":"fixture"}`, "pdigest", `{"static":"pass"}`)
	if err != nil {
		t.Fatal(err)
	}
	trials := []NewTrial{{TaskVersionID: v1.ID, ProfileVersionID: pv.ID, RepeatIndex: 0}}
	first, err := s.SubmitExperiment(ctx, "user", "key-1", "body-a", domain.ModeAgentProfile, `{"name":"single-pass-v1"}`, `{"tasks":1}`, trials)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SubmitExperiment(ctx, "user", "key-1", "body-a", domain.ModeAgentProfile, `{"name":"single-pass-v1"}`, `{"tasks":1}`, trials)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate submit created %s and %s", first.ID, second.ID)
	}
	n, err := s.CountTrials(ctx, first.ID)
	if err != nil || n != 1 {
		t.Fatalf("trials %d err %v", n, err)
	}
	var experiments int
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(1) FROM experiments`).Scan(&experiments); err != nil || experiments != 1 {
		t.Fatalf("experiments %d err %v", experiments, err)
	}
	if _, err := s.SubmitExperiment(ctx, "user", "key-1", "body-b", domain.ModeAgentProfile, `{"name":"single-pass-v1"}`, `{"tasks":2}`, trials); err != ErrIdempotent {
		t.Fatalf("different body err=%v", err)
	}
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(1) FROM experiments`).Scan(&experiments); err != nil || experiments != 1 {
		t.Fatalf("conflict created an experiment: %d", experiments)
	}
}

func TestAccountCapacityIsSharedUntilCleanup(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	project, err := s.CreateProject(ctx, "orders", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(ctx, project.ID, "t", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	tv, err := s.PublishTaskVersion(ctx, task.ID, `{}`, "d", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	account, err := s.CreateAccount(ctx, "official_login", "cred", 1)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := s.CreateProfile(ctx, "a", account.ID, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreateProfile(ctx, "b", account.ID, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := s.PublishProfileVersion(ctx, p1.ID, `{"n":1}`, "1", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := s.PublishProfileVersion(ctx, p2.ID, `{"n":2}`, "2", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := s.SubmitExperiment(ctx, "user", "k", "b", domain.ModeAgentProfile, `{}`, `{}`, []NewTrial{
		{TaskVersionID: tv.ID, ProfileVersionID: v1.ID, RepeatIndex: 0},
		{TaskVersionID: tv.ID, ProfileVersionID: v2.ID, RepeatIndex: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.DB().QueryContext(ctx, `SELECT id FROM trials WHERE experiment_id=? ORDER BY id`, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 2 {
		t.Fatalf("trials %v", ids)
	}
	if _, err := s.CreateAttempt(ctx, ids[0], account.ID, `{}`, "", 2, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAttempt(ctx, ids[1], account.ID, `{}`, "", 2, 1); err != ErrConflict {
		t.Fatalf("same account second profile err=%v", err)
	}
	n, err := s.ActiveAttempts(ctx, account.ID)
	if err != nil || n != 1 {
		t.Fatalf("active %d err %v", n, err)
	}
}

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
