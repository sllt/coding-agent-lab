// Package app is the use-case layer. It does not import Pi.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	SourceDir      string `json:"source_dir"`
	VerifierRoot   string `json:"verifier_root"`
	BaseCommit     string `json:"base_commit"`
	ContentDigest  string `json:"content_digest,omitempty"`
	SnapshotDir    string `json:"snapshot_dir,omitempty"`
	VerifierDigest string `json:"verifier_digest,omitempty"`
}

type ProfileSnapshot struct {
	Adapter      string `json:"adapter"`
	Model        string `json:"model"`
	FakeMode     string `json:"fake_mode"`
	Executor     string `json:"executor"`
	Network      string `json:"network"`
	DisplayName  string `json:"display_name"`
	Executable   string `json:"executable,omitempty"`
	ApproveTools bool   `json:"approve_tools,omitempty"`
}

var (
	ErrUnexecutable   = errors.New("unexecutable")
	ErrMaintenance    = errors.New("maintenance")
	ErrAdminExists    = errors.New("admin exists")
)

func (s *Service) SetMaintenance(on bool) { s.maint.Store(on) }
func (s *Service) Maintenance() bool      { return s.maint.Load() }

func (s *Service) Setup(ctx context.Context, username, password string) error {
	n, err := s.Store.UserCount(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrAdminExists
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
	_ = s.Store.Audit(ctx, id, "login", id, `{}`)
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
	raw := []byte(task.DraftJSON)
	snap, err := decodeTaskDraft(raw)
	if err != nil {
		return sqlite.TaskVersion{}, ErrUnexecutable
	}
	if strings.TrimSpace(snap.Name) == "" || strings.TrimSpace(snap.Prompt) == "" || strings.Contains(task.DraftJSON, "REPLACE_") {
		return sqlite.TaskVersion{}, ErrUnexecutable
	}
	project, err := s.Store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return sqlite.TaskVersion{}, err
	}
	roots := allowedRoots(project.SourceSpec)
	deny := []string{}
	if s.DataDir != "" {
		deny = append(deny, s.DataDir)
	}
	origin := snap.SourceDir
	if origin == "" {
		origin = snap.VerifierRoot
	}
	if origin != "" {
		if err := workspace.WithinAllowed(origin, roots, deny); err != nil {
			return sqlite.TaskVersion{}, ErrUnexecutable
		}
	}
	if snap.VerifierRoot != "" && !samePath(snap.VerifierRoot, origin) {
		if err := workspace.WithinAllowed(snap.VerifierRoot, roots, deny); err != nil {
			return sqlite.TaskVersion{}, ErrUnexecutable
		}
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
	if origin != "" {
		dir, digest, commit, err := workspace.Freeze(origin, filepath.Join(s.DataDir, "task-snapshots"))
		if err != nil {
			return sqlite.TaskVersion{}, err
		}
		snap.SnapshotDir = dir
		snap.ContentDigest = digest
		if snap.BaseCommit == "" {
			snap.BaseCommit = commit
		}
		if snap.VerifierRoot != "" && samePath(snap.VerifierRoot, origin) {
			snap.VerifierRoot = dir
			snap.VerifierDigest = digest
		}
	}
	if snap.VerifierRoot != "" && snap.SnapshotDir != "" && !samePath(snap.VerifierRoot, snap.SnapshotDir) && !strings.Contains(snap.VerifierRoot, snap.ContentDigest) {
		dir, digest, _, err := workspace.Freeze(snap.VerifierRoot, filepath.Join(s.DataDir, "task-snapshots"))
		if err != nil {
			return sqlite.TaskVersion{}, err
		}
		snap.VerifierRoot = dir
		snap.VerifierDigest = digest
	}
	body, err := json.Marshal(snap)
	if err != nil {
		return sqlite.TaskVersion{}, err
	}
	digest, err := domain.Digest(snap)
	if err != nil {
		return sqlite.TaskVersion{}, err
	}
	return s.Store.PublishTaskVersion(ctx, taskID, string(body), digest, preJSON)
}

func (s *Service) PublishProfile(ctx context.Context, profileID string) (sqlite.ProfileVersion, error) {
	profile, err := s.Store.GetProfile(ctx, profileID)
	if err != nil {
		return sqlite.ProfileVersion{}, err
	}
	raw := []byte(profile.DraftJSON)
	snap, err := decodeProfileDraft(raw)
	if err != nil {
		return sqlite.ProfileVersion{}, ErrUnexecutable
	}
	if snap.Adapter == "" {
		return sqlite.ProfileVersion{}, ErrUnexecutable
	}
	requestedExecutor := snap.Executor
	if snap.Executor == "" || snap.Executor == "docker" {
		snap.Executor = "native-trusted"
	}
	if snap.Network == "" || snap.Network == "restricted" {
		snap.Network = "unrestricted"
	}
	report := doctor.Static(ctx, snap.Adapter, snap.Executable, snap.Model, false)
	report.Executor = snap.Executor
	if snap.Adapter == "cursor" && !snap.ApproveTools {
		report.Note = strings.TrimSpace(report.Note + " 未批准 --force，Cursor 不能写文件。")
	}
	if requestedExecutor == "docker" {
		report.Note = strings.TrimSpace(report.Note + " 请求的执行器是 docker。本机没有把它当成已验证沙箱，发布快照改记为 native-trusted。")
	}
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
	if req.Repetitions <= 0 || req.Repetitions > domain.MaxRepetitions || len(req.TaskVersionIDs) == 0 || len(req.ProfileVersionIDs) == 0 || req.IdempotencyKey == "" {
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
		attempt, err := s.Store.CreateAttempt(ctx, trial.ID, account, NativeRuntime(), "", globalLimit, 1)
		if err != nil {
			if errors.Is(err, sqlite.ErrConflict) || errors.Is(err, sqlite.ErrCapacity) || errors.Is(err, sqlite.ErrAccountBlocked) {
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
	if profile.Adapter == "" {
		profile.Adapter = "fixture"
	}
	if profile.Adapter == "fixture" && profile.FakeMode == "" {
		profile.FakeMode = "success"
	}
	work := filepath.Join(s.DataDir, "attempts", attempt.ID, "work")
	base := filepath.Join(s.DataDir, "attempts", attempt.ID, "baseline")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	src := snap.SnapshotDir
	if (snap.SourceDir != "" || snap.VerifierRoot != "") && src == "" {
		return errors.New("unfrozen task snapshot")
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
	env := map[string]string{}
	if profile.Adapter == "fixture" {
		env["AGENTLAB_FAKE_MODE"] = profile.FakeMode
		solutionRoot := snap.VerifierRoot
		if solutionRoot == "" {
			solutionRoot = snap.SourceDir
		}
		if solutionRoot != "" && (profile.FakeMode == "success" || profile.FakeMode == "fail") {
			sub := "correct"
			if profile.FakeMode == "fail" {
				sub = "wrong"
			}
			dir := filepath.Join(solutionRoot, "solutions", sub)
			if abs, err := filepath.Abs(dir); err == nil {
				dir = abs
			}
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				env["AGENTLAB_FAKE_MODE"] = "copy"
				env["AGENTLAB_SOLUTION_DIR"] = dir
			}
		}
	}
	identityToken := domain.NewID("idn")
	_ = s.Store.UpdateRuntime(ctx, attempt.ID, withIdentity(attempt.RuntimeJSON, identityToken, 0, ""))
	spec := runner.Spec{
		SchemaVersion: "agentlab.exec/v1", AttemptID: attempt.ID, Fence: attempt.Fence, Token: attempt.Token,
		WorkDir: work, BaselineDir: base, Executor: "native-trusted", Network: "unrestricted", WallSeconds: s.wall(),
		Launch: agent.LaunchSpec{Executable: exe, Args: []string{"fake-agent"}, Env: env, WorkDir: work},
		Prompt: snap.Prompt, IdentityToken: identityToken,
	}
	if profile.Adapter != "" && profile.Adapter != "fixture" {
		ad, ok := agent.ByName(profile.Adapter)
		if !ok {
			return errors.New("unknown adapter")
		}
		spec.Launch, err = ad.BuildLaunch(ctx, agent.LaunchRequest{ModelID: profile.Model, Prompt: snap.Prompt, WorkDir: work, Env: env, ApproveTools: profile.ApproveTools})
		if err != nil {
			return err
		}
		if profile.Executable != "" {
			spec.Launch.Executable = profile.Executable
		}
		spec.Launch.Env = env
	}
	pr, pw := io.Pipe()
	start, err := json.Marshal(map[string]any{"type": "Start", "token": spec.Token, "spec": spec})
	if err != nil {
		return err
	}
	attemptDir := filepath.Join(s.DataDir, "attempts", attempt.ID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return err
	}
	eventFile, err := os.OpenFile(filepath.Join(attemptDir, "events.ndjson"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	output := &eventSink{file: eventFile}
	errCh := make(chan error, 1)
	go func() { errCh <- runner.Serve(ctx, pr, output) }()
	if _, err := pw.Write(append(start, '\n')); err != nil {
		_ = eventFile.Close()
		return err
	}
	watchStop := make(chan struct{})
	go s.watch(ctx, watchStop, pw, trial.ID, attempt, identityToken)
	runErr := <-errCh
	close(watchStop)
	_ = pw.Close()
	output.finish()
	_ = eventFile.Sync()
	_ = eventFile.Close()
	events := output.Bytes()
	s.indexEvents(ctx, attempt.ID, events)
	s.recordUsage(ctx, attempt.ID, events)
	s.persistIdentity(ctx, attempt, identityToken)
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
	cancelled := fresh.CancelRequested || current.State == string(domain.ExecCancelling)
	logBody, _ := os.ReadFile(filepath.Join(work, "agent.log"))
	if !cancelled && authenticationFailed(logBody) {
		_ = s.Store.BlockAccount(ctx, attempt.AccountID, "authentication_failed")
		_ = s.Store.Audit(ctx, "control", "block_account", attempt.AccountID, `{"reason":"authentication_failed"}`)
		return s.Store.FinishAttempt(ctx, attempt.ID, attempt.Fence, string(domain.ExecAborted), string(domain.VerdictInconclusive), string(domain.CleanupClean), "authentication_failed")
	}
	verdict, cleanup, reason := s.judge(ctx, snap, profile, tv.Digest, attempt.ID, base, work, events)
	if cancelled {
		if current.State == string(domain.ExecRunning) || current.State == string(domain.ExecPreparing) {
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

func (s *Service) watch(ctx context.Context, stop <-chan struct{}, pw *io.PipeWriter, trialID string, attempt sqlite.Attempt, identityToken string) {
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	marked := false
	savedIdentity := false
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			s.writeControl(pw, "Shutdown", "context")
			return
		case <-ticker.C:
			if !savedIdentity {
				if s.persistIdentity(ctx, attempt, identityToken) {
					savedIdentity = true
				}
			}
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
	if runnerQuarantined(events) {
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
	_ = s.Store.RepairExperimentStates(ctx)
	open, err := s.Store.OpenAttempts(ctx)
	if err != nil {
		return err
	}
	for _, attempt := range open {
		started := attempt.AgentStarted
		pid, startTime, confirmedRecord := runtimeIdentity(attempt.RuntimeJSON)
		confirmed := false
		alive := false
		if confirmedRecord {
			kernel, ok := runner.ProcStartTime(pid)
			processExists := syscall.Kill(pid, 0) == nil
			if ok && kernel == startTime && processExists {
				confirmed = true
				alive = true
				_ = syscall.Kill(-pid, syscall.SIGTERM)
				time.Sleep(50 * time.Millisecond)
				if syscall.Kill(pid, 0) == nil {
					_ = syscall.Kill(-pid, syscall.SIGKILL)
				}
				alive = syscall.Kill(pid, 0) == nil
			} else if processExists {
				// PID was reused or the start time does not match. Do not signal.
				confirmed = false
				alive = true
			}
		} else if started {
			// The journal is agent-writable. Without a control-plane identity, quarantine.
			alive = true
			confirmed = false
		}
		decision := scheduler.Reconcile(started, alive, confirmed, 0)
		if alive && !confirmed {
			decision.Cleanup = domain.CleanupQuarantined
			decision.Reason = "identity_unconfirmed"
		}
		if alive && confirmed {
			decision.Cleanup = domain.CleanupQuarantined
			decision.Reason = "identity_confirmed_but_still_alive"
		}
		if decision.Cleanup == domain.CleanupQuarantined {
			_ = s.Store.Audit(ctx, "control", "cleanup", attempt.ID, `{"cleanup":"quarantined"}`)
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

const (
	maxEventMemory = 8 << 20
	maxEventFile   = 64 << 20
)

// eventSink keeps runner lines and at most 8 MiB of agent lines in memory,
// while the disk log can grow to the per-attempt file cap.
type eventSink struct {
	mu         sync.Mutex
	mem        []byte
	agentBytes int
	truncated  bool
	pending    []byte
	file       *os.File
	fileN      int64
	noted      bool
}

func (e *eventSink) Write(p []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending = append(e.pending, p...)
	for {
		i := bytes.IndexByte(e.pending, '\n')
		if i < 0 {
			break
		}
		line := append([]byte(nil), e.pending[:i+1]...)
		e.pending = e.pending[i+1:]
		e.keep(line)
	}
	return len(p), nil
}

func (e *eventSink) keep(line []byte) {
	if e.file != nil && e.fileN < maxEventFile {
		n, _ := e.file.Write(line)
		e.fileN += int64(n)
	}
	runnerLine := bytes.Contains(line, []byte(`"origin":"runner"`))
	if runnerLine || e.agentBytes+len(line) <= maxEventMemory {
		e.mem = append(e.mem, line...)
		if !runnerLine {
			e.agentBytes += len(line)
		}
		return
	}
	e.truncated = true
}

func (e *eventSink) finish() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.pending) > 0 {
		e.keep(append(append([]byte(nil), e.pending...), '\n'))
		e.pending = nil
	}
	if e.truncated && !e.noted {
		note := []byte("{\"origin\":\"runner\",\"type\":\"Truncated\",\"payload\":{\"reason\":\"event_buffer_8MiB\"}}\n")
		e.mem = append(e.mem, note...)
		if e.file != nil {
			_, _ = e.file.Write(note)
		}
		e.noted = true
	}
}

func (e *eventSink) Bytes() []byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]byte(nil), e.mem...)
}

func (s *Service) persistIdentity(ctx context.Context, attempt sqlite.Attempt, token string) bool {
	id, ok := runner.RecallIdentity(token)
	if !ok {
		return false
	}
	current, err := s.Store.GetAttempt(ctx, attempt.ID)
	base := attempt.RuntimeJSON
	if err == nil && current.RuntimeJSON != "" {
		base = current.RuntimeJSON
	}
	return s.Store.UpdateRuntime(ctx, attempt.ID, withIdentity(base, token, id.PID, id.StartTime)) == nil
}

func NativeRuntime() string {
	return withIdentity(`{}`, "", 0, "")
}

func withIdentity(runtime, token string, pid int, start string) string {
	m := map[string]any{}
	_ = json.Unmarshal([]byte(runtime), &m)
	if m == nil {
		m = map[string]any{}
	}
	m["executor"] = "native-trusted"
	m["network"] = "unrestricted"
	m["home_inherited"] = true
	m["isolation"] = "observational/native"
	m["limitation"] = "继承了控制进程的 HOME，未做隔离加载"
	if token != "" {
		m["identity_token"] = token
	}
	if pid > 0 {
		m["pid"] = pid
		m["starttime"] = start
	}
	body, err := json.Marshal(m)
	if err != nil {
		return NativeRuntime()
	}
	return string(body)
}

func runtimeIdentity(runtime string) (pid int, start string, ok bool) {
	var doc struct {
		PID       int    `json:"pid"`
		StartTime string `json:"starttime"`
	}
	if json.Unmarshal([]byte(runtime), &doc) != nil || doc.PID <= 0 || doc.StartTime == "" {
		return 0, "", false
	}
	return doc.PID, doc.StartTime, true
}

func runnerQuarantined(events []byte) bool {
	for _, line := range bytesSplit(events) {
		var ev struct {
			Origin  string `json:"origin"`
			Type    string `json:"type"`
			Payload struct {
				State string `json:"state"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		if ev.Origin == "runner" && ev.Type == "CleanupCompleted" && ev.Payload.State == "quarantined" {
			return true
		}
	}
	return false
}

func authenticationFailed(logBody []byte) bool {
	text := strings.ToLower(string(logBody))
	for _, needle := range []string{"authentication failed", "not logged in", "unauthenticated", "login required", "auth_error"} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func (s *Service) recordUsage(ctx context.Context, attemptID string, raw []byte) {
	for _, line := range bytesSplit(raw) {
		var ev struct {
			Sequence int64           `json:"sequence"`
			Origin   string          `json:"origin"`
			Type     string          `json:"type"`
			Payload  json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(line, &ev) != nil || ev.Type != "usage" || ev.Origin != "agent_observation" {
			continue
		}
		var payload map[string]json.RawMessage
		if json.Unmarshal(ev.Payload, &payload) != nil {
			continue
		}
		usage := domain.Usage{
			AttemptID:     attemptID,
			Source:        "agent_observation",
			SourceEventID: strconv.FormatInt(ev.Sequence, 10),
			BillingPath:   "reported",
			Confidence:    "reported",
		}
		usage.InputTokens = optInt(payload["input_tokens"])
		usage.OutputTokens = optInt(payload["output_tokens"])
		usage.CachedInputTokens = optInt(payload["cached_input_tokens"])
		usage.CostMicroUSD = optInt(payload["cost_microusd"])
		if rawCurrency, ok := payload["currency"]; ok && string(rawCurrency) != "null" {
			var currency string
			if json.Unmarshal(rawCurrency, &currency) == nil && currency != "" {
				usage.Currency = &currency
			}
		}
		if rawCum, ok := payload["is_cumulative"]; ok {
			var cum bool
			if json.Unmarshal(rawCum, &cum) == nil {
				usage.IsCumulative = cum
			}
		}
		_ = s.Store.UpsertUsage(ctx, attemptID, usage)
	}
}

func optInt(raw json.RawMessage) *int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var n int64
	if json.Unmarshal(raw, &n) != nil {
		return nil
	}
	return &n
}

func decodeTaskDraft(raw []byte) (TaskSnapshot, error) {
	if bytes.Contains(raw, []byte(domain.TaskSchema)) || bytes.Contains(raw, []byte(`"kind":"TaskDraft"`)) {
		draft, err := domain.DecodeTask(raw)
		if err != nil {
			return TaskSnapshot{}, err
		}
		if blockers := domain.PublishBlockers(draft); len(blockers) > 0 {
			return TaskSnapshot{}, errors.New(strings.Join(blockers, ","))
		}
		return TaskSnapshot{Name: draft.Name, Prompt: draft.Prompt, BaseCommit: draft.Source.BaseRef}, nil
	}
	var snap TaskSnapshot
	if err := domain.UnmarshalStrict(raw, &snap); err != nil {
		return TaskSnapshot{}, err
	}
	return snap, nil
}

func decodeProfileDraft(raw []byte) (ProfileSnapshot, error) {
	if bytes.Contains(raw, []byte(domain.ProfileSchema)) || bytes.Contains(raw, []byte(`"kind":"AgentProfileDraft"`)) {
		draft, err := domain.DecodeProfile(raw)
		if err != nil {
			return ProfileSnapshot{}, err
		}
		if blockers := domain.PublishBlockers(draft); len(blockers) > 0 {
			return ProfileSnapshot{}, errors.New(strings.Join(blockers, ","))
		}
		return ProfileSnapshot{
			Adapter: draft.Adapter, Model: draft.Model.RequestedID, Executor: draft.Execution.Executor,
			DisplayName: draft.Name, Executable: draft.CLI.Executable,
		}, nil
	}
	var snap ProfileSnapshot
	if err := domain.UnmarshalStrict(raw, &snap); err != nil {
		return ProfileSnapshot{}, err
	}
	return snap, nil
}

func allowedRoots(spec string) []string {
	var doc struct {
		AllowedRoots []string `json:"allowed_roots"`
	}
	_ = json.Unmarshal([]byte(spec), &doc)
	return doc.AllowedRoots
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

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
