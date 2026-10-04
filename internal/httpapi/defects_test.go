package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sllt/agentlab/internal/app"
	"github.com/sllt/agentlab/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestSetupCancelAndStrictDraftMessages(t *testing.T) {
	_, rt, svc := newAPI(t)
	token, csrf := setupLogin(t, rt)
	again := call(rt, http.MethodPost, "/api/v1/setup", map[string]string{"username": "other", "password": "local-pass"}, "", "")
	if again.Code != http.StatusConflict || !strings.Contains(again.Body.String(), "管理员已经存在") {
		t.Fatalf("setup %d %s", again.Code, again.Body.String())
	}
	var n int
	if err := svc.Store.DB().QueryRow(`SELECT COUNT(1) FROM audit_events WHERE action='login'`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("login audit %d %v", n, err)
	}
	project := call(rt, http.MethodPost, "/api/v1/projects", map[string]string{"name": "严格"}, token, csrf)
	if project.Code != http.StatusCreated {
		t.Fatalf("project %d %s", project.Code, project.Body.String())
	}
	var box struct {
		Data struct {
			ID string `json:"ID"`
		} `json:"data"`
	}
	if err := json.Unmarshal(project.Body.Bytes(), &box); err != nil || box.Data.ID == "" {
		t.Fatalf("project id %s", project.Body.String())
	}
	id := box.Data.ID
	bad := call(rt, http.MethodPost, "/api/v1/projects/"+id+"/tasks", map[string]any{"name": "题", "prompt": "做", "extra": true}, token, csrf)
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), "未知字段") {
		t.Fatalf("draft %d %s", bad.Code, bad.Body.String())
	}
	task, profile := publishPair(t, svc)
	ctx := context.Background()
	exp, err := svc.SubmitExperiment(ctx, app.ExperimentRequest{
		Actor: "local", IdempotencyKey: "done", Mode: domain.ModeAgentProfile,
		TaskVersionIDs: []string{task}, ProfileVersionIDs: []string{profile}, Repetitions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	trials, err := svc.Store.ListTrials(ctx, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	account, err := svc.Store.ProfileAccount(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := svc.Store.CreateAttempt(ctx, trials[0].ID, account, app.NativeRuntime(), "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Store.DB().Exec(`UPDATE attempts SET state=?, verdict=?, cleanup_state=? WHERE id=?`, string(domain.ExecCompleted), string(domain.VerdictPass), string(domain.CleanupClean), attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Store.DB().Exec(`UPDATE trials SET execution_state=?, verdict=? WHERE id=?`, string(domain.ExecCompleted), string(domain.VerdictPass), trials[0].ID); err != nil {
		t.Fatal(err)
	}
	cancel := call(rt, http.MethodPost, "/api/v1/trials/"+trials[0].ID+"/cancel", map[string]any{}, token, csrf)
	if cancel.Code != http.StatusConflict || !strings.Contains(cancel.Body.String(), "已经结束，取消不会再执行") {
		t.Fatalf("cancel %d %s", cancel.Code, cancel.Body.String())
	}
	huge := callKey(rt, "/api/v1/experiments", map[string]any{
		"mode": "agent_profile", "task_version_ids": []string{task}, "profile_version_ids": []string{profile},
		"repetitions": 100000, "protocol": "single-pass-v1",
	}, token, csrf, "huge")
	if huge.Code != 422 {
		t.Fatalf("repetitions %d %s", huge.Code, huge.Body.String())
	}
}

func TestWebhookDoesNotMintSecretAndRejectsBodyMismatch(t *testing.T) {
	api, rt, svc := newAPI(t)
	secretPath := filepath.Join(svc.DataDir, "webhook.secret")
	raw := []byte(`{"n":1}`)
	rec := webhook(rt, raw, "", "dedupe-x")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "密钥未配置") {
		t.Fatalf("unsigned %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(secretPath); !os.IsNotExist(err) {
		t.Fatalf("secret created on unsigned post: %v", err)
	}
	secret, err := api.webhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	first := webhook(rt, raw, Sign(secret, raw), "dedupe-y")
	if first.Code != http.StatusOK {
		t.Fatalf("first %d %s", first.Code, first.Body.String())
	}
	other := []byte(`{"n":2}`)
	mismatch := webhook(rt, other, Sign(secret, other), "dedupe-y")
	if mismatch.Code != http.StatusConflict || !strings.Contains(mismatch.Body.String(), "不同正文") {
		t.Fatalf("mismatch %d %s", mismatch.Code, mismatch.Body.String())
	}
}

func TestBackupWaitsAndSSEHasHeartbeat(t *testing.T) {
	_, rt, svc := newAPI(t)
	token, csrf := setupLogin(t, rt)
	ctx := context.Background()
	task, profile := publishPair(t, svc)
	exp, err := svc.SubmitExperiment(ctx, app.ExperimentRequest{Actor: "local", IdempotencyKey: "bak", Mode: domain.ModeAgentProfile, TaskVersionIDs: []string{task}, ProfileVersionIDs: []string{profile}, Repetitions: 1})
	if err != nil {
		t.Fatal(err)
	}
	trials, err := svc.Store.ListTrials(ctx, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	account, err := svc.Store.ProfileAccount(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := svc.Store.CreateAttempt(ctx, trials[0].ID, account, app.NativeRuntime(), "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	old := backupWait
	backupWait = 30 * time.Millisecond
	t.Cleanup(func() { backupWait = old })
	rec := call(rt, http.MethodPost, "/api/v1/maintenance/backup", map[string]any{}, token, csrf)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "未生成备份") {
		t.Fatalf("backup %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(svc.DataDir, "backups")); !os.IsNotExist(err) {
		t.Fatalf("backup dir appeared while an attempt was open: %v", err)
	}
	if svc.Maintenance() {
		t.Fatal("maintenance stayed on after refusal")
	}
	dir := filepath.Join(svc.DataDir, "attempts", attempt.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.ndjson"), []byte("{\"sequence\":1,\"type\":\"Ready\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prevMax := maxEventStreams
	maxEventStreams = 0
	t.Cleanup(func() { maxEventStreams = prevMax })
	capped := call(rt, http.MethodGet, "/api/v1/attempts/"+attempt.ID+"/events", nil, token, csrf)
	if capped.Code != http.StatusTooManyRequests {
		t.Fatalf("cap %d %s", capped.Code, capped.Body.String())
	}
	maxEventStreams = 32
	prevHeart := heartbeatEvery
	heartbeatEvery = 20 * time.Millisecond
	t.Cleanup(func() { heartbeatEvery = prevHeart })
	req := httptest.NewRequest(http.MethodGet, "/api/v1/attempts/"+attempt.ID+"/events", nil)
	req.Host = "127.0.0.1"
	req.AddCookie(&http.Cookie{Name: "agentlab_session", Value: token})
	ctxTimeout, cancel := context.WithTimeout(req.Context(), 120*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctxTimeout)
	stream := httptest.NewRecorder()
	rt.Handler.ServeHTTP(stream, req)
	if stream.Header().Get("Cache-Control") != "no-cache, no-transform" || !strings.Contains(stream.Body.String(), ": heartbeat") {
		t.Fatalf("sse headers %v body %s", stream.Header(), stream.Body.String())
	}
}

func TestExportLinkComparisonsAndOpenAPIContract(t *testing.T) {
	_, rt, svc := newAPI(t)
	token, csrf := setupLogin(t, rt)
	ctx := context.Background()
	task, profile := publishPair(t, svc)
	exp, err := svc.SubmitExperiment(ctx, app.ExperimentRequest{Actor: "local", IdempotencyKey: "cmp", Mode: domain.ModeAgentProfile, TaskVersionIDs: []string{task}, ProfileVersionIDs: []string{profile}, Repetitions: 1})
	if err != nil {
		t.Fatal(err)
	}
	exported := call(rt, http.MethodPost, "/api/v1/experiments/"+exp.ID+"/export", map[string]any{}, token, csrf)
	if exported.Code != http.StatusAccepted || !strings.Contains(exported.Body.String(), "download_path") {
		t.Fatalf("export %d %s", exported.Code, exported.Body.String())
	}
	var n int
	if err := svc.Store.DB().QueryRow(`SELECT COUNT(1) FROM audit_events WHERE action='export'`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("export audit %d %v", n, err)
	}
	trials, err := svc.Store.ListTrials(ctx, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	account, err := svc.Store.ProfileAccount(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	runtime := `{"executor":"docker","network":"restricted"}`
	attempt, err := svc.Store.CreateAttempt(ctx, trials[0].ID, account, runtime, "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Store.DB().Exec(`UPDATE attempts SET state=?, verdict=?, cleanup_state=? WHERE id=?`, string(domain.ExecCompleted), string(domain.VerdictPass), string(domain.CleanupClean), attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Store.DB().Exec(`UPDATE trials SET execution_state=?, verdict=? WHERE id=?`, string(domain.ExecCompleted), string(domain.VerdictPass), trials[0].ID); err != nil {
		t.Fatal(err)
	}
	board := call(rt, http.MethodGet, "/api/v1/comparisons?experiment_id="+exp.ID, nil, token, csrf)
	if board.Code != http.StatusOK || strings.Contains(board.Body.String(), `"executor":"docker"`) || !strings.Contains(board.Body.String(), `"executor":"native-trusted"`) || !strings.Contains(board.Body.String(), `"network":"unrestricted"`) {
		t.Fatalf("compare %d %s", board.Code, board.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
		Paths map[string]map[string]struct {
			RequestBody map[string]any `yaml:"requestBody"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Components.Schemas) == 0 {
		t.Fatal("openapi has no component schemas")
	}
	var bodies int
	for _, methods := range doc.Paths {
		for _, op := range methods {
			if len(op.RequestBody) > 0 {
				bodies++
			}
		}
	}
	if bodies == 0 {
		t.Fatal("openapi has no requestBody")
	}
	client, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "generated", "client.ts"))
	if err != nil || !strings.Contains(string(client), "requestJSON") || !strings.Contains(string(client), "#/components/schemas/ExperimentPlan") {
		t.Fatalf("generated client missing: %v", err)
	}
}
