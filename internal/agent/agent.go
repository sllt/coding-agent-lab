// Package agent defines the adapter boundary. Adapters prepare argv and parse
// official output. They do not decide verdicts or kill processes.
package agent

import (
	"context"
	"encoding/json"
)

type LaunchSpec struct {
	Executable string            `json:"executable"`
	Args       []string          `json:"args"`
	WorkDir    string            `json:"work_dir"`
	Env        map[string]string `json:"env"`
	StdinFile  string            `json:"stdin_file,omitempty"`
}

type LaunchRequest struct {
	ModelID      string
	Prompt       string
	WorkDir      string
	ApproveTools bool
	Env          map[string]string
}

type ProbeRequest struct {
	Executable string
	ModelID    string
	// CredentialEnv names environment variables the profile forwards. Only the
	// names are stored; values are read from the control-plane environment.
	CredentialEnv []string
	// InheritHome lets the agent see the real HOME (and its login files).
	InheritHome bool
	// LookupEnv and Home are for tests. Zero values use the process environment.
	LookupEnv func(string) (string, bool)
	Home      string
}

type Capabilities struct {
	CLIVersion            string `json:"cli_version"`
	Executable            string `json:"executable"`
	SupportsHeadless      bool   `json:"supports_headless"`
	SupportsStructuredLog bool   `json:"supports_structured_log"`
	ReportsModelID        bool   `json:"reports_model_id"`
	ReportsUsage          bool   `json:"reports_usage"`
	SupportsNativeBudget  bool   `json:"supports_native_budget"`
	SupportsSessionResume bool   `json:"supports_session_resume"`
	TestedPlatform        string `json:"tested_platform"`
	// Verified is true only after an authorized execution fixture, not after a static probe.
	Verified bool `json:"verified"`
	// Present distinguishes "probed and false" from "not probed".
	Probed bool `json:"probed"`
	// Readiness is the furthest level the static probe reached.
	Readiness string `json:"readiness"`
	// CredentialSources names where a credential was found ("env:NAME",
	// "file:~/path"). It never contains a secret value.
	CredentialSources []string `json:"credential_sources"`
	Blockers          []string `json:"blockers"`
	// Missing lists headless flag groups the help text did not mention.
	Missing []string `json:"missing,omitempty"`
	Hints   []string `json:"hints,omitempty"`
}

type ObservedEvent struct {
	Origin  string          `json:"origin"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type EventDecoder interface {
	Decode(stream string, frame []byte) ([]ObservedEvent, error)
}

type Adapter interface {
	Name() string
	Probe(ctx context.Context, req ProbeRequest) (Capabilities, error)
	BuildLaunch(ctx context.Context, req LaunchRequest) (LaunchSpec, error)
	NewDecoder() EventDecoder
}

func TextEvent(stream string, frame []byte) ObservedEvent {
	return ObservedEvent{Origin: "agent_observation", Type: "message", Payload: must(map[string]string{"stream": stream, "text": string(frame)})}
}

func must(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
