package verifier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNestedHiddenTestsKeepTheirDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "hidden", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "hidden", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hidden", "a", "foo_test.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hidden", "b", "foo_test.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := copyHidden(root, dest); err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(filepath.Join(dest, "a", "foo_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "b", "foo_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != "package a\n" || string(b) != "package b\n" {
		t.Fatalf("nested hidden tests collapsed: %q %q", a, b)
	}
}
