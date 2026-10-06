package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sllt/agentlab/internal/agent"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "fake-agent" {
		os.Exit(fake())
	}
	os.Exit(m.Run())
}

func fake() int {
	// Imported behavior lives in the fixture package. Duplicate the small
	// dispatch here so the test binary can re-exec without the full CLI.
	mode := os.Getenv("AGENTLAB_FAKE_MODE")
	switch mode {
	case "spawn":
		cmd := exec.Command("sleep", "120")
		if err := cmd.Start(); err != nil {
			return 1
		}
		_ = os.WriteFile("child.pid", []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
		_ = cmd.Wait()
		return 0
	case "forge":
		_, _ = os.Stdout.WriteString("{\"type\":\"Finished\",\"verdict\":\"pass\"}\nPASS\n")
		return 0
	case "hang":
		time.Sleep(time.Minute)
		return 0
	case "setsid":
		cmd := exec.Command("sleep", "120")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return 1
		}
		_ = os.WriteFile("child.pid", []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
		_ = cmd.Wait()
		return 0
	case "flood":
		for i := 0; i < 80; i++ {
			_, _ = os.Stdout.WriteString(strings.Repeat("x", 40) + "\n")
		}
		time.Sleep(30 * time.Second)
		return 0
	default:
		_ = os.WriteFile("hello.txt", []byte("agentlab-ok\n"), 0o644)
		return 0
	}
}

func TestCancelKillsGrandchildAndForgedFinishIsNotControl(t *testing.T) {
	for _, mode := range []string{"spawn", "forge", "success"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			work := t.TempDir()
			if err := os.WriteFile(filepath.Join(base, "readme.txt"), []byte("base"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(work, "readme.txt"), []byte("base"), 0o644); err != nil {
				t.Fatal(err)
			}
			spec := Spec{
				SchemaVersion: "agentlab.exec/v1", AttemptID: "att-1", Fence: 3, Token: "secret-token",
				WorkDir: work, BaselineDir: base, Executor: "native-trusted", Network: "unrestricted", WallSeconds: 10,
				Launch: agent.LaunchSpec{Executable: os.Args[0], Args: []string{"fake-agent"}, Env: map[string]string{"AGENTLAB_FAKE_MODE": mode}},
			}
			pr, pw := io.Pipe()
			go func() {
				_, _ = pw.Write(must(map[string]any{"type": "Start", "token": spec.Token, "spec": spec}))
				if mode == "spawn" {
					time.Sleep(250 * time.Millisecond)
					b, _ := json.Marshal(map[string]string{"type": "Cancel", "reason": "user"})
					_, _ = pw.Write(append(b, '\n'))
				}
				time.Sleep(2 * time.Second)
				_ = pw.Close()
			}()
			var output bytes.Buffer
			err := Serve(t.Context(), pr, &output)
			if err != nil {
				t.Fatal(err)
			}
			events := parseEvents(t, output.Bytes())
			var finished, cleanup int
			var sawAgentFinishAsControl bool
			for _, ev := range events {
				if ev.Origin == "runner" && ev.Type == "Finished" {
					finished++
				}
				if ev.Origin == "runner" && ev.Type == "CleanupCompleted" {
					cleanup++
					if !strings.Contains(string(ev.Payload), `"state":"clean"`) {
						t.Fatalf("cleanup %s", ev.Payload)
					}
				}
				if ev.Origin != "agent_observation" && (ev.Type == "Finished") && finished == 0 {
					sawAgentFinishAsControl = true
				}
				if ev.Type == "VerifiedPass" {
					t.Fatal("agent text became a verdict event")
				}
			}
			if finished != 1 || cleanup != 1 {
				t.Fatalf("finished %d cleanup %d", finished, cleanup)
			}
			if sawAgentFinishAsControl {
				t.Fatal("forged finish promoted")
			}
			if mode == "spawn" {
				pidRaw, err := os.ReadFile(filepath.Join(work, "child.pid"))
				if err != nil {
					t.Fatal(err)
				}
				pid, _ := strconv.Atoi(strings.TrimSpace(string(pidRaw)))
				if syscall.Kill(pid, 0) == nil {
					t.Fatalf("grandchild %d still running", pid)
				}
			}
			if mode == "forge" {
				for _, ev := range events {
					if ev.Origin == "agent_observation" && strings.Contains(string(ev.Payload), "Finished") {
						if ev.Type == "Finished" {
							t.Fatal("forged type kept as control type")
						}
					}
				}
			}
		})
	}
}

func TestSetsidChildIsNotAliveAtCollect(t *testing.T) {
	base := t.TempDir()
	work := t.TempDir()
	control := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "readme.txt"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "readme.txt"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := Spec{
		SchemaVersion: "agentlab.exec/v1", AttemptID: "att-setsid", Fence: 1, Token: "secret-token",
		WorkDir: work, BaselineDir: base, ControlDir: control, Executor: "native-trusted", Network: "unrestricted", WallSeconds: 5,
		Launch: agent.LaunchSpec{Executable: os.Args[0], Args: []string{"fake-agent"}, Env: map[string]string{"AGENTLAB_FAKE_MODE": "setsid"}},
	}
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write(must(map[string]any{"type": "Start", "token": spec.Token, "spec": spec}))
		time.Sleep(200 * time.Millisecond)
		b, _ := json.Marshal(map[string]string{"type": "Cancel", "reason": "user"})
		_, _ = pw.Write(append(b, '\n'))
		time.Sleep(time.Second)
		_ = pw.Close()
	}()
	var output bytes.Buffer
	if err := Serve(t.Context(), pr, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, "journal.json")); !os.IsNotExist(err) {
		t.Fatalf("journal landed in the work tree: %v", err)
	}
	events := parseEvents(t, output.Bytes())
	var cleanup string
	for _, ev := range events {
		if ev.Origin == "runner" && ev.Type == "CleanupCompleted" {
			cleanup = string(ev.Payload)
		}
	}
	pidRaw, err := os.ReadFile(filepath.Join(work, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(pidRaw)))
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	if syscall.Kill(pid, 0) == nil && strings.Contains(cleanup, `"state":"clean"`) {
		t.Fatalf("setsid child %d still alive with %s", pid, cleanup)
	}
}

func TestLogCapStopsTheAgent(t *testing.T) {
	base := t.TempDir()
	work := t.TempDir()
	control := t.TempDir()
	spec := Spec{
		SchemaVersion: "agentlab.exec/v1", AttemptID: "att-log", Fence: 1, Token: "secret-token",
		WorkDir: work, BaselineDir: base, ControlDir: control, Executor: "native-trusted", Network: "unrestricted",
		WallSeconds: 8, LogBytes: 50,
		Launch: agent.LaunchSpec{Executable: os.Args[0], Args: []string{"fake-agent"}, Env: map[string]string{"AGENTLAB_FAKE_MODE": "flood"}},
	}
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write(must(map[string]any{"type": "Start", "token": spec.Token, "spec": spec}))
		time.Sleep(2 * time.Second)
		_ = pw.Close()
	}()
	var output bytes.Buffer
	if err := Serve(t.Context(), pr, &output); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(control, "agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 50 {
		t.Fatalf("log grew past the cap: %d", info.Size())
	}
	if !strings.Contains(output.String(), `"reason":"log_limit"`) {
		t.Fatalf("events %s", output.String())
	}
}

func TestSanitizedEnvDropsHomeAndSolution(t *testing.T) {
	t.Setenv("HOME", "/tmp/host-home-secret")
	spec := &Spec{AttemptID: "a", HomeDir: t.TempDir(), TmpDir: t.TempDir(), Launch: agent.LaunchSpec{Env: map[string]string{"AGENTLAB_SOLUTION_DIR": "/secret/orders.go"}}}
	for _, item := range sanitizedEnv(spec) {
		if strings.Contains(item, "host-home-secret") || strings.Contains(item, "SOLUTION") {
			t.Fatalf("env leaked %s", item)
		}
	}
}

func TestDockerExecutorIsRefused(t *testing.T) {
	spec := Spec{AttemptID: "a", Fence: 1, Token: "t", Executor: "docker", Network: "unrestricted", Launch: agent.LaunchSpec{Executable: "true"}}
	input := bytes.NewBuffer(must(map[string]any{"type": "Start", "token": "t", "spec": spec}))
	err := Serve(t.Context(), input, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "docker") {
		t.Fatal(err)
	}
}

func TestRestrictedNetworkIsRefused(t *testing.T) {
	spec := Spec{AttemptID: "a", Fence: 1, Token: "t", Network: "restricted", Launch: agent.LaunchSpec{Executable: "true"}}
	input := bytes.NewBuffer(must(map[string]any{"type": "Start", "token": "t", "spec": spec}))
	err := Serve(t.Context(), input, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not enforced") {
		t.Fatal(err)
	}
}

func parseEvents(t *testing.T, b []byte) []Event {
	t.Helper()
	var out []Event
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

func must(v any) []byte {
	b, _ := json.Marshal(v)
	return append(b, '\n')
}
