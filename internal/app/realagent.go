package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sllt/agentlab/internal/agent"
	"github.com/sllt/agentlab/internal/sandbox"
	"github.com/sllt/agentlab/internal/workspace"
)

// accountCredentialEnv maps an account credential reference of the form
// "env:NAME" to the variable name. Other reference kinds are not forwarded.
func (s *Service) accountCredentialEnv(ctx context.Context, accountID string) string {
	if accountID == "" {
		return ""
	}
	acc, err := s.Store.GetAccount(ctx, accountID)
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(acc.CredentialRef)
	if !strings.HasPrefix(ref, "env:") {
		return ""
	}
	name := strings.TrimPrefix(ref, "env:")
	if !agent.ValidCredentialEnv(name) {
		return ""
	}
	return name
}

func appendUniqueString(list []string, v string) []string {
	for _, item := range list {
		if item == v {
			return list
		}
	}
	return append(list, v)
}

// agentAuthSuspected looks for a non-zero agent exit together with an
// authentication error in the agent's own output. It is a heuristic that may
// only downgrade a result to inconclusive.
func agentAuthSuspected(events []byte) bool {
	exitCode := 0
	var text strings.Builder
	for _, line := range bytesSplit(events) {
		var ev struct {
			Origin  string          `json:"origin"`
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch {
		case ev.Origin == "runner" && ev.Type == "Finished":
			var p struct {
				ExitCode int `json:"exit_code"`
			}
			if json.Unmarshal(ev.Payload, &p) == nil {
				exitCode = p.ExitCode
			}
		case ev.Origin == "agent_observation" && text.Len() < 256<<10:
			text.Write(ev.Payload)
			text.WriteByte('\n')
		}
	}
	return exitCode != 0 && agent.AuthFailureText(text.String())
}

func workspaceUnchanged(base, work string) bool {
	patch, err := workspace.Collect(base, work, workspace.Limits{})
	return err == nil && len(patch.Changes) == 0
}

// sandboxFor builds the Landlock policy for a real agent run and the runtime
// record that documents what it does and does not isolate. A nil policy means
// the run is unconfined; the record says why.
func (s *Service) sandboxFor(executable, work, home, tmp string, inheritHome bool) (*sandbox.Policy, map[string]any) {
	limits := "仅限制文件系统读写范围。不限制网络、进程可见性（/proc）、对同用户其他进程发信号、IPC 和资源用量；不是容器或虚拟机隔离。"
	if sandbox.Disabled() {
		return nil, map[string]any{"sandbox": map[string]any{"mode": "none", "reason": "disabled_by_operator", "limits": limits}}
	}
	st := sandbox.Probe()
	if !st.Supported {
		return nil, map[string]any{"sandbox": map[string]any{"mode": "none", "reason": "landlock_unavailable", "detail": st.Reason, "limits": limits}}
	}
	exe := executable
	if resolved, err := exec.LookPath(executable); err == nil {
		exe = resolved
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	dataDir, _ := filepath.Abs(s.DataDir)
	writable := []string{work, home, tmp}
	deny := []string{dataDir}
	realHome, _ := os.UserHomeDir()
	if inheritHome && realHome != "" {
		writable = append(writable, realHome)
	} else if realHome != "" {
		deny = append(deny, realHome)
	}
	// Many CLIs ignore TMPDIR and write to /tmp. Allow it unless the data
	// directory itself lives there, which would expose other attempts.
	if !strings.HasPrefix(dataDir+"/", "/tmp/") {
		writable = append(writable, "/tmp")
	}
	policy, exposed := sandbox.Build(exe, os.Getenv("PATH"), writable, deny)
	rel := func(list []string) []string {
		out := make([]string, 0, len(list))
		for _, p := range list {
			if dataDir != "" && strings.HasPrefix(p, dataDir) {
				p = "$DATA" + strings.TrimPrefix(p, dataDir)
			}
			out = append(out, p)
		}
		return out
	}
	if exposed == nil {
		exposed = []string{}
	}
	return &policy, map[string]any{
		"isolation": "landlock-fs/native",
		"sandbox": map[string]any{
			"mode": "landlock", "abi": st.ABI,
			"read_only": rel(policy.ReadOnly), "read_write": rel(policy.ReadWrite),
			"exposed": rel(exposed), "limits": limits,
		},
	}
}
