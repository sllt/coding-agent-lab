// Package doctor reports whether an adapter can be used. A static probe never
// starts a paid model call. network=restricted is never claimed here.
package doctor

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sllt/agentlab/internal/agent"
)

type Report struct {
	Adapter          string   `json:"adapter"`
	Executable       string   `json:"executable"`
	CLIVersion       string   `json:"cli_version"`
	ModelID          string   `json:"model_id"`
	ModelCall        string   `json:"model_call"`
	Network          string   `json:"network"`
	Executor         string   `json:"executor"`
	StaticPassed     bool     `json:"static_passed"`
	Verified         bool     `json:"verified"`
	SupportsHeadless bool     `json:"supports_headless"`
	HeadlessProbed   bool     `json:"headless_probed"`
	Blockers         []string `json:"blockers"`
	Note             string   `json:"note"`
}

func Static(ctx context.Context, adapterName, executable, modelID string, allowModelCall bool) Report {
	report := Report{Adapter: adapterName, Executable: executable, ModelID: modelID, ModelCall: "not_run", Network: "unrestricted", Executor: "unconfigured"}
	if strings.Contains(modelID, "REPLACE_") || strings.Contains(executable, "REPLACE_") {
		report.Blockers = append(report.Blockers, "placeholder_field")
	}
	if f, err := os.CreateTemp("", "agentlab-doctor-*"); err != nil {
		report.Blockers = append(report.Blockers, "temp_not_writable")
	} else {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
	}
	ad, ok := agent.ByName(adapterName)
	if !ok {
		report.Blockers = append(report.Blockers, "unknown_adapter")
		return report
	}
	if adapterName == "fixture" {
		report.CLIVersion = "fixture"
		report.Executor = "native-trusted"
		report.StaticPassed = len(report.Blockers) == 0
		// A static look at the fixture is not an execution fixture. verified stays false.
		report.Verified = false
		report.Note = "本地假 CLI，不调用模型供应商。静态检查不能标成 verified，只有执行夹具通过才算已验证。"
		return report
	}
	if executable == "" {
		switch adapterName {
		case "cursor":
			executable = "agent"
		case "grok":
			executable = "grok"
		case "opencode":
			executable = "opencode"
		}
		report.Executable = executable
	}
	path, err := exec.LookPath(executable)
	if err != nil {
		report.Blockers = append(report.Blockers, "cli_not_found")
	} else {
		report.Executable = path
		versionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(versionCtx, path, "--version")
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			report.CLIVersion = firstLine(out)
		} else {
			report.Blockers = append(report.Blockers, "version_probe_failed")
		}
	}
	if _, err := ad.NewDecoder().Decode("stdout", []byte(`{"type":"assistant","text":"sample"}`)); err != nil {
		report.Blockers = append(report.Blockers, "decoder_failed")
	}
	if allowModelCall {
		report.Blockers = append(report.Blockers, "model_call_not_authorized_in_this_build")
		report.ModelCall = "refused"
	}
	caps, err := ad.Probe(ctx, agent.ProbeRequest{Executable: report.Executable, ModelID: modelID})
	if err != nil || !caps.Probed {
		report.SupportsHeadless = false
		report.HeadlessProbed = false
		report.Note = "静态检查不能证明模型可调用或额度可共享。能力未经探测，无头模式不会被写成已支持。"
	} else {
		report.SupportsHeadless = caps.SupportsHeadless
		report.HeadlessProbed = true
		report.Verified = caps.Verified
		report.Note = "静态检查不能证明模型可调用或额度可共享"
	}
	// version probe failure and a refused model call both block publish.
	report.StaticPassed = len(report.Blockers) == 0
	if !caps.Verified {
		report.Verified = false
	}
	return report
}

func firstLine(b []byte) string {
	line := string(bytes.TrimSpace(b))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if len(line) > 200 {
		line = line[:200]
	}
	return strings.TrimSpace(line)
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
