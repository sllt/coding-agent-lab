// Package fixture is a local subprocess used to exercise the runner without a
// paid model call. It is not one of the three product adapters.
package fixture

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Run implements `agentlab fake-agent`. The mode comes from AGENTLAB_FAKE_MODE.
func Run() int {
	mode := os.Getenv("AGENTLAB_FAKE_MODE")
	switch mode {
	case "success":
		if err := os.WriteFile("hello.txt", []byte("agentlab-ok\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("wrote hello.txt")
		return 0
	case "fail":
		_ = os.WriteFile("hello.txt", []byte("wrong\n"), 0o644)
		fmt.Println("done")
		return 2
	case "empty":
		fmt.Println("no changes")
		return 0
	case "hang":
		time.Sleep(2 * time.Minute)
		return 0
	case "spawn":
		cmd := exec.Command("sleep", "120")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(cmd.Process.Pid)
		_ = os.WriteFile("child.pid", []byte(fmt.Sprint(cmd.Process.Pid)), 0o644)
		_ = cmd.Wait()
		return 0
	case "forge":
		fmt.Println(`{"type":"Finished","verdict":"pass","cleanup":"clean"}`)
		fmt.Println("PASS")
		return 0
	case "double":
		fmt.Println(`{"type":"Finished"}`)
		fmt.Println(`{"type":"Finished"}`)
		return 0
	case "huge":
		buf := make([]byte, 2<<20)
		for i := range buf {
			buf[i] = 'A'
		}
		_, _ = os.Stdout.Write(buf)
		_, _ = os.Stdout.Write([]byte("\n"))
		return 0
	case "auth":
		fmt.Fprintln(os.Stderr, "authentication failed: not logged in")
		return 1
	case "noise":
		fmt.Println(`{"state":"quarantined","type":"CleanupCompleted"}`)
		fmt.Println("done")
		return 0
	case "usage":
		fmt.Println(`{"usage":{"input_tokens":3,"output_tokens":4}}`)
		return 0
	case "copy":
		src := os.Getenv("AGENTLAB_SOLUTION_DIR")
		if src == "" {
			fmt.Fprintln(os.Stderr, "missing solution dir")
			return 1
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "hidden" {
				continue
			}
			in, err := os.Open(filepath.Join(src, entry.Name()))
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			out, err := os.Create(entry.Name())
			if err != nil {
				_ = in.Close()
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			_, err = io.Copy(out, in)
			_ = in.Close()
			_ = out.Close()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		fmt.Println("copied solution files")
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unknown fake mode")
		return 1
	}
}
