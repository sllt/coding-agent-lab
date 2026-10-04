package domain

import (
	"os"
	"strings"
	"testing"
)

func TestIllegalTransitionCancelRaceAndStaleFence(t *testing.T) {
	if CanTransition(ExecCompleted, ExecCancelling) {
		t.Fatal("completed cannot cancel")
	}
	if CanTransition(ExecRunning, ExecCompleted) {
		t.Fatal("running cannot skip collect and verify")
	}
	attempt := AttemptSnapshot{ID: "a1", Fence: 2, State: ExecCancelling, Verdict: VerdictUnverified, Cleanup: CleanupPending}
	trial := TrialSnapshot{CurrentAttemptID: "a1", State: ExecCancelling, Verdict: VerdictUnverified}
	_, _, err := ApplyReport(trial, attempt, Report{AttemptID: "a1", Fence: 2, State: ExecCompleted, Verdict: VerdictPass, Cleanup: CleanupClean})
	if err == nil {
		t.Fatal("cancel must win over a late pass")
	}
	trial.CurrentAttemptID = "a2"
	_, _, err = ApplyReport(trial, attempt, Report{AttemptID: "a1", Fence: 2, State: ExecCancelled, Verdict: VerdictUnverified, Cleanup: CleanupClean})
	if err == nil || !strings.Contains(err.Error(), "stale attempt") {
		t.Fatalf("old attempt err=%v", err)
	}
	trial.CurrentAttemptID = "a1"
	_, _, err = ApplyReport(trial, attempt, Report{AttemptID: "a1", Fence: 1, State: ExecCancelled, Verdict: VerdictUnverified, Cleanup: CleanupClean})
	if err == nil || !strings.Contains(err.Error(), "stale fence") {
		t.Fatalf("old fence err=%v", err)
	}
	nextTrial, nextAttempt, err := ApplyReport(trial, attempt, Report{AttemptID: "a1", Fence: 2, State: ExecCancelled, Verdict: VerdictUnverified, Cleanup: CleanupPending})
	if err != nil {
		t.Fatal(err)
	}
	if nextAttempt.Cleanup.ReleasesCapacity() {
		t.Fatal("pending cleanup must keep capacity")
	}
	if nextTrial.Verdict != VerdictUnverified {
		t.Fatalf("verdict %s", nextTrial.Verdict)
	}
	_, _, err = ApplyReport(nextTrial, nextAttempt, Report{AttemptID: "a1", Fence: 2, State: ExecCompleted, Verdict: VerdictPass, Cleanup: CleanupClean})
	if err == nil {
		t.Fatal("terminal attempt was overwritten")
	}
}

func TestJudgeDoesNotPassEmptySkippedOrForgedInfrastructure(t *testing.T) {
	pass := Check{ID: "regression", Required: true, Kind: "regression", Outcome: OutcomePass, CaseCount: 2, MinCases: 1}
	if Judge([]Check{pass, {ID: "acceptance", Required: true, Kind: "acceptance", Outcome: OutcomePass, CaseCount: 1, MinCases: 1}}, true) != VerdictPass {
		t.Fatal("expected pass")
	}
	if Judge([]Check{{ID: "acceptance", Required: true, Kind: "acceptance", Outcome: OutcomePass, CaseCount: 0, MinCases: 1}}, true) != VerdictFail {
		t.Fatal("zero cases must not pass")
	}
	if Judge([]Check{pass, {ID: "acceptance", Required: true, Kind: "acceptance", Outcome: OutcomeSkipped, CaseCount: 0}}, true) != VerdictFail {
		t.Fatal("skipped required must not pass")
	}
	if Judge([]Check{pass}, false) != VerdictInconclusive {
		t.Fatal("missing evidence is inconclusive")
	}
	if Judge([]Check{pass, {ID: "acceptance", Required: true, Kind: "acceptance", Outcome: OutcomeError, FailureClass: ClassVerifierFailed}}, true) != VerdictInconclusive {
		t.Fatal("verifier failure is inconclusive")
	}
	if Judge([]Check{pass, {ID: "style", Required: true, Kind: "manual", Outcome: OutcomeMissing}}, true) != VerdictInconclusive {
		t.Fatal("missing manual result is inconclusive")
	}
	if Judge(nil, true) != VerdictInconclusive {
		t.Fatal("no checks must not pass")
	}
}

func TestFingerprintsAndComparisonDoNotMergeUnlikeRuns(t *testing.T) {
	base := ExecutionIdentity{TaskDigest: "t", ProfileDigest: "p", EnvironmentDigest: "e", ProtocolDigest: "proto", VerifierDigest: "v"}
	fp, err := ExecutionFingerprint(base)
	if err != nil || fp == "" {
		t.Fatal(err)
	}
	base.ProfileDigest = "other"
	fp2, err := ExecutionFingerprint(base)
	if err != nil || fp == fp2 {
		t.Fatal("profile change must change execution fingerprint")
	}
	a := ComparisonInput{Mode: ModeAgentProfile, TaskDigest: "t", EnvironmentDigest: "e", ProtocolDigest: "proto", VerifierDigest: "v", BudgetDigest: "b", Executor: "docker", Network: "unrestricted", ProfileDigest: "p1"}
	b := a
	b.ProfileDigest = "p2"
	ok, reasons, err := SameBoard(a, b)
	if err != nil || !ok || len(reasons) != 0 {
		t.Fatalf("agent profile compare ok=%v reasons=%v err=%v", ok, reasons, err)
	}
	b.EnvironmentDigest = "other-env"
	ok, reasons, err = SameBoard(a, b)
	if err != nil || ok || !contains(reasons, "comparison_key_mismatch") {
		t.Fatalf("env mismatch ok=%v reasons=%v", ok, reasons)
	}
	c := a
	c.ModelMismatch = true
	got, err := ComparisonOf(c)
	if err != nil || got.Comparable {
		t.Fatal("model mismatch must leave the default board")
	}
	modelA := ComparisonInput{Mode: ModeControlledModel, TaskDigest: "t", EnvironmentDigest: "e", ProtocolDigest: "proto", VerifierDigest: "v", BudgetDigest: "b", Executor: "docker", Network: "unrestricted", ProfileDigest: "profile-sans-model", ModelID: "model-a"}
	modelB := modelA
	modelB.ModelID = "model-b"
	ok, reasons, err = SameBoard(modelA, modelB)
	if err != nil || !ok {
		t.Fatalf("controlled model variable must stay on one board: %v %v", ok, reasons)
	}
	flowA := ComparisonInput{Mode: ModeWorkflow, TaskDigest: "t", EnvironmentDigest: "e", ProtocolDigest: "single", VerifierDigest: "v", BudgetDigest: "b", Executor: "docker", Network: "unrestricted", ProfileDigest: "p"}
	flowB := flowA
	flowB.ProtocolDigest = "review-fix"
	ok, _, err = SameBoard(flowA, flowB)
	if err != nil || !ok {
		t.Fatal("workflow comparison keeps task and budget, not the protocol")
	}
	native := a
	native.Executor = "native-trusted"
	ok, reasons, err = SameBoard(a, native)
	if err != nil || ok || !contains(reasons, "executor_mismatch") {
		t.Fatalf("executor mismatch ok=%v reasons=%v", ok, reasons)
	}
}

func TestDraftExamplesRejectPlaceholdersAndUnknownFields(t *testing.T) {
	raw, err := os.ReadFile("../../examples/task-draft.yaml")
	if err != nil {
		t.Fatal(err)
	}
	task, err := DecodeTask(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(PublishBlockers(task)) != 0 {
		t.Fatalf("task blockers %#v", PublishBlockers(task))
	}
	if _, err := DecodeTask(append(raw, []byte("\nunknown_field: 1\n")...)); err == nil {
		t.Fatal("unknown field must fail")
	}
	dup := []byte("schema_version: agentlab.task/v1alpha1\nkind: TaskDraft\nkind: TaskDraft\n")
	if _, err := DecodeTask(dup); err == nil {
		t.Fatal("duplicate key must fail")
	}
	profileRaw, err := os.ReadFile("../../examples/profile-draft.yaml")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := DecodeProfile(profileRaw)
	if err != nil {
		t.Fatal(err)
	}
	blockers := PublishBlockers(profile)
	if !contains(blockers, "placeholder_field") {
		t.Fatalf("REPLACE_ must block publish, got %v", blockers)
	}
	expRaw, err := os.ReadFile("../../examples/experiment-draft.yaml")
	if err != nil {
		t.Fatal(err)
	}
	exp, err := DecodeExperiment(expRaw)
	if err != nil {
		t.Fatal(err)
	}
	if TrialCount(len(exp.TaskVersionIDs), len(exp.ProfileVersionIDs), exp.Repetitions) != 3 {
		t.Fatalf("trial count %d", TrialCount(1, 3, 1))
	}
	if exp.Budget.ObservedCostLimitMicroUSD != nil {
		t.Fatal("null cost must stay nil")
	}
}

func TestUsageNullIsNotZeroAndCumulativeIsNotSummed(t *testing.T) {
	folded, err := FoldUsage(nil)
	if err != nil || folded.InputTokens != nil || folded.CostMicroUSD != nil {
		t.Fatalf("empty usage %+v err=%v", folded, err)
	}
	one := Int64Ptr(10)
	again := Int64Ptr(10)
	summed, err := FoldUsage([]Usage{
		{Source: "cli", SourceEventID: "e1", InputTokens: one},
		{Source: "cli", SourceEventID: "e1", InputTokens: again},
		{Source: "cli", SourceEventID: "e2", OutputTokens: Int64Ptr(4)},
	})
	if err != nil || summed.InputTokens == nil || *summed.InputTokens != 10 || summed.OutputTokens == nil || *summed.OutputTokens != 4 || summed.CostMicroUSD != nil {
		t.Fatalf("folded %+v err=%v", summed, err)
	}
	cumulative, err := FoldUsage([]Usage{
		{Source: "cli", SourceEventID: "c1", IsCumulative: true, InputTokens: Int64Ptr(3)},
		{Source: "cli", SourceEventID: "c2", IsCumulative: true, InputTokens: Int64Ptr(8), CostMicroUSD: nil},
	})
	if err != nil || cumulative.InputTokens == nil || *cumulative.InputTokens != 8 || cumulative.CostMicroUSD != nil {
		t.Fatalf("cumulative %+v err=%v", cumulative, err)
	}
}

func TestCanonicalRejectsFloatsAndOrdersKeys(t *testing.T) {
	a, err := Canonical(map[string]any{"b": 1, "a": "x"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Canonical(map[string]any{"a": "x", "b": 1})
	if err != nil || string(a) != string(b) || string(a) != `{"a":"x","b":1}` {
		t.Fatalf("canonical %s %s", a, b)
	}
	if _, err := Canonical(map[string]any{"n": 1.5}); err == nil {
		t.Fatal("float must be rejected")
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
