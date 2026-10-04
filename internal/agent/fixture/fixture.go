// Package fixture is a local subprocess used to exercise the runner without a
// paid model call. It is not one of the three product adapters.
package fixture

import (
	"fmt"
	"os"
	"os/exec"
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
	default:
		fmt.Fprintln(os.Stderr, "unknown fake mode")
		return 1
	}
}
