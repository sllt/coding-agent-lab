// Package sandbox applies a Linux Landlock filesystem policy to an agent
// process before it starts.
//
// What it does: the agent (and everything it spawns) can only read and
// execute under the listed read-only roots and can only write under the listed
// read-write roots. Everything else on the filesystem, notably the control
// plane's data directory, other users' homes and the operator's real HOME
// (unless inherit_home is on) becomes inaccessible.
//
// What it does not do: it does not restrict network access, process
// visibility (/proc), signals to other processes of the same user, IPC,
// resource usage or syscalls in general. Paths under a read-only root are all
// readable, so a data directory placed under one of them stays readable. It is
// partial isolation for trusted-but-fallible agents, not a security boundary
// against a hostile one; use a VM or container for that.
package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Policy lists the filesystem roots the sandboxed process may use.
type Policy struct {
	ReadOnly  []string `json:"ro"`
	ReadWrite []string `json:"rw"`
}

// Status describes whether Landlock can be used on this host.
type Status struct {
	Supported bool   `json:"supported"`
	ABI       int    `json:"abi"`
	Reason    string `json:"reason,omitempty"`
}

// ErrUnsupported is returned when the kernel offers no Landlock.
var ErrUnsupported = errors.New("landlock is not available on this host")

// Disabled reports whether the operator turned the sandbox off.
func Disabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AGENTLAB_SANDBOX")))
	return v == "off" || v == "0" || v == "false" || v == "none"
}

// SystemReadOnly is the default read-only root set: system binaries,
// libraries and configuration. Only roots that exist are returned.
func SystemReadOnly() []string {
	candidates := []string{"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/etc", "/opt", "/proc", "/sys", "/run", "/nix", "/snap", "/var/lib", "/usr/local"}
	return existing(candidates)
}

// Build returns a policy for one agent run. exe is the resolved agent binary;
// pathEnv is the PATH the agent will see. writable lists directories the agent
// must write (work tree, private HOME, TMPDIR). deny lists directories that
// must not be reachable; any read-only root containing one of them is
// reported in exposed so the run can record it.
func Build(exe, pathEnv string, writable, deny []string) (Policy, []string) {
	ro := SystemReadOnly()
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir != "" && filepath.IsAbs(dir) {
			ro = append(ro, dir)
		}
	}
	if exe != "" {
		ro = append(ro, filepath.Dir(exe))
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			// node-style CLIs live in <prefix>/lib/node_modules/<pkg>/bin; the
			// interpreter and the package tree both sit under the prefix.
			ro = append(ro, filepath.Dir(real), filepath.Dir(filepath.Dir(real)))
			if prefix := installPrefix(real); prefix != "" {
				ro = append(ro, prefix)
			}
		}
	}
	rw := append([]string{"/dev"}, writable...)
	ro = dedupe(existing(ro))
	rw = dedupe(existing(rw))
	// A read-only root that contains a denied directory would expose it.
	// Roots derived from PATH or the binary location are dropped in that
	// case; system roots are kept (the agent cannot run without them) and the
	// exposure is reported so the run can record it.
	system := map[string]bool{}
	for _, r := range SystemReadOnly() {
		system[r] = true
	}
	var keep []string
	var exposed []string
	for _, root := range ro {
		if root == "/" {
			continue
		}
		hit := false
		for _, d := range deny {
			if d != "" && within(root, d) {
				hit = true
				if system[root] {
					exposed = append(exposed, d+" (under "+root+")")
				}
			}
		}
		if hit && !system[root] {
			continue
		}
		keep = append(keep, root)
	}
	return Policy{ReadOnly: keep, ReadWrite: rw}, dedupe(exposed)
}

// installPrefix returns the directory above .../lib/node_modules or
// .../libexec when exe sits inside such a tree.
func installPrefix(path string) string {
	for _, marker := range []string{"/lib/node_modules/", "/libexec/"} {
		if i := strings.Index(path, marker); i > 0 {
			return path[:i]
		}
	}
	return ""
}

func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

func existing(paths []string) []string {
	var out []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Encode serialises a policy for the sandbox-exec argument.
func (p Policy) Encode() (string, error) {
	b, err := json.Marshal(p)
	return string(b), err
}

// Exec is the "sandbox-exec <policy-json> -- <exe> [args...]" helper. It
// applies the policy to the current thread and replaces the process with
// exe. It only returns on failure, and then the caller must exit non-zero:
// the agent never runs unconfined when a policy was requested.
func Exec(args []string) error {
	if len(args) < 3 || args[1] != "--" {
		return errors.New("usage: sandbox-exec <policy-json> -- <exe> [args...]")
	}
	var p Policy
	if err := json.Unmarshal([]byte(args[0]), &p); err != nil {
		return fmt.Errorf("sandbox policy: %w", err)
	}
	argv := args[2:]
	exe := argv[0]
	if !strings.ContainsRune(exe, '/') {
		found, err := exec.LookPath(exe)
		if err != nil {
			return err
		}
		exe = found
	}
	return applyAndExec(p, exe, argv, os.Environ())
}
