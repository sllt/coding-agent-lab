package sqlite

import (
	"context"
	"encoding/json"
)

// TaskVersionInfo is a published task version with its task and project
// names, for pickers and labels.
type TaskVersionInfo struct {
	VersionID   string `json:"version_id"`
	TaskID      string `json:"task_id"`
	TaskName    string `json:"task_name"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Version     int    `json:"version"`
	Digest      string `json:"digest"`
	CreatedAt   string `json:"created_at"`
}

// ProfileVersionInfo is a published profile version with the fields a picker
// needs. Readiness comes from the doctor report frozen at publish time.
type ProfileVersionInfo struct {
	VersionID   string `json:"version_id"`
	ProfileID   string `json:"profile_id"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Adapter     string `json:"adapter"`
	Model       string `json:"model"`
	DisplayName string `json:"display_name"`
	Readiness   string `json:"readiness"`
	Digest      string `json:"digest"`
	CreatedAt   string `json:"created_at"`
}

// CatalogTasks lists every published task version, newest version first
// within each task, in one query.
func (s *Store) CatalogTasks(ctx context.Context) ([]TaskVersionInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT v.id, t.id, t.name, p.id, p.name, v.version, v.digest, v.created_at
		FROM task_versions v JOIN tasks t ON t.id=v.task_id JOIN projects p ON p.id=t.project_id
		ORDER BY p.created_at, t.created_at, t.rowid, v.version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskVersionInfo{}
	for rows.Next() {
		var v TaskVersionInfo
		if err := rows.Scan(&v.VersionID, &v.TaskID, &v.TaskName, &v.ProjectID, &v.ProjectName, &v.Version, &v.Digest, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CatalogProfiles lists every published profile version in one query.
func (s *Store) CatalogProfiles(ctx context.Context) ([]ProfileVersionInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT v.id, p.id, p.name, v.version, v.snapshot_json, v.doctor_json, v.digest, v.created_at
		FROM profile_versions v JOIN profiles p ON p.id=v.profile_id
		ORDER BY p.created_at, p.rowid, v.version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProfileVersionInfo{}
	for rows.Next() {
		var v ProfileVersionInfo
		var snap, doctor string
		if err := rows.Scan(&v.VersionID, &v.ProfileID, &v.Name, &v.Version, &snap, &doctor, &v.Digest, &v.CreatedAt); err != nil {
			return nil, err
		}
		var sn struct {
			Adapter     string `json:"adapter"`
			Model       string `json:"model"`
			DisplayName string `json:"display_name"`
		}
		_ = json.Unmarshal([]byte(snap), &sn)
		var dr struct {
			Readiness string `json:"readiness"`
			Verified  bool   `json:"verified"`
		}
		_ = json.Unmarshal([]byte(doctor), &dr)
		v.Adapter, v.Model, v.DisplayName = sn.Adapter, sn.Model, sn.DisplayName
		v.Readiness = dr.Readiness
		if v.Readiness == "" && dr.Verified {
			v.Readiness = "verified"
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
