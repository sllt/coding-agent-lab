package domain

import "fmt"

type ExecutionState string

const (
	ExecQueued     ExecutionState = "queued"
	ExecPreparing  ExecutionState = "preparing"
	ExecRunning    ExecutionState = "running"
	ExecCollecting ExecutionState = "collecting"
	ExecVerifying  ExecutionState = "verifying"
	ExecCompleted  ExecutionState = "completed"
	ExecCancelling ExecutionState = "cancelling"
	ExecCancelled  ExecutionState = "cancelled"
	ExecAborted    ExecutionState = "aborted"
)

type Verdict string

const (
	VerdictUnverified   Verdict = "unverified"
	VerdictPass         Verdict = "pass"
	VerdictFail         Verdict = "fail"
	VerdictInconclusive Verdict = "inconclusive"
)

type CleanupState string

const (
	CleanupPending     CleanupState = "pending"
	CleanupClean       CleanupState = "clean"
	CleanupFailed      CleanupState = "failed"
	CleanupQuarantined CleanupState = "quarantined"
)

func (s ExecutionState) Terminal() bool {
	switch s {
	case ExecCompleted, ExecCancelled, ExecAborted:
		return true
	default:
		return false
	}
}

func (s CleanupState) ReleasesCapacity() bool { return s == CleanupClean }

// CanTransition is the legal execution edge. Cleanup is a separate field.
func CanTransition(from, to ExecutionState) bool {
	if from == to {
		return true
	}
	switch from {
	case ExecQueued:
		return to == ExecPreparing || to == ExecCancelling || to == ExecAborted
	case ExecPreparing:
		return to == ExecRunning || to == ExecCancelling || to == ExecAborted
	case ExecRunning:
		return to == ExecCollecting || to == ExecCancelling || to == ExecAborted
	case ExecCollecting:
		return to == ExecVerifying || to == ExecCancelling || to == ExecAborted
	case ExecVerifying:
		return to == ExecCompleted || to == ExecCancelling || to == ExecAborted
	case ExecCancelling:
		return to == ExecCancelled || to == ExecAborted
	default:
		return false
	}
}

type AttemptSnapshot struct {
	ID           string
	TrialID      string
	Fence        int64
	State        ExecutionState
	Verdict      Verdict
	Cleanup      CleanupState
	AgentStarted bool
}

type TrialSnapshot struct {
	ID               string
	CurrentAttemptID string
	State            ExecutionState
	Verdict          Verdict
	CancelRequested  bool
}

type Report struct {
	AttemptID string
	Fence     int64
	State     ExecutionState
	Verdict   Verdict
	Cleanup   CleanupState
	Reason    string
}

// ApplyReport rejects stale fences and terminal overwrites. A cancel that is
// already stored wins over a late success report from the same attempt.
func ApplyReport(trial TrialSnapshot, attempt AttemptSnapshot, report Report) (TrialSnapshot, AttemptSnapshot, error) {
	if report.AttemptID != attempt.ID {
		return trial, attempt, fmt.Errorf("attempt id mismatch")
	}
	if report.Fence != attempt.Fence {
		return trial, attempt, fmt.Errorf("stale fence %d", report.Fence)
	}
	if trial.CurrentAttemptID != "" && trial.CurrentAttemptID != attempt.ID {
		return trial, attempt, fmt.Errorf("stale attempt %s", attempt.ID)
	}
	if attempt.State.Terminal() && report.State != attempt.State {
		return trial, attempt, fmt.Errorf("attempt %s is already %s", attempt.ID, attempt.State)
	}
	if !CanTransition(attempt.State, report.State) {
		return trial, attempt, fmt.Errorf("illegal transition %s -> %s", attempt.State, report.State)
	}
	if !validVerdict(report.Verdict) || !validCleanup(report.Cleanup) {
		return trial, attempt, fmt.Errorf("invalid verdict or cleanup")
	}
	attempt.State = report.State
	attempt.Verdict = report.Verdict
	attempt.Cleanup = report.Cleanup
	trial.CurrentAttemptID = attempt.ID
	trial.State = report.State
	trial.Verdict = report.Verdict
	return trial, attempt, nil
}

func validVerdict(v Verdict) bool {
	switch v {
	case VerdictUnverified, VerdictPass, VerdictFail, VerdictInconclusive:
		return true
	default:
		return false
	}
}

func validCleanup(s CleanupState) bool {
	switch s {
	case CleanupPending, CleanupClean, CleanupFailed, CleanupQuarantined:
		return true
	default:
		return false
	}
}
