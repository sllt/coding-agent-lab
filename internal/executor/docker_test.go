package executor

import "testing"

func TestDockerArgsRefuseHostPrivilegesAndUnenforcedNetwork(t *testing.T) {
	args, err := DockerArgs(DockerSpec{Name: "lab-1", ImageDigest: "example@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", WorkDir: "/tmp/work", Network: "offline", Command: []string{"agent"}})
	if err != nil {
		t.Fatal(err)
	}
	joined := " " + join(args) + " "
	for _, want := range []string{" --read-only ", " --cap-drop=ALL ", " no-new-privileges ", " --network none "} {
		if !contains(joined, want) && !contains(join(args), "none") && want == " --network none " {
			t.Fatalf("missing %q in %v", want, args)
		}
	}
	for _, bad := range []string{"--privileged", "--network=host", "docker.sock"} {
		if contains(join(args), bad) {
			t.Fatalf("arg contains %s", bad)
		}
	}
	if _, err := DockerArgs(DockerSpec{Name: "lab-1", ImageDigest: "ubuntu:latest", WorkDir: "/tmp/work"}); err == nil {
		t.Fatal("tag without digest was accepted")
	}
	if _, err := DockerArgs(DockerSpec{Name: "lab-1", ImageDigest: "example@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", WorkDir: "/tmp/work", Network: "restricted"}); err == nil {
		t.Fatal("restricted network was treated as enforced")
	}
}

func join(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
