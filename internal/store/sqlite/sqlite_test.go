package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sllt/agentlab/internal/domain"
)

func TestPragmaPerConnectionTransactionAndRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lab.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO projects(id, name, source_spec_json, created_at) VALUES('p', 'demo', '{}', '2026-10-04T00:00:00Z')`)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO tasks(id, project_id, name, draft_json, row_version, created_at, updated_at) VALUES('t', 'p', 'task', '{}', 1, '2026-10-04T00:00:00Z', '2026-10-04T00:00:00Z')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := CASUpdate(ctx, tx, `UPDATE tasks SET name='renamed', row_version=row_version+1, updated_at='2026-10-04T00:00:01Z' WHERE id=? AND row_version=?`, "t", 1); err != nil {
			return err
		}
		return CASUpdate(ctx, tx, `UPDATE tasks SET name='nope', row_version=row_version+1 WHERE id=? AND row_version=?`, "t", 1)
	})
	if err != ErrConflict {
		t.Fatalf("want cas conflict, got %v", err)
	}
	var value string
	var version int
	if err := s.DB().QueryRowContext(ctx, `SELECT name, row_version FROM tasks WHERE id='t'`).Scan(&value, &version); err != nil {
		t.Fatal(err)
	}
	if value != "task" || version != 1 {
		t.Fatalf("rolled-back cas changed row to %s/%d", value, version)
	}
	if err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO projects(id, name, source_spec_json, created_at) VALUES('p2', 'half', '{}', '2026-10-04T00:00:00Z')`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tasks(id, project_id, name, draft_json, row_version, created_at, updated_at) VALUES('t2', 'p2', 'half', '{}', 1, '2026-10-04T00:00:00Z', '2026-10-04T00:00:00Z')`); err != nil {
			return err
		}
		return fmtError()
	}); err == nil {
		t.Fatal("expected rollback")
	}
	var n int
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(1) FROM tasks WHERE id='t2'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("failed transaction left %d tasks", n)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if err := s2.DB().QueryRowContext(ctx, `SELECT name, row_version FROM tasks WHERE id='t'`).Scan(&value, &version); err != nil {
		t.Fatal(err)
	}
	if value != "task" || version != 1 {
		t.Fatalf("reopen lost state %s/%d", value, version)
	}
	_, err = s2.DB().ExecContext(ctx, `INSERT INTO task_versions(id, task_id, version, snapshot_json, digest, created_at) VALUES('tv', 't', 1, '{}', 'abc', '2026-10-04T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s2.DB().ExecContext(ctx, `UPDATE task_versions SET snapshot_json='{"changed":true}' WHERE id='tv'`)
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("published version update err=%v", err)
	}
}

func TestUsageOrderFollowsEventID(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	early := domain.Usage{Source: "cli", SourceEventID: "1", IsCumulative: true, InputTokens: int64Ptr(3)}
	final := domain.Usage{Source: "cli", SourceEventID: "2", IsCumulative: true, InputTokens: int64Ptr(8)}
	insertUsage(t, s, "zzz-sorts-last", early)
	insertUsage(t, s, "aaa-sorts-first", final)
	rows, err := s.ListUsage(ctx, "att-usage")
	if err != nil || len(rows) != 2 {
		t.Fatalf("%+v %v", rows, err)
	}
	folded, err := domain.FoldUsage(rows)
	if err != nil || folded.InputTokens == nil || *folded.InputTokens != 8 {
		t.Fatalf("folded %+v %v", folded, err)
	}
}

func TestConcurrentSetupCreatesOneAdmin(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- s.CreateAdmin(ctx, fmt.Sprintf("user-%d", i), "hash")
		}(i)
	}
	wg.Wait()
	close(errCh)
	ok := 0
	for err := range errCh {
		if err == nil {
			ok++
		} else if !strings.Contains(err.Error(), "admin exists") && err != ErrAdminExists {
			t.Fatal(err)
		}
	}
	if ok != 1 {
		t.Fatalf("admins created %d", ok)
	}
	n, err := s.UserCount(ctx)
	if err != nil || n != 1 {
		t.Fatalf("users %d %v", n, err)
	}
}

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func insertUsage(t *testing.T, s *Store, id string, usage domain.Usage) {
	t.Helper()
	usage.AttemptID = "att-usage"
	body, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO usage_records(id, attempt_id, source, source_event_id, measures_json) VALUES(?,?,?,?,?)`, id, "att-usage", usage.Source, usage.SourceEventID, string(body)); err != nil {
		t.Fatal(err)
	}
}

func int64Ptr(n int64) *int64 { return &n }

func fmtError() error { return errStop }

var errStop = errString("stop")

type errString string

func (e errString) Error() string { return string(e) }
