package agent

import "context"

// Fixture launches this same binary as `fake-agent`. It never calls a provider.
type Fixture struct {
	Executable string
}

func (f Fixture) Name() string { return "fixture" }
func (f Fixture) Probe(context.Context, ProbeRequest) (Capabilities, error) {
	return Capabilities{CLIVersion: "fixture", Executable: f.exe(), SupportsHeadless: true, SupportsStructuredLog: false, ReportsUsage: false, TestedPlatform: "linux/amd64", Probed: true, Verified: true}, nil
}
func (f Fixture) NewDecoder() EventDecoder { return NewLineDecoder() }
func (f Fixture) exe() string {
	if f.Executable != "" {
		return f.Executable
	}
	return "agentlab"
}
func (f Fixture) BuildLaunch(_ context.Context, req LaunchRequest) (LaunchSpec, error) {
	env := baseEnv(req)
	if env["AGENTLAB_FAKE_MODE"] == "" {
		env["AGENTLAB_FAKE_MODE"] = "success"
	}
	return LaunchSpec{Executable: f.exe(), Args: []string{"fake-agent"}, WorkDir: req.WorkDir, Env: env}, nil
}
