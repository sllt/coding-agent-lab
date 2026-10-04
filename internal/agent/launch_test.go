package agent

import (
	"context"
	"strings"
	"testing"
)

func TestLaunchArgsStayDistinctAndDoNotApproveByDefault(t *testing.T) {
	ctx := context.Background()
	req := LaunchRequest{ModelID: "example-model", Prompt: "修复分页", WorkDir: "/work"}
	cursor, err := (Cursor{}).BuildLaunch(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cursor.Args, " ") != "-p --output-format stream-json --model example-model 修复分页" {
		t.Fatalf("cursor args %v", cursor.Args)
	}
	if contains(cursor.Args, "--force") {
		t.Fatal("force was added without approval")
	}
	approved := req
	approved.ApproveTools = true
	cursor, err = (Cursor{}).BuildLaunch(ctx, approved)
	if err != nil || !contains(cursor.Args, "--force") {
		t.Fatalf("approved cursor %v %v", cursor.Args, err)
	}
	grok, err := (Grok{}).BuildLaunch(ctx, req)
	if err != nil || !contains(grok.Args, "--no-auto-update") || !contains(grok.Args, "streaming-json") {
		t.Fatalf("grok args %v %v", grok.Args, err)
	}
	if _, err := (OpenCode{}).BuildLaunch(ctx, req); err == nil {
		t.Fatal("opencode must reject a bare model id")
	}
	oc, err := (OpenCode{}).BuildLaunch(ctx, LaunchRequest{ModelID: "deepseek/example", Prompt: "修复分页", WorkDir: "/work"})
	if err != nil || !contains(oc.Args, "json") || contains(oc.Args, "--continue") {
		t.Fatalf("opencode %v %v", oc.Args, err)
	}
	for _, probe := range []Adapter{Cursor{}, Grok{}, OpenCode{}} {
		caps, err := probe.Probe(ctx, ProbeRequest{})
		if err != nil || caps.Probed || caps.SupportsHeadless || caps.Verified {
			t.Fatalf("%s probe claimed support %+v %v", probe.Name(), caps, err)
		}
	}
	events, err := DecodeLine("stdout", []byte(`{"type":"Finished","verdict":"pass","usage":{"input_tokens":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Origin != "agent_observation" || ev.Type == "Finished" || ev.Type == "VerifiedPass" {
			t.Fatalf("promoted event %+v", ev)
		}
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
