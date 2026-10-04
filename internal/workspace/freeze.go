package workspace

import (
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

// Freeze copies src into destRoot/<content digest> and returns that directory,
// the content digest, and a commit id. A directory that is not its own git
// root is pinned by content digest instead of a parent repository's HEAD.
func Freeze(src, destRoot string) (dir, digest, commit string, err error) {
	digest, err = ContentDigest(src)
	if err != nil {
		return "", "", "", err
	}
	dir = filepath.Join(destRoot, digest)
	if _, statErr := os.Stat(dir); statErr != nil {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", "", "", err
		}
		if err := CopyBaseline(src, dir, Limits{}); err != nil {
			return "", "", "", err
		}
	}
	commit = gitCommit(src)
	if commit == "" {
		commit = "content:" + digest
	}
	return dir, digest, commit, nil
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
