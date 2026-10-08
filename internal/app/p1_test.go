package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sllt/agentlab/internal/domain"
	"github.com/sllt/agentlab/internal/sandbox"
)

func TestRealAgentIsSandboxedAndSecretsAreRedacted(t *testing.T) {
	if st := sandbox.Probe(); !st.Supported {
		t.Skipf("landlock unavailable: %s", st.Reason)
	}
	ctx := context.Background()
	svc := newLab(t)
	t.Setenv("CURSOR_API_KEY", "sk-very-secret-value-123")
	db := filepath.Join(svc.DataDir, "lab.db")
	if _, err := os.Stat(db); err != nil {
		// newLab may name it differently; any file in the data dir will do.
		db = filepath.Join(svc.DataDir, "probe.txt")
		if err := os.WriteFile(db, []byte("control-plane"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cli := writeFakeCursor(t, `if cat '`+db+`' >/dev/null 2>&1; then echo readable > leak.txt; else echo denied > leak.txt; fi
echo "my key is $CURSOR_API_KEY"
echo '{"type":"result"}'
exit 0`)
	snap := ProfileSnapshot{Adapter: "cursor", Model: "fast", Executor: "native-trusted", Network: "unrestricted", DisplayName: "Cursor", Executable: cli, ApproveTools: true, CredentialEnv: []string{"CURSOR_API_KEY"}}
	trialID, err := seedProfileTrial(t, svc, snap, "env:CURSOR_API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Pump(ctx, 1); err != nil {
		t.Fatal(err)
	}
	attempts, err := svc.Store.ListAttempts(ctx, trialID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("%v %v", attempts, err)
	}
	a := attempts[0]
	if a.State != string(domain.ExecCompleted) {
		t.Fatalf("attempt %+v", a)
	}
	leak, err := os.ReadFile(filepath.Join(svc.DataDir, "attempts", a.ID, "work", "leak.txt"))
	if err != nil || strings.TrimSpace(string(leak)) != "denied" {
		t.Fatalf("control-plane file reachable from the agent: %q %v", leak, err)
	}
	var rt struct {
		Isolation string `json:"isolation"`
		Sandbox   struct {
			Mode string `json:"mode"`
		} `json:"sandbox"`
	}
	_ = json.Unmarshal([]byte(a.RuntimeJSON), &rt)
	if rt.Sandbox.Mode != "landlock" || rt.Isolation != "landlock-fs/native" {
		t.Fatalf("runtime %s", a.RuntimeJSON)
	}
	logBody, _ := os.ReadFile(filepath.Join(svc.DataDir, "attempts", a.ID, "control", "agent.log"))
	events, _ := os.ReadFile(filepath.Join(svc.DataDir, "attempts", a.ID, "events.ndjson"))
	for name, body := range map[string][]byte{"agent.log": logBody, "events": events} {
		if strings.Contains(string(body), "sk-very-secret-value-123") {
			t.Fatalf("%s leaked the credential: %s", name, body)
		}
	}
	if !strings.Contains(string(logBody), "[REDACTED]") {
		t.Fatalf("agent.log not redacted: %q", logBody)
	}
}

func TestSandboxCanBeDisabled(t *testing.T) {
	t.Setenv("AGENTLAB_SANDBOX", "off")
	svc := &Service{DataDir: t.TempDir()}
	policy, record := svc.sandboxFor("sh", t.TempDir(), t.TempDir(), t.TempDir(), false)
	if policy != nil {
		t.Fatal("policy despite opt-out")
	}
	box, _ := record["sandbox"].(map[string]any)
	if box["mode"] != "none" || box["reason"] != "disabled_by_operator" {
		t.Fatalf("%v", record)
	}
}
