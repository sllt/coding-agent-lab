package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNotAllowed = errors.New("path is outside an allowed root")

// WithinAllowed reports whether src resolves inside one of roots and outside
// every deny prefix. Local task trees may only come from an administrator's
// registered roots. The control-plane data directory is always denied.
func WithinAllowed(src string, roots, deny []string) error {
	abs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = filepath.Clean(resolved)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: not a directory", ErrNotAllowed)
	}
	for _, blocked := range deny {
		if blocked == "" {
			continue
		}
		b, err := filepath.Abs(blocked)
		if err != nil {
			continue
		}
		b = filepath.Clean(b)
		if resolved, err := filepath.EvalSymlinks(b); err == nil {
			b = filepath.Clean(resolved)
		}
		if abs == b || inside(b, abs) {
			return fmt.Errorf("%w: %s is inside a denied directory", ErrNotAllowed, abs)
		}
	}
	if len(roots) == 0 {
		return fmt.Errorf("%w: no allowed root is registered", ErrNotAllowed)
	}
	for _, root := range roots {
		r, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		r = filepath.Clean(r)
		if resolved, err := filepath.EvalSymlinks(r); err == nil {
			r = filepath.Clean(resolved)
		}
		if abs == r || inside(r, abs) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotAllowed, abs)
}

// Freeze copies src into destRoot/<pre-image digest> and returns that directory,
// the digest of the frozen tree, and a resolved commit id. A directory that is
// not its own git root is pinned as content:<digest> instead of a parent HEAD.
// The returned digest is recomputed from the snapshot, so a later edit of src
// cannot change what execution reads.
func Freeze(src, destRoot string) (dir, digest, commit string, err error) {
	key, err := ContentDigest(src)
	if err != nil {
		return "", "", "", err
	}
	dir = filepath.Join(destRoot, key)
	if _, statErr := os.Stat(dir); statErr != nil {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", "", "", err
		}
		if err := CopyBaseline(src, dir, Limits{}); err != nil {
			return "", "", "", err
		}
		if err := writeManifest(dir); err != nil {
			return "", "", "", err
		}
	}
	digest, err = ContentDigest(dir)
	if err != nil {
		return "", "", "", err
	}
	commit = gitCommit(src)
	if commit == "" {
		commit = "content:" + digest
	}
	return dir, digest, commit, nil
}

// ResolveCommit turns a git ref into a commit id. A content pin is returned
// unchanged. Anything else that does not resolve is an error, not a label.
func ResolveCommit(dir, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("empty commit ref")
	}
	if strings.HasPrefix(ref, "content:") {
		return ref, nil
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}").Output()
	if err != nil {
		return "", fmt.Errorf("commit ref %q did not resolve", ref)
	}
	sha := strings.TrimSpace(string(out))
	if len(sha) != 40 {
		return "", fmt.Errorf("commit ref %q resolved to %q", ref, sha)
	}
	return sha, nil
}

func writeManifest(dir string) error {
	entries, err := Inventory(dir, Limits{})
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"files": entries})
	if err != nil {
		return err
	}
	// The manifest sits beside the snapshot so it is not part of the agent tree.
	return os.WriteFile(dir+".manifest.json", body, 0o644)
}

// ContentDigest is the stable tree digest used as an execution snapshot.
func ContentDigest(root string) (string, error) {
	entries, err := Inventory(root, Limits{})
	if err != nil {
		return "", err
	}
	return digestEntries(entries)
}

func gitCommit(dir string) string {
	top, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	topAbs, err := filepath.Abs(strings.TrimSpace(string(top)))
	if err != nil || filepath.Clean(abs) != filepath.Clean(topAbs) {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
