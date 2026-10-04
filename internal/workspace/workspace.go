// Package workspace copies an authorized baseline and collects a bounded
// change set from the filesystem. It does not trust git diff or the index.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	DefaultMaxFiles     = 10000
	DefaultMaxFileBytes = 8 << 20
	DefaultMaxBytes     = 512 << 20
)

type Limits struct {
	MaxFiles     int
	MaxFileBytes int64
	MaxBytes     int64
}

func (l Limits) normalize() Limits {
	if l.MaxFiles <= 0 {
		l.MaxFiles = DefaultMaxFiles
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = DefaultMaxFileBytes
	}
	if l.MaxBytes <= 0 {
		l.MaxBytes = DefaultMaxBytes
	}
	return l
}

type Entry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	Binary bool   `json:"binary"`
	Mode   uint32 `json:"mode"`
}

type Change struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Digest  string `json:"digest,omitempty"`
	Size    int64  `json:"size"`
	Binary  bool   `json:"binary"`
	Mode    uint32 `json:"mode,omitempty"`
	Content []byte `json:"-"`
}

type Patch struct {
	BaseDigest string   `json:"base_digest"`
	Digest     string   `json:"digest"`
	Changes    []Change `json:"changes"`
}

var (
	ErrSymlink  = errors.New("symlink rejected")
	ErrSpecial  = errors.New("special file rejected")
	ErrEscape   = errors.New("path escapes workspace")
	ErrTooLarge = errors.New("workspace limit exceeded")
	ErrHardlink = errors.New("hard link rejected")
)

// CopyBaseline copies regular files. Symlinks, devices and .git are rejected
// or skipped: .git is skipped so the source history is not exposed.
func CopyBaseline(src, dst string, limits Limits) error {
	limits = limits.normalize()
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	var files int
	var total int64
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := safeRel(rel); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSymlink, rel)
		}
		target := filepath.Join(dst, rel)
		if !inside(dst, target) {
			return fmt.Errorf("%w: %s", ErrEscape, rel)
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", ErrSpecial, rel)
		}
		if err := rejectHardlink(info); err != nil {
			return fmt.Errorf("%w: %s", err, rel)
		}
		files++
		total += info.Size()
		if files > limits.MaxFiles || info.Size() > limits.MaxFileBytes || total > limits.MaxBytes {
			return ErrTooLarge
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func Inventory(root string, limits Limits) ([]Entry, error) {
	limits = limits.normalize()
	root = filepath.Clean(root)
	var out []Entry
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.Name() == ".git" && d.IsDir() {
			return filepath.SkipDir
		}
		if err := safeRel(rel); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSymlink, rel)
		}
		if d.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", ErrSpecial, rel)
		}
		if err := rejectHardlink(info); err != nil {
			return fmt.Errorf("%w: %s", err, rel)
		}
		if info.Size() > limits.MaxFileBytes || len(out)+1 > limits.MaxFiles || total+info.Size() > limits.MaxBytes {
			return ErrTooLarge
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		total += info.Size()
		out = append(out, Entry{
			Path: filepath.ToSlash(rel), Digest: hex.EncodeToString(sum[:]), Size: info.Size(),
			Binary: bytesBinary(body), Mode: uint32(info.Mode().Perm()),
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

func Collect(baseRoot, nextRoot string, limits Limits) (Patch, error) {
	base, err := Inventory(baseRoot, limits)
	if err != nil {
		return Patch{}, err
	}
	next, err := Inventory(nextRoot, limits)
	if err != nil {
		return Patch{}, err
	}
	baseMap := map[string]Entry{}
	for _, e := range base {
		baseMap[e.Path] = e
	}
	nextMap := map[string]Entry{}
	for _, e := range next {
		nextMap[e.Path] = e
	}
	var changes []Change
	for _, e := range next {
		old, ok := baseMap[e.Path]
		if ok && old.Digest == e.Digest && old.Mode == e.Mode {
			continue
		}
		body, err := os.ReadFile(filepath.Join(nextRoot, filepath.FromSlash(e.Path)))
		if err != nil {
			return Patch{}, err
		}
		action := "add"
		if ok {
			action = "modify"
		}
		changes = append(changes, Change{Path: e.Path, Action: action, Digest: e.Digest, Size: e.Size, Binary: e.Binary, Mode: e.Mode, Content: body})
	}
	for _, e := range base {
		if _, ok := nextMap[e.Path]; !ok {
			changes = append(changes, Change{Path: e.Path, Action: "delete", Digest: e.Digest, Size: e.Size, Binary: e.Binary})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	baseDigest, err := digestEntries(base)
	if err != nil {
		return Patch{}, err
	}
	nextDigest, err := digestEntries(next)
	if err != nil {
		return Patch{}, err
	}
	return Patch{BaseDigest: baseDigest, Digest: nextDigest, Changes: changes}, nil
}

// Apply copies baseline and overlays the patch. The returned digest is computed
// from the rebuilt tree and must match the collected digest.
func Apply(baseline, dest string, patch Patch, limits Limits) (string, error) {
	if err := CopyBaseline(baseline, dest, limits); err != nil {
		return "", err
	}
	for _, c := range patch.Changes {
		if err := safeRel(c.Path); err != nil {
			return "", err
		}
		target := filepath.Join(dest, filepath.FromSlash(c.Path))
		if !inside(dest, target) {
			return "", ErrEscape
		}
		switch c.Action {
		case "delete":
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		case "add", "modify":
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			mode := os.FileMode(c.Mode)
			if mode == 0 {
				mode = 0o644
			}
			if err := os.WriteFile(target, c.Content, mode); err != nil {
				return "", err
			}
			sum := sha256.Sum256(c.Content)
			if hex.EncodeToString(sum[:]) != c.Digest {
				return "", fmt.Errorf("blob digest mismatch for %s", c.Path)
			}
		default:
			return "", fmt.Errorf("unsupported change %s", c.Action)
		}
	}
	entries, err := Inventory(dest, limits)
	if err != nil {
		return "", err
	}
	return digestEntries(entries)
}

func digestEntries(entries []Entry) (string, error) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s %s %d %t\n", e.Path, e.Digest, e.Mode, e.Binary)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func safeRel(rel string) error {
	if rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || strings.Contains(rel, "\x00") {
		return ErrEscape
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return ErrEscape
	}
	return nil
}

func inside(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func rejectHardlink(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if ok && stat.Nlink > 1 {
		return ErrHardlink
	}
	return nil
}

func bytesBinary(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
