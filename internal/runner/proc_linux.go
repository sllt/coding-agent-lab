package runner

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const prSetChildSubreaper = 36

func enableSubreaper() {
	_, _, _ = syscall.RawSyscall6(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0, 0, 0, 0)
}

// stopManaged signals the process group and any descendants that reparented
// to this process after setsid. Native mode cannot see a cgroup, so callers
// must treat a non-empty descendant set as unproven.
func stopManaged(cmd *exec.Cmd) error {
	_ = killTree(cmd)
	agent := 0
	if cmd != nil && cmd.Process != nil {
		agent = cmd.Process.Pid
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		kids := childPIDs(os.Getpid())
		if len(kids) == 1 && kids[0] == -1 {
			return syscall.ESRCH
		}
		orphans := false
		for _, pid := range kids {
			if pid == agent {
				continue
			}
			orphans = true
			_ = syscall.Kill(pid, syscall.SIGKILL)
			var ws syscall.WaitStatus
			_, _ = syscall.Wait4(pid, &ws, syscall.WNOHANG, nil)
		}
		if agent > 0 {
			var ws syscall.WaitStatus
			_, _ = syscall.Wait4(-agent, &ws, syscall.WNOHANG, nil)
		}
		if !orphans && processGroupGone(cmd) {
			return nil
		}
		if time.Now().After(deadline) {
			return syscall.ESRCH
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func managedGone(cmd *exec.Cmd) bool {
	if !processGroupGone(cmd) {
		return false
	}
	return len(childPIDs(os.Getpid())) == 0
}

func childPIDs(parent int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []int{-1}
	}
	var out []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 || pid == parent {
			continue
		}
		stat, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if err != nil {
			continue
		}
		if parsePPID(stat) == parent {
			out = append(out, pid)
		}
	}
	return out
}

func parsePPID(stat []byte) int {
	i := strings.LastIndexByte(string(stat), ')')
	if i < 0 || i+2 >= len(stat) {
		return 0
	}
	fields := strings.Fields(string(stat[i+2:]))
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(fields[1])
	return n
}
