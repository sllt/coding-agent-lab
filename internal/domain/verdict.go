package domain

// Check is an independent verifier result. Agent prose is never an input.
type Check struct {
	ID             string
	Required       bool
	Kind           string
	Outcome        string
	FailureClass   string
	CaseCount      int
	MinCases       int
	MissingCaseIDs []string
	SkippedCaseIDs []string
	Assurance      string
	// ExitCode, DurationMS and EvidenceDigest are recorded by the verifier.
	// A nil exit code means the check did not reach a process. Zero is a real exit.
	ExitCode       *int   `json:"ExitCode,omitempty"`
	DurationMS     int64  `json:"DurationMS,omitempty"`
	EvidenceDigest string `json:"EvidenceDigest,omitempty"`
}

const (
	OutcomePass    = "pass"
	OutcomeFail    = "fail"
	OutcomeSkipped = "skipped"
	OutcomeError   = "error"
	OutcomeMissing = "missing"

	ClassTestFailed     = "test_failed"
	ClassVerifierFailed = "verifier_failed"
)

// Judge returns the platform verdict. Zero cases, skipped required checks,
// and policy failures cannot pass. Verifier crashes are inconclusive.
func Judge(checks []Check, evidenceComplete bool) Verdict {
	if !evidenceComplete || len(checks) == 0 {
		return VerdictInconclusive
	}
	sawRequired := false
	manualPending := false
	failed := false
	for _, c := range checks {
		if c.FailureClass == ClassVerifierFailed || c.Outcome == OutcomeError || c.Outcome == OutcomeMissing {
			return VerdictInconclusive
		}
		if !c.Required {
			continue
		}
		sawRequired = true
		if c.Kind == "manual" && c.Outcome != OutcomePass {
			manualPending = true
			continue
		}
		if c.Outcome == OutcomeSkipped || len(c.SkippedCaseIDs) > 0 || len(c.MissingCaseIDs) > 0 || c.Outcome == OutcomeFail {
			failed = true
			continue
		}
		if c.MinCases > 0 && c.CaseCount < c.MinCases {
			failed = true
			continue
		}
		if c.CaseCount == 0 && c.Kind != "policy" && c.Kind != "manual" {
			failed = true
			continue
		}
		if c.Outcome != OutcomePass {
			failed = true
		}
	}
	if !sawRequired {
		return VerdictInconclusive
	}
	if failed {
		return VerdictFail
	}
	if manualPending {
		return VerdictUnverified
	}
	return VerdictPass
}
