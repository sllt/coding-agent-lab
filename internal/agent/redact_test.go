package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyntheticRedactionFixturesStayObservations(t *testing.T) {
	matches, err := filepath.Glob("testdata/*-redacted.json")
	if err != nil || len(matches) != 3 {
		t.Fatalf("fixtures %v %v", matches, err)
	}
	for _, name := range matches {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "synthetic contract sample") {
			t.Fatalf("%s is not marked synthetic", name)
		}
		events, err := DecodeLine("stdout", raw)
		if err != nil || len(events) == 0 {
			t.Fatal(err)
		}
		for _, ev := range events {
			if ev.Origin != "agent_observation" {
				t.Fatalf("%s origin %s", name, ev.Origin)
			}
			if ev.Type == "Finished" || ev.Type == "CleanupCompleted" || ev.Type == "VerifiedPass" || ev.Type == "AuthenticationFailed" {
				t.Fatalf("%s promoted %s", name, ev.Type)
			}
		}
	}
}
