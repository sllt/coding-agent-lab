package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/stats"
)

func TestChatterDoesNotBlockOrChangeVerdict(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	trial := seedTrial(t, svc, "", "chatter")
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Store.GetTrial(ctx, trial)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExecutionState != string(domain.ExecCompleted) || got.Verdict == string(domain.VerdictInconclusive) {
		t.Fatalf("chatter should not become inconclusive: %+v", got)
	}
	account, err := svc.Store.ProfileAccount(ctx, mustProfile(t, svc, trial))
	if err != nil {
		t.Fatal(err)
	}
	var blocked string
	if err := svc.Store.DB().QueryRow(`SELECT COALESCE(blocked_reason, '') FROM accounts WHERE id=?`, account).Scan(&blocked); err != nil {
		t.Fatal(err)
	}
	if blocked != "" {
		t.Fatalf("agent text blocked the account: %s", blocked)
	}
}

func TestGlobalLimitOverlapsTwoAttempts(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	first := seedTrial(t, svc, "", "hang")
	second := seedTrial(t, svc, "", "hang")
	done := make(chan error, 1)
	go func() { done <- svc.Pump(ctx, 2) }()
	deadline := time.Now().Add(8 * time.Second)
	both := false
	for time.Now().Before(deadline) {
		a, _ := svc.Store.ListAttempts(ctx, first)
		b, _ := svc.Store.ListAttempts(ctx, second)
		if len(a) == 1 && len(b) == 1 && a[0].State == string(domain.ExecRunning) && b[0].State == string(domain.ExecRunning) {
			both = true
			_, _ = svc.Store.RequestCancel(ctx, first)
			_, _ = svc.Store.RequestCancel(ctx, second)
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !both {
		t.Fatal("second attempt was not running before the first execute returned")
	}
}

func TestRunnerChildDoesNotOpenDatabase(t *testing.T) {
	svc := newLab(t)
	db := svc.Store.Path()
	cmd := exec.Command(os.Args[0], "runner")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	fds, err := os.ReadDir("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/fd")
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid), "fd", fd.Name()))
		if err != nil {
			continue
		}
		if target == db || strings.HasSuffix(target, "lab.db") {
			t.Fatalf("runner opened %s", target)
		}
	}
}

func TestPoisonedRunnerOriginStillCounts(t *testing.T) {
	sink := &eventSink{}
	pad := strings.Repeat("A", 32<<10)
	line := []byte(`{"origin":"agent_observation","type":"text","payload":{"origin":"runner","pad":"` + pad + `"}}` + "\n")
	for i := 0; i < 300; i++ {
		if _, err := sink.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	sink.finish()
	if sink.agentBytes > maxEventMemory || sink.agentBytes == 0 || !bytes.Contains(sink.Bytes(), []byte("event_buffer_8MiB")) {
		t.Fatalf("agent bytes %d", sink.agentBytes)
	}
}

func TestRepairPassDoesNotRewriteFirstAttempt(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	root := filepath.Join("..", "..", "fixtures", "tasks", "orders-pagination")
	trialID := seedRepairTrial(t, svc, root, "fail")
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trialID)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("attempts %+v %v", attempts, err)
	}
	if attempts[0].Verdict != string(domain.VerdictFail) {
		t.Fatalf("first verdict %s", attempts[0].Verdict)
	}
	if !strings.HasPrefix(attempts[1].Reason, "protocol_repair") {
		t.Fatalf("second reason %s", attempts[1].Reason)
	}
	if _, err := svc.Store.DB().Exec(`UPDATE attempts SET verdict=? WHERE id=?`, string(domain.VerdictPass), attempts[1].ID); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Store.ListAttempts(ctx, trialID)
	if err != nil || again[0].Verdict != string(domain.VerdictFail) || again[1].Verdict != string(domain.VerdictPass) {
		t.Fatalf("after repair pass %+v %v", again, err)
	}
}

func TestTaskMacroAverageIgnoresRepetitionWeight(t *testing.T) {
	report := stats.Aggregate([]stats.Row{
		{TaskID: "a", ProfileID: "p", Pass: 1, Fail: 0},
		{TaskID: "b", ProfileID: "p", Pass: 0, Fail: 1},
	}, 7, 20)
	if report.Method != stats.MethodVersion || report.Seed != 7 || report.MacroAverage["p"] != 0.5 {
		body, _ := json.Marshal(report)
		t.Fatalf("stats %s", body)
	}
}

func TestQuotaBlocksNewExperiment(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	if _, err := svc.SaveSettings(LabSettings{GlobalLimit: 1, DiskQuotaBytes: 1, Retention: Retention{LogDays: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc.DataDir, "blob"), []byte("too-big"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SubmitExperiment(ctx, ExperimentRequest{
		Actor: "local", IdempotencyKey: "quota", Mode: domain.ModeAgentProfile,
		TaskVersionIDs: []string{"missing"}, ProfileVersionIDs: []string{"missing"}, Repetitions: 1,
	}); err == nil {
		t.Fatal("quota did not block")
	}
}

func seedRepairTrial(t *testing.T, svc *Service, root, mode string) string {
	t.Helper()
	id := seedTrial(t, svc, root, mode)
	if _, err := svc.Store.RequestCancel(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	tr := mustTrial(t, svc, id)
	ctx := context.Background()
	exp, err := svc.SubmitExperiment(ctx, ExperimentRequest{
		Actor: "usr", IdempotencyKey: "repair-" + id, Mode: domain.ModeAgentProfile,
		TaskVersionIDs: []string{tr.TaskVersionID}, ProfileVersionIDs: []string{tr.ProfileVersionID},
		Repetitions: 1, Protocol: "repair-once-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	trials, err := svc.Store.ListTrials(ctx, exp.ID)
	if err != nil || len(trials) != 1 {
		t.Fatal(err)
	}
	return trials[0].ID
}
