package scheduler

import (
	"testing"

	"github.com/sllt/agentlab/internal/domain"
)

func TestReconcileDoesNotRerunAPaidAttempt(t *testing.T) {
	lost := Reconcile(true, false, true, 0)
	if lost.Requeue || lost.Verdict != domain.VerdictInconclusive || lost.Reason != "agent_started_result_missing" {
		t.Fatalf("%+v", lost)
	}
	never := Reconcile(false, false, true, 0)
	if never.Requeue || never.Reason != "never_started" {
		t.Fatalf("%+v", never)
	}
	unknown := Reconcile(true, true, false, 3)
	if unknown.Cleanup != domain.CleanupQuarantined || unknown.Requeue {
		t.Fatalf("%+v", unknown)
	}
	alive := Reconcile(true, true, true, 0)
	if alive.Requeue || alive.Reason != "stop_existing_do_not_replace" {
		t.Fatalf("%+v", alive)
	}
}
