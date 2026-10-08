package agent

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// SmokeRequest is an authorized, paid model call that proves a profile can
// run end to end. It is only started when the operator asks for it.
type SmokeRequest struct {
	Executable    string
	ModelID       string
	CredentialEnv []string
	InheritHome   bool
	ApproveTools  bool
	Timeout       time.Duration
}

type SmokeResult struct {
	Passed     bool   `json:"passed"`
	ExitCode   int    `json:"exit_code"`
	JSONLines  int    `json:"json_lines"`
	DurationMS int64  `json:"duration_ms"`
	Reason     string `json:"reason,omitempty"`
	// AuthSuspected is a heuristic on the CLI's own output. It only explains a
	// failure; it never turns a failure into a pass.
	AuthSuspected bool `json:"auth_suspected"`
}

const smokePrompt = "This is a connectivity check. Reply with the single word OK. Do not read, create or modify any files."

// Smoke runs the adapter once with a tiny prompt in an empty directory.
func Smoke(ctx context.Context, ad Adapter, req SmokeRequest) SmokeResult {
	if req.Timeout <= 0 {
		req.Timeout = 120 * time.Second
	}
	work, err := os.MkdirTemp("", "agentlab-smoke-*")
	if err != nil {
		return SmokeResult{Reason: "temp_dir_failed"}
	}
	defer os.RemoveAll(work)
	home := work + "/.home"
	_ = os.MkdirAll(home, 0o700)
	spec, err := ad.BuildLaunch(ctx, LaunchRequest{ModelID: req.ModelID, Prompt: smokePrompt, WorkDir: work, ApproveTools: false})
	if err != nil {
		return SmokeResult{Reason: "launch_rejected: " + err.Error()}
	}
	if req.Executable != "" {
		spec.Executable = req.Executable
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + work, "NO_COLOR=1", "CI=1"}
	if req.InheritHome {
		env = append(env, "HOME="+os.Getenv("HOME"))
	} else {
		env = append(env, "HOME="+home)
	}
	for _, name := range req.CredentialEnv {
		if !ValidCredentialEnv(name) {
			continue
		}
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, spec.Executable, spec.Args...)
	cmd.Dir = work
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &limitedBuffer{buf: &out, max: 1 << 20}
	cmd.Stderr = cmd.Stdout
	start := time.Now()
	runErr := cmd.Run()
	res := SmokeResult{DurationMS: time.Since(start).Milliseconds()}
	if runCtx.Err() != nil {
		res.Reason = "timeout"
		res.ExitCode = -1
		return res
	}
	if runErr != nil {
		res.ExitCode = -1
		if ee, ok := runErr.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		}
	}
	for _, line := range bytes.Split(out.Bytes(), []byte{'\n'}) {
		events, _ := DecodeLine("stdout", line)
		for _, ev := range events {
			if ev.Type == "agent_event" {
				res.JSONLines++
			}
		}
	}
	res.AuthSuspected = res.ExitCode != 0 && AuthFailureText(out.String())
	switch {
	case res.ExitCode != 0 && res.AuthSuspected:
		res.Reason = "authentication_failed"
	case res.ExitCode != 0:
		res.Reason = "nonzero_exit"
	case res.JSONLines == 0:
		res.Reason = "no_structured_output"
	default:
		res.Passed = true
	}
	return res
}

var authMarkers = []string{
	"not logged in", "not authenticated", "unauthenticated", "unauthorized", "authentication failed",
	"authentication required", "invalid api key", "invalid_api_key", "api key not found", "missing api key",
	"no api key", "please log in", "please login", "login required", "status 401", "http 401", "error 401", "401 unauthorized", "403 forbidden", "token expired",
	"credentials not found", "permission denied: api",
}

// AuthFailureText is a heuristic over CLI output. Callers must only use it to
// downgrade a result to inconclusive, never to upgrade one.
func AuthFailureText(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range authMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
