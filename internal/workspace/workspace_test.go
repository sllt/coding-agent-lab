package workspace

import (
	"encoding/json"
	"errors"
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

func TestCollectSkipsRunnerBookkeeping(t *testing.T) {
	base := t.TempDir()
	next := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "orders.go"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(next, "orders.go"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"journal.json", "agent.log", "patch.json"} {
		if err := os.WriteFile(filepath.Join(next, name), []byte("runner"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(next, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(next, "notes", "journal.json"), []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch, err := Collect(base, next, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, c := range patch.Changes {
		actions[c.Path] = c.Action
	}
	if actions["orders.go"] != "modify" || actions["notes/journal.json"] != "add" {
		t.Fatalf("actions %#v", actions)
	}
	for _, name := range []string{"journal.json", "agent.log", "patch.json"} {
		if _, ok := actions[name]; ok {
			t.Fatalf("runner file included: %#v", actions)
		}
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

func TestPatchRoundTripKeepsBytesAndMode(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "run.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	next := t.TempDir()
	if err := CopyBaseline(base, next, Limits{}); err != nil {
		t.Fatal(err)
	}
	body := []byte("#!/bin/sh\necho ok\n")
	if err := os.WriteFile(filepath.Join(next, "run.sh"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(next, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	patch, err := Collect(base, next, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		t.Fatal(err)
	}
	var again Patch
	if err := json.Unmarshal(raw, &again); err != nil {
		t.Fatal(err)
	}
	if len(again.Changes) != 1 || string(again.Changes[0].Content) != string(body) || again.Changes[0].Mode != 0o755 {
		t.Fatalf("round trip %+v", again.Changes)
	}
	dest := t.TempDir()
	if _, err := Apply(base, dest, again, Limits{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dest, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	bad := again
	bad.Digest = "not-the-tree"
	if _, err := Apply(base, t.TempDir(), bad, Limits{}); err == nil {
		t.Fatal("wrong tree digest was accepted")
	}
}

func TestCopyRejectsOverlapAndDeepTrees(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyBaseline(src, filepath.Join(src, "inside"), Limits{}); !errors.Is(err, ErrOverlap) {
		t.Fatalf("overlap err %v", err)
	}
	deep := t.TempDir()
	cur := deep
	for i := 0; i < 40; i++ {
		cur = filepath.Join(cur, "d")
	}
	if err := os.MkdirAll(cur, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cur, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyBaseline(deep, t.TempDir(), Limits{}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("depth err %v", err)
	}
}
