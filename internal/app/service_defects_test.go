package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/sllt/agentlab/internal/domain"
)

func TestRecoverDoesNotSignalUnconfirmedPID(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
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
	if err := svc.Store.UpdateRuntime(ctx, attempt.ID, withIdentity(`{}`, "forged", cmd.Process.Pid, "not-the-kernel-starttime")); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(svc.DataDir, "attempts", attempt.ID, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	journal := []byte(`{"agent_start":"started","pid":` + strconv.Itoa(cmd.Process.Pid) + `}`)
	if err := os.WriteFile(filepath.Join(work, "journal.json"), journal, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("recovery signaled a pid it could not confirm: %v", err)
	}
	got, err := svc.Store.GetAttempt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CleanupState != string(domain.CleanupQuarantined) || got.Verdict != string(domain.VerdictInconclusive) {
		t.Fatalf("unconfirmed identity %+v", got)
	}
}

func TestAgentQuarantineTextDoesNotHoldCapacity(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	trial := seedTrial(t, svc, "", "noise")
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trial)
	if err != nil || len(attempts) != 1 {
		t.Fatal(err)
	}
	if attempts[0].CleanupState != string(domain.CleanupClean) {
		t.Fatalf("agent text changed cleanup: %s", attempts[0].CleanupState)
	}
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, trial))
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.Store.ActiveAttempts(ctx, account)
	if err != nil || n != 0 {
		t.Fatalf("capacity still held: %d %v", n, err)
	}
}

func TestPublishFreezesBytesAndRefusesDataDir(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.Store.CreateProject(ctx, "冻结", `{"kind":"local","allowed_roots":["`+abs+`"]}`)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(TaskSnapshot{Name: "标记", Prompt: "保持当时的文件", SourceDir: abs})
	task, err := svc.Store.CreateTask(ctx, project.ID, "标记", string(body))
	if err != nil {
		t.Fatal(err)
	}
	version, err := svc.PublishTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	var snap TaskSnapshot
	if err := json.Unmarshal([]byte(version.SnapshotJSON), &snap); err != nil {
		t.Fatal(err)
	}
	frozen, err := os.ReadFile(filepath.Join(snap.SnapshotDir, "marker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(frozen) != "v1" || snap.ContentDigest == "" {
		t.Fatalf("snapshot followed the live tree: %q %+v", frozen, snap)
	}
	denied, err := svc.Store.CreateProject(ctx, "库", `{"kind":"local","allowed_roots":["`+svc.DataDir+`"]}`)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(TaskSnapshot{Name: "库", Prompt: "不要复制控制面", SourceDir: svc.DataDir})
	bad, err := svc.Store.CreateTask(ctx, denied.ID, "库", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishTask(ctx, bad.ID); !errors.Is(err, ErrUnexecutable) {
		t.Fatalf("data dir publish err %v", err)
	}
}

func TestCursorLaunchDoesNotReceiveSolution(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	account, err := svc.Store.CreateAccount(ctx, "manual_import", "ref:cursor", 1)
	if err != nil {
		t.Fatal(err)
	}
	reject := func(name string, snap ProfileSnapshot) {
		t.Helper()
		draft, _ := json.Marshal(snap)
		profile, err := svc.Store.CreateProfile(ctx, name, account.ID, string(draft))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.PublishProfile(ctx, profile.ID); !errors.Is(err, ErrUnexecutable) {
			t.Fatalf("%s published instead of being rejected: %v", name, err)
		}
	}
	reject("docker-offline", ProfileSnapshot{Adapter: "cursor", Model: "cursor-local", Executor: "docker", Network: "offline", DisplayName: "Cursor", Executable: "/bin/true", BillingPath: "subscription", EntitlementVerifiedAt: "2026-10-04T00:00:00Z"})
	reject("restricted", ProfileSnapshot{Adapter: "cursor", Model: "cursor-local", Executor: "native-trusted", Network: "restricted", DisplayName: "Cursor", Executable: "/bin/true"})
	reject("unverified", ProfileSnapshot{Adapter: "cursor", Model: "never-verified-model", Executor: "native-trusted", Network: "unrestricted", DisplayName: "Cursor", Executable: "/bin/true", BillingPath: "subscription"})
	reject("fake-mode", ProfileSnapshot{Adapter: "cursor", Model: "cursor-local", Executor: "native-trusted", Network: "unrestricted", DisplayName: "Cursor", FakeMode: "success", Executable: "/bin/true"})

	trial := seedTrial(t, svc, "", "success")
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trial)
	if err != nil || len(attempts) != 1 {
		t.Fatal(err)
	}
	runtime := []byte(attempts[0].RuntimeJSON)
	if !bytes.Contains(runtime, []byte(`"home_inherited":false`)) || !bytes.Contains(runtime, []byte("observational/native")) {
		t.Fatalf("runtime %s", attempts[0].RuntimeJSON)
	}
	if bytes.Contains(runtime, []byte("AGENTLAB_SOLUTION_DIR")) {
		t.Fatalf("solution path stored in runtime: %s", attempts[0].RuntimeJSON)
	}
}

func TestAuthFailureAbortsAndUsageIsStored(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	authTrial := seedTrial(t, svc, "", "auth")
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Store.GetTrial(ctx, authTrial)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExecutionState != string(domain.ExecAborted) || got.Verdict != string(domain.VerdictInconclusive) {
		t.Fatalf("auth trial %+v", got)
	}
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, authTrial))
	if err != nil {
		t.Fatal(err)
	}
	var blocked string
	if err := svc.Store.DB().QueryRow(`SELECT COALESCE(blocked_reason, '') FROM accounts WHERE id=?`, account).Scan(&blocked); err != nil {
		t.Fatal(err)
	}
	if blocked == "" {
		t.Fatal("authentication failure did not block the account")
	}
	usageTrial := seedTrial(t, svc, "", "usage")
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, usageTrial)
	if err != nil || len(attempts) != 1 {
		t.Fatal(err)
	}
	records, err := svc.Store.ListUsage(ctx, attempts[0].ID)
	if err != nil || len(records) != 1 || records[0].InputTokens == nil || *records[0].InputTokens != 3 || records[0].OutputTokens == nil || *records[0].OutputTokens != 4 {
		raw, _ := os.ReadFile(filepath.Join(svc.DataDir, "attempts", attempts[0].ID, "events.ndjson"))
		work := filepath.Join(svc.DataDir, "attempts", attempts[0].ID, "work")
		entries, _ := os.ReadDir(work)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		logBody, _ := os.ReadFile(filepath.Join(work, "agent.log"))
		t.Fatalf("usage %+v err %v state %s files %v log %q events %s", records, err, attempts[0].State, names, logBody, raw)
	}
}

func TestBlockedAccountDoesNotStallTheQueue(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	blockedTrial := seedTrial(t, svc, "", "success")
	other := seedTrial(t, svc, "", "empty")
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, blockedTrial))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.BlockAccount(ctx, account, "held"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Pump(ctx, 2); err != nil {
		t.Fatal(err)
	}
	stalled, err := svc.Store.GetTrial(ctx, blockedTrial)
	if err != nil || stalled.ExecutionState != string(domain.ExecQueued) {
		t.Fatalf("blocked trial %+v %v", stalled, err)
	}
	ran, err := svc.Store.GetTrial(ctx, other)
	if err != nil || ran.ExecutionState != string(domain.ExecCompleted) {
		t.Fatalf("other account %+v %v", ran, err)
	}
}

func TestEventMemoryCapAndLiveFile(t *testing.T) {
	sink := &eventSink{}
	chunk := append([]byte(`{"origin":"agent_observation","type":"text","payload":"`), bytes.Repeat([]byte("A"), 64<<10)...)
	chunk = append(chunk, []byte("\"}\n")...)
	for i := 0; i < 200; i++ {
		if _, err := sink.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sink.Write([]byte("{\"origin\":\"runner\",\"type\":\"Ready\"}\n")); err != nil {
		t.Fatal(err)
	}
	sink.finish()
	body := sink.Bytes()
	if sink.agentBytes > maxEventMemory || !bytes.Contains(body, []byte("event_buffer_8MiB")) || !bytes.Contains(body, []byte(`"origin":"runner"`)) {
		t.Fatalf("agent bytes %d truncated %v", sink.agentBytes, bytes.Contains(body, []byte("event_buffer_8MiB")))
	}

	ctx := context.Background()
	svc := newLab(t)
	svc.WallSeconds = 30
	trial := seedTrial(t, svc, "", "hang")
	done := make(chan error, 1)
	go func() { done <- svc.Pump(ctx, 1) }()
	deadline := time.Now().Add(5 * time.Second)
	var path string
	for time.Now().Before(deadline) {
		attempts, err := svc.Store.ListAttempts(ctx, trial)
		if err == nil && len(attempts) == 1 && attempts[0].State == string(domain.ExecRunning) {
			path = filepath.Join(svc.DataDir, "attempts", attempts[0].ID, "events.ndjson")
			if info, err := os.Stat(path); err == nil && info.Size() > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(raw, []byte("Ready")) {
		t.Fatalf("events were not on disk while running: %v %q", err, raw)
	}
	if _, err := svc.Store.RequestCancel(ctx, trial); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRepetitionCapAndExperimentRepair(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	trial := seedTrial(t, svc, "", "empty")
	tr, err := svc.Store.GetTrial(ctx, trial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SubmitExperiment(ctx, ExperimentRequest{
		Actor: "local", IdempotencyKey: "too-many", Mode: domain.ModeAgentProfile,
		TaskVersionIDs: []string{tr.TaskVersionID}, ProfileVersionIDs: []string{tr.ProfileVersionID}, Repetitions: 100000,
	}); !errors.Is(err, ErrUnexecutable) {
		t.Fatalf("cap err %v", err)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.SetExperimentState(ctx, tr.ExperimentID, string(domain.ExecQueued)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	exp, err := svc.Store.GetExperiment(ctx, tr.ExperimentID)
	if err != nil || exp.State != string(domain.ExecCompleted) {
		t.Fatalf("experiment %+v %v", exp, err)
	}
}

func TestMissingIdentityQuarantinesAndHoldsCapacity(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	trial := seedTrial(t, svc, "", "success")
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, trial))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := svc.Store.CreateAttempt(ctx, trial, account, NativeRuntime(), "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.MarkAgentStarted(ctx, attempt.ID, attempt.Fence); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.UpdateRuntime(ctx, attempt.ID, mergeRuntime(NativeRuntime(), map[string]any{"launch_attempted": true})); err != nil {
		t.Fatal(err)
	}
	if err := svc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Store.GetAttempt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CleanupState != string(domain.CleanupQuarantined) || got.Reason != "identity_missing" {
		t.Fatalf("missing identity %+v", got)
	}
	active, err := svc.Store.GlobalActiveAttempts(ctx)
	if err != nil || active != 1 {
		t.Fatalf("capacity released: %d %v", active, err)
	}
}

func TestCancelAfterPrepareDoesNotLaunch(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	trialID := seedTrial(t, svc, "", "success")
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, trialID))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := svc.Store.CreateAttempt(ctx, trialID, account, NativeRuntime(), "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Store.RequestCancel(ctx, trialID); err != nil {
		t.Fatal(err)
	}
	trial, err := svc.Store.GetTrial(ctx, trialID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.execute(ctx, trial, attempt); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Store.GetAttempt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != string(domain.ExecCancelled) {
		t.Fatalf("started after cancel: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(svc.DataDir, "attempts", attempt.ID, "events.ndjson")); !os.IsNotExist(err) {
		t.Fatalf("runner wrote events after cancel: %v", err)
	}
}

func TestEvidenceWriteFailureIsNotPass(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	root, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "tasks", "orders-pagination"))
	if err != nil {
		t.Fatal(err)
	}
	verdict, _, reason := svc.judge(ctx, TaskSnapshot{VerifierRoot: root}, ProfileSnapshot{}, "digest", "missing-attempt", t.TempDir(), t.TempDir(), nil)
	if verdict == domain.VerdictPass || reason != "evidence_missing" {
		t.Fatalf("verdict %s reason %s", verdict, reason)
	}
}

func TestTamperedSnapshotDoesNotRun(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.Store.CreateProject(ctx, "漂移", `{"kind":"local","allowed_roots":["`+abs+`"]}`)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(TaskSnapshot{Name: "漂移", Prompt: "保持发布时的字节", SourceDir: abs})
	task, err := svc.Store.CreateTask(ctx, project.ID, "漂移", string(body))
	if err != nil {
		t.Fatal(err)
	}
	version, err := svc.PublishTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var snap TaskSnapshot
	if err := json.Unmarshal([]byte(version.SnapshotJSON), &snap); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap.SnapshotDir, "note.txt"), []byte("changed-after-publish"), 0o644); err != nil {
		t.Fatal(err)
	}
	account, err := svc.Store.CreateAccount(ctx, "manual_import", "ref:tamper", 1)
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := json.Marshal(ProfileSnapshot{Adapter: "fixture", Model: "fixture-local", FakeMode: "empty", Executor: "native-trusted", Network: "unrestricted", DisplayName: "假 CLI"})
	profile, err := svc.Store.CreateProfile(ctx, "假", account.ID, string(draft))
	if err != nil {
		t.Fatal(err)
	}
	pv, err := svc.PublishProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := svc.SubmitExperiment(ctx, ExperimentRequest{
		Actor: "local", IdempotencyKey: "tamper", Mode: domain.ModeAgentProfile,
		TaskVersionIDs: []string{version.ID}, ProfileVersionIDs: []string{pv.ID}, Repetitions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	trials, err := svc.Store.ListTrials(ctx, exp.ID)
	if err != nil || len(trials) != 1 {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trials[0].ID)
	if err != nil || len(attempts) != 1 || attempts[0].Verdict != string(domain.VerdictInconclusive) || attempts[0].Reason != "snapshot digest mismatch" {
		t.Fatalf("%+v %v", attempts, err)
	}
}

func TestUnresolvedCommitCannotPublish(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.Store.CreateProject(ctx, "提交", `{"kind":"local","allowed_roots":["`+abs+`"]}`)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(TaskSnapshot{Name: "提交", Prompt: "必须是真实提交", SourceDir: abs, BaseCommit: "HEAD"})
	task, err := svc.Store.CreateTask(ctx, project.ID, "提交", string(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishTask(ctx, task.ID); !errors.Is(err, ErrUnexecutable) {
		t.Fatalf("unresolved commit err %v", err)
	}
}

func TestClosedDatabaseStopsTheLoop(t *testing.T) {
	svc := newLab(t)
	if err := svc.Store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if err := svc.Loop(ctx); err != nil {
		t.Fatal(err)
	}
	if !svc.Maintenance() || svc.FatalError() == "" {
		t.Fatalf("maintenance %v fatal %q", svc.Maintenance(), svc.FatalError())
	}
}
