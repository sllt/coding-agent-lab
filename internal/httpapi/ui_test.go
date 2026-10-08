package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The redesigned UI builds its pickers, matrix labels and readiness badges
// from these read-only projections. They must name published versions and
// never require one request per task or profile.
func TestCatalogLabelsAndLatestProfile(t *testing.T) {
	_, rt, svc := newAPI(t)
	token, csrf := setupLogin(t, rt)
	task, profile := publishPair(t, svc)

	rec := call(rt, http.MethodGet, "/api/v1/catalog", nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog %d %s", rec.Code, rec.Body.String())
	}
	var cat struct {
		Data struct {
			Tasks []struct {
				VersionID string `json:"version_id"`
				TaskName  string `json:"task_name"`
				Version   int    `json:"version"`
			} `json:"tasks"`
			Profiles []struct {
				VersionID string `json:"version_id"`
				ProfileID string `json:"profile_id"`
				Adapter   string `json:"adapter"`
				Readiness string `json:"readiness"`
			} `json:"profiles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.Data.Tasks) != 1 || cat.Data.Tasks[0].VersionID != task || cat.Data.Tasks[0].TaskName == "" || cat.Data.Tasks[0].Version != 1 {
		t.Fatalf("tasks %+v", cat.Data.Tasks)
	}
	if len(cat.Data.Profiles) != 1 || cat.Data.Profiles[0].VersionID != profile || cat.Data.Profiles[0].Adapter == "" {
		t.Fatalf("profiles %+v", cat.Data.Profiles)
	}

	rec = call(rt, http.MethodGet, "/api/v1/profiles", nil, token, "")
	var list struct {
		Data struct {
			Latest map[string]struct {
				VersionID string `json:"version_id"`
			} `json:"latest"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if got := list.Data.Latest[cat.Data.Profiles[0].ProfileID].VersionID; got != profile {
		t.Fatalf("latest %q want %q: %s", got, profile, rec.Body.String())
	}

	sub := callKey(rt, "/api/v1/experiments", map[string]any{
		"name": "矩阵", "mode": "agent_profile", "task_version_ids": []string{task}, "profile_version_ids": []string{profile},
		"repetitions": 2, "protocol": "single-pass-v1",
	}, token, csrf, "ui-1")
	if sub.Code != http.StatusAccepted {
		t.Fatalf("submit %d %s", sub.Code, sub.Body.String())
	}
	rec = call(rt, http.MethodGet, "/api/v1/experiments/"+experimentID(sub.Body.Bytes()), nil, token, "")
	var detail struct {
		Data struct {
			Experiment struct {
				Name string `json:"name"`
			} `json:"experiment"`
			Labels struct {
				Tasks    map[string]json.RawMessage `json:"tasks"`
				Profiles map[string]json.RawMessage `json:"profiles"`
			} `json:"labels"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &detail)
	if detail.Data.Experiment.Name != "矩阵" || detail.Data.Labels.Tasks[task] == nil || detail.Data.Labels.Profiles[profile] == nil {
		t.Fatalf("detail %s", rec.Body.String())
	}

	rec = call(rt, http.MethodGet, "/api/v1/settings", nil, token, "")
	var st struct {
		Data struct {
			Sandbox *struct {
				Supported bool `json:"supported"`
			} `json:"sandbox"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Data.Sandbox == nil {
		t.Fatalf("settings without sandbox status: %s", rec.Body.String())
	}
}
