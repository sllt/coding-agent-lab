package app

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/sllt/agentlab/internal/domain"
)

type RetentionPlan struct {
	LogFiles     []string `json:"log_files"`
	ReportFiles  []string `json:"report_files"`
	KeptPatches  []string `json:"kept_patches"`
	Irreversible string   `json:"irreversible"`
	Apply        bool     `json:"apply"`
}

// PlanRetention lists files that a typed retention pass would remove.
// Patch artifacts referenced by a report stay. PatchDays of 0 never deletes patches.
func (s *Service) PlanRetention(ctx context.Context, apply bool) (RetentionPlan, error) {
	settings := s.LoadSettings()
	plan := RetentionPlan{
		Irreversible: "删掉的原始日志和过期报告不能从这份策略里恢复。被报告引用的补丁和检查摘要会留下。隔离工作区的清理是另一项操作。",
		Apply:        apply,
	}
	now := time.Now()
	root := filepath.Join(s.DataDir, "attempts")
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return plan, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		for _, name := range []string{"events.ndjson", filepath.Join("work", "agent.log")} {
			path := filepath.Join(dir, name)
			info, err := os.Stat(path)
			if err != nil || settings.Retention.LogDays <= 0 {
				continue
			}
			if now.Sub(info.ModTime()) < time.Duration(settings.Retention.LogDays)*24*time.Hour {
				continue
			}
			plan.LogFiles = append(plan.LogFiles, path)
			if apply {
				_ = os.Remove(path)
			}
		}
		patch := filepath.Join(dir, "patch.json")
		if _, err := os.Stat(patch); err == nil {
			plan.KeptPatches = append(plan.KeptPatches, patch)
		}
	}
	exports := filepath.Join(s.DataDir, "exports")
	files, _ := os.ReadDir(exports)
	for _, entry := range files {
		path := filepath.Join(exports, entry.Name())
		info, err := os.Stat(path)
		if err != nil || settings.Retention.ReportDays <= 0 {
			continue
		}
		if now.Sub(info.ModTime()) < time.Duration(settings.Retention.ReportDays)*24*time.Hour {
			continue
		}
		plan.ReportFiles = append(plan.ReportFiles, path)
		if apply {
			_ = os.Remove(path)
		}
	}
	if plan.LogFiles == nil {
		plan.LogFiles = []string{}
	}
	if plan.ReportFiles == nil {
		plan.ReportFiles = []string{}
	}
	if plan.KeptPatches == nil {
		plan.KeptPatches = []string{}
	}
	if apply {
		_ = s.Store.Audit(ctx, "control", "retention", "local", `{"apply":true}`)
	}
	return plan, nil
}

type QuarantineCleanup struct {
	Removed      []string `json:"removed"`
	KeptEvidence []string `json:"kept_evidence"`
	Note         string   `json:"note"`
}

// CleanQuarantine removes only the workspace copy of attempts the runner
// marked quarantined. Patches, events and check rows stay.
func (s *Service) CleanQuarantine(ctx context.Context) (QuarantineCleanup, error) {
	out := QuarantineCleanup{
		Note: "将删除隔离 Attempt 的工作区副本。补丁、事件文件和检查摘要还在。工作区里没被收成制品的文件不能恢复。这不会把结论改成通过，也不会合并到源目录。",
	}
	items, err := s.Store.ListQuarantine(ctx)
	if err != nil {
		return out, err
	}
	for _, item := range items {
		work := filepath.Join(s.DataDir, "attempts", item.ID, "work")
		if _, err := os.Stat(work); err == nil {
			if err := os.RemoveAll(work); err != nil {
				return out, err
			}
			out.Removed = append(out.Removed, work)
		}
		patch := filepath.Join(s.DataDir, "attempts", item.ID, "patch.json")
		if _, err := os.Stat(patch); err == nil {
			out.KeptEvidence = append(out.KeptEvidence, patch)
		}
		events := filepath.Join(s.DataDir, "attempts", item.ID, "events.ndjson")
		if _, err := os.Stat(events); err == nil {
			out.KeptEvidence = append(out.KeptEvidence, events)
		}
		_ = s.Store.Audit(ctx, "control", "quarantine_cleanup", item.ID, `{"removed":"work"}`)
	}
	if out.Removed == nil {
		out.Removed = []string{}
	}
	if out.KeptEvidence == nil {
		out.KeptEvidence = []string{}
	}
	_ = domain.CleanupQuarantined
	return out, nil
}
