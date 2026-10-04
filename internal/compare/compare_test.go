package compare

import (
	"testing"

	"github.com/sllt/agentlab/internal/domain"
)

func TestBoardsDoNotRankOrScore(t *testing.T) {
	rows := []Input{
		{Mode: domain.ModeAgentProfile, Protocol: "single-pass-v1", TaskID: "t1", TaskDigest: "task", ProfileID: "p1", ProfileName: "甲", ProfileDig: "a", Executor: "native-trusted", Network: "unrestricted", Verdict: "pass", Terminal: true},
		{Mode: domain.ModeAgentProfile, Protocol: "single-pass-v1", TaskID: "t1", TaskDigest: "task", ProfileID: "p2", ProfileName: "乙", ProfileDig: "b", Executor: "native-trusted", Network: "unrestricted", Verdict: "fail", Terminal: true},
		{Mode: domain.ModeAgentProfile, Protocol: "single-pass-v1", TaskID: "t1", TaskDigest: "task", ProfileID: "p3", ProfileName: "丙", ProfileDig: "c", Executor: "docker", Network: "unrestricted", Verdict: "pass", Terminal: true},
	}
	boards, err := Boards(rows, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) != 2 {
		t.Fatalf("different executors must not share a board: %+v", boards)
	}
	for _, board := range boards {
		if board.Method != "counts-v1" || !board.Exploratory {
			t.Fatalf("%+v", board)
		}
		if board.Note == "" {
			t.Fatal("missing note")
		}
	}
	restricted := rows[0]
	restricted.ProfileID = "p4"
	restricted.Network = "restricted"
	boards, err = Boards(append(rows[:2], restricted), 1)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, board := range boards {
		if !board.Comparable {
			saw = true
		}
	}
	if !saw {
		t.Fatal("restricted network was treated as comparable")
	}
}
