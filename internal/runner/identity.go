package runner

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// ProcessIdentity is recorded by the runner in memory. The agent work
// directory's journal is not a source of truth for process signals.
type ProcessIdentity struct {
	PID       int
	StartTime string
	Token     string
}

var liveIdentities sync.Map

// RememberIdentity stores the kernel identity of a process this runner started.
func RememberIdentity(token string, pid int, startTime string) {
	if token == "" || pid <= 0 {
		return
	}
	liveIdentities.Store(token, ProcessIdentity{PID: pid, StartTime: startTime, Token: token})
}

// RecallIdentity returns the identity recorded for token.
func RecallIdentity(token string) (ProcessIdentity, bool) {
	if token == "" {
		return ProcessIdentity{}, false
	}
	v, ok := liveIdentities.Load(token)
	if !ok {
		return ProcessIdentity{}, false
	}
	id, ok := v.(ProcessIdentity)
	return id, ok
}

// ProcStartTime reads the kernel start time of pid. A reused PID will not match
// the value recorded when the process was created.
func ProcStartTime(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", false
	}
	s := string(b)
	i := strings.LastIndex(s, ")")
	if i < 0 || i+2 >= len(s) {
		return "", false
	}
	fields := strings.Fields(s[i+2:])
	// Field 22 of /proc/pid/stat is starttime. After the comm field it is index 19.
	if len(fields) < 20 {
		return "", false
	}
	return fields[19], true
}
