// Package doctor reports whether an adapter can be used. A static probe never
// starts a paid model call; a smoke run only happens when the operator asks
// for it explicitly. network=restricted is never claimed here.
package doctor

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/sllt/agentlab/internal/agent"
)

type Report struct {
	Adapter           string             `json:"adapter"`
	Executable        string             `json:"executable"`
	CLIVersion        string             `json:"cli_version"`
	ModelID           string             `json:"model_id"`
	ModelCall         string             `json:"model_call"`
	Network           string             `json:"network"`
	Executor          string             `json:"executor"`
	StaticPassed      bool               `json:"static_passed"`
	Verified          bool               `json:"verified"`
	Readiness         string             `json:"readiness"`
	Runnable          bool               `json:"runnable"`
	SupportsHeadless  bool               `json:"supports_headless"`
	HeadlessProbed    bool               `json:"headless_probed"`
	CredentialSources []string           `json:"credential_sources"`
	Blockers          []string           `json:"blockers"`
	Hints             []string           `json:"hints"`
	Smoke             *agent.SmokeResult `json:"smoke,omitempty"`
	CheckedAt         string             `json:"checked_at"`
	Note              string             `json:"note"`
}

// Request is one doctor run.
type Request struct {
	Adapter        string
	Executable     string
	ModelID        string
	CredentialEnv  []string
	InheritHome    bool
	ApproveTools   bool
	AllowModelCall bool
	SmokeTimeout   time.Duration
}

// Static keeps the original signature: a probe without a model call unless
// allowModelCall is set.
func Static(ctx context.Context, adapterName, executable, modelID string, allowModelCall bool) Report {
	return Run(ctx, Request{Adapter: adapterName, Executable: executable, ModelID: modelID, AllowModelCall: allowModelCall})
}

func Run(ctx context.Context, req Request) Report {
	report := Report{
		Adapter: req.Adapter, Executable: req.Executable, ModelID: req.ModelID, ModelCall: "not_run",
		Network: "unrestricted", Executor: "unconfigured", Readiness: string(agent.ReadinessUnavailable),
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if strings.Contains(req.ModelID, "REPLACE_") || strings.Contains(req.Executable, "REPLACE_") {
		report.Blockers = append(report.Blockers, "placeholder_field")
	}
	if f, err := os.CreateTemp("", "agentlab-doctor-*"); err != nil {
		report.Blockers = append(report.Blockers, "temp_not_writable")
	} else {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
	}
	ad, ok := agent.ByName(req.Adapter)
	if !ok {
		report.Blockers = append(report.Blockers, "unknown_adapter")
		return finish(report)
	}
	if req.Adapter == "fixture" {
		report.CLIVersion = "fixture"
		report.Executor = "native-trusted"
		report.StaticPassed = len(report.Blockers) == 0
		report.Readiness = string(agent.ReadinessReadyUnverified)
		report.CredentialSources = []string{"none_required"}
		// A static look at the fixture is not an execution fixture. verified stays false.
		report.Verified = false
		report.Note = "本地假 CLI，不调用模型供应商。静态检查不能标成 verified，只有执行夹具通过才算已验证。"
		return finish(report)
	}
	caps, err := ad.Probe(ctx, agent.ProbeRequest{Executable: req.Executable, ModelID: req.ModelID, CredentialEnv: req.CredentialEnv, InheritHome: req.InheritHome})
	if err != nil {
		report.Blockers = append(report.Blockers, "probe_failed")
		return finish(report)
	}
	report.Executable = caps.Executable
	report.CLIVersion = caps.CLIVersion
	report.SupportsHeadless = caps.SupportsHeadless
	report.HeadlessProbed = caps.Probed && caps.CLIVersion != ""
	report.Readiness = caps.Readiness
	report.CredentialSources = caps.CredentialSources
	report.Hints = append(report.Hints, caps.Hints...)
	report.Blockers = append(report.Blockers, caps.Blockers...)
	if len(caps.Missing) > 0 {
		report.Hints = append(report.Hints, "帮助文本里没找到这些无头参数："+strings.Join(caps.Missing, "、"))
	}
	if _, err := ad.NewDecoder().Decode("stdout", []byte(`{"type":"assistant","text":"sample"}`)); err != nil {
		report.Blockers = append(report.Blockers, "decoder_failed")
	}
	if len(report.Blockers) > 0 && agent.Readiness(report.Readiness).Runnable() {
		// A placeholder or invalid field blocks even a CLI that is otherwise ready.
		report.Readiness = string(agent.ReadinessCLIDetected)
	}
	report.Note = "静态探测：找到 CLI、读取版本和帮助、确认无头参数和凭据来源。它不调用模型，所以最多到 ready_unverified。"
	if req.AllowModelCall {
		if !agent.Readiness(report.Readiness).Runnable() {
			report.ModelCall = "refused"
			report.Hints = append(report.Hints, "静态探测未通过，没有发起付费调用。")
		} else {
			smoke := agent.Smoke(ctx, ad, agent.SmokeRequest{
				Executable: req.Executable, ModelID: req.ModelID, CredentialEnv: req.CredentialEnv,
				InheritHome: req.InheritHome, ApproveTools: req.ApproveTools, Timeout: req.SmokeTimeout,
			})
			report.Smoke = &smoke
			if smoke.Passed {
				report.ModelCall = "passed"
				report.Verified = true
				report.Readiness = string(agent.ReadinessVerified)
				report.Note = "已获授权的冒烟调用成功：CLI 能在无头模式下返回结构化输出。"
			} else {
				report.ModelCall = "failed"
				report.Hints = append(report.Hints, "冒烟调用失败："+smoke.Reason)
				if smoke.AuthSuspected {
					report.Readiness = string(agent.ReadinessNeedsCredentials)
				}
			}
		}
	}
	return finish(report)
}

func finish(r Report) Report {
	r.Runnable = agent.Readiness(r.Readiness).Runnable() && len(r.Blockers) == 0
	r.StaticPassed = len(r.Blockers) == 0 && r.Readiness != string(agent.ReadinessUnavailable)
	if r.Adapter != "fixture" && !r.Runnable {
		r.StaticPassed = false
	}
	if r.Blockers == nil {
		r.Blockers = []string{}
	}
	if r.Hints == nil {
		r.Hints = []string{}
	}
	if r.CredentialSources == nil {
		r.CredentialSources = []string{}
	}
	return r
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
