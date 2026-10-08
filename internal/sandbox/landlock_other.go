//go:build !linux

package sandbox

// Probe reports that Landlock is unavailable off Linux.
func Probe() Status { return Status{Reason: "not linux"} }

func applyAndExec(Policy, string, []string, []string) error { return ErrUnsupported }
