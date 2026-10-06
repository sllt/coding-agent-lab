package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sllt/agentlab/internal/agent/fixture"
	"github.com/sllt/agentlab/internal/app"
	"github.com/sllt/agentlab/internal/bootstrap"
	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/runner"
	"github.com/sllt/agentlab/internal/store/sqlite"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "fake-agent" {
		os.Exit(fixture.Run())
	}
	if len(os.Args) > 1 && os.Args[1] == "runner" {
		if err := runner.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestUnauthenticatedAndBadCSRF(t *testing.T) {
	_, rt, _ := newAPI(t)
	rec := call(rt, http.MethodPost, "/api/v1/projects", map[string]string{"name": "x"}, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	token, csrf := setupLogin(t, rt)
	rec = call(rt, http.MethodPost, "/api/v1/projects", map[string]string{"name": "订单"}, token, "nope")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("csrf status %d body %s", rec.Code, rec.Body.String())
	}
	rec = call(rt, http.MethodPost, "/api/v1/projects", map[string]string{"name": "订单"}, token, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Host = "127.0.0.1"
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(&http.Cookie{Name: "agentlab_session", Value: token})
	bad := httptest.NewRecorder()
	rt.Handler.ServeHTTP(bad, req)
	if bad.Code != http.StatusForbidden {
		t.Fatalf("origin %d %s", bad.Code, bad.Body.String())
	}
}

func TestIdempotentExperimentAndCancel(t *testing.T) {
	_, rt, svc := newAPI(t)
	token, csrf := setupLogin(t, rt)
	task, profile := publishPair(t, svc)
	body := map[string]any{
		"mode": "agent_profile", "task_version_ids": []string{task}, "profile_version_ids": []string{profile},
		"repetitions": 1, "protocol": "single-pass-v1",
	}
	first := callKey(rt, "/api/v1/experiments", body, token, csrf, "study-1")
	if first.Code != http.StatusAccepted {
		t.Fatalf("submit %d %s", first.Code, first.Body.String())
	}
	second := callKey(rt, "/api/v1/experiments", body, token, csrf, "study-1")
	if second.Code != http.StatusAccepted || !strings.Contains(second.Body.String(), experimentID(first.Body.Bytes())) {
		t.Fatalf("replay %d %s", second.Code, second.Body.String())
	}
	other := callKey(rt, "/api/v1/experiments", map[string]any{
		"mode": "agent_profile", "task_version_ids": []string{task}, "profile_version_ids": []string{profile},
		"repetitions": 2, "protocol": "single-pass-v1",
	}, token, csrf, "study-1")
	if other.Code != http.StatusConflict {
		t.Fatalf("conflict %d %s", other.Code, other.Body.String())
	}
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &payload)
	trials, err := svc.Store.ListTrials(context.Background(), payload.Data.ID)
	if err != nil || len(trials) != 1 {
		t.Fatal(err)
	}
	rec := call(rt, http.MethodPost, "/api/v1/trials/"+trials[0].ID+"/cancel", map[string]any{}, token, csrf)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "取消意图") {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"cleanup_state":"clean"`) {
		t.Fatalf("cancel looked finished: %s", rec.Body.String())
	}
}

func TestReviewCannotFlipVerdictAndUsageStaysNull(t *testing.T) {
	_, rt, svc := newAPI(t)
	token, csrf := setupLogin(t, rt)
	ctx := context.Background()
	task, profile := publishPair(t, svc)
	exp, err := svc.SubmitExperiment(ctx, app.ExperimentRequest{
		Actor: "local", IdempotencyKey: "rev", Mode: domain.ModeAgentProfile,
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
	attempt, err := svc.Store.CreateAttempt(ctx, trials[0].ID, account, `{}`, "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.FinishAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecAborted), string(domain.VerdictFail), string(domain.CleanupClean), "fixture"); err != nil {
		t.Fatal(err)
	}
	rec := call(rt, http.MethodPost, "/api/v1/attempts/"+attempt.ID+"/reviews", map[string]string{"kind": "note", "body": "我觉得应该通过"}, token, csrf)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"verdict":"fail"`) || !strings.Contains(rec.Body.String(), `"review_changes_verdict":false`) {
		t.Fatalf("review %d %s", rec.Code, rec.Body.String())
	}
	usage := call(rt, http.MethodGet, "/api/v1/attempts/"+attempt.ID+"/usage", nil, token, csrf)
	if !strings.Contains(usage.Body.String(), `"cost_microusd":null`) {
		t.Fatalf("usage %s", usage.Body.String())
	}
}

func TestHarborWebhookExportAndSSEGap(t *testing.T) {
	api, rt, svc := newAPI(t)
	token, csrf := setupLogin(t, rt)
	raw := []byte(`{"schema_version":"harbor.subset/v1","name":"分页","instruction":"修游标","environment":{"image":"x"}}`)
	rec := call(rt, http.MethodPost, "/api/v1/imports/harbor", json.RawMessage(raw), token, csrf)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "environment") || strings.Contains(rec.Body.String(), `"auto_merge":true`) {
		t.Fatalf("harbor %d %s", rec.Code, rec.Body.String())
	}
	secret, err := api.webhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	mac := Sign(secret, raw)
	bad := webhook(rt, raw, "00", "dedupe-1")
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad sig %d", bad.Code)
	}
	good := webhook(rt, raw, mac, "dedupe-1")
	if good.Code != http.StatusOK || !strings.Contains(good.Body.String(), `"duplicate":false`) {
		t.Fatalf("webhook %d %s", good.Code, good.Body.String())
	}
	again := webhook(rt, raw, mac, "dedupe-1")
	if !strings.Contains(again.Body.String(), `"duplicate":true`) {
		t.Fatalf("dup %s", again.Body.String())
	}
	ctx := context.Background()
	task, profile := publishPair(t, svc)
	exp, err := svc.SubmitExperiment(ctx, app.ExperimentRequest{Actor: "local", IdempotencyKey: "ex", Mode: domain.ModeAgentProfile, TaskVersionIDs: []string{task}, ProfileVersionIDs: []string{profile}, Repetitions: 1, Protocol: "repair-v1"})
	if err != nil {
		t.Fatal(err)
	}
	exported := call(rt, http.MethodPost, "/api/v1/experiments/"+exp.ID+"/export", map[string]any{}, token, csrf)
	if exported.Code != http.StatusAccepted {
		t.Fatalf("export %d %s", exported.Code, exported.Body.String())
	}
	var box struct {
		Data struct {
			ArtifactID string `json:"artifact_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(exported.Body.Bytes(), &box)
	file := call(rt, http.MethodGet, "/api/v1/artifacts/"+box.Data.ArtifactID+"/download", nil, token, csrf)
	text := file.Body.String()
	if strings.Contains(text, "credential_ref") || strings.Contains(text, "solutions/") || !strings.Contains(text, `"included_reference_solution": false`) {
		t.Fatalf("export body %s", text)
	}
	account, _ := svc.Store.ProfileAccount(ctx, profile)
	trials, _ := svc.Store.ListTrials(ctx, exp.ID)
	attempt, err := svc.Store.CreateAttempt(ctx, trials[0].ID, account, `{}`, "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(svc.DataDir, "attempts", attempt.ID)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "events.ndjson"), []byte("{\"sequence\":3,\"type\":\"Ready\"}\n"), 0o644)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/attempts/"+attempt.ID+"/events", nil)
	req.Host = "127.0.0.1"
	req.Header.Set("Last-Event-ID", attempt.ID+":1")
	req.AddCookie(&http.Cookie{Name: "agentlab_session", Value: token})
	rec = httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "event: gap") {
		t.Fatalf("sse %s", rec.Body.String())
	}
}

func newAPI(t *testing.T) (*API, *bootstrap.Runtime, *app.Service) {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.Open(filepath.Join(dir, "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := &app.Service{Store: store, DataDir: dir, ExecPath: os.Args[0], GlobalLimit: 1}
	api := New(svc)
	rt, err := bootstrap.Build(bootstrap.Config{Addr: "127.0.0.1:0", Register: api.Register, Wrap: api.Wrap})
	if err != nil {
		t.Fatal(err)
	}
	return api, rt, svc
}

func setupLogin(t *testing.T, rt *bootstrap.Runtime) (string, string) {
	t.Helper()
	rec := call(rt, http.MethodPost, "/api/v1/setup", map[string]string{"username": "malong", "password": "local-pass"}, "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup %d %s", rec.Code, rec.Body.String())
	}
	rec = call(rt, http.MethodPost, "/api/v1/login", map[string]string{"username": "malong", "password": "local-pass"}, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}
	cookie := rec.Header().Get("Set-Cookie")
	token := ""
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "agentlab_session=") {
			token = strings.TrimPrefix(part, "agentlab_session=")
		}
	}
	var payload struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || token == "" || payload.Data.CSRF == "" {
		t.Fatalf("cookie %q body %s", cookie, rec.Body.String())
	}
	return token, payload.Data.CSRF
}

func publishPair(t *testing.T, svc *app.Service) (string, string) {
	t.Helper()
	ctx := context.Background()
	project, err := svc.Store.CreateProject(ctx, "演示", `{"kind":"local"}`)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(app.TaskSnapshot{Name: "笔记", Prompt: "写下一行说明"})
	task, err := svc.Store.CreateTask(ctx, project.ID, "笔记", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	tv, err := svc.PublishTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	account, err := svc.Store.CreateAccount(ctx, "manual_import", "ref:fixture", 1)
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := json.Marshal(app.ProfileSnapshot{Adapter: "fixture", Model: "fixture-local", FakeMode: "empty", Executor: "native-trusted", Network: "unrestricted", DisplayName: "假 CLI"})
	profile, err := svc.Store.CreateProfile(ctx, "假 CLI", account.ID, string(draft))
	if err != nil {
		t.Fatal(err)
	}
	pv, err := svc.PublishProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	return tv.ID, pv.ID
}

func call(rt *bootstrap.Runtime, method, path string, body any, token, csrf string) *httptest.ResponseRecorder {
	return callKey(rt, path, body, token, csrf, "", method)
}

func callKey(rt *bootstrap.Runtime, path string, body any, token, csrf, key string, method ...string) *httptest.ResponseRecorder {
	m := http.MethodPost
	if len(method) > 0 && method[0] != "" {
		m = method[0]
	}
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(m, path, reader)
	req.Host = "127.0.0.1"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "agentlab_session", Value: token})
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	return rec
}

func experimentID(body []byte) string {
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &payload)
	return payload.Data.ID
}

func TestSPAFallbackDoesNotEscapeRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("lab-shell"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("js"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/", "/experiments/new", "/compare"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		serveSPA(rec, req, root)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "lab-shell") {
			t.Fatalf("%s status %d body %s", p, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	serveSPA(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil), root)
	if rec.Body.String() != "js" {
		t.Fatalf("asset %s", rec.Body.String())
	}
	outside := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = "/../../secret.txt"
	rec = httptest.NewRecorder()
	serveSPA(rec, req, root)
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("escape status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest && !strings.Contains(rec.Body.String(), "lab-shell") {
		t.Fatalf("escape status %d body %s", rec.Code, rec.Body.String())
	}
}

func webhook(rt *bootstrap.Runtime, body []byte, sig, dedupe string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))
	req.Host = "127.0.0.1"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agentlab-Signature", sig)
	req.Header.Set("X-Agentlab-Dedupe", dedupe)
	rec := httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	return rec
}
