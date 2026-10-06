package verifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/workspace"
)

func TestCorrectWrongAndEmptyPatches(t *testing.T) {
	root, err := filepath.Abs("../../fixtures/tasks/orders-pagination")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pre, err := Precheck(ctx, OrdersPagination(root))
	if err != nil {
		t.Fatal(err)
	}
	if pre.BaselineBlocked || pre.Verdict == domain.VerdictPass {
		t.Fatalf("baseline precheck %+v", pre)
	}
	correct, err := PatchFromFile(filepath.Join(root, "orders.go"), filepath.Join(root, "solutions/correct/orders.go"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Evaluate(ctx, OrdersPagination(root), correct)
	if err != nil || got.Verdict != domain.VerdictPass {
		t.Fatalf("correct %+v %v", got, err)
	}
	wrong, err := PatchFromFile(filepath.Join(root, "orders.go"), filepath.Join(root, "solutions/wrong/orders.go"))
	if err != nil {
		t.Fatal(err)
	}
	got, err = Evaluate(ctx, OrdersPagination(root), wrong)
	if err != nil || got.Verdict != domain.VerdictFail {
		t.Fatalf("wrong %+v %v", got, err)
	}
	empty, err := PatchFromFile(filepath.Join(root, "orders.go"), "")
	if err != nil {
		t.Fatal(err)
	}
	got, err = Evaluate(ctx, OrdersPagination(root), empty)
	if err != nil || got.Verdict != domain.VerdictFail {
		t.Fatalf("empty %+v %v", got, err)
	}
}

func TestFilterTaskBaselineIsRed(t *testing.T) {
	root, err := filepath.Abs("../../fixtures/tasks/status-filter")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := Precheck(context.Background(), OrdersPagination(root))
	if err != nil {
		t.Fatal(err)
	}
	if pre.BaselineBlocked {
		t.Fatalf("filter baseline %+v", pre)
	}
}

func TestReplacedRegressionCannotPass(t *testing.T) {
	dir := miniTask(t, true)
	pre, err := Precheck(context.Background(), GoChecks(dir))
	if err != nil || pre.BaselineBlocked {
		t.Fatalf("pre %+v %v", pre, err)
	}
	staged, err := stage(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(staged) })
	next, err := os.MkdirTemp("", "agentlab-weak-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(next) })
	if err := workspace.CopyBaseline(staged, next, workspace.Limits{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(next, "orders.go"), []byte("package example\nfunc Stable() bool { return false }\nfunc Feature() bool { return true }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(next, "regression_test.go"), []byte("package example\nimport \"testing\"\nfunc TestRegressionStable(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch, err := workspace.Collect(staged, next, workspace.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Evaluate(context.Background(), GoChecks(dir), patch)
	if err != nil || got.Verdict == domain.VerdictPass {
		t.Fatalf("weakened regression %+v %v", got, err)
	}
}

func TestZeroAcceptanceBlocksPublish(t *testing.T) {
	dir := miniTask(t, false)
	pre, err := Precheck(context.Background(), GoChecks(dir))
	if err != nil || !pre.BaselineBlocked {
		t.Fatalf("zero acceptance %+v %v", pre, err)
	}
}

func TestToolchainUnavailableIsVerifierFailed(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\necho toolchain unavailable >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	dir := miniTask(t, true)
	check, _, _ := runGoTest(context.Background(), dir, "^TestRegression")
	if check.FailureClass != domain.ClassVerifierFailed || check.Outcome == domain.OutcomeFail && check.FailureClass == domain.ClassTestFailed {
		t.Fatalf("%+v", check)
	}
	if check.FailureClass == domain.ClassTestFailed {
		t.Fatalf("toolchain became a candidate failure: %+v", check)
	}
}

func miniTask(t *testing.T, withAcceptance bool) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example\n\ngo 1.22\n")
	write("orders.go", "package example\nfunc Stable() bool { return true }\nfunc Feature() bool { return false }\n")
	write("regression_test.go", "package example\nimport \"testing\"\nfunc TestRegressionStable(t *testing.T) { if !Stable() { t.Fatal(\"stable\") } }\n")
	acc := "package example\nimport \"testing\"\nfunc TestNotTheAcceptance(t *testing.T) {}\n"
	if withAcceptance {
		acc = "package example\nimport \"testing\"\nfunc TestAcceptanceFeature(t *testing.T) { if !Feature() { t.Fatal(\"feature\") } }\n"
	}
	write("hidden/acceptance_test.go", acc)
	return dir
}
