// Package app is the use-case layer. It does not import Pi.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sllt/agentlab/internal/agent"
	"github.com/sllt/agentlab/internal/doctor"
	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/runner"
	"github.com/sllt/agentlab/internal/scheduler"
	"github.com/sllt/agentlab/internal/store/sqlite"
	"github.com/sllt/agentlab/internal/verifier"
	"github.com/sllt/agentlab/internal/workspace"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	Store       *sqlite.Store
	DataDir     string
	ExecPath    string
	GlobalLimit int
	WallSeconds int
	maint       atomic.Bool
}

type TaskSnapshot struct {
	Name         string `json:"name"`
	Prompt       string `json:"prompt"`
	SourceDir    string `json:"source_dir"`
	VerifierRoot string `json:"verifier_root"`
	BaseCommit   string `json:"base_commit"`
}

type ProfileSnapshot struct {
	Adapter     string `json:"adapter"`
	Model       string `json:"model"`
	FakeMode    string `json:"fake_mode"`
	Executor    string `json:"executor"`
	Network     string `json:"network"`
	DisplayName string `json:"display_name"`
}

var (
	ErrUnexecutable = errors.New("unexecutable")
	ErrMaintenance  = errors.New("maintenance")
)

func (s *Service) SetMaintenance(on bool) { s.maint.Store(on) }
func (s *Service) Maintenance() bool      { return s.maint.Load() }

func (s *Service) Setup(ctx context.Context, username, password string) error {
	n, err := s.Store.UserCount(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return sqlite.ErrConflict
	}
	if len(password) < 8 {
		return errors.New("password too short")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.Store.CreateUser(ctx, username, string(hash))
}

func (s *Service) Login(ctx context.Context, username, password string) (token, csrf string, err error) {
	id, hash, err := s.Store.UserByName(ctx, username)
	if err != nil {
		return "", "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", "", sqlite.ErrNotFound
	}
	token = domain.NewID("tok")
	csrf = domain.NewID("csrf")
	sum := sha256.Sum256([]byte(token))
	if err := s.Store.CreateSession(ctx, id, hex.EncodeToString(sum[:]), csrf); err != nil {
		return "", "", err
	}
	return token, csrf, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Store.DeleteSession(ctx, HashToken(token))
}

func (s *Service) PublishTask(ctx context.Context, taskID string) (sqlite.TaskVersion, error) {
	task, err := s.Store.GetTask(ctx, taskID)
	if err != nil {
		return sqlite.TaskVersion{}, err
	}
	var snap TaskSnapshot
	if err := json.Unmarshal([]byte(task.DraftJSON), &snap); err != nil {
		return sqlite.TaskVersion{}, err
	}
	if strings.Contains(task.DraftJSON, "REPLACE_") {
		return sqlite.TaskVersion{}, ErrUnexecutable
	}
	preJSON := `{"baseline":"exploratory"}`
	if snap.VerifierRoot != "" {
		pre, err := verifier.Precheck(ctx, verifier.GoChecks(snap.VerifierRoot))
		if err != nil {
			return sqlite.TaskVersion{}, err
		}
		body, err := json.Marshal(pre)
		if err != nil {
			return sqlite.TaskVersion{}, err
		}
		preJSON = string(body)
		if pre.BaselineBlocked {
			return sqlite.TaskVersion{}, ErrUnexecutable
		}
	}
	digest, err := domain.Digest(snap)
	if err != nil {
		return sqlite.TaskVersion{}, err
	}
	return s.Store.PublishTaskVersion(ctx, taskID, task.DraftJSON, digest, preJSON)
}

func (s *Service) PublishProfile(ctx context.Context, profileID string) (sqlite.ProfileVersion, error) {
	profile, err := s.Store.GetProfile(ctx, profileID)
	if err != nil {
		return sqlite.ProfileVersion{}, err
	}
	var snap ProfileSnapshot
	if err := json.Unmarshal([]byte(profile.DraftJSON), &snap); err != nil {
		return sqlite.ProfileVersion{}, err
	}
	if snap.Adapter == "" {
		return sqlite.ProfileVersion{}, ErrUnexecutable
	}
	if snap.Executor == "" {
		snap.Executor = "native-trusted"
	}
	if snap.Network == "" || snap.Network == "restricted" {
		snap.Network = "unrestricted"
	}
	report := doctor.Static(ctx, snap.Adapter, "", snap.Model, false)
	doc, err := json.Marshal(report)
	if err != nil {
		return sqlite.ProfileVersion{}, err
	}
	if !report.StaticPassed || strings.Contains(profile.DraftJSON, "REPLACE_") {
		return sqlite.ProfileVersion{}, ErrUnexecutable
	}
	body, err := json.Marshal(snap)
	if err != nil {
		return sqlite.ProfileVersion{}, err
	}
	digest, err := domain.Digest(snap)
	if err != nil {
		return sqlite.ProfileVersion{}, err
	}
	return s.Store.PublishProfileVersion(ctx, profileID, string(body), digest, string(doc))
}

type ExperimentRequest struct {
	Actor             string
	IdempotencyKey    string
	Mode              string
	TaskVersionIDs    []string
	ProfileVersionIDs []string
	Repetitions       int
	Protocol          string
}

func (s *Service) SubmitExperiment(ctx context.Context, req ExperimentRequest) (sqlite.Experiment, error) {
	if s.Maintenance() {
		return sqlite.Experiment{}, ErrMaintenance
	}
	if req.Protocol == "" {
		req.Protocol = "single-pass-v1"
	}
	if req.Mode == "" {
		req.Mode = domain.ModeAgentProfile
	}
	if req.Repetitions <= 0 || len(req.TaskVersionIDs) == 0 || len(req.ProfileVersionIDs) == 0 || req.IdempotencyKey == "" {
		return sqlite.Experiment{}, ErrUnexecutable
	}
	for _, id := range req.TaskVersionIDs {
		if _, err := s.Store.GetTaskVersion(ctx, id); err != nil {
			return sqlite.Experiment{}, err
		}
	}
	for _, id := range req.ProfileVersionIDs {
		if _, err := s.Store.GetProfileVersion(ctx, id); err != nil {
			return sqlite.Experiment{}, err
		}
	}
	var trials []sqlite.NewTrial
	for _, taskID := range req.TaskVersionIDs {
		for _, profileID := range req.ProfileVersionIDs {
			for i := 1; i <= req.Repetitions; i++ {
				trials = append(trials, sqlite.NewTrial{TaskVersionID: taskID, ProfileVersionID: profileID, RepeatIndex: i})
			}
		}
	}
	if len(trials) != domain.TrialCount(len(req.TaskVersionIDs), len(req.ProfileVersionIDs), req.Repetitions) {
		return sqlite.Experiment{}, ErrUnexecutable
	}
	plan, err := json.Marshal(req)
	if err != nil {
		return sqlite.Experiment{}, err
	}
	digest, err := domain.Digest(json.RawMessage(plan))
	if err != nil {
		return sqlite.Experiment{}, err
	}
	return s.Store.SubmitExperiment(ctx, req.Actor, req.IdempotencyKey, digest, req.Mode, req.Protocol, string(plan), trials)
}

func (s *Service) Loop(ctx context.Context) error {
	_ = s.Recover(ctx)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if s.Maintenance() {
				continue
			}
			_ = s.Pump(ctx, s.limit())
		}
	}
}

func (s *Service) limit() int {
	if s.GlobalLimit <= 0 {
		return 1
	}
	return s.GlobalLimit
}

func (s *Service) wall() int {
	if s.WallSeconds <= 0 {
		return 20
	}
	return s.WallSeconds
}

// Pump starts queued trials up to the global limit. A single trial failure
// does not stop the loop.
func (s *Service) Pump(ctx context.Context, globalLimit int) error {
	if globalLimit <= 0 {
		globalLimit = s.limit()
	}
	queued, err := s.Store.ListQueued(ctx)
	if err != nil {
		return err
	}
	for _, trial := range queued {
		if s.Maintenance() {
			return nil
		}
		active, err := s.Store.GlobalActiveAttempts(ctx)
		if err != nil {
			return err
		}
		if active >= globalLimit {
			return nil
		}
		account, err := s.Store.ProfileAccount(ctx, trial.ProfileVersionID)
		if err != nil {
			return err
		}
		attempt, err := s.Store.CreateAttempt(ctx, trial.ID, account, `{"executor":"native-trusted","network":"unrestricted"}`, "", globalLimit, 1)
		if err != nil {
			if errors.Is(err, sqlite.ErrConflict) || errors.Is(err, sqlite.ErrCapacity) {
				continue
			}
			return err
		}
		if err := s.execute(ctx, trial, attempt); err != nil {
			_ = s.Store.FinishAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecAborted), string(domain.VerdictInconclusive), string(domain.CleanupClean), err.Error())
		}
	}
	return nil
}

func (s *Service) execute(ctx context.Context, trial sqlite.Trial, attempt sqlite.Attempt) error {
	tv, err := s.Store.GetTaskVersion(ctx, trial.TaskVersionID)
	if err != nil {
		return err
	}
	var snap TaskSnapshot
	if err := json.Unmarshal([]byte(tv.SnapshotJSON), &snap); err != nil {
		return err
	}
	pv, err := s.Store.GetProfileVersion(ctx, trial.ProfileVersionID)
	if err != nil {
		return err
	}
	var profile ProfileSnapshot
	if err := json.Unmarshal([]byte(pv.SnapshotJSON), &profile); err != nil {
		return err
	}
	if profile.FakeMode == "" {
		profile.FakeMode = "success"
	}
	if profile.Adapter == "" {
		profile.Adapter = "fixture"
	}
	work := filepath.Join(s.DataDir, "attempts", attempt.ID, "work")
	base := filepath.Join(s.DataDir, "attempts", attempt.ID, "baseline")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	src := snap.SourceDir
	if src == "" {
		src = snap.VerifierRoot
	}
	if src == "" {
		if err := os.MkdirAll(base, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(work, "README.txt"), []byte(snap.Prompt), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(base, "README.txt"), []byte(snap.Prompt), 0o644); err != nil {
			return err
		}
	} else {
		if err := workspace.CopyBaseline(src, base, workspace.Limits{}); err != nil {
			return err
		}
		if err := workspace.CopyBaseline(src, work, workspace.Limits{}); err != nil {
			return err
		}
		stripPrivate(base)
		stripPrivate(work)
	}
	exe := s.ExecPath
	if exe == "" {
		exe, err = os.Executable()
		if err != nil {
			return err
		}
	}
	env := map[string]string{"AGENTLAB_FAKE_MODE": profile.FakeMode}
	if snap.VerifierRoot != "" && (profile.FakeMode == "success" || profile.FakeMode == "fail") {
		sub := "correct"
		if profile.FakeMode == "fail" {
			sub = "wrong"
		}
		dir := filepath.Join(snap.VerifierRoot, "solutions", sub)
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			env["AGENTLAB_FAKE_MODE"] = "copy"
			env["AGENTLAB_SOLUTION_DIR"] = dir
		}
	}
	spec := runner.Spec{
		SchemaVersion: "agentlab.exec/v1", AttemptID: attempt.ID, Fence: attempt.Fence, Token: attempt.Token,
		WorkDir: work, BaselineDir: base, Executor: "native-trusted", Network: "unrestricted", WallSeconds: s.wall(),
		Launch: agent.LaunchSpec{Executable: exe, Args: []string{"fake-agent"}, Env: env, WorkDir: work},
		Prompt: snap.Prompt,
	}
	if profile.Adapter != "" && profile.Adapter != "fixture" {
		ad, ok := agent.ByName(profile.Adapter)
		if !ok {
			return errors.New("unknown adapter")
		}
		spec.Launch, err = ad.BuildLaunch(ctx, agent.LaunchRequest{ModelID: profile.Model, Prompt: snap.Prompt, WorkDir: work, Env: env})
		if err != nil {
			return err
		}
	}
	pr, pw := io.Pipe()
	start, err := json.Marshal(map[string]any{"type": "Start", "token": spec.Token, "spec": spec})
	if err != nil {
		return err
	}
	var output bytesBuffer
	errCh := make(chan error, 1)
	go func() { errCh <- runner.Serve(ctx, pr, &output) }()
	if _, err := pw.Write(append(start, '\n')); err != nil {
		return err
	}
	watchStop := make(chan struct{})
	go s.watch(ctx, watchStop, pw, trial.ID, attempt)
	runErr := <-errCh
	close(watchStop)
	_ = pw.Close()
	dir := filepath.Join(s.DataDir, "attempts", attempt.ID)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "events.ndjson"), output.Bytes(), 0o644)
	s.indexEvents(ctx, attempt.ID, output.Bytes())
	if runErr != nil {
		return runErr
	}
	_ = s.noteStarted(ctx, attempt)
	current, err := s.Store.GetAttempt(ctx, attempt.ID)
	if err != nil {
		return err
	}
	fresh, err := s.Store.GetTrial(ctx, trial.ID)
	if err != nil {
		return err
	}
	verdict, cleanup, reason := s.judge(ctx, snap, profile, tv.Digest, attempt.ID, base, work, output.Bytes())
	if fresh.CancelRequested || current.State == string(domain.ExecCancelling) {
		if current.State == string(domain.ExecRunning) {
			_ = s.Store.AdvanceAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecCancelling))
		} else if current.State == string(domain.ExecPreparing) {
			_ = s.Store.AdvanceAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecCancelling))
		}
		return s.Store.FinishAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecCancelled), string(domain.VerdictUnverified), string(cleanup), "cancelled")
	}
	if current.State == string(domain.ExecPreparing) {
		return s.Store.FinishAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecAborted), string(domain.VerdictInconclusive), string(cleanup), "agent_not_started")
	}
	if current.State == string(domain.ExecRunning) {
		if err := s.Store.AdvanceAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecCollecting)); err != nil {
			return err
		}
		if err := s.Store.AdvanceAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecVerifying)); err != nil {
			return err
		}
	}
	if reason != "" && verdict == domain.VerdictPass && profile.FakeMode == "forge" {
		verdict = domain.VerdictInconclusive
	}
	return s.Store.FinishAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecCompleted), string(verdict), string(cleanup), reason)
}

func (s *Service) watch(ctx context.Context, stop <-chan struct{}, pw *io.PipeWriter, trialID string, attempt sqlite.Attempt) {
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	marked := false
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			s.writeControl(pw, "Shutdown", "context")
			return
		case <-ticker.C:
			if !marked {
				if s.noteStarted(ctx, attempt) == nil {
					marked = true
				}
			}
			trial, err := s.Store.GetTrial(ctx, trialID)
			if err == nil && trial.CancelRequested {
				s.writeControl(pw, "Cancel", "user")
				return
			}
		}
	}
}

func (s *Service) writeControl(pw *io.PipeWriter, typ, reason string) {
	b, err := json.Marshal(map[string]string{"type": typ, "reason": reason})
	if err != nil {
		return
	}
	_, _ = pw.Write(append(b, '\n'))
}

func (s *Service) noteStarted(ctx context.Context, attempt sqlite.Attempt) error {
	raw, err := os.ReadFile(filepath.Join(s.DataDir, "attempts", attempt.ID, "work", "journal.json"))
	if err != nil || !bytesContains(raw, `"agent_start":"started"`) {
		return errors.New("not started")
	}
	err = s.Store.MarkAgentStarted(ctx, attempt.ID, attempt.Fence)
	if errors.Is(err, sqlite.ErrConflict) {
		return nil
	}
	return err
}

func (s *Service) judge(ctx context.Context, snap TaskSnapshot, profile ProfileSnapshot, verifierDigest, attemptID, base, work string, events []byte) (domain.Verdict, domain.CleanupState, string) {
	cleanup := domain.CleanupClean
	if bytesContains(events, `"state":"quarantined"`) {
		cleanup = domain.CleanupQuarantined
	}
	if snap.VerifierRoot == "" {
		return domain.VerdictUnverified, cleanup, ""
	}
	patch, err := workspace.Collect(base, work, workspace.Limits{})
	if err != nil {
		return domain.VerdictInconclusive, cleanup, "collect_failed"
	}
	out, err := verifier.Evaluate(ctx, verifier.GoChecks(snap.VerifierRoot), patch)
	if err != nil {
		return domain.VerdictInconclusive, cleanup, "verifier_error"
	}
	for _, check := range out.Checks {
		_ = s.Store.InsertCheck(ctx, attemptID, verifierDigest, check, 1)
	}
	if profile.FakeMode == "forge" && out.Verdict == domain.VerdictPass {
		return domain.VerdictInconclusive, cleanup, "forged output cannot pass"
	}
	reason := ""
	if !out.EvidenceComplete {
		reason = "evidence_incomplete"
	}
	_ = s.recordPatch(ctx, attemptID, patch)
	return out.Verdict, cleanup, reason
}

func (s *Service) recordPatch(ctx context.Context, attemptID string, patch workspace.Patch) error {
	dir := filepath.Join(s.DataDir, "attempts", attemptID)
	path := filepath.Join(dir, "patch.json")
	body, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	_, err = s.Store.InsertArtifact(ctx, sqlite.Artifact{
		AttemptID: attemptID, Kind: "patch", Digest: hex.EncodeToString(sum[:]),
		Bytes: int64(len(body)), StorageKey: path, Status: "present",
	})
	return err
}

func (s *Service) indexEvents(ctx context.Context, attemptID string, raw []byte) {
	for _, line := range bytesSplit(raw) {
		var ev runner.Event
		if err := json.Unmarshal(line, &ev); err != nil || ev.Sequence == 0 {
			continue
		}
		_ = s.Store.InsertEvent(ctx, attemptID, ev.Sequence, ev.Origin, ev.Type, ev.ObservedAt)
	}
}

func (s *Service) Recover(ctx context.Context) error {
	open, err := s.Store.OpenAttempts(ctx)
	if err != nil {
		return err
	}
	for _, attempt := range open {
		journalPath := filepath.Join(s.DataDir, "attempts", attempt.ID, "work", "journal.json")
		raw, readErr := os.ReadFile(journalPath)
		started := attempt.AgentStarted
		pid := 0
		if readErr == nil {
			var doc struct {
				AgentStart string `json:"agent_start"`
				PID        int    `json:"pid"`
			}
			_ = json.Unmarshal(raw, &doc)
			if doc.AgentStart == "started" {
				started = true
			}
			pid = doc.PID
		}
		alive := pid > 0 && syscall.Kill(pid, 0) == nil
		if alive {
			_ = syscall.Kill(-pid, syscall.SIGTERM)
			time.Sleep(50 * time.Millisecond)
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			alive = syscall.Kill(pid, 0) == nil
		}
		decision := scheduler.Reconcile(started, alive, true, 0)
		if alive {
			decision.Cleanup = domain.CleanupQuarantined
		}
		_ = s.Store.FinishAttempt(ctx, attempt.ID, attempt.Fence, string(decision.State), string(decision.Verdict), string(decision.Cleanup), decision.Reason)
	}
	return nil
}

func (s *Service) Events(attemptID string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.DataDir, "attempts", attemptID, "events.ndjson"))
}

func stripPrivate(root string) {
	_ = os.RemoveAll(filepath.Join(root, "hidden"))
	_ = os.RemoveAll(filepath.Join(root, "solutions"))
}

type bytesBuffer struct{ b []byte }

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.b = append(b.b, p...)
	return len(p), nil
}
func (b *bytesBuffer) Bytes() []byte { return append([]byte(nil), b.b...) }

func bytesContains(b []byte, s string) bool {
	return len(b) > 0 && index(string(b), s) >= 0
}

func index(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

func bytesSplit(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
