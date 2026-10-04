package verifier

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sllt/agentlab/internal/domain"
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
