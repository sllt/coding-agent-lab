package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sllt/agentlab/internal/agent/fixture"
	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/store/sqlite"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "fake-agent" {
		os.Exit(fixture.Run())
	}
	os.Exit(m.Run())
}

func TestFixtureLoopPassFailEmptyAndForge(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join("..", "..", "fixtures", "tasks", "orders-pagination")
	for _, tc := range []struct {
		mode string
		want domain.Verdict
	}{
		{"success", domain.VerdictPass},
		{"fail", domain.VerdictFail},
		{"empty", domain.VerdictFail},
		{"forge", domain.VerdictFail},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			svc := newLab(t)
			trial := seedTrial(t, svc, root, tc.mode)
			if err := svc.Pump(ctx, 1); err != nil {
				t.Fatal(err)
			}
			got, err := svc.Store.GetTrial(ctx, trial)
			if err != nil {
				t.Fatal(err)
			}
			if got.ExecutionState != string(domain.ExecCompleted) || got.Verdict != string(tc.want) {
				t.Fatalf("state %s verdict %s", got.ExecutionState, got.Verdict)
			}
			attempts, err := svc.Store.ListAttempts(ctx, trial)
			if err != nil || len(attempts) != 1 {
				t.Fatalf("attempts %+v err %v", attempts, err)
			}
			if attempts[0].CleanupState != string(domain.CleanupClean) {
				t.Fatalf("cleanup %s", attempts[0].CleanupState)
			}
			if _, err := os.Stat(filepath.Join(root, "orders.go")); err != nil {
				t.Fatal(err)
			}
			hidden := filepath.Join(svc.DataDir, "attempts", attempts[0].ID, "work", "hidden")
			if _, err := os.Stat(hidden); !os.IsNotExist(err) {
				t.Fatalf("hidden material visible to the agent: %v", err)
			}
		})
	}
}

func TestQueuedCancelDoesNotStartRunner(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	trial := seedTrial(t, svc, filepath.Join("..", "..", "fixtures", "tasks", "orders-pagination"), "success")
	out, err := svc.Store.RequestCancel(ctx, trial)
	if err != nil {
		t.Fatal(err)
	}
	if out.ExecutionState != string(domain.ExecCancelled) || out.Verdict != string(domain.VerdictUnverified) {
		t.Fatalf("%+v", out)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trial)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 0 {
		t.Fatalf("runner started: %+v", attempts)
	}
}

func TestCancelWhileRunningIsNotImmediatelyClean(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	svc.WallSeconds = 30
	trial := seedTrial(t, svc, "", "hang")
	done := make(chan error, 1)
	go func() { done <- svc.Pump(ctx, 1) }()
	deadline := time.Now().Add(5 * time.Second)
	var attemptID string
	for time.Now().Before(deadline) {
		attempts, err := svc.Store.ListAttempts(ctx, trial)
		if err == nil && len(attempts) == 1 && attempts[0].State == string(domain.ExecRunning) {
			attemptID = attempts[0].ID
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if attemptID == "" {
		t.Fatal("attempt did not reach running")
	}
	if _, err := svc.Store.RequestCancel(ctx, trial); err != nil {
		t.Fatal(err)
	}
	fresh, err := svc.Store.GetAttempt(ctx, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.CleanupState == string(domain.CleanupClean) {
		t.Fatal("cancel returned while cleanup was already clean")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	final, err := svc.Store.GetAttempt(ctx, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != string(domain.ExecCancelled) || final.Verdict != string(domain.VerdictUnverified) {
		t.Fatalf("final %+v", final)
	}
	if final.CleanupState != string(domain.CleanupClean) {
		t.Fatalf("cleanup %s", final.CleanupState)
	}
}

func TestRecoverDoesNotRequeuePaidRun(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	trial := seedTrial(t, svc, "", "success")
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, trial))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := svc.Store.CreateAttempt(ctx, trial, account, `{}`, "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.MarkAgentStarted(ctx, attempt.ID, attempt.Fence); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(svc.DataDir, "attempts", attempt.ID, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "journal.json"), []byte("{\"agent_start\":\"started\",\"pid\":0}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Store.GetTrial(ctx, trial)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExecutionState != string(domain.ExecAborted) || got.Verdict != string(domain.VerdictInconclusive) {
		t.Fatalf("%+v", got)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trial)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("requeued: %+v", attempts)
	}
}

func TestStaleFenceIsAudited(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	trial := seedTrial(t, svc, "", "empty")
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, trial))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := svc.Store.CreateAttempt(ctx, trial, account, `{}`, "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Store.FinishAttempt(ctx, attempt.ID, attempt.Fence-1, string(domain.ExecAborted), string(domain.VerdictInconclusive), string(domain.CleanupClean), "stale")
	if !errors.Is(err, sqlite.ErrConflict) {
		t.Fatalf("err %v", err)
	}
	n, err := svc.Store.CountAudit(ctx, "stale_fence", attempt.ID)
	if err != nil || n != 1 {
		t.Fatalf("audit %d err %v", n, err)
	}
	fresh, err := svc.Store.GetAttempt(ctx, attempt.ID)
	if err != nil || fresh.State == string(domain.ExecAborted) {
		t.Fatalf("attempt overwritten %+v err %v", fresh, err)
	}
}

func TestBaselineMismatchBlocksPublish(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	dir := t.TempDir()
	absRoot, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.Store.CreateProject(ctx, "坏任务", `{"kind":"local","allowed_roots":["`+absRoot+`"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/empty\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(TaskSnapshot{Name: "空", Prompt: "做点什么", VerifierRoot: dir, SourceDir: dir})
	task, err := svc.Store.CreateTask(ctx, project.ID, "空", string(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishTask(ctx, task.ID); !errors.Is(err, ErrUnexecutable) {
		t.Fatalf("err %v", err)
	}
	versions, err := svc.Store.TaskVersions(ctx, task.ID)
	if err != nil || len(versions) != 0 {
		t.Fatalf("versions %+v err %v", versions, err)
	}
}

func TestBackupRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	if _, err := svc.Store.CreateProject(ctx, "备份", `{}`); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := svc.Store.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	restored, err := sqlite.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	projects, _, err := restored.ListProjects(ctx, "", 10)
	if err != nil || len(projects) != 1 || projects[0].Name != "备份" {
		t.Fatalf("%+v err %v", projects, err)
	}
}

func TestManualRetryKeepsFirstAttempt(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	root := filepath.Join("..", "..", "fixtures", "tasks", "orders-pagination")
	trial := seedTrial(t, svc, root, "fail")
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Store.ListAttempts(ctx, trial)
	if err != nil || len(first) != 1 || first[0].Verdict != string(domain.VerdictFail) {
		t.Fatalf("%+v", first)
	}
	if err := svc.Store.PrepareRetry(ctx, trial); err != nil {
		t.Fatal(err)
	}
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, trial))
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Store.CreateAttempt(ctx, trial, account, `{}`, "人工重试", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.execute(ctx, mustTrial(t, svc, trial), second); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trial)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("%+v err %v", attempts, err)
	}
	if attempts[0].ID != first[0].ID || attempts[0].Verdict != string(domain.VerdictFail) {
		t.Fatalf("first attempt changed: %+v", attempts[0])
	}
	if attempts[1].Number != 2 {
		t.Fatalf("second number %d", attempts[1].Number)
	}
}

func newLab(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.Open(filepath.Join(dir, "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Service{Store: store, DataDir: dir, ExecPath: os.Args[0], WallSeconds: 15}
}

func seedTrial(t *testing.T, svc *Service, root, mode string) string {
	t.Helper()
	ctx := context.Background()
	spec := `{"kind":"local"}`
	snap := TaskSnapshot{Name: "分页", Prompt: "修复分页游标", SourceDir: root, VerifierRoot: root}
	if root == "" {
		snap = TaskSnapshot{Name: "空跑", Prompt: "不要调用模型", SourceDir: "", VerifierRoot: ""}
	} else if abs, err := filepath.Abs(root); err == nil {
		snap.SourceDir = abs
		snap.VerifierRoot = abs
		spec = `{"kind":"local","allowed_roots":["` + abs + `"]}`
	}
	project, err := svc.Store.CreateProject(ctx, "订单", spec)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(snap)
	task, err := svc.Store.CreateTask(ctx, project.ID, snap.Name, string(body))
	if err != nil {
		t.Fatal(err)
	}
	tv, err := svc.PublishTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	account, err := svc.Store.CreateAccount(ctx, "manual_import", "ref:local-fixture", 1)
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := json.Marshal(ProfileSnapshot{Adapter: "fixture", Model: "fixture-local", FakeMode: mode, Executor: "native-trusted", Network: "unrestricted", DisplayName: "本地假 CLI"})
	profile, err := svc.Store.CreateProfile(ctx, "假 CLI", account.ID, string(draft))
	if err != nil {
		t.Fatal(err)
	}
	pv, err := svc.PublishProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := svc.SubmitExperiment(ctx, ExperimentRequest{
		Actor: "usr", IdempotencyKey: "k-" + mode + "-" + project.ID, Mode: domain.ModeAgentProfile,
		TaskVersionIDs: []string{tv.ID}, ProfileVersionIDs: []string{pv.ID}, Repetitions: 1, Protocol: "single-pass-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	trials, err := svc.Store.ListTrials(ctx, exp.ID)
	if err != nil || len(trials) != 1 {
		t.Fatalf("%+v err %v", trials, err)
	}
	return trials[0].ID
}

func mustProfile(t *testing.T, svc *Service, trialID string) string {
	t.Helper()
	tr, err := svc.Store.GetTrial(context.Background(), trialID)
	if err != nil {
		t.Fatal(err)
	}
	return tr.ProfileVersionID
}

func mustTrial(t *testing.T, svc *Service, id string) sqlite.Trial {
	t.Helper()
	tr, err := svc.Store.GetTrial(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}
