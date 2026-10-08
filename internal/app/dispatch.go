package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/store/sqlite"
	"github.com/sllt/agentlab/internal/workspace"
)

const (
	// defaultWallSeconds applies when neither the task, the profile, the
	// settings nor the environment set an agent wall clock. Real coding agents
	// routinely need tens of minutes; 20 seconds made every real run time out.
	defaultWallSeconds = 30 * 60
	minWallSeconds     = 5
	maxWallSeconds     = 24 * 60 * 60
	shutdownDrain      = 15 * time.Second
)

// Loop runs the scheduler. Each tick refills free slots: a finished attempt
// wakes the loop at once, and a long attempt never blocks admission of the
// next queued trial while capacity remains.
func (s *Service) Loop(ctx context.Context) error {
	if err := s.Recover(ctx); err != nil {
		s.noteFatal(err)
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	wake := s.wakeChan()
	for {
		select {
		case <-ctx.Done():
			s.drain(shutdownDrain)
			return nil
		case <-ticker.C:
		case <-wake:
		}
		if s.Maintenance() {
			continue
		}
		s.RetryNotifications(ctx)
		if _, err := s.dispatch(ctx, s.limit()); err != nil && fatalStore(err) {
			s.noteFatal(err)
		}
	}
}

// Pump admits queued trials up to the limit and waits for the attempts it
// started. Tests and one-shot tools use it; the long-running Loop uses
// dispatch directly and never waits for a batch.
func (s *Service) Pump(ctx context.Context, globalLimit int) error {
	batch, err := s.dispatch(ctx, globalLimit)
	batch.Wait()
	return err
}

// dispatch admits queued trials in FIFO order while global capacity remains
// and starts each one in its own goroutine. A trial whose account is full or
// blocked is skipped, so it cannot hold up trials on other accounts.
func (s *Service) dispatch(ctx context.Context, globalLimit int) (*sync.WaitGroup, error) {
	batch := &sync.WaitGroup{}
	if globalLimit <= 0 {
		globalLimit = s.limit()
	}
	queued, err := s.Store.ListQueued(ctx)
	if err != nil {
		return batch, err
	}
	for _, trial := range queued {
		if s.Maintenance() || ctx.Err() != nil {
			break
		}
		active, err := s.Store.GlobalActiveAttempts(ctx)
		if err != nil {
			return batch, err
		}
		if active >= globalLimit {
			break
		}
		account, err := s.Store.ProfileAccount(ctx, trial.ProfileVersionID)
		if err != nil {
			if errors.Is(err, sqlite.ErrNotFound) {
				continue
			}
			return batch, err
		}
		reason := ""
		repairMark := filepath.Join(s.DataDir, "repairs", trial.ID)
		if _, err := os.Stat(repairMark); err == nil {
			reason = "protocol_repair"
		}
		attempt, err := s.Store.CreateAttempt(ctx, trial.ID, account, NativeRuntime(), reason, globalLimit, 0)
		if err != nil {
			if errors.Is(err, sqlite.ErrConflict) || errors.Is(err, sqlite.ErrCapacity) || errors.Is(err, sqlite.ErrAccountBlocked) {
				continue
			}
			return batch, err
		}
		if reason != "" {
			_ = os.Remove(repairMark)
		}
		batch.Add(1)
		s.inflight.Add(1)
		go func(trial sqlite.Trial, attempt sqlite.Attempt) {
			defer s.inflight.Done()
			defer batch.Done()
			defer s.poke()
			s.runAttempt(ctx, trial, attempt)
		}(trial, attempt)
	}
	return batch, nil
}

func (s *Service) runAttempt(ctx context.Context, trial sqlite.Trial, attempt sqlite.Attempt) {
	if err := s.execute(ctx, trial, attempt); err != nil {
		if errors.Is(err, sqlite.ErrConflict) {
			_ = s.finishCancelled(ctx, trial.ID, attempt)
			return
		}
		if fatalStore(err) {
			s.noteFatal(err)
			return
		}
		// The control context may already be cancelled at shutdown; the
		// terminal write must still land, so it uses a fresh context.
		fin, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.Store.FinishAttempt(fin, attempt.ID, attempt.Fence, string(domain.ExecAborted), string(domain.VerdictInconclusive), string(domain.CleanupQuarantined), err.Error())
		cancel()
	}
	s.scheduleRepair(ctx, trial.ID)
}

func (s *Service) wakeChan() chan struct{} {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	if s.wake == nil {
		s.wake = make(chan struct{}, 1)
	}
	return s.wake
}

func (s *Service) poke() {
	select {
	case s.wakeChan() <- struct{}{}:
	default:
	}
}

// drain waits for in-flight attempts after shutdown was requested. Runners
// receive Shutdown through the cancelled context and stop their agents.
func (s *Service) drain(limit time.Duration) {
	done := make(chan struct{})
	go func() {
		s.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(limit):
	}
}

// wallFor resolves the agent wall clock for one attempt and names its source.
// Order: task limits, profile, settings.json, AGENTLAB_AGENT_WALL_SECONDS,
// the Service default, then defaultWallSeconds.
func (s *Service) wallFor(task TaskSnapshot, profile ProfileSnapshot) (int, string) {
	if task.Limits != nil && task.Limits.AgentWallSeconds > 0 {
		return clampWall(task.Limits.AgentWallSeconds), "task"
	}
	if profile.WallSeconds > 0 {
		return clampWall(profile.WallSeconds), "profile"
	}
	if n := s.LoadSettings().AgentWallSeconds; n > 0 {
		return clampWall(n), "settings"
	}
	if raw := os.Getenv("AGENTLAB_AGENT_WALL_SECONDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return clampWall(n), "env"
		}
	}
	if s.WallSeconds > 0 {
		return clampWall(s.WallSeconds), "service"
	}
	return defaultWallSeconds, "default"
}

func clampWall(n int) int {
	if n < minWallSeconds {
		return minWallSeconds
	}
	if n > maxWallSeconds {
		return maxWallSeconds
	}
	return n
}

// attemptBudget is the resource envelope an attempt ran under. Its digest is
// part of the comparison key: two results are only on one board when they had
// the same wall clock, log and workspace limits.
type attemptBudget struct {
	WallSeconds   int    `json:"wall_seconds"`
	WallSource    string `json:"wall_source"`
	LogBytes      int64  `json:"log_bytes"`
	MaxFiles      int    `json:"max_files"`
	MaxBytes      int64  `json:"max_bytes"`
	MaxFileBytes  int64  `json:"max_file_bytes"`
	NativeBudget  bool   `json:"native_budget"`
	CostLimitUSD  string `json:"cost_limit"`
	ApproveTools  bool   `json:"approve_tools"`
	BudgetVersion string `json:"budget_version"`
}

func budgetFor(task TaskSnapshot, profile ProfileSnapshot, wall int, source string) (attemptBudget, workspace.Limits, int64) {
	limits := workspace.Limits{}
	var logBytes int64 = 64 << 20
	if task.Limits != nil {
		if task.Limits.MaxFiles > 0 {
			limits.MaxFiles = task.Limits.MaxFiles
		}
		if task.Limits.ArtifactBytes > 0 {
			limits.MaxBytes = int64(task.Limits.ArtifactBytes)
		}
		if task.Limits.LogBytes > 0 {
			logBytes = int64(task.Limits.LogBytes)
		}
	}
	norm := limits.Normalized()
	return attemptBudget{
		WallSeconds: wall, WallSource: source, LogBytes: logBytes,
		MaxFiles: norm.MaxFiles, MaxBytes: norm.MaxBytes, MaxFileBytes: norm.MaxFileBytes,
		NativeBudget: false, CostLimitUSD: "none", ApproveTools: profile.ApproveTools,
		BudgetVersion: "agentlab.budget/v1",
	}, limits, logBytes
}

// budgetDigest leaves out WallSource: the same 1800 s from a task or from
// settings is the same budget.
func budgetDigest(b attemptBudget) string {
	b.WallSource = ""
	d, err := domain.Digest(b)
	if err != nil {
		return ""
	}
	return d
}
