package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/sllt/agentlab/internal/version"
)

// LabSettings is the local control-plane policy. Zero quota means the
// watermark is not configured, so new work is not blocked on a guessed limit.
type LabSettings struct {
	GlobalLimit    int       `json:"global_limit"`
	Retention      Retention `json:"retention"`
	DiskQuotaBytes int64     `json:"disk_quota_bytes"`
	UpdatedAt      string    `json:"updated_at,omitempty"`
}

type Retention struct {
	LogDays    int `json:"log_days"`
	PatchDays  int `json:"patch_days"`
	ReportDays int `json:"report_days"`
}

func DefaultSettings() LabSettings {
	return LabSettings{
		GlobalLimit: 1,
		Retention:   Retention{LogDays: 30, PatchDays: 0, ReportDays: 365},
	}
}

func (s *Service) settingsPath() string {
	return filepath.Join(s.DataDir, "settings.json")
}

func (s *Service) LoadSettings() LabSettings {
	out := DefaultSettings()
	raw, err := os.ReadFile(s.settingsPath())
	if err != nil {
		if s.GlobalLimit > 0 {
			out.GlobalLimit = s.GlobalLimit
		}
		return out
	}
	if json.Unmarshal(raw, &out) != nil {
		return DefaultSettings()
	}
	if out.GlobalLimit <= 0 {
		out.GlobalLimit = 1
	}
	if out.GlobalLimit > 8 {
		out.GlobalLimit = 8
	}
	return out
}

func (s *Service) SaveSettings(next LabSettings) (LabSettings, error) {
	if next.GlobalLimit < 1 || next.GlobalLimit > 8 {
		return LabSettings{}, errors.New("global limit")
	}
	if next.Retention.LogDays < 0 || next.Retention.PatchDays < 0 || next.Retention.ReportDays < 0 {
		return LabSettings{}, errors.New("retention")
	}
	if next.DiskQuotaBytes < 0 {
		return LabSettings{}, errors.New("quota")
	}
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	body, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return LabSettings{}, err
	}
	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		return LabSettings{}, err
	}
	if err := os.WriteFile(s.settingsPath(), body, 0o644); err != nil {
		return LabSettings{}, err
	}
	s.GlobalLimit = next.GlobalLimit
	return next, nil
}

func (s *Service) limit() int {
	if s.DataDir != "" {
		if _, err := os.Stat(s.settingsPath()); err == nil {
			return s.LoadSettings().GlobalLimit
		}
	}
	if s.GlobalLimit <= 0 {
		return 1
	}
	return s.GlobalLimit
}

func (s *Service) quotaBlocked() error {
	settings := s.LoadSettings()
	if settings.DiskQuotaBytes <= 0 || s.DataDir == "" {
		return nil
	}
	used, err := dirBytes(s.DataDir)
	if err != nil {
		return err
	}
	if used >= settings.DiskQuotaBytes {
		return ErrUnexecutable
	}
	return nil
}

func dirBytes(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info != nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

type UpgradeReport struct {
	Product        string   `json:"product"`
	Version        string   `json:"version"`
	Migrations     []string `json:"migrations"`
	BackupRequired bool     `json:"backup_required"`
	DownMigration  bool     `json:"down_migration"`
	AutoCLIUpdate  bool     `json:"auto_cli_update"`
	Note           string   `json:"note"`
}

func (s *Service) UpgradeReport() UpgradeReport {
	names, _ := s.Store.AppliedMigrations(context.Background())
	return UpgradeReport{
		Product: version.Product, Version: version.Version, Migrations: names,
		BackupRequired: true, DownMigration: false, AutoCLIUpdate: false,
		Note: "升级前先做一致性备份。本程序不提供向下迁移，也不在升级时更新 Cursor、Grok 或 OpenCode。旧程序和备份留着才能回退。",
	}
}
