// Package executor builds containment commands. A network name is not a sandbox.
package executor

import "fmt"

type DockerSpec struct {
	Name        string
	ImageDigest string
	WorkDir     string
	Command     []string
	Network     string
	CPUs        string
	Memory      string
	PIDs        string
	User        string
}

// DockerArgs returns a non-privileged docker run argv. restricted is refused
// because this process has no enforceable egress filter.
func DockerArgs(spec DockerSpec) ([]string, error) {
	if spec.ImageDigest == "" || !contains(spec.ImageDigest, "sha256:") {
		return nil, fmt.Errorf("image must be pinned by digest")
	}
	switch spec.Network {
	case "", "unrestricted":
	case "offline":
	case "restricted":
		return nil, fmt.Errorf("network=restricted is not enforced")
	default:
		return nil, fmt.Errorf("unknown network %s", spec.Network)
	}
	if spec.Name == "" {
		return nil, fmt.Errorf("container name is required")
	}
	args := []string{
		"run", "--name", spec.Name, "--rm",
		"--read-only", "--cap-drop=ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", or(spec.PIDs, "256"),
		"--memory", or(spec.Memory, "1g"),
		"--cpus", or(spec.CPUs, "1"),
		"--user", or(spec.User, "65534:65534"),
		"--mount", "type=bind,src=" + spec.WorkDir + ",dst=/work",
		"--workdir", "/work",
		"--tmpfs", "/tmp:rw,nosuid,size=64m",
	}
	if spec.Network == "offline" {
		args = append(args, "--network", "none")
	}
	for _, bad := range []string{"--privileged", "--network=host", "/var/run/docker.sock"} {
		for _, a := range args {
			if a == bad || contains(a, "/var/run/docker.sock") {
				return nil, fmt.Errorf("refusing %s", bad)
			}
		}
	}
	args = append(args, spec.ImageDigest)
	args = append(args, spec.Command...)
	return args, nil
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func contains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || indexOf(s, part) >= 0)
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
