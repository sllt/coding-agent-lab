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
