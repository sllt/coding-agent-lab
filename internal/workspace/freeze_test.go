package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreezePinsBytesOutsideTheParentRepository(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "marker.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	dir, digest, commit, err := Freeze(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || !strings.HasPrefix(commit, "content:") {
		t.Fatalf("parent repository HEAD was used: %s %s", digest, commit)
	}
	if err := os.WriteFile(filepath.Join(src, "marker.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "marker.txt"))
	if err != nil || string(body) != "v1" {
		t.Fatalf("frozen bytes %q err %v", body, err)
	}
	again, _, _, err := Freeze(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if again == dir {
		t.Fatal("a changed tree reused the previous snapshot directory")
	}
}

func TestDataDirIsDeniedEvenWhenListedAsRoot(t *testing.T) {
	root := t.TempDir()
	if err := WithinAllowed(root, []string{root}, []string{root}); err == nil {
		t.Fatal("a denied directory was accepted as an allowed root")
	}
}
