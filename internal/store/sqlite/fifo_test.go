package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestQueueIsFIFOAndRetryGoesToTheBack(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var want []string
	for e := 0; e < 3; e++ {
		var trials []NewTrial
		for i := 1; i <= 6; i++ {
			trials = append(trials, NewTrial{TaskVersionID: fmt.Sprintf("tv%d", e), ProfileVersionID: "pv", RepeatIndex: i})
		}
		exp, err := store.SubmitExperiment(ctx, "usr", fmt.Sprintf("k%d", e), fmt.Sprintf("d%d", e), "agent_profile", "single-pass-v1", "{}", trials)
		if err != nil {
			t.Fatal(err)
		}
		listed, err := store.ListTrials(ctx, exp.ID)
		if err != nil {
			t.Fatal(err)
		}
		for i, tr := range listed {
			if tr.RepeatIndex != i+1 {
				t.Fatalf("plan order broken: %+v", listed)
			}
			want = append(want, tr.ID)
		}
	}
	queued, err := store.ListQueued(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range want {
		if queued[i].ID != want[i] {
			t.Fatalf("queue position %d: got %s want %s", i, queued[i].ID, want[i])
		}
	}
	recent, err := store.RecentTrials(ctx, 3)
	if err != nil || len(recent) != 3 || recent[0].ID != want[len(want)-1] {
		t.Fatalf("recent is not newest first: %+v %v", recent, err)
	}
	// Cancel the head and retry it: it must re-enter at the back.
	if _, err := store.RequestCancel(ctx, want[0]); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestRetry(ctx, want[0], "人工重试"); err != nil {
		t.Fatal(err)
	}
	queued, err = store.ListQueued(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if queued[0].ID != want[1] || queued[len(queued)-1].ID != want[0] || queued[len(queued)-1].QueuedAt == "" {
		t.Fatalf("retry did not go to the back: head=%s tail=%s", queued[0].ID, queued[len(queued)-1].ID)
	}
	counts, err := store.TrialStateCounts(ctx)
	if err != nil || counts["queued"] != len(want) {
		t.Fatalf("counts %v %v", counts, err)
	}
}
