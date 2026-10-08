package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Readiness is how far a static probe got. Only ReadyUnverified and Verified
// are runnable. Verified additionally needs an authorized smoke run, which a
// static probe never performs.
type Readiness string

const (
	ReadinessUnavailable      Readiness = "unavailable"
	ReadinessCLIDetected      Readiness = "cli_detected"
	ReadinessNeedsCredentials Readiness = "needs_credentials"
	ReadinessReadyUnverified  Readiness = "ready_unverified"
	ReadinessVerified         Readiness = "verified"
)

// Runnable reports whether a profile at this level may be published and run.
func (r Readiness) Runnable() bool {
	return r == ReadinessReadyUnverified || r == ReadinessVerified
}

// cliSpec describes what the probe looks for. Help text is matched as whole
// flag tokens so "--formatted" does not satisfy "--format".
type cliSpec struct {
	defaults []string
	helpArgs [][]string
	// headless lists groups of flags; every group needs one match.
	headless [][]string
	// structured lists tokens of which one must appear for JSON output.
	structured []string
	credEnv    []string
	credFiles  []string
}

var specs = map[string]cliSpec{
	"cursor": {
		defaults:   []string{"agent", "cursor-agent"},
		helpArgs:   [][]string{{"--help"}},
		headless:   [][]string{{"-p", "--print"}, {"--output-format"}},
		structured: []string{"stream-json", "json"},
		credEnv:    []string{"CURSOR_API_KEY"},
		credFiles:  []string{".cursor/cli-config.json", ".config/cursor/auth.json", ".cursor/auth.json"},
	},
	"grok": {
		defaults:   []string{"grok"},
		helpArgs:   [][]string{{"--help"}},
		headless:   [][]string{{"-p", "--prompt"}, {"--output-format"}},
		structured: []string{"streaming-json", "stream-json", "json"},
		credEnv:    []string{"GROK_API_KEY", "XAI_API_KEY"},
		credFiles:  []string{".grok/user-settings.json", ".grok/auth.json", ".config/grok/auth.json"},
	},
	"opencode": {
		defaults:   []string{"opencode"},
		helpArgs:   [][]string{{"run", "--help"}, {"--help"}},
		headless:   [][]string{{"run"}, {"--format"}, {"--model", "-m"}},
		structured: []string{"json"},
		credEnv:    nil, // derived from the provider prefix of the model id
		credFiles:  []string{".local/share/opencode/auth.json"},
	},
}

var providerEnv = map[string][]string{
	"anthropic":  {"ANTHROPIC_API_KEY"},
	"openai":     {"OPENAI_API_KEY"},
	"openrouter": {"OPENROUTER_API_KEY"},
	"xai":        {"XAI_API_KEY"},
	"google":     {"GOOGLE_GENERATIVE_AI_API_KEY", "GEMINI_API_KEY"},
	"deepseek":   {"DEEPSEEK_API_KEY"},
	"groq":       {"GROQ_API_KEY"},
	"mistral":    {"MISTRAL_API_KEY"},
}

// DefaultExecutable is the first default command name for an adapter.
func DefaultExecutable(adapter string) string {
	if spec, ok := specs[adapter]; ok && len(spec.defaults) > 0 {
		return spec.defaults[0]
	}
	return ""
}

// KnownCredentialEnv lists the environment variable names an adapter's CLI
// reads for an API key. Values are never read into a profile.
func KnownCredentialEnv(adapter, modelID string) []string {
	spec := specs[adapter]
	out := append([]string{}, spec.credEnv...)
	if adapter == "opencode" {
		if i := strings.IndexByte(modelID, '/'); i > 0 {
			out = append(out, providerEnv[strings.ToLower(modelID[:i])]...)
		}
	}
	return out
}

var envName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

// ValidCredentialEnv reports whether name may be forwarded to an agent.
// Loader and interpreter variables are refused: forwarding them would let a
// profile change what code the runner executes.
func ValidCredentialEnv(name string) bool {
	if !envName.MatchString(name) {
		return false
	}
	switch name {
	case "PATH", "HOME", "TMPDIR", "SHELL", "USER", "PWD", "IFS", "ENV", "BASH_ENV", "PYTHONPATH", "PYTHONSTARTUP", "NODE_OPTIONS", "PERL5LIB", "RUBYOPT", "GIT_DIR", "GIT_WORK_TREE":
		return false
	}
	for _, prefix := range []string{"LD_", "DYLD_", "AGENTLAB_", "GOFLAGS"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func probeCLI(ctx context.Context, adapter string, req ProbeRequest) Capabilities {
	spec := specs[adapter]
	caps := Capabilities{Probed: true, TestedPlatform: "linux/amd64", Readiness: string(ReadinessUnavailable)}
	lookup := req.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	home := req.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	for _, name := range req.CredentialEnv {
		if !ValidCredentialEnv(name) {
			caps.Blockers = appendOnce(caps.Blockers, "credential_env_invalid")
		}
	}
	candidates := spec.defaults
	if req.Executable != "" {
		candidates = []string{req.Executable}
	}
	path := ""
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			path = p
			break
		}
	}
	if path == "" {
		caps.Executable = strings.Join(candidates, " | ")
		caps.Blockers = appendOnce(caps.Blockers, "cli_not_found")
		return caps
	}
	caps.Executable = path
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "NO_COLOR=1", "TERM=dumb", "CI=1"}
	out, err := runProbe(ctx, path, []string{"--version"}, env, 5*time.Second)
	if err != nil {
		caps.Blockers = appendOnce(caps.Blockers, "version_probe_failed")
		return caps
	}
	caps.CLIVersion = firstLine(out)
	caps.Readiness = string(ReadinessCLIDetected)
	var help []byte
	for _, args := range spec.helpArgs {
		// Many CLIs print help to stderr or exit non-zero; the text is what counts.
		text, _ := runProbe(ctx, path, args, env, 5*time.Second)
		help = append(help, text...)
		help = append(help, '\n')
	}
	caps.SupportsHeadless = len(spec.headless) > 0
	for _, group := range spec.headless {
		if !hasAnyToken(help, group) {
			caps.SupportsHeadless = false
			caps.Missing = append(caps.Missing, strings.Join(group, "|"))
		}
	}
	caps.SupportsStructuredLog = hasAnyToken(help, spec.structured)
	if !caps.SupportsHeadless || !caps.SupportsStructuredLog {
		caps.Blockers = appendOnce(caps.Blockers, "headless_flags_not_found")
		return caps
	}
	if adapter == "opencode" && (req.ModelID == "" || !strings.Contains(req.ModelID, "/")) {
		caps.Blockers = appendOnce(caps.Blockers, "model_id_must_be_provider_slash_model")
	}
	configured := map[string]bool{}
	for _, name := range req.CredentialEnv {
		configured[name] = true
		if v, ok := lookup(name); ok && strings.TrimSpace(v) != "" && ValidCredentialEnv(name) {
			caps.CredentialSources = appendOnce(caps.CredentialSources, "env:"+name)
		} else if ValidCredentialEnv(name) {
			caps.Hints = append(caps.Hints, "已配置环境变量名 "+name+"，但控制面进程里没有这个值。")
		}
	}
	for _, name := range KnownCredentialEnv(adapter, req.ModelID) {
		if configured[name] {
			continue
		}
		if v, ok := lookup(name); ok && strings.TrimSpace(v) != "" {
			caps.Hints = append(caps.Hints, "控制面环境里有 "+name+"，但配置没有把它列进 credential_env，运行时不会传给 Agent。")
		}
	}
	for _, rel := range spec.credFiles {
		full := filepath.Join(home, rel)
		if st, err := os.Stat(full); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
			if req.InheritHome {
				caps.CredentialSources = appendOnce(caps.CredentialSources, "file:~/"+rel)
			} else {
				caps.Hints = append(caps.Hints, "找到登录文件 ~/"+rel+"，但运行用的是空 HOME。打开 inherit_home 才能用它。")
			}
		}
	}
	if len(caps.CredentialSources) == 0 {
		caps.Readiness = string(ReadinessNeedsCredentials)
		caps.Blockers = appendOnce(caps.Blockers, "credentials_not_found")
		return caps
	}
	if len(caps.Blockers) > 0 {
		return caps
	}
	caps.Readiness = string(ReadinessReadyUnverified)
	return caps
}

func runProbe(ctx context.Context, path string, args, env []string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	cmd.Dir = os.TempDir()
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = time.Second
	var buf bytes.Buffer
	cmd.Stdout = &limitedBuffer{buf: &buf, max: 256 << 10}
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	if ctx.Err() != nil {
		return buf.Bytes(), errors.New("probe timed out")
	}
	return buf.Bytes(), err
}

type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

func hasAnyToken(text []byte, tokens []string) bool {
	for _, tok := range tokens {
		re := regexp.MustCompile(`(^|[\s,\[\(|"'=])` + regexp.QuoteMeta(tok) + `($|[\s,\]\)|"'=<.:])`)
		if re.Match(text) {
			return true
		}
	}
	return false
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

func appendOnce(list []string, v string) []string {
	for _, item := range list {
		if item == v {
			return list
		}
	}
	list = append(list, v)
	sort.Strings(list)
	return list
}
