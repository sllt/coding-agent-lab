package scheduler

import "github.com/sllt/agentlab/internal/domain"

type Decision struct {
	State   domain.ExecutionState
	Verdict domain.Verdict
	Cleanup domain.CleanupState
	Requeue bool
	Reason  string
}

// Reconcile decides what a restarted control plane may do. It never treats a
// lost final event as proof that the provider was not called.
func Reconcile(agentStarted, runnerAlive, identityConfirmed bool, autoRetries int) Decision {
	if runnerAlive && !identityConfirmed {
		return Decision{State: domain.ExecAborted, Verdict: domain.VerdictInconclusive, Cleanup: domain.CleanupQuarantined, Reason: "identity_unconfirmed"}
	}
	if runnerAlive && identityConfirmed {
		return Decision{State: domain.ExecAborted, Verdict: domain.VerdictInconclusive, Cleanup: domain.CleanupPending, Requeue: false, Reason: "stop_existing_do_not_replace"}
	}
	if !agentStarted {
		return Decision{State: domain.ExecAborted, Verdict: domain.VerdictInconclusive, Cleanup: domain.CleanupClean, Requeue: autoRetries > 0, Reason: "never_started"}
	}
	return Decision{State: domain.ExecAborted, Verdict: domain.VerdictInconclusive, Cleanup: domain.CleanupClean, Requeue: false, Reason: "agent_started_result_missing"}
}
