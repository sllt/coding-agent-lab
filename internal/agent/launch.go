package agent

import (
	"context"
	"fmt"
	"strings"
)

const maxPromptArg = 200_000

func promptArg(prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt is empty")
	}
	if len(prompt) > maxPromptArg {
		return "", fmt.Errorf("prompt exceeds argv limit and this adapter has no verified file input")
	}
	return prompt, nil
}

func baseEnv(req LaunchRequest) map[string]string {
	env := map[string]string{}
	for k, v := range req.Env {
		if k == "" || strings.Contains(k, "=") {
			continue
		}
		env[k] = v
	}
	return env
}

type Cursor struct{}

func (Cursor) Name() string { return "cursor" }
func (Cursor) Probe(ctx context.Context, req ProbeRequest) (Capabilities, error) {
	return probeCLI(ctx, "cursor", req), nil
}
func (Cursor) NewDecoder() EventDecoder { return NewLineDecoder() }
func (Cursor) BuildLaunch(_ context.Context, req LaunchRequest) (LaunchSpec, error) {
	prompt, err := promptArg(req.Prompt)
	if err != nil {
		return LaunchSpec{}, err
	}
	args := []string{"-p", "--output-format", "stream-json"}
	if req.ModelID != "" {
		args = append(args, "--model", req.ModelID)
	}
	if req.ApproveTools {
		args = append(args, "--force")
	}
	args = append(args, prompt)
	return LaunchSpec{Executable: DefaultExecutable("cursor"), Args: args, WorkDir: req.WorkDir, Env: baseEnv(req)}, nil
}

type Grok struct{}

func (Grok) Name() string { return "grok" }
func (Grok) Probe(ctx context.Context, req ProbeRequest) (Capabilities, error) {
	return probeCLI(ctx, "grok", req), nil
}
func (Grok) NewDecoder() EventDecoder { return NewLineDecoder() }
func (Grok) BuildLaunch(_ context.Context, req LaunchRequest) (LaunchSpec, error) {
	prompt, err := promptArg(req.Prompt)
	if err != nil {
		return LaunchSpec{}, err
	}
	args := []string{"--no-auto-update", "-p", prompt, "--output-format", "streaming-json"}
	if req.ModelID != "" {
		args = append(args, "-m", req.ModelID)
	}
	return LaunchSpec{Executable: "grok", Args: args, WorkDir: req.WorkDir, Env: baseEnv(req)}, nil
}

type OpenCode struct{}

func (OpenCode) Name() string { return "opencode" }
func (OpenCode) Probe(ctx context.Context, req ProbeRequest) (Capabilities, error) {
	return probeCLI(ctx, "opencode", req), nil
}
func (OpenCode) NewDecoder() EventDecoder { return NewLineDecoder() }
func (OpenCode) BuildLaunch(_ context.Context, req LaunchRequest) (LaunchSpec, error) {
	prompt, err := promptArg(req.Prompt)
	if err != nil {
		return LaunchSpec{}, err
	}
	if req.ModelID == "" || !strings.Contains(req.ModelID, "/") {
		return LaunchSpec{}, fmt.Errorf("opencode model id must be provider/model")
	}
	args := []string{"run", "--format", "json", "--model", req.ModelID, prompt}
	return LaunchSpec{Executable: "opencode", Args: args, WorkDir: req.WorkDir, Env: baseEnv(req)}, nil
}

func ByName(name string) (Adapter, bool) {
	switch name {
	case "cursor":
		return Cursor{}, true
	case "grok":
		return Grok{}, true
	case "opencode":
		return OpenCode{}, true
	case "fixture":
		return Fixture{}, true
	default:
		return nil, false
	}
}
