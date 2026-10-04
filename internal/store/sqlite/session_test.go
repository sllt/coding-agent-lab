package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sllt/agentlab/internal/domain"
)

func TestSessionRejectsSentinelAndPastExpiry(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.CreateUser(ctx, "malong", "hash"); err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := s.DB().QueryRow(`SELECT id FROM users WHERE username=?`, "malong").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, userID, "token-hash", "csrf"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Session(ctx, "token-hash"); err != nil {
		t.Fatal(err)
	}
	var expires string
	if err := s.DB().QueryRow(`SELECT expires_at FROM sessions WHERE token_hash=?`, "token-hash").Scan(&expires); err != nil {
		t.Fatal(err)
	}
	when, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || when.After(time.Now().Add(31*time.Minute)) || when.Year() >= 2099 {
		t.Fatalf("idle expiry %s", expires)
	}
	if _, err := s.DB().Exec(`UPDATE sessions SET expires_at=? WHERE token_hash=?`, "2099-01-01T00:00:00.000000000Z", "token-hash"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Session(ctx, "token-hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("2099 session err %v", err)
	}
}

func TestWebhookDigestConflictAndExperimentRepair(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dup, err := s.AcceptWebhook(ctx, "k", "aaa")
	if err != nil || dup {
		t.Fatalf("first %v %v", dup, err)
	}
	dup, err = s.AcceptWebhook(ctx, "k", "aaa")
	if err != nil || !dup {
		t.Fatalf("same body %v %v", dup, err)
	}
	if _, err := s.AcceptWebhook(ctx, "k", "bbb"); !errors.Is(err, ErrConflict) {
		t.Fatalf("different body %v", err)
	}
	exp, err := s.SubmitExperiment(ctx, "actor", "key", "digest", domain.ModeAgentProfile, "single-pass-v1", `{}`, []NewTrial{{TaskVersionID: "tv", ProfileVersionID: "pv", RepeatIndex: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`UPDATE trials SET execution_state=? WHERE experiment_id=?`, string(domain.ExecCompleted), exp.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RepairExperimentStates(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetExperiment(ctx, exp.ID)
	if err != nil || got.State != string(domain.ExecCompleted) {
		t.Fatalf("state %+v %v", got, err)
	}
}