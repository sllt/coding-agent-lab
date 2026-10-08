package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "sandbox-exec" {
		if err := Exec(os.Args[2:]); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(126)
		}
	}
	os.Exit(m.Run())
}

func TestLandlockConfinesFilesystem(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	if st := Probe(); !st.Supported {
		t.Skipf("landlock unavailable: %s", st.Reason)
	}
	work := t.TempDir()
	secret := t.TempDir()
	if err := os.WriteFile(filepath.Join(secret, "s.txt"), []byte("top"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	policy, _ := Build(sh, "/usr/bin:/bin", []string{work}, []string{secret})
	enc, err := policy.Encode()
	if err != nil {
		t.Fatal(err)
	}
	script := `echo ok > "$1/out.txt" && cat "$2/s.txt"; echo "exit=$?"; echo x > "$2/w.txt"; echo "wexit=$?"`
	cmd := exec.Command(os.Args[0], "sandbox-exec", enc, "--", sh, "-c", script, "sh", work, secret)
	out, _ := cmd.CombinedOutput()
	text := string(out)
	if b, err := os.ReadFile(filepath.Join(work, "out.txt")); err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("work dir not writable: %v %q (%s)", err, b, text)
	}
	if strings.Contains(text, "top") || !strings.Contains(text, "exit=1") {
		t.Fatalf("secret readable: %s", text)
	}
	if _, err := os.Stat(filepath.Join(secret, "w.txt")); err == nil {
		t.Fatalf("secret dir writable: %s", text)
	}
}

func TestBuildDropsRootsThatContainDeniedDirs(t *testing.T) {
	parent := t.TempDir()
	data := filepath.Join(parent, "data")
	bin := filepath.Join(parent, "bin")
	for _, d := range []string{data, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := Build(filepath.Join(bin, "agent"), "/usr/bin:"+parent, nil, []string{data})
	for _, r := range p.ReadOnly {
		if r == parent {
			t.Fatalf("root containing the data dir kept: %v", p.ReadOnly)
		}
	}
	found := false
	for _, r := range p.ReadOnly {
		if r == bin {
			found = true
		}
	}
	if !found {
		t.Fatalf("binary dir missing: %v", p.ReadOnly)
	}
}
