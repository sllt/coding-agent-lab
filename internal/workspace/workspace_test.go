package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectsUntrackedAndBinaryWithoutTouchingSource(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, ".git", "index"), []byte("index-v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "keep.txt"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	if err := CopyBaseline(src, base, Limits{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, ".git")); !os.IsNotExist(err) {
		t.Fatal(".git was copied into the workspace")
	}
	next := t.TempDir()
	if err := CopyBaseline(src, next, Limits{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(next, "new.txt"), []byte("created"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(next, "blob.bin"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(next, "keep.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(next, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(next, ".git", "index"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch, err := Collect(base, next, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, c := range patch.Changes {
		actions[c.Path] = c.Action
		if c.Path == ".git/index" {
			t.Fatal("index change was treated as a source change")
		}
	}
	if actions["new.txt"] != "add" || actions["blob.bin"] != "add" || actions["keep.txt"] != "delete" {
		t.Fatalf("actions %#v", actions)
	}
	rebuilt := t.TempDir()
	got, err := Apply(base, rebuilt, patch, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if got != patch.Digest {
		t.Fatalf("rebuilt digest %s want %s", got, patch.Digest)
	}
	source, err := os.ReadFile(filepath.Join(src, "keep.txt"))
	if err != nil || string(source) != "same" {
		t.Fatalf("source changed: %q %v", source, err)
	}
	index, err := os.ReadFile(filepath.Join(src, ".git", "index"))
	if err != nil || string(index) != "index-v1" {
		t.Fatalf("source git index changed: %q", index)
	}
}

func TestRejectsSymlinkEscapeAndSpecialFiles(t *testing.T) {
	src := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("hidden-verifier"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(src, "leak")); err != nil {
		t.Fatal(err)
	}
	if err := CopyBaseline(src, t.TempDir(), Limits{}); err == nil {
		t.Fatal("symlink was copied")
	}
	if err := os.Remove(filepath.Join(src, "leak")); err != nil {
		t.Fatal(err)
	}
	if err := syscallMknod(filepath.Join(src, "fifo")); err != nil {
		t.Skip(err.Error())
	}
	if err := CopyBaseline(src, t.TempDir(), Limits{}); err == nil {
		t.Fatal("fifo was copied")
	}
}

func TestPathEscapeIsRejectedOnApply(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := Patch{Changes: []Change{{Path: "../outside.txt", Action: "add", Digest: "x", Content: []byte("no")}}}
	if _, err := Apply(base, t.TempDir(), patch, Limits{}); err == nil {
		t.Fatal("path escape applied")
	}
}
