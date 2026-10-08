package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/sllt/pi/pkg/pi"

	"github.com/sllt/agentlab/internal/app"
	"github.com/sllt/agentlab/internal/harbor"
	"github.com/sllt/agentlab/internal/store/sqlite"
)

func (a *API) patchSettings(c *pi.Context) (any, error) {
	var body app.LabSettings
	if err := c.Bind(&body); err != nil {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	saved, err := a.svc.SaveSettings(body)
	if err != nil {
		return fail(c, 422, "unexecutable", "并发要在 1 到 8 之间，保留天数和配额不能是负数")
	}
	a.audit(c, "settings", "local")
	return ok(c, http.StatusOK, saved)
}

func (a *API) listAudit(c *pi.Context) (any, error) {
	limit, _ := strconv.Atoi(c.Param("limit"))
	items, err := a.svc.Store.ListAudit(c, limit)
	if err != nil {
		return a.fromErr(c, err)
	}
	return ok(c, http.StatusOK, map[string]any{"items": items})
}

func (a *API) retention(c *pi.Context) (any, error) {
	var body struct {
		Apply bool `json:"apply"`
	}
	_ = c.Bind(&body)
	plan, err := a.svc.PlanRetention(c, body.Apply)
	if err != nil {
		return a.fromErr(c, err)
	}
	rel := func(list []string) []string {
		out := make([]string, 0, len(list))
		for _, p := range list {
			out = append(out, dataRel(a.svc.DataDir, p))
		}
		return out
	}
	plan.LogFiles, plan.ReportFiles, plan.KeptPatches = rel(plan.LogFiles), rel(plan.ReportFiles), rel(plan.KeptPatches)
	if body.Apply {
		a.audit(c, "retention_apply", "local")
	}
	return ok(c, http.StatusOK, plan)
}

func (a *API) cleanQuarantine(c *pi.Context) (any, error) {
	out, err := a.svc.CleanQuarantine(c)
	if err != nil {
		return a.fromErr(c, err)
	}
	for i := range out.Removed {
		out.Removed[i] = dataRel(a.svc.DataDir, out.Removed[i])
	}
	for i := range out.KeptEvidence {
		out.KeptEvidence[i] = dataRel(a.svc.DataDir, out.KeptEvidence[i])
	}
	return ok(c, http.StatusOK, out)
}

func (a *API) upgrade(c *pi.Context) (any, error) {
	return ok(c, http.StatusOK, a.svc.UpgradeReport())
}

func (a *API) exportHarbor(c *pi.Context) (any, error) {
	var body struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
	}
	if err := c.Bind(&body); err != nil || body.Name == "" || body.Prompt == "" {
		return fail(c, 422, "unexecutable", "Harbor 导出需要名称和提示词")
	}
	return ok(c, http.StatusOK, harbor.Export(body.Name, body.Prompt))
}

func (a *API) markFlaky(c *pi.Context) (any, error) {
	var body struct {
		TaskVersionID string `json:"task_version_id"`
		FailRate      string `json:"fail_rate"`
		Note          string `json:"note"`
		Flaky         *bool  `json:"flaky"`
	}
	if err := c.Bind(&body); err != nil || body.TaskVersionID == "" {
		return fail(c, http.StatusBadRequest, "bad_request", "请求无法处理")
	}
	flaky := true
	if body.Flaky != nil {
		flaky = *body.Flaky
	}
	if err := a.svc.Store.SetTaskFlag(c, sqlite.TaskFlag{TaskVersionID: body.TaskVersionID, Flaky: flaky, FailRate: body.FailRate, Note: body.Note}); err != nil {
		return a.fromErr(c, err)
	}
	a.audit(c, "flaky", body.TaskVersionID)
	return ok(c, http.StatusOK, map[string]any{"task_version_id": body.TaskVersionID, "flaky": flaky, "note": "有 flaky 记录的任务会离开默认对比集。"})
}

func (a *API) adoptPatch(c *pi.Context) (any, error) {
	trial, err := a.svc.Store.GetTrial(c, c.PathParam("id"))
	if err != nil {
		return a.fromErr(c, err)
	}
	a.audit(c, "adopt_patch", trial.ID)
	return ok(c, http.StatusAccepted, map[string]any{
		"trial_id": trial.ID, "merged": false,
		"note": "只记录采纳这一个补丁。没有写入源目录，也没有自动合并。",
	})
}

func processRSS() int64 {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				return kb * 1024
			}
		}
	}
	return 0
}

func loopbackNotify(body []byte) string {
	var doc struct {
		NotifyURL string `json:"notify_url"`
	}
	_ = json.Unmarshal(body, &doc)
	if doc.NotifyURL == "" {
		return "local"
	}
	return doc.NotifyURL
}
