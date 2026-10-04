package doctor

import (
	"context"
	"testing"
)

func TestStaticDoctorDoesNotCallAModel(t *testing.T) {
	ctx := context.Background()
	missing := Static(ctx, "cursor", "agentlab-cli-that-does-not-exist", "REPLACE_WITH_VERIFIED_MODEL_ID", false)
	if missing.ModelCall != "not_run" || missing.Verified || missing.Network != "unrestricted" {
		t.Fatalf("%+v", missing)
	}
	if !has(missing.Blockers, "placeholder_field") || !has(missing.Blockers, "cli_not_found") {
		t.Fatalf("blockers %v", missing.Blockers)
	}
	fixture := Static(ctx, "fixture", "agentlab", "fixture-local", false)
	if !fixture.StaticPassed || fixture.Verified || fixture.ModelCall == "passed" {
		t.Fatalf("fixture %+v", fixture)
	}
	refused := Static(ctx, "fixture", "agentlab", "fixture-local", true)
	if refused.ModelCall != "not_run" || refused.Verified {
		// fixture returns before the allowModelCall branch and is not verified
	}
	versionFail := Static(ctx, "grok", "/bin/false", "grok-model", false)
	if versionFail.StaticPassed || !has(versionFail.Blockers, "version_probe_failed") || versionFail.SupportsHeadless {
		t.Fatalf("version probe %+v", versionFail)
	}
	real := Static(ctx, "grok", "grok", "grok-model", true)
	if real.ModelCall != "refused" && !has(real.Blockers, "cli_not_found") {
		t.Fatalf("paid probe was not refused: %+v", real)
	}
}
