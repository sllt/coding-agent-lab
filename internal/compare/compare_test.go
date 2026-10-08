package compare

import (
	"bytes"
	"encoding/json"
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

func TestAssistedRetryDoesNotEnterTheFirstPassCount(t *testing.T) {
	rows := []Input{
		{Mode: domain.ModeAgentProfile, Protocol: "single-pass-v1", TaskID: "t1", TaskDigest: "task", ProfileID: "p1", ProfileName: "甲", ProfileDig: "a", Executor: "native-trusted", Network: "unrestricted", Verdict: "fail", Terminal: true, Assisted: "pass"},
	}
	boards, err := Boards(rows, 1)
	if err != nil || len(boards) != 1 || len(boards[0].Cells) != 1 {
		t.Fatalf("%+v %v", boards, err)
	}
	cell := boards[0].Cells[0]
	if cell.Pass != 0 || cell.Fail != 1 || cell.AssistedPass != 1 {
		t.Fatalf("retry polluted the first attempt: %+v", cell)
	}
}

func TestMissingDurationStaysNull(t *testing.T) {
	base := Input{
		Mode: domain.ModeAgentProfile, Protocol: "single-pass-v1", TaskID: "t1", TaskDigest: "task",
		ProfileID: "p1", ProfileName: "甲", ProfileDig: "a", Executor: "native-trusted", Network: "unrestricted",
		Verdict: "pass", Terminal: true,
	}
	boards, err := Boards([]Input{base}, 1)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(boards[0].Cells[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"agent_millis":null`)) || !bytes.Contains(body, []byte(`"end_to_end_millis":null`)) {
		t.Fatalf("%s", body)
	}
	zero := int64(0)
	measured := base
	measured.AgentMillis = &zero
	measured.EndToEndMillis = &zero
	boards, err = Boards([]Input{measured}, 1)
	if err != nil {
		t.Fatal(err)
	}
	body, err = json.Marshal(boards[0].Cells[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"agent_millis":0`)) || !bytes.Contains(body, []byte(`"end_to_end_millis":0`)) {
		t.Fatalf("%s", body)
	}
	three, four := int64(3), int64(4)
	first, second := base, base
	first.AgentMillis, first.EndToEndMillis = &three, &three
	second.AgentMillis, second.EndToEndMillis = &four, &four
	boards, err = Boards([]Input{first, second}, 1)
	if err != nil || boards[0].Cells[0].AgentMillis == nil || *boards[0].Cells[0].AgentMillis != 7 {
		t.Fatalf("%+v %v", boards, err)
	}
	boards, err = Boards([]Input{first, base}, 1)
	if err != nil || boards[0].Cells[0].AgentMillis != nil || boards[0].Cells[0].EndToEndMillis != nil {
		t.Fatalf("missing duration became a number: %+v %v", boards[0].Cells[0], err)
	}
}

func TestRecordedBudgetMakesBoardsComparable(t *testing.T) {
	base := Input{Mode: domain.ModeAgentProfile, Protocol: "single-pass-v1", TaskID: "t1", TaskDigest: "task", Executor: "native-trusted", Network: "unrestricted", Verdict: "pass", Terminal: true}
	a, b := base, base
	a.ProfileID, a.ProfileDig, a.BudgetDigest = "p1", "a", "budget-1"
	b.ProfileID, b.ProfileDig, b.BudgetDigest, b.Verdict = "p2", "b", "budget-1", "fail"
	boards, err := Boards([]Input{a, b}, 1)
	if err != nil || len(boards) != 1 || !boards[0].Comparable || len(boards[0].Cells) != 2 {
		t.Fatalf("recorded budget %+v %v", boards, err)
	}
	b.BudgetDigest = "budget-2"
	boards, err = Boards([]Input{a, b}, 1)
	if err != nil || len(boards) != 2 {
		t.Fatalf("different budgets shared a board %+v %v", boards, err)
	}
	a.BudgetDigest, b.BudgetDigest = "", ""
	boards, err = Boards([]Input{a, b}, 1)
	if err != nil || boards[0].Comparable {
		t.Fatalf("missing budget was comparable %+v", boards)
	}
}
