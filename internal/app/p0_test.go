package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sllt/agentlab/internal/domain"
)

const fakeCursorHelp = `Usage: agent [options] [prompt...]
  -p, --print              Print responses to console
  --output-format <format> Output format: text | json | stream-json
  --force                  Allow commands`

// writeFakeCursor creates a shell script that behaves like a headless coding
// CLI: it answers --version/--help and otherwise runs body in the work dir.
func writeFakeCursor(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-cursor")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 'fake-cursor 9.9.9'; exit 0; fi\n" +
		"if [ \"$1\" = \"--help\" ]; then cat <<'HELP'\n" + fakeCursorHelp + "\nHELP\nexit 0; fi\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func seedProfileTrial(t *testing.T, svc *Service, snap ProfileSnapshot, accountRef string) (string, error) {
	t.Helper()
	ctx := context.Background()
	project, err := svc.Store.CreateProject(ctx, "真实 CLI", `{"kind":"local"}`)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(TaskSnapshot{Name: "空跑", Prompt: "写一个文件"})
	task, err := svc.Store.CreateTask(ctx, project.ID, "空跑", string(body))
	if err != nil {
		t.Fatal(err)
	}
	tv, err := svc.PublishTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	account, err := svc.Store.CreateAccount(ctx, "api_key", accountRef, 1)
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := json.Marshal(snap)
	profile, err := svc.Store.CreateProfile(ctx, snap.DisplayName, account.ID, string(draft))
	if err != nil {
		t.Fatal(err)
	}
	pv, err := svc.PublishProfile(ctx, profile.ID)
	if err != nil {
		return "", err
	}
	exp, err := svc.SubmitExperiment(ctx, ExperimentRequest{
		Actor: "usr", IdempotencyKey: "k-" + project.ID, Mode: domain.ModeAgentProfile,
		TaskVersionIDs: []string{tv.ID}, ProfileVersionIDs: []string{pv.ID}, Repetitions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	trials, err := svc.Store.ListTrials(ctx, exp.ID)
	if err != nil || len(trials) != 1 {
		t.Fatalf("%v %v", trials, err)
	}
	return trials[0].ID, nil
}

func TestRealAdapterPublishesAtReadyUnverifiedAndRuns(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	t.Setenv("CURSOR_API_KEY", "sk-test-value")
	t.Setenv("ACCOUNT_TOKEN", "acct-value")
	t.Setenv("OTHER_SECRET", "must-not-leak")
	cli := writeFakeCursor(t, `printf '%s|%s|%s' "${CURSOR_API_KEY:+key}" "${ACCOUNT_TOKEN:+acct}" "${OTHER_SECRET:+leak}" > seen.txt
echo '{"type":"result","usage":{"input_tokens":3,"output_tokens":4}}'
exit 0`)
	snap := ProfileSnapshot{Adapter: "cursor", Model: "fast", Executor: "native-trusted", Network: "unrestricted", DisplayName: "Cursor", Executable: cli, ApproveTools: true, CredentialEnv: []string{"CURSOR_API_KEY"}}
	trialID, err := seedProfileTrial(t, svc, snap, "env:ACCOUNT_TOKEN")
	if err != nil {
		t.Fatalf("ready_unverified profile was not publishable: %v", err)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trialID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("%v %v", attempts, err)
	}
	a := attempts[0]
	if a.State != string(domain.ExecCompleted) || a.Reason == "profile_not_verified" {
		t.Fatalf("real adapter did not run: %+v", a)
	}
	seen, err := os.ReadFile(filepath.Join(svc.DataDir, "attempts", a.ID, "work", "seen.txt"))
	if err != nil || string(seen) != "key|acct|" {
		t.Fatalf("credential forwarding %q %v", seen, err)
	}
	if strings.Contains(a.RuntimeJSON, "sk-test-value") || strings.Contains(a.RuntimeJSON, "acct-value") {
		t.Fatal("credential value reached the database")
	}
	var rt struct {
		BudgetDigest  string   `json:"budget_digest"`
		CredentialEnv []string `json:"credential_env"`
		Budget        struct {
			WallSeconds int    `json:"wall_seconds"`
			WallSource  string `json:"wall_source"`
		} `json:"budget"`
	}
	if err := json.Unmarshal([]byte(a.RuntimeJSON), &rt); err != nil {
		t.Fatal(err)
	}
	if rt.BudgetDigest == "" || rt.Budget.WallSeconds != 15 || rt.Budget.WallSource != "service" || len(rt.CredentialEnv) != 2 {
		t.Fatalf("runtime %s", a.RuntimeJSON)
	}
	pvs, err := svc.Store.ProfileVersions(ctx, mustProfileID(t, svc, trialID))
	if err != nil || len(pvs) != 1 || !strings.Contains(pvs[0].DoctorJSON, `"readiness":"ready_unverified"`) {
		t.Fatalf("doctor json %v %v", pvs, err)
	}
}

func mustProfileID(t *testing.T, svc *Service, trialID string) string {
	t.Helper()
	pv, err := svc.Store.GetProfileVersion(context.Background(), mustTrial(t, svc, trialID).ProfileVersionID)
	if err != nil {
		t.Fatal(err)
	}
	return pv.ProfileID
}

func TestRealAdapterWithoutCredentialsIsNotPublishable(t *testing.T) {
	svc := newLab(t)
	t.Setenv("CURSOR_API_KEY", "")
	cli := writeFakeCursor(t, "exit 0")
	snap := ProfileSnapshot{Adapter: "cursor", Model: "fast", Executor: "native-trusted", Network: "unrestricted", DisplayName: "Cursor", Executable: cli, CredentialEnv: []string{"CURSOR_API_KEY"}}
	if _, err := seedProfileTrial(t, svc, snap, "ref:none"); err == nil {
		t.Fatal("profile without a credential source was published")
	}
	snap.CredentialEnv = []string{"LD_PRELOAD"}
	if _, err := seedProfileTrial(t, svc, snap, "ref:none"); err == nil {
		t.Fatal("LD_PRELOAD forwarding was published")
	}
}

func TestRealAdapterAuthFailureIsInconclusive(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	t.Setenv("CURSOR_API_KEY", "sk-test")
	cli := writeFakeCursor(t, `echo 'Error: not logged in. Run agent login first.' >&2
exit 1`)
	snap := ProfileSnapshot{Adapter: "cursor", Model: "fast", Executor: "native-trusted", Network: "unrestricted", DisplayName: "Cursor", Executable: cli, CredentialEnv: []string{"CURSOR_API_KEY"}}
	trialID, err := seedProfileTrial(t, svc, snap, "ref:none")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	got := mustTrial(t, svc, trialID)
	if got.Verdict != string(domain.VerdictInconclusive) || got.ExecutionState != string(domain.ExecAborted) {
		t.Fatalf("auth failure %+v", got)
	}
	attempts, _ := svc.Store.ListAttempts(ctx, trialID)
	if attempts[0].Reason != "authentication_failed" {
		t.Fatalf("reason %q", attempts[0].Reason)
	}
}

func TestRealAdapterMissingAtRunTimeIsInfrastructureFailure(t *testing.T) {
	ctx := context.Background()
	svc := newLab(t)
	t.Setenv("CURSOR_API_KEY", "sk-test")
	cli := writeFakeCursor(t, "exit 0")
	snap := ProfileSnapshot{Adapter: "cursor", Model: "fast", Executor: "native-trusted", Network: "unrestricted", DisplayName: "Cursor", Executable: cli, CredentialEnv: []string{"CURSOR_API_KEY"}}
	trialID, err := seedProfileTrial(t, svc, snap, "ref:none")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cli); err != nil {
		t.Fatal(err)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, _ := svc.Store.ListAttempts(ctx, trialID)
	if len(attempts) != 1 || attempts[0].Reason != "cli_not_found" || attempts[0].Verdict != string(domain.VerdictInconclusive) {
		t.Fatalf("%+v", attempts)
	}
}

func TestDispatchRefillsSlotsWithoutWaitingForTheBatch(t *testing.T) {
	svc := newLab(t)
	svc.GlobalLimit = 2
	svc.WallSeconds = 60
	hang := seedTrial(t, svc, "", "hang")
	first := seedTrial(t, svc, "", "success")
	third := seedTrial(t, svc, "", "success")
	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan error, 1)
	go func() { loopDone <- svc.Loop(ctx) }()
	deadline := time.Now().Add(20 * time.Second)
	ok := false
	for time.Now().Before(deadline) {
		if mustTrial(t, svc, third).ExecutionState == string(domain.ExecCompleted) {
			ok = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	hangState := mustTrial(t, svc, hang).ExecutionState
	if _, err := svc.Store.RequestCancel(context.Background(), hang); err != nil {
		t.Logf("cancel: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-loopDone
	if !ok {
		t.Fatalf("third trial waited for the hung batch: first=%s third=%s", mustTrial(t, svc, first).ExecutionState, mustTrial(t, svc, third).ExecutionState)
	}
	if domain.ExecutionState(hangState).Terminal() {
		t.Fatalf("hang trial should still have been running, got %s", hangState)
	}
}

func TestWallClockResolution(t *testing.T) {
	svc := newLab(t)
	svc.WallSeconds = 0
	t.Setenv("AGENTLAB_AGENT_WALL_SECONDS", "")
	if got, src := svc.wallFor(TaskSnapshot{}, ProfileSnapshot{}); got != defaultWallSeconds || src != "default" {
		t.Fatalf("default %d %s", got, src)
	}
	t.Setenv("AGENTLAB_AGENT_WALL_SECONDS", "900")
	if got, src := svc.wallFor(TaskSnapshot{}, ProfileSnapshot{}); got != 900 || src != "env" {
		t.Fatalf("env %d %s", got, src)
	}
	if _, err := svc.SaveSettings(LabSettings{GlobalLimit: 1, AgentWallSeconds: 600}); err != nil {
		t.Fatal(err)
	}
	if got, src := svc.wallFor(TaskSnapshot{}, ProfileSnapshot{}); got != 600 || src != "settings" {
		t.Fatalf("settings %d %s", got, src)
	}
	if got, src := svc.wallFor(TaskSnapshot{}, ProfileSnapshot{WallSeconds: 300}); got != 300 || src != "profile" {
		t.Fatalf("profile %d %s", got, src)
	}
	task := TaskSnapshot{Limits: &domain.Limits{AgentWallSeconds: 120}}
	if got, src := svc.wallFor(task, ProfileSnapshot{WallSeconds: 300}); got != 120 || src != "task" {
		t.Fatalf("task %d %s", got, src)
	}
	if _, err := svc.SaveSettings(LabSettings{GlobalLimit: 1, AgentWallSeconds: 1}); err == nil {
		t.Fatal("a 1 second wall clock was accepted")
	}
	// The same numeric budget from different sources has one digest.
	a, _, _ := budgetFor(TaskSnapshot{}, ProfileSnapshot{}, 600, "settings")
	b, _, _ := budgetFor(TaskSnapshot{}, ProfileSnapshot{}, 600, "task")
	c, _, _ := budgetFor(TaskSnapshot{}, ProfileSnapshot{}, 601, "task")
	if budgetDigest(a) != budgetDigest(b) || budgetDigest(a) == budgetDigest(c) {
		t.Fatal("budget digest does not track the numeric budget")
	}
}
