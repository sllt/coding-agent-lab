// Package verifier runs checks in a clean tree. Agent stdout is not evidence.
package verifier

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/workspace"
)

type Spec struct {
	Root       string
	Regression string
	Acceptance string
}

func OrdersPagination(root string) Spec {
	return GoChecks(root)
}

func GoChecks(root string) Spec {
	return Spec{Root: root, Regression: "^TestRegression", Acceptance: "^TestAcceptance"}
}

type Outcome struct {
	Checks           []domain.Check
	Verdict          domain.Verdict
	BaselineBlocked  bool
	BaselineReason   string
	EvidenceComplete bool
}

// Precheck stages the baseline and requires regression to pass and acceptance to fail.
func Precheck(ctx context.Context, spec Spec) (Outcome, error) {
	dir, err := stage(spec.Root, true)
	if err != nil {
		return Outcome{}, err
	}
	defer os.RemoveAll(dir)
	reg := runGoTest(ctx, dir, spec.Regression)
	acc := runGoTest(ctx, dir, spec.Acceptance)
	out := Outcome{EvidenceComplete: reg.FailureClass != domain.ClassVerifierFailed && acc.FailureClass != domain.ClassVerifierFailed}
	out.Checks = []domain.Check{reg, acc}
	if reg.FailureClass == domain.ClassVerifierFailed || acc.FailureClass == domain.ClassVerifierFailed {
		out.Verdict = domain.VerdictInconclusive
		out.BaselineBlocked = true
		out.BaselineReason = "verifier_failed"
		return out, nil
	}
	if reg.Outcome != domain.OutcomePass || acc.Outcome != domain.OutcomeFail {
		out.BaselineBlocked = true
		out.BaselineReason = "baseline_mismatch"
		out.Verdict = domain.VerdictInconclusive
		return out, nil
	}
	out.Verdict = domain.VerdictUnverified
	return out, nil
}

// Evaluate applies a patch to a clean baseline and judges the hidden checks.
func Evaluate(ctx context.Context, spec Spec, patch workspace.Patch) (Outcome, error) {
	base, err := stage(spec.Root, false)
	if err != nil {
		return Outcome{}, err
	}
	defer os.RemoveAll(base)
	dest, err := os.MkdirTemp("", "agentlab-candidate-")
	if err != nil {
		return Outcome{}, err
	}
	defer os.RemoveAll(dest)
	if _, err := workspace.Apply(base, dest, patch, workspace.Limits{}); err != nil {
		return Outcome{Verdict: domain.VerdictInconclusive, BaselineReason: err.Error(), EvidenceComplete: false, Checks: []domain.Check{{
			ID: "apply", Required: true, Kind: "policy", Outcome: domain.OutcomeError, FailureClass: domain.ClassVerifierFailed,
		}}}, nil
	}
	if err := copyHidden(spec.Root, dest); err != nil {
		return Outcome{}, err
	}
	reg := runGoTest(ctx, dest, spec.Regression)
	acc := runGoTest(ctx, dest, spec.Acceptance)
	checks := []domain.Check{reg, acc}
	return Outcome{Checks: checks, Verdict: domain.Judge(checks, true), EvidenceComplete: true}, nil
}

func stage(root string, withHidden bool) (string, error) {
	dest, err := os.MkdirTemp("", "agentlab-stage-")
	if err != nil {
		return "", err
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if rel == "solutions" || strings.HasPrefix(rel, "solutions"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if (rel == "hidden" || strings.HasPrefix(rel, "hidden"+string(filepath.Separator))) && !withHidden {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dest, rel)
		if rel == "hidden" || strings.HasPrefix(rel, "hidden"+string(filepath.Separator)) {
			target = filepath.Join(dest, strings.TrimPrefix(rel, "hidden"+string(filepath.Separator)))
			if rel == "hidden" {
				return nil
			}
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
	if err != nil {
		os.RemoveAll(dest)
		return "", err
	}
	return dest, nil
}

func copyHidden(root, dest string) error {
	hidden := filepath.Join(root, "hidden")
	return filepath.WalkDir(hidden, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(hidden, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func runGoTest(ctx context.Context, dir, regex string) domain.Check {
	check := domain.Check{ID: "regression", Required: true, Kind: "regression", MinCases: 1, Assurance: "inprocess"}
	if strings.Contains(regex, "Acceptance") {
		check.ID = "acceptance"
		check.Kind = "acceptance"
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-json", "-count=1", "-run", regex, ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		check.Outcome = domain.OutcomeError
		check.FailureClass = domain.ClassVerifierFailed
		return check
	}
	count, failed, skipped := parseGoTest(out)
	check.CaseCount = count
	if skipped > 0 {
		check.Outcome = domain.OutcomeSkipped
		check.SkippedCaseIDs = []string{fmt.Sprintf("skipped:%d", skipped)}
		return check
	}
	if count == 0 {
		check.Outcome = domain.OutcomeFail
		check.FailureClass = domain.ClassTestFailed
		return check
	}
	if failed || err != nil {
		check.Outcome = domain.OutcomeFail
		check.FailureClass = domain.ClassTestFailed
		return check
	}
	check.Outcome = domain.OutcomePass
	return check
}

func parseGoTest(out []byte) (ran int, failed bool, skipped int) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		var ev struct {
			Action string
			Test   string
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "run":
			ran++
		case "fail":
			failed = true
		case "skip":
			skipped++
		}
	}
	return ran, failed, skipped
}

// PatchFromFile builds a single-file replacement against a staged baseline.
func PatchFromFile(baselineFile, replacement string) (workspace.Patch, error) {
	baseDir, err := os.MkdirTemp("", "agentlab-basefile-")
	if err != nil {
		return workspace.Patch{}, err
	}
	defer os.RemoveAll(baseDir)
	nextDir, err := os.MkdirTemp("", "agentlab-nextfile-")
	if err != nil {
		return workspace.Patch{}, err
	}
	defer os.RemoveAll(nextDir)
	name := filepath.Base(baselineFile)
	if err := copyFile(baselineFile, filepath.Join(baseDir, name)); err != nil {
		return workspace.Patch{}, err
	}
	if err := copyFile(baselineFile, filepath.Join(nextDir, name)); err != nil {
		return workspace.Patch{}, err
	}
	if replacement != "" {
		body, err := os.ReadFile(replacement)
		if err != nil {
			return workspace.Patch{}, err
		}
		if err := os.WriteFile(filepath.Join(nextDir, name), body, 0o644); err != nil {
			return workspace.Patch{}, err
		}
	}
	return workspace.Collect(baseDir, nextDir, workspace.Limits{})
}

var errNoTests = errors.New("no tests")

func _() { _ = errNoTests }
