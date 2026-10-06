// Package verifier runs checks in a clean tree. Agent stdout is not evidence.
package verifier

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
	Logs             []Log
}

// Log is the raw verifier output for one check. It is not an agent log.
type Log struct {
	Name string
	Body []byte
	Exit int
}

// Precheck stages the baseline and requires regression to pass and acceptance
// to fail on cases that actually ran. Zero cases are a verifier error.
func Precheck(ctx context.Context, spec Spec) (Outcome, error) {
	dir, err := stage(spec.Root, true)
	if err != nil {
		return Outcome{}, err
	}
	defer os.RemoveAll(dir)
	reg, regLog, regNames := runGoTest(ctx, dir, spec.Regression)
	acc, accLog, _ := runGoTest(ctx, dir, spec.Acceptance)
	out := Outcome{EvidenceComplete: reg.FailureClass != domain.ClassVerifierFailed && acc.FailureClass != domain.ClassVerifierFailed}
	out.Checks = []domain.Check{reg, acc}
	out.Logs = []Log{{Name: reg.ID, Body: regLog, Exit: exitOf(reg)}, {Name: acc.ID, Body: accLog, Exit: exitOf(acc)}}
	if reg.FailureClass == domain.ClassVerifierFailed || acc.FailureClass == domain.ClassVerifierFailed || reg.CaseCount < 1 || acc.CaseCount < 1 {
		out.Verdict = domain.VerdictInconclusive
		out.BaselineBlocked = true
		out.BaselineReason = "verifier_failed"
		out.EvidenceComplete = false
		return out, nil
	}
	required := regressionNames(dir)
	if missing := missingNames(required, regNames); len(missing) > 0 {
		out.BaselineBlocked = true
		out.BaselineReason = "regression_cases_missing"
		out.Verdict = domain.VerdictInconclusive
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

// Evaluate applies a patch to a clean baseline, restores approved regression
// tests, injects hidden acceptance tests, and judges that tree.
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
	rebuilt, err := workspace.Apply(base, dest, patch, workspace.Limits{})
	if err != nil {
		return Outcome{Verdict: domain.VerdictInconclusive, BaselineReason: err.Error(), EvidenceComplete: false, Checks: []domain.Check{{
			ID: "apply", Required: true, Kind: "policy", Outcome: domain.OutcomeError, FailureClass: domain.ClassVerifierFailed,
		}}}, nil
	}
	if rebuilt != patch.Digest {
		return Outcome{Verdict: domain.VerdictInconclusive, BaselineReason: "tree digest mismatch", EvidenceComplete: false, Checks: []domain.Check{{
			ID: "apply", Required: true, Kind: "policy", Outcome: domain.OutcomeError, FailureClass: domain.ClassVerifierFailed,
		}}}, nil
	}
	if err := restoreRegression(base, dest); err != nil {
		return Outcome{}, err
	}
	if err := copyHidden(spec.Root, dest); err != nil {
		return Outcome{}, err
	}
	required := regressionNames(dest)
	reg, regLog, regNames := runGoTest(ctx, dest, spec.Regression)
	if missing := missingNames(required, regNames); len(missing) > 0 {
		reg.Outcome = domain.OutcomeFail
		reg.FailureClass = domain.ClassTestFailed
		reg.MissingCaseIDs = missing
	}
	acc, accLog, _ := runGoTest(ctx, dest, spec.Acceptance)
	checks := []domain.Check{reg, acc}
	complete := reg.FailureClass != domain.ClassVerifierFailed && acc.FailureClass != domain.ClassVerifierFailed
	return Outcome{
		Checks: checks, Verdict: domain.Judge(checks, complete), EvidenceComplete: complete,
		Logs: []Log{{Name: reg.ID, Body: regLog, Exit: exitOf(reg)}, {Name: acc.ID, Body: accLog, Exit: exitOf(acc)}},
	}, nil
}

// Calibrate checks the bundled correct, wrong and empty patches against this
// same spec. A tree without solutions/correct is not a calibration fixture.
func Calibrate(ctx context.Context, spec Spec) error {
	correct := filepath.Join(spec.Root, "solutions", "correct")
	wrong := filepath.Join(spec.Root, "solutions", "wrong")
	if _, err := os.Stat(correct); err != nil {
		return nil
	}
	if _, err := os.Stat(wrong); err != nil {
		return fmt.Errorf("solutions/wrong is missing")
	}
	base, err := stage(spec.Root, false)
	if err != nil {
		return err
	}
	defer os.RemoveAll(base)
	empty, err := workspace.Collect(base, base, workspace.Limits{})
	if err != nil {
		return err
	}
	correctPatch, err := overlayPatch(base, correct)
	if err != nil {
		return err
	}
	wrongPatch, err := overlayPatch(base, wrong)
	if err != nil {
		return err
	}
	pass, err := Evaluate(ctx, spec, correctPatch)
	if err != nil {
		return err
	}
	if pass.Verdict != domain.VerdictPass {
		return fmt.Errorf("correct patch verdict %s", pass.Verdict)
	}
	bad, err := Evaluate(ctx, spec, wrongPatch)
	if err != nil {
		return err
	}
	if bad.Verdict == domain.VerdictPass {
		return fmt.Errorf("wrong patch passed")
	}
	none, err := Evaluate(ctx, spec, empty)
	if err != nil {
		return err
	}
	if none.Verdict == domain.VerdictPass {
		return fmt.Errorf("empty patch passed")
	}
	return nil
}

func overlayPatch(base, solution string) (workspace.Patch, error) {
	next, err := os.MkdirTemp("", "agentlab-overlay-")
	if err != nil {
		return workspace.Patch{}, err
	}
	defer os.RemoveAll(next)
	if err := workspace.CopyBaseline(base, next, workspace.Limits{}); err != nil {
		return workspace.Patch{}, err
	}
	if err := overlayTree(solution, next); err != nil {
		return workspace.Patch{}, err
	}
	return workspace.Collect(base, next, workspace.Limits{})
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
		if skippedTree(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return placeFile(root, dest, rel, d)
	})
	if err != nil {
		os.RemoveAll(dest)
		return "", err
	}
	if withHidden {
		if err := copyHidden(root, dest); err != nil {
			os.RemoveAll(dest)
			return "", err
		}
	}
	return dest, nil
}

func skippedTree(rel string) bool {
	return rel == "solutions" || strings.HasPrefix(rel, "solutions"+string(filepath.Separator)) ||
		rel == "hidden" || strings.HasPrefix(rel, "hidden"+string(filepath.Separator))
}

func copyHidden(root, dest string) error {
	hidden := filepath.Join(root, "hidden")
	info, err := os.Lstat(hidden)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: hidden", workspace.ErrSpecial)
	}
	return copyTree(hidden, dest)
}

func copyTree(src, dest string) error {
	return walkCopy(src, dest, false)
}

func overlayTree(src, dest string) error {
	return walkCopy(src, dest, true)
}

func walkCopy(src, dest string, overwrite bool) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		return placeCopied(src, dest, rel, d, overwrite)
	})
}

func placeFile(root, dest, rel string, d os.DirEntry) error {
	return placeCopied(root, dest, rel, d, false)
}

func placeCopied(root, dest, rel string, d os.DirEntry, overwrite bool) error {
	if err := safeRel(rel); err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Join(root, rel))
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", workspace.ErrSymlink, rel)
	}
	target := filepath.Join(dest, rel)
	if d.IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", workspace.ErrSpecial, rel)
	}
	if existing, err := os.Lstat(target); err == nil {
		if existing.IsDir() {
			return fmt.Errorf("hidden path conflict: %s", rel)
		}
		prev, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		next, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return err
		}
		if !bytes.Equal(prev, next) {
			if !overwrite {
				return fmt.Errorf("hidden path conflict: %s", rel)
			}
			return copyFile(filepath.Join(root, rel), target)
		}
		return nil
	}
	return copyFile(filepath.Join(root, rel), target)
}

func restoreRegression(base, dest string) error {
	return filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		if !strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(body, []byte("func TestRegression")) {
			return nil
		}
		return copyFile(path, filepath.Join(dest, rel))
	})
}

var testName = regexp.MustCompile(`func (TestRegression[A-Za-z0-9_]*)\(`)

func regressionNames(root string) []string {
	var names []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range testName.FindAllSubmatch(body, -1) {
			names = append(names, string(m[1]))
		}
		return nil
	})
	return names
}

func missingNames(required, ran []string) []string {
	seen := map[string]struct{}{}
	for _, name := range ran {
		seen[name] = struct{}{}
	}
	var missing []string
	for _, name := range required {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func safeRel(rel string) error {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") || strings.Contains(rel, "\x00") {
		return workspace.ErrEscape
	}
	return nil
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

func runGoTest(ctx context.Context, dir, regex string) (domain.Check, []byte, []string) {
	check := domain.Check{ID: "regression", Required: true, Kind: "regression", MinCases: 1, Assurance: "inprocess"}
	if strings.Contains(regex, "Acceptance") {
		check.ID = "acceptance"
		check.Kind = "acceptance"
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, "go", "test", "-json", "-count=1", "-run", regex, ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	check.DurationMS = time.Since(start).Milliseconds()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	check.ExitCode = &code
	sum := sha256.Sum256(out)
	check.EvidenceDigest = hex.EncodeToString(sum[:])
	raw := append([]byte(nil), out...)
	if ctx.Err() != nil {
		check.Outcome = domain.OutcomeError
		check.FailureClass = domain.ClassVerifierFailed
		return check, raw, nil
	}
	if err != nil && len(out) == 0 {
		check.Outcome = domain.OutcomeError
		check.FailureClass = domain.ClassVerifierFailed
		return check, raw, nil
	}
	count, names, failed, skipped, parsed := parseGoTest(out)
	check.CaseCount = count
	if !parsed || (count == 0 && !failed) {
		if looksLikeCandidateBuild(out) {
			check.Outcome = domain.OutcomeFail
			check.FailureClass = domain.ClassTestFailed
			return check, raw, names
		}
		check.Outcome = domain.OutcomeError
		check.FailureClass = domain.ClassVerifierFailed
		check.MissingCaseIDs = []string{"no_cases"}
		return check, raw, names
	}
	if skipped > 0 {
		check.Outcome = domain.OutcomeSkipped
		check.SkippedCaseIDs = []string{fmt.Sprintf("skipped:%d", skipped)}
		return check, raw, names
	}
	if count == 0 {
		check.Outcome = domain.OutcomeError
		check.FailureClass = domain.ClassVerifierFailed
		check.MissingCaseIDs = []string{"no_cases"}
		return check, raw, names
	}
	if failed || err != nil {
		check.Outcome = domain.OutcomeFail
		check.FailureClass = domain.ClassTestFailed
		return check, raw, names
	}
	check.Outcome = domain.OutcomePass
	return check, raw, names
}

func looksLikeCandidateBuild(out []byte) bool {
	text := string(out)
	if strings.Contains(text, "toolchain unavailable") || strings.Contains(text, "command not found") {
		return false
	}
	return strings.Contains(text, ".go:") && (strings.Contains(text, "syntax error") || strings.Contains(text, "undefined:") || strings.Contains(text, "expected "))
}

func parseGoTest(out []byte) (ran int, names []string, failed bool, skipped int, parsed bool) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	sawJSON := false
	for sc.Scan() {
		var ev struct {
			Action string
			Test   string
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		sawJSON = true
		if ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "run":
			ran++
			names = append(names, ev.Test)
		case "fail":
			failed = true
		case "skip":
			skipped++
		}
	}
	if sc.Err() != nil {
		return 0, nil, false, 0, false
	}
	return ran, names, failed, skipped, sawJSON || len(bytes.TrimSpace(out)) == 0
}

func exitOf(check domain.Check) int {
	if check.ExitCode == nil {
		return -1
	}
	return *check.ExitCode
}

// PatchFromFile builds a single-file replacement against the staged module
// that contains baselineFile, so the patch base digest matches Evaluate.
func PatchFromFile(baselineFile, replacement string) (workspace.Patch, error) {
	root := filepath.Dir(baselineFile)
	baseDir, err := stage(root, false)
	if err != nil {
		return workspace.Patch{}, err
	}
	defer os.RemoveAll(baseDir)
	nextDir, err := os.MkdirTemp("", "agentlab-nextfile-")
	if err != nil {
		return workspace.Patch{}, err
	}
	defer os.RemoveAll(nextDir)
	if err := workspace.CopyBaseline(baseDir, nextDir, workspace.Limits{}); err != nil {
		return workspace.Patch{}, err
	}
	if replacement != "" {
		body, err := os.ReadFile(replacement)
		if err != nil {
			return workspace.Patch{}, err
		}
		rel, err := filepath.Rel(root, baselineFile)
		if err != nil {
			return workspace.Patch{}, err
		}
		target := filepath.Join(nextDir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return workspace.Patch{}, err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return workspace.Patch{}, err
		}
	}
	return workspace.Collect(baseDir, nextDir, workspace.Limits{})
}

var errNoTests = errors.New("no tests")

func _() { _ = errNoTests }
