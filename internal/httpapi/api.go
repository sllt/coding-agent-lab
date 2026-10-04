// Package httpapi is the /api/v1 contract. Pi only carries the HTTP call.
package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sllt/pi/pkg/pi"
	"github.com/sllt/pi/pkg/pi/http/response"

	"github.com/sllt/agentlab/internal/app"
	"github.com/sllt/agentlab/internal/bootstrap"
	"github.com/sllt/agentlab/internal/compare"
	"github.com/sllt/agentlab/internal/doctor"
	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/harbor"
	"github.com/sllt/agentlab/internal/store/sqlite"
)

type userKey struct{}
type idemKey struct{}
type lastKey struct{}
type tokenKey struct{}
type csrfKey struct{}
type webhookBodyKey struct{}
type webhookSigKey struct{}
type webhookDedupeKey struct{}

type API struct {
	svc *app.Service
}

func New(svc *app.Service) *API { return &API{svc: svc} }

func (a *API) Register(appPi *pi.App) {
	appPi.POST("/api/v1/setup", a.setup)
	appPi.POST("/api/v1/login", a.login)
	appPi.POST("/api/v1/logout", a.logout)
	appPi.GET("/api/v1/session", a.session)
	appPi.GET("/api/v1/overview", a.overview)
	appPi.POST("/api/v1/projects", a.createProject)
	appPi.GET("/api/v1/projects", a.listProjects)
	appPi.POST("/api/v1/projects/{id}/tasks", a.createTask)
	appPi.GET("/api/v1/projects/{id}/tasks", a.listTasks)
	appPi.PATCH("/api/v1/tasks/{id}", a.patchTask)
	appPi.POST("/api/v1/tasks/{id}/publish", a.publishTask)
	appPi.GET("/api/v1/tasks/{id}/versions", a.taskVersions)
	appPi.POST("/api/v1/accounts", a.createAccount)
	appPi.GET("/api/v1/accounts", a.listAccounts)
	appPi.POST("/api/v1/profiles", a.createProfile)
	appPi.GET("/api/v1/profiles", a.listProfiles)
	appPi.POST("/api/v1/profiles/{id}/doctor", a.doctor)
	appPi.POST("/api/v1/profiles/{id}/publish", a.publishProfile)
	appPi.GET("/api/v1/profiles/{id}/versions", a.profileVersions)
	appPi.POST("/api/v1/environments", a.createEnvironment)
	appPi.GET("/api/v1/environments", a.listEnvironments)
	appPi.POST("/api/v1/experiments/preview", a.previewExperiment)
	appPi.POST("/api/v1/experiments", a.createExperiment)
	appPi.GET("/api/v1/experiments", a.listExperiments)
	appPi.GET("/api/v1/experiments/{id}", a.getExperiment)
	appPi.POST("/api/v1/experiments/{id}/export", a.exportExperiment)
	appPi.POST("/api/v1/trials/{id}/cancel", a.cancelTrial)
	appPi.POST("/api/v1/trials/{id}/attempts", a.retryTrial)
	appPi.GET("/api/v1/trials/{id}", a.getTrial)
	appPi.GET("/api/v1/attempts/{id}/events", a.events)
	appPi.GET("/api/v1/attempts/{id}/checks", a.checks)
	appPi.GET("/api/v1/attempts/{id}/artifacts", a.artifacts)
	appPi.GET("/api/v1/attempts/{id}/usage", a.usage)
	appPi.POST("/api/v1/attempts/{id}/reviews", a.addReview)
	appPi.GET("/api/v1/attempts/{id}/reviews", a.listReviews)
	appPi.GET("/api/v1/artifacts/{id}/download", a.download)
	appPi.GET("/api/v1/comparisons", a.comparisons)
	appPi.GET("/api/v1/settings", a.settings)
	appPi.POST("/api/v1/maintenance/backup", a.backup)
	appPi.POST("/api/v1/imports/harbor", a.importHarbor)
	appPi.POST("/api/v1/webhooks", a.webhook)
}

func (a *API) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			a.serveStatic(w, r)
			return
		}
		if r.URL.Path == "/api/v1/webhooks" && r.Method == http.MethodPost {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				writeHTTP(w, r, http.StatusBadRequest, "bad_request", "请求无法处理")
				return
			}
			ctx := context.WithValue(r.Context(), webhookBodyKey{}, body)
			ctx = context.WithValue(ctx, webhookSigKey{}, r.Header.Get("X-Agentlab-Signature"))
			ctx = context.WithValue(ctx, webhookDedupeKey{}, r.Header.Get("X-Agentlab-Dedupe"))
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if public(r) {
			next.ServeHTTP(w, r)
			return
		}
		token := sessionToken(r)
		if token == "" {
			writeHTTP(w, r, http.StatusUnauthorized, "unauthenticated", "需要登录")
			return
		}
		_, csrf, err := a.svc.Store.Session(r.Context(), app.HashToken(token))
		if err != nil {
			writeHTTP(w, r, http.StatusUnauthorized, "unauthenticated", "需要登录")
			return
		}
		if mutating(r.Method) && r.Header.Get("X-CSRF-Token") != csrf {
			writeHTTP(w, r, http.StatusForbidden, "csrf", "缺少或错误的 CSRF 令牌")
			return
		}
		ctx := context.WithValue(r.Context(), userKey{}, "local")
		ctx = context.WithValue(ctx, tokenKey{}, token)
		ctx = context.WithValue(ctx, csrfKey{}, csrf)
		ctx = context.WithValue(ctx, idemKey{}, r.Header.Get("Idempotency-Key"))
		ctx = context.WithValue(ctx, lastKey{}, r.Header.Get("Last-Event-ID"))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func public(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/setup", "/api/v1/login", "/api/v1/health", "/api/v1/health/stream":
		return true
	default:
		return false
	}
}

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie("agentlab_session"); err == nil && c.Value != "" {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func writeHTTP(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      map[string]string{"code": code, "message": message},
		"request_id": bootstrap.RequestID(r.Context()),
	})
}

func ok(c *pi.Context, status int, data any) (any, error) {
	return response.Raw{StatusCode: status, Data: map[string]any{"data": data, "request_id": bootstrap.RequestID(c)}}, nil
}

func fail(c *pi.Context, status int, code, message string) (any, error) {
	return response.Raw{StatusCode: status, Data: map[string]any{
		"error":      map[string]string{"code": code, "message": message},
		"request_id": bootstrap.RequestID(c),
	}}, nil
}

func (a *API) fromErr(c *pi.Context, err error) (any, error) {
	switch {
	case err == nil:
		return ok(c, http.StatusOK, map[string]any{})
	case errors.Is(err, sqlite.ErrNotFound):
		return fail(c, http.StatusNotFound, "not_found", "资源不存在")
	case errors.Is(err, sqlite.ErrIdempotent), errors.Is(err, sqlite.ErrConflict):
		return fail(c, http.StatusConflict, "conflict", "状态、版本或幂等键冲突")
	case errors.Is(err, sqlite.ErrCapacity):
		return fail(c, http.StatusTooManyRequests, "capacity", "本机容量已满")
	case errors.Is(err, app.ErrUnexecutable):
		return fail(c, 422, "unexecutable", "配置不能执行")
	case errors.Is(err, app.ErrMaintenance):
		return fail(c, http.StatusServiceUnavailable, "maintenance", "维护模式中，不接受新实验")
	default:
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
}

func (a *API) setup(c *pi.Context) (any, error) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	if err := a.svc.Setup(c, body.Username, body.Password); err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusCreated, map[string]any{"username": body.Username})
}

func (a *API) login(c *pi.Context) (any, error) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	token, csrf, err := a.svc.Login(c, body.Username, body.Password)
	if err != nil {
		return fail(c, http.StatusUnauthorized, "unauthenticated", "用户名或密码不正确")
	}
	payload := map[string]any{"data": map[string]string{"username": body.Username, "csrf_token": csrf}, "request_id": bootstrap.RequestID(c)}
	return response.Result{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Set-Cookie": "agentlab_session=" + token + "; HttpOnly; SameSite=Strict; Path=/"},
		Data:       response.Raw{StatusCode: http.StatusOK, Data: payload},
	}, nil
}

func (a *API) logout(c *pi.Context) (any, error) {
	token, _ := c.Value(tokenKey{}).(string)
	if token == "" {
		return fail(c, http.StatusUnauthorized, "unauthenticated", "需要登录")
	}
	_ = a.svc.Logout(c, token)
	return response.Result{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Set-Cookie": "agentlab_session=; HttpOnly; SameSite=Strict; Path=/; Max-Age=0"},
		Data:       response.Raw{StatusCode: http.StatusOK, Data: map[string]any{"data": map[string]bool{"ok": true}, "request_id": bootstrap.RequestID(c)}},
	}, nil
}

func (a *API) session(c *pi.Context) (any, error) {
	csrf, _ := c.Value(csrfKey{}).(string)
	return ok(c, http.StatusOK, map[string]any{"authenticated": true, "csrf_token": csrf})
}

func (a *API) overview(c *pi.Context) (any, error) {
	trials, err := a.svc.Store.ListTrials(c, "")
	if err != nil {
		return a.fromErr(c, err)
	}
	accounts, err := a.svc.Store.ListAccounts(c)
	if err != nil {
		return a.fromErr(c, err)
	}
	var running, queued, blocked int
	for _, trial := range trials {
		switch trial.ExecutionState {
		case string(domain.ExecRunning), string(domain.ExecPreparing), string(domain.ExecCollecting), string(domain.ExecVerifying), string(domain.ExecCancelling):
			running++
		case string(domain.ExecQueued):
			queued++
		}
	}
	for _, account := range accounts {
		if account.BlockedReason != "" {
			blocked++
		}
	}
	return ok(c, http.StatusOK, map[string]any{
		"running": running, "queued": queued, "blocked_accounts": blocked,
		"recent": trials, "note": "没有真实运行时这里保持空白，不会填入演示分数。",
	})
}

func (a *API) createProject(c *pi.Context) (any, error) {
	var body struct {
		Name       string          `json:"name"`
		SourceSpec json.RawMessage `json:"source_spec"`
	}
	if err := c.Bind(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	spec := string(body.SourceSpec)
	if spec == "" || spec == "null" {
		spec = `{"kind":"local"}`
	}
	project, err := a.svc.Store.CreateProject(c, body.Name, spec)
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusCreated, project)
}

func (a *API) listProjects(c *pi.Context) (any, error) {
	limit, _ := strconv.Atoi(c.Param("limit"))
	items, next, err := a.svc.Store.ListProjects(c, c.Param("cursor"), limit)
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

func (a *API) createTask(c *pi.Context) (any, error) {
	var body app.TaskSnapshot
	if err := c.Bind(&body); err != nil || strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Prompt) == "" {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	raw, _ := json.Marshal(body)
	task, err := a.svc.Store.CreateTask(c, c.PathParam("id"), body.Name, string(raw))
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusCreated, task)
}

func (a *API) listTasks(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ListTasks(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	if items == nil {
		items = []sqlite.Task{}
	}
	return ok(c, http.StatusOK, map[string]any{"items": items})
}

func (a *API) patchTask(c *pi.Context) (any, error) {
	var body struct {
		Name       string `json:"name"`
		Draft      string `json:"draft_json"`
		RowVersion int64  `json:"row_version"`
	}
	if err := c.Bind(&body); err != nil {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	task, err := a.svc.Store.UpdateTaskDraft(c, c.PathParam("id"), body.RowVersion, body.Name, body.Draft)
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusOK, task)
}

func (a *API) publishTask(c *pi.Context) (any, error) {
	version, err := a.svc.PublishTask(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusAccepted, version)
}

func (a *API) taskVersions(c *pi.Context) (any, error) {
	items, err := a.svc.Store.TaskVersions(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	if items == nil {
		items = []sqlite.TaskVersion{}
	}
	return ok(c, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createAccount(c *pi.Context) (any, error) {
	var body struct {
		AuthKind      string `json:"auth_kind"`
		CredentialRef string `json:"credential_ref"`
		Concurrency   int    `json:"concurrency"`
	}
	if err := c.Bind(&body); err != nil || body.CredentialRef == "" {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	account, err := a.svc.Store.CreateAccount(c, body.AuthKind, body.CredentialRef, body.Concurrency)
	if err != nil {
		return a.fromErr(c, err)
	}
	account.CredentialRef = "已保存引用"
	return ok(c, http.StatusCreated, account)
}

func (a *API) listAccounts(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ListAccounts(c)
	if err != nil {
		return a.fromErr(c, err)
	}
	for i := range items {
		if items[i].CredentialRef != "" {
			items[i].CredentialRef = "已保存引用"
		}
	}
	return ok(c, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createProfile(c *pi.Context) (any, error) {
	var body struct {
		Name      string              `json:"name"`
		Snap      app.ProfileSnapshot `json:"profile"`
		AccountID string              `json:"account_id"`
	}
	if err := c.Bind(&body); err != nil || body.Name == "" || body.AccountID == "" {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	raw, _ := json.Marshal(body.Snap)
	profile, err := a.svc.Store.CreateProfile(c, body.Name, body.AccountID, string(raw))
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusCreated, profile)
}

func (a *API) listProfiles(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ListProfiles(c)
	if err != nil {
		return a.fromErr(c, err)
	}
	if items == nil {
		items = []sqlite.Profile{}
	}
	return ok(c, http.StatusOK, map[string]any{"items": items})
}

func (a *API) doctor(c *pi.Context) (any, error) {
	var body struct {
		AllowModelCall bool `json:"allow_model_call"`
	}
	_ = c.Bind(&body)
	profile, err := a.svc.Store.GetProfile(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	var snap app.ProfileSnapshot
	_ = json.Unmarshal([]byte(profile.DraftJSON), &snap)
	report := doctor.Static(c, snap.Adapter, "", snap.Model, body.AllowModelCall)
	return ok(c, http.StatusOK, report)
}

func (a *API) publishProfile(c *pi.Context) (any, error) {
	version, err := a.svc.PublishProfile(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusAccepted, version)
}

func (a *API) profileVersions(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ProfileVersions(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	if items == nil {
		items = []sqlite.ProfileVersion{}
	}
	return ok(c, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createEnvironment(c *pi.Context) (any, error) {
	var body struct {
		Name     string `json:"name"`
		Executor string `json:"executor"`
		Network  string `json:"network"`
		Image    string `json:"image"`
	}
	if err := c.Bind(&body); err != nil || body.Name == "" {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	if body.Network == "" || body.Network == "restricted" {
		body.Network = "unrestricted"
	}
	raw, _ := json.Marshal(body)
	digest, err := domain.Digest(body)
	if err != nil {
		return a.fromErr(c, err)
	}
	env, err := a.svc.Store.SaveEnvironment(c, body.Name, string(raw), digest)
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusCreated, env)
}

func (a *API) listEnvironments(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ListEnvironments(c)
	if err != nil {
		return a.fromErr(c, err)
	}
	if items == nil {
		items = []sqlite.Environment{}
	}
	return ok(c, http.StatusOK, map[string]any{"items": items, "note": "network=restricted 尚未强制执行，保存时会记为 unrestricted。"})
}

func (a *API) previewExperiment(c *pi.Context) (any, error) {
	var body experimentBody
	if err := c.Bind(&body); err != nil {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	n := domain.TrialCount(len(body.TaskVersionIDs), len(body.ProfileVersionIDs), body.Repetitions)
	return ok(c, http.StatusOK, map[string]any{
		"trial_count": n,
		"formula":     formula(len(body.TaskVersionIDs), len(body.ProfileVersionIDs), body.Repetitions),
		"note":        "这是计划规模，不是预计 API 请求数，也不是费用估算。预览不会启动运行。",
	})
}

func (a *API) createExperiment(c *pi.Context) (any, error) {
	var body experimentBody
	if err := c.Bind(&body); err != nil {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	key, _ := c.Value(idemKey{}).(string)
	if key == "" {
		return fail(c, http.StatusBadRequest, "bad_request", "需要 Idempotency-Key")
	}
	exp, err := a.svc.SubmitExperiment(c, app.ExperimentRequest{
		Actor: "local", IdempotencyKey: key, Mode: body.Mode, TaskVersionIDs: body.TaskVersionIDs,
		ProfileVersionIDs: body.ProfileVersionIDs, Repetitions: body.Repetitions, Protocol: body.Protocol,
	})
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusAccepted, map[string]any{"id": exp.ID, "state": exp.State, "trial_count": exp.TrialCount, "protocol": exp.ProtocolJSON})
}

func (a *API) listExperiments(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ListExperiments(c)
	if err != nil {
		return a.fromErr(c, err)
	}
	if items == nil {
		items = []sqlite.Experiment{}
	}
	for i := range items {
		items[i] = a.projectState(c, items[i])
	}
	return ok(c, http.StatusOK, map[string]any{"items": items})
}

func (a *API) getExperiment(c *pi.Context) (any, error) {
	exp, err := a.svc.Store.GetExperiment(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	trials, err := a.svc.Store.ListTrials(c, exp.ID)
	if err != nil {
		return a.fromErr(c, err)
	}
	exp = a.projectState(c, exp)
	return ok(c, http.StatusOK, map[string]any{"experiment": exp, "trials": a.withCleanup(c, trials), "summary": summarize(trials)})
}

func (a *API) projectState(c *pi.Context, exp sqlite.Experiment) sqlite.Experiment {
	trials, err := a.svc.Store.ListTrials(c, exp.ID)
	if err != nil || len(trials) == 0 {
		return exp
	}
	states := make([]domain.ExecutionState, len(trials))
	for i, trial := range trials {
		states[i] = domain.ExecutionState(trial.ExecutionState)
	}
	exp.State = string(domain.RollupExecution(states))
	return exp
}

type trialView struct {
	sqlite.Trial
	CleanupState string `json:"cleanup_state"`
}

func (a *API) withCleanup(c *pi.Context, trials []sqlite.Trial) []trialView {
	out := make([]trialView, 0, len(trials))
	for _, trial := range trials {
		view := trialView{Trial: trial, CleanupState: "not_started"}
		if trial.CurrentAttemptID != "" {
			if attempt, err := a.svc.Store.GetAttempt(c, trial.CurrentAttemptID); err == nil && attempt.CleanupState != "" {
				view.CleanupState = attempt.CleanupState
			}
		}
		out = append(out, view)
	}
	return out
}

func (a *API) cancelTrial(c *pi.Context) (any, error) {
	trial, err := a.svc.Store.RequestCancel(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	cleanup := "not_started"
	if trial.CurrentAttemptID != "" {
		attempt, err := a.svc.Store.GetAttempt(c, trial.CurrentAttemptID)
		if err == nil {
			cleanup = attempt.CleanupState
		}
	}
	return ok(c, http.StatusAccepted, map[string]any{
		"trial": trial, "cleanup_state": cleanup,
		"note": "已记录取消意图。这不表示进程已经停止，也不表示容量已经释放。",
	})
}

func (a *API) retryTrial(c *pi.Context) (any, error) {
	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.Bind(&body); err != nil || strings.TrimSpace(body.Reason) == "" {
		return fail(c, 422, "unexecutable", "人工重试必须写明原因")
	}
	trial, err := a.svc.Store.GetTrial(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	if err := a.svc.Store.PrepareRetry(c, trial.ID); err != nil {
		return a.fromErr(c, err)
	}
	account, err := a.svc.Store.ProfileAccount(c, trial.ProfileVersionID)
	if err != nil {
		return a.fromErr(c, err)
	}
	limit := a.svc.GlobalLimit
	if limit <= 0 {
		limit = 1
	}
	attempt, err := a.svc.Store.CreateAttempt(c, trial.ID, account, `{"executor":"native-trusted"}`, body.Reason, limit, 1)
	if err != nil {
		return a.fromErr(c, err)
	}
	attempt.Token = ""
	return ok(c, http.StatusAccepted, map[string]any{"attempt": attempt, "note": "新 Attempt 不会改写上一次的结论。"})
}

func (a *API) getTrial(c *pi.Context) (any, error) {
	trial, err := a.svc.Store.GetTrial(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	attempts, err := a.svc.Store.ListAttempts(c, trial.ID)
	if err != nil {
		return a.fromErr(c, err)
	}
	for i := range attempts {
		attempts[i].Token = ""
		attempts[i].CapabilityHash = ""
	}
	return ok(c, http.StatusOK, map[string]any{"trial": trial, "attempts": attempts})
}

func (a *API) checks(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ListChecks(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	if items == nil {
		items = []string{}
	}
	return ok(c, http.StatusOK, map[string]any{"items": items, "note": "这些检查来自独立验证器，不是 Agent 自己的输出。"})
}

func (a *API) artifacts(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ListArtifacts(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		status := item.Status
		if !a.underData(item.StorageKey) {
			status = "missing"
		} else if _, err := os.Stat(item.StorageKey); err != nil {
			status = "missing"
		}
		out = append(out, map[string]any{"id": item.ID, "kind": item.Kind, "digest": item.Digest, "bytes": item.Bytes, "status": status})
	}
	return ok(c, http.StatusOK, map[string]any{"items": out})
}

func (a *API) usage(c *pi.Context) (any, error) {
	records, err := a.svc.Store.ListUsage(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	folded, err := domain.FoldUsage(records)
	if err != nil {
		return fail(c, 422, "unexecutable", "用量记录不能混合累计值和增量")
	}
	if folded.Confidence == "" {
		folded.Confidence = "unknown"
	}
	return ok(c, http.StatusOK, map[string]any{
		"input_tokens": folded.InputTokens, "output_tokens": folded.OutputTokens,
		"cached_input_tokens": folded.CachedInputTokens, "cost_microusd": folded.CostMicroUSD,
		"currency": folded.Currency, "confidence": folded.Confidence, "billing_path": folded.BillingPath,
		"note": "空值表示未知，不是零。",
	})
}

func (a *API) addReview(c *pi.Context) (any, error) {
	var body struct {
		Kind string `json:"kind"`
		Body string `json:"body"`
	}
	if err := c.Bind(&body); err != nil || body.Body == "" {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	attempt, err := a.svc.Store.GetAttempt(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	before := attempt.Verdict
	raw, _ := json.Marshal(body)
	if err := a.svc.Store.AddReview(c, attempt.ID, body.Kind, string(raw)); err != nil {
		return a.fromErr(c, err)
	}
	after, err := a.svc.Store.GetAttempt(c, attempt.ID)
	if err != nil {
		return a.fromErr(c, err)
	}
	if after.Verdict != before {
		return fail(c, http.StatusConflict, "conflict", "人工意见不能改写验收结论")
	}
	return ok(c, http.StatusCreated, map[string]any{"verdict": after.Verdict, "review_changes_verdict": false})
}

func (a *API) listReviews(c *pi.Context) (any, error) {
	items, err := a.svc.Store.ListReviews(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	if items == nil {
		items = []string{}
	}
	return ok(c, http.StatusOK, map[string]any{"items": items})
}

func (a *API) download(c *pi.Context) (any, error) {
	item, err := a.svc.Store.GetArtifact(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	if !a.underData(item.StorageKey) {
		return fail(c, http.StatusNotFound, "not_found", "制品不存在")
	}
	body, err := os.ReadFile(item.StorageKey)
	if err != nil {
		return fail(c, http.StatusNotFound, "not_found", "制品文件缺失")
	}
	return response.Stream{StatusCode: http.StatusOK, ContentType: "application/octet-stream", Run: func(_ context.Context, w io.Writer) error {
		_, err := w.Write(body)
		return err
	}}, nil
}

func (a *API) events(c *pi.Context) (any, error) {
	id := c.PathParam("id")
	if _, err := a.svc.Store.GetAttempt(c, id); err != nil {
		return a.fromErr(c, err)
	}
	last := parseLast(c.Value(lastKey{}))
	path := filepath.Join(a.svc.DataDir, "attempts", id, "events.ndjson")
	once := c.Param("once") == "1"
	return response.Stream{
		StatusCode:  http.StatusOK,
		ContentType: "text/event-stream",
		Run: func(ctx context.Context, w io.Writer) error {
			return writeEvents(ctx, w, id, path, last, once)
		},
	}, nil
}

func (a *API) comparisons(c *pi.Context) (any, error) {
	expID := c.Param("experiment_id")
	trials, err := a.svc.Store.ListTrials(c, expID)
	if err != nil {
		return a.fromErr(c, err)
	}
	var rows []compare.Input
	tasks := map[string]struct{}{}
	for _, trial := range trials {
		exp, err := a.svc.Store.GetExperiment(c, trial.ExperimentID)
		if err != nil {
			return a.fromErr(c, err)
		}
		tv, err := a.svc.Store.GetTaskVersion(c, trial.TaskVersionID)
		if err != nil {
			return a.fromErr(c, err)
		}
		pv, err := a.svc.Store.GetProfileVersion(c, trial.ProfileVersionID)
		if err != nil {
			return a.fromErr(c, err)
		}
		var snap app.ProfileSnapshot
		_ = json.Unmarshal([]byte(pv.SnapshotJSON), &snap)
		if snap.Executor == "" {
			snap.Executor = "native-trusted"
		}
		if snap.Network == "" {
			snap.Network = "unrestricted"
		}
		tasks[trial.TaskVersionID] = struct{}{}
		rows = append(rows, compare.Input{
			Mode: exp.Mode, Protocol: exp.ProtocolJSON, TaskID: trial.TaskVersionID, TaskDigest: tv.Digest,
			ProfileID: trial.ProfileVersionID, ProfileName: snap.DisplayName, ProfileDig: pv.Digest,
			Executor: snap.Executor, Network: snap.Network, Verdict: trial.Verdict, Terminal: domain.ExecutionState(trial.ExecutionState).Terminal(),
		})
	}
	boards, err := compare.Boards(rows, len(tasks))
	if err != nil {
		return a.fromErr(c, err)
	}
	if boards == nil {
		boards = []compare.Board{}
	}
	return ok(c, http.StatusOK, map[string]any{"boards": boards})
}

func (a *API) settings(c *pi.Context) (any, error) {
	_, err := os.Stat(filepath.Join(a.svc.DataDir, "webhook.secret"))
	return ok(c, http.StatusOK, map[string]any{
		"executor_default":   "native-trusted",
		"executor_note":      "本机进程执行，不提供容器级隔离。Docker 参数可以生成，但本机没有把它当成已验证的沙箱。",
		"network_restricted": "未实现",
		"maintenance":        a.svc.Maintenance(),
		"webhook_configured": err == nil,
		"credential_display": "只显示引用，不回传密钥",
	})
}

func (a *API) backup(c *pi.Context) (any, error) {
	a.svc.SetMaintenance(true)
	defer a.svc.SetMaintenance(false)
	if err := os.MkdirAll(a.svc.DataDir, 0o755); err != nil {
		return a.fromErr(c, err)
	}
	dest := filepath.Join(a.svc.DataDir, "backups", time.Now().UTC().Format("20060102T150405Z")+".db")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return a.fromErr(c, err)
	}
	if err := a.svc.Store.Backup(c, dest); err != nil {
		return fail(c, http.StatusServiceUnavailable, "backup_failed", "无法生成一致性备份")
	}
	return ok(c, http.StatusOK, map[string]any{"path": dest, "note": "备份在维护模式下生成。活跃任务不会被假装成已清理。"})
}

func (a *API) importHarbor(c *pi.Context) (any, error) {
	var raw json.RawMessage
	if err := c.Bind(&raw); err != nil {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	parsed, err := harbor.Import(raw)
	if err != nil {
		return fail(c, 422, "unexecutable", "Harbor 子集无法导入")
	}
	return ok(c, http.StatusOK, parsed)
}

func (a *API) webhook(c *pi.Context) (any, error) {
	secret, err := a.webhookSecret()
	if err != nil {
		return fail(c, http.StatusServiceUnavailable, "unavailable", "webhook 密钥不可用")
	}
	body, _ := c.Value(webhookBodyKey{}).([]byte)
	sig, _ := c.Value(webhookSigKey{}).(string)
	dedupe, _ := c.Value(webhookDedupeKey{}).(string)
	if dedupe == "" || !VerifyWebhook(secret, body, sig) {
		return fail(c, http.StatusUnauthorized, "unauthenticated", "webhook 签名不正确")
	}
	sum := sha256.Sum256(body)
	dup, err := a.svc.Store.AcceptWebhook(c, dedupe, hex.EncodeToString(sum[:]))
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusOK, map[string]any{"accepted": true, "duplicate": dup, "auto_merge": false})
}

func (a *API) exportExperiment(c *pi.Context) (any, error) {
	exp, err := a.svc.Store.GetExperiment(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	trials, err := a.svc.Store.ListTrials(c, exp.ID)
	if err != nil {
		return a.fromErr(c, err)
	}
	type row struct {
		TrialID string   `json:"trial_id"`
		State   string   `json:"execution_state"`
		Verdict string   `json:"verdict"`
		Checks  []string `json:"checks"`
	}
	var rows []row
	for _, trial := range trials {
		item := row{TrialID: trial.ID, State: trial.ExecutionState, Verdict: trial.Verdict}
		if trial.CurrentAttemptID != "" {
			item.Checks, _ = a.svc.Store.ListChecks(c, trial.CurrentAttemptID)
		}
		rows = append(rows, item)
	}
	payload := map[string]any{
		"experiment_id": exp.ID, "protocol": exp.ProtocolJSON, "trials": rows,
		"included_credentials": false, "included_reference_solution": false,
		"note": "导出不含凭据、隐藏测试和参考解。",
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return a.fromErr(c, err)
	}
	dir := filepath.Join(a.svc.DataDir, "exports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return a.fromErr(c, err)
	}
	path := filepath.Join(dir, exp.ID+".json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return a.fromErr(c, err)
	}
	sum := sha256.Sum256(body)
	art, err := a.svc.Store.InsertArtifact(c, sqlite.Artifact{
		AttemptID: exp.ID, Kind: "export", Digest: hex.EncodeToString(sum[:]), Bytes: int64(len(body)), StorageKey: path, Status: "present",
	})
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusAccepted, map[string]any{"artifact_id": art.ID, "bytes": art.Bytes})
}

func (a *API) underData(path string) bool {
	root, err := filepath.Abs(a.svc.DataDir)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (a *API) serveStatic(w http.ResponseWriter, r *http.Request) {
	root, err := filepath.Abs(filepath.Join("web", "dist"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serveSPA(w, r, root)
}

func serveSPA(w http.ResponseWriter, r *http.Request, root string) {
	if _, err := os.Stat(root); err != nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(pathClean(r.URL.Path), "/")
	target := root
	if rel != "" {
		target = filepath.Join(root, filepath.FromSlash(rel))
	}
	if !insideDir(root, target) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		http.ServeFile(w, r, target)
		return
	}
	http.ServeFile(w, r, filepath.Join(root, "index.html"))
}

func pathClean(p string) string {
	if p == "" {
		return "/"
	}
	clean := path.Clean("/" + p)
	return clean
}

func insideDir(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (a *API) webhookSecret() ([]byte, error) {
	path := filepath.Join(a.svc.DataDir, "webhook.secret")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return b, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(a.svc.DataDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		return nil, err
	}
	return buf, nil
}

type experimentBody struct {
	Mode              string   `json:"mode"`
	TaskVersionIDs    []string `json:"task_version_ids"`
	ProfileVersionIDs []string `json:"profile_version_ids"`
	Repetitions       int      `json:"repetitions"`
	Protocol          string   `json:"protocol"`
}

func formula(tasks, profiles, repetitions int) string {
	return strconv.Itoa(tasks) + " 个任务 × " + strconv.Itoa(profiles) + " 个配置 × " + strconv.Itoa(repetitions) + " 次 = " + strconv.Itoa(domain.TrialCount(tasks, profiles, repetitions)) + " 个 Trial"
}

func summarize(trials []sqlite.Trial) map[string]any {
	var done, pass, fail, inc, unver, incomplete int
	for _, trial := range trials {
		if !domain.ExecutionState(trial.ExecutionState).Terminal() {
			incomplete++
			continue
		}
		done++
		switch domain.Verdict(trial.Verdict) {
		case domain.VerdictPass:
			pass++
		case domain.VerdictFail:
			fail++
		case domain.VerdictInconclusive:
			inc++
		default:
			unver++
		}
	}
	return map[string]any{
		"planned": len(trials), "terminal": done, "incomplete": incomplete,
		"pass": pass, "fail": fail, "inconclusive": inc, "unverified": unver,
		"note": "分母是计划中的 Trial。未完成样本单独列出，不会被算成失败或通过。",
	}
}

func parseLast(v any) int64 {
	s, _ := v.(string)
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func writeEvents(ctx context.Context, w io.Writer, attemptID, path string, last int64, once bool) error {
	body, err := os.ReadFile(path)
	if err != nil && last > 0 {
		_, err = io.WriteString(w, "event: gap\ndata: {\"gap\":true,\"reason\":\"snapshot_required\"}\n\n")
		return err
	}
	lines := splitLines(body)
	var min int64
	for _, line := range lines {
		seq := eventSeq(line)
		if seq == 0 {
			continue
		}
		if min == 0 || seq < min {
			min = seq
		}
	}
	if last > 0 && min > last+1 {
		_, err = io.WriteString(w, "event: gap\ndata: {\"gap\":true,\"reason\":\"snapshot_required\"}\n\n")
		return err
	}
	for _, line := range lines {
		seq := eventSeq(line)
		if seq <= last {
			continue
		}
		if _, err = io.WriteString(w, "id: "+attemptID+":"+strconv.FormatInt(seq, 10)+"\ndata: "+string(line)+"\n\n"); err != nil {
			return err
		}
	}
	if once {
		return nil
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	seen := len(lines)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			body, err = os.ReadFile(path)
			if err != nil {
				continue
			}
			lines = splitLines(body)
			for _, line := range lines[minInt(seen, len(lines)):] {
				seq := eventSeq(line)
				if _, err = io.WriteString(w, "id: "+attemptID+":"+strconv.FormatInt(seq, 10)+"\ndata: "+string(line)+"\n\n"); err != nil {
					return err
				}
			}
			seen = len(lines)
		}
	}
}

func splitLines(body []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range body {
		if b == '\n' {
			if i > start {
				out = append(out, body[start:i])
			}
			start = i + 1
		}
	}
	if start < len(body) {
		out = append(out, body[start:])
	}
	return out
}

func eventSeq(line []byte) int64 {
	var ev struct {
		Sequence int64 `json:"sequence"`
	}
	if err := json.Unmarshal(line, &ev); err != nil {
		return 0
	}
	return ev.Sequence
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// VerifyWebhook checks the signature. It is used by the signed webhook route.
func Sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyWebhook(secret, body []byte, signature string) bool {
	return hmac.Equal([]byte(Sign(secret, body)), []byte(signature))
}
