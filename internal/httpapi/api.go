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
