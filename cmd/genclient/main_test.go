package main

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIIncludesControlRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc document
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ops, err := operations(&doc.Paths)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]operation{}
	for _, op := range ops {
		got[op.method+" "+op.path] = op
	}
	want := map[string]string{
		"GET /api/v1/audit":                   "listAudit",
		"GET /api/v1/upgrade":                 "upgrade",
		"GET /api/v1/settings":                "settings",
		"PATCH /api/v1/settings":              "patchSettings",
		"POST /api/v1/maintenance/retention":  "retention",
		"POST /api/v1/maintenance/quarantine": "cleanQuarantine",
		"POST /api/v1/exports/harbor":         "exportHarbor",
		"POST /api/v1/tasks/{id}/flaky":       "markFlaky",
		"POST /api/v1/trials/{id}/adopt":      "adoptPatch",
	}
	for key, id := range want {
		op, ok := got[key]
		if !ok || op.id != id {
			t.Fatalf("%s => %+v", key, op)
		}
	}
	for _, key := range []string{
		"PATCH /api/v1/settings",
		"POST /api/v1/maintenance/retention",
		"POST /api/v1/exports/harbor",
		"POST /api/v1/tasks/{id}/flaky",
	} {
		if got[key].schema == "" {
			t.Fatalf("%s has no request schema", key)
		}
	}
}
