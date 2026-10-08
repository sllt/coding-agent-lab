package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeCLI writes an executable shell script that answers --version and --help.
func fakeCLI(t *testing.T, help string, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-cli")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 'fake-cli 1.2.3'; exit 0; fi\n" +
		"if [ \"$1\" = \"--help\" ] || [ \"$2\" = \"--help\" ]; then cat <<'HELP'\n" + help + "\nHELP\nexit 0; fi\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

const cursorHelp = `Usage: agent [options] [prompt...]
  -p, --print              Print responses to console
  --output-format <format> Output format: text | json | stream-json
  --force                  Allow commands`

func envMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestProbeReadinessLevels(t *testing.T) {
	ctx := context.Background()
	cli := fakeCLI(t, cursorHelp, "exit 0")
	home := t.TempDir()

	caps, _ := (Cursor{}).Probe(ctx, ProbeRequest{Executable: cli, Home: home, LookupEnv: envMap(nil)})
	if caps.Readiness != string(ReadinessNeedsCredentials) || !caps.SupportsHeadless || caps.CLIVersion != "fake-cli 1.2.3" {
		t.Fatalf("no credentials %+v", caps)
	}

	caps, _ = (Cursor{}).Probe(ctx, ProbeRequest{Executable: cli, Home: home, CredentialEnv: []string{"CURSOR_API_KEY"}, LookupEnv: envMap(map[string]string{"CURSOR_API_KEY": "sk-secret-value"})})
	if caps.Readiness != string(ReadinessReadyUnverified) || caps.Verified {
		t.Fatalf("env credential %+v", caps)
	}
	if len(caps.CredentialSources) != 1 || caps.CredentialSources[0] != "env:CURSOR_API_KEY" {
		t.Fatalf("sources %v", caps.CredentialSources)
	}
	for _, s := range append(caps.CredentialSources, caps.Hints...) {
		if strings.Contains(s, "sk-secret-value") {
			t.Fatal("probe leaked a credential value")
		}
	}

	// A known variable that is present but not configured is a hint, not a source.
	caps, _ = (Cursor{}).Probe(ctx, ProbeRequest{Executable: cli, Home: home, LookupEnv: envMap(map[string]string{"CURSOR_API_KEY": "x"})})
	if caps.Readiness != string(ReadinessNeedsCredentials) || len(caps.Hints) == 0 {
		t.Fatalf("unconfigured env %+v", caps)
	}

	// A login file only counts when the agent will see the real HOME.
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".cursor", "cli-config.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	caps, _ = (Cursor{}).Probe(ctx, ProbeRequest{Executable: cli, Home: home, LookupEnv: envMap(nil)})
	if caps.Readiness != string(ReadinessNeedsCredentials) {
		t.Fatalf("file without inherit_home %+v", caps)
	}
	caps, _ = (Cursor{}).Probe(ctx, ProbeRequest{Executable: cli, Home: home, InheritHome: true, LookupEnv: envMap(nil)})
	if caps.Readiness != string(ReadinessReadyUnverified) || caps.CredentialSources[0] != "file:~/.cursor/cli-config.json" {
		t.Fatalf("file with inherit_home %+v", caps)
	}

	// Help text without the headless flags stops at cli_detected.
	bare := fakeCLI(t, "Usage: agent [prompt]\n  --formatted   pretty", "exit 0")
	caps, _ = (Cursor{}).Probe(ctx, ProbeRequest{Executable: bare, Home: home, InheritHome: true, LookupEnv: envMap(nil)})
	if caps.Readiness != string(ReadinessCLIDetected) || caps.SupportsHeadless || len(caps.Missing) == 0 {
		t.Fatalf("no headless flags %+v", caps)
	}

	// A failing --version is unavailable.
	caps, _ = (Grok{}).Probe(ctx, ProbeRequest{Executable: "/bin/false", Home: home})
	if caps.Readiness != string(ReadinessUnavailable) {
		t.Fatalf("version failure %+v", caps)
	}

	// Loader variables can never be forwarded.
	caps, _ = (Cursor{}).Probe(ctx, ProbeRequest{Executable: cli, Home: home, CredentialEnv: []string{"LD_PRELOAD"}, LookupEnv: envMap(map[string]string{"LD_PRELOAD": "/x.so"})})
	if Readiness(caps.Readiness).Runnable() {
		t.Fatalf("LD_PRELOAD accepted %+v", caps)
	}
}

func TestOpenCodeNeedsProviderModelAndProviderKey(t *testing.T) {
	ctx := context.Background()
	help := "opencode run [message..]\n  --model, -m  provider/model\n  --format     format: default | json"
	cli := fakeCLI(t, help, "exit 0")
	caps, _ := (OpenCode{}).Probe(ctx, ProbeRequest{Executable: cli, ModelID: "anthropic/claude", Home: t.TempDir(), CredentialEnv: []string{"ANTHROPIC_API_KEY"}, LookupEnv: envMap(map[string]string{"ANTHROPIC_API_KEY": "k"})})
	if caps.Readiness != string(ReadinessReadyUnverified) {
		t.Fatalf("opencode %+v", caps)
	}
	caps, _ = (OpenCode{}).Probe(ctx, ProbeRequest{Executable: cli, ModelID: "bare-model", Home: t.TempDir(), CredentialEnv: []string{"ANTHROPIC_API_KEY"}, LookupEnv: envMap(map[string]string{"ANTHROPIC_API_KEY": "k"})})
	if Readiness(caps.Readiness).Runnable() {
		t.Fatalf("bare model accepted %+v", caps)
	}
	if got := KnownCredentialEnv("opencode", "openrouter/x"); len(got) != 1 || got[0] != "OPENROUTER_API_KEY" {
		t.Fatalf("provider env %v", got)
	}
}

func TestValidCredentialEnv(t *testing.T) {
	for _, ok := range []string{"CURSOR_API_KEY", "XAI_API_KEY", "MY_TOKEN_2"} {
		if !ValidCredentialEnv(ok) {
			t.Fatalf("%s rejected", ok)
		}
	}
	for _, bad := range []string{"PATH", "HOME", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES", "AGENTLAB_DATA", "NODE_OPTIONS", "lower", "A=B", ""} {
		if ValidCredentialEnv(bad) {
			t.Fatalf("%s accepted", bad)
		}
	}
}

func TestSmokeRequiresStructuredOutputAndFlagsAuth(t *testing.T) {
	ctx := context.Background()
	good := fakeCLI(t, cursorHelp, `echo '{"type":"result","text":"OK"}'; exit 0`)
	res := Smoke(ctx, Cursor{}, SmokeRequest{Executable: good, Timeout: 10 * time.Second})
	if !res.Passed || res.JSONLines != 1 {
		t.Fatalf("good smoke %+v", res)
	}
	plain := fakeCLI(t, cursorHelp, `echo OK; exit 0`)
	if res := Smoke(ctx, Cursor{}, SmokeRequest{Executable: plain, Timeout: 10 * time.Second}); res.Passed || res.Reason != "no_structured_output" {
		t.Fatalf("plain smoke %+v", res)
	}
	auth := fakeCLI(t, cursorHelp, `echo 'Error: not logged in. Run agent login.' >&2; exit 1`)
	if res := Smoke(ctx, Cursor{}, SmokeRequest{Executable: auth, Timeout: 10 * time.Second}); res.Passed || !res.AuthSuspected || res.Reason != "authentication_failed" {
		t.Fatalf("auth smoke %+v", res)
	}
	if AuthFailureText("processed 401 records") {
		t.Fatal("a bare number was read as an auth failure")
	}
}
