// Package compare builds count boards. It never emits a 0–100 score.
package compare

import (
	"github.com/sllt/agentlab/internal/domain"
)

type Input struct {
	Mode          string
	Protocol      string
	TaskID        string
	TaskDigest    string
	ProfileID     string
	ProfileName   string
	ProfileDig    string
	Executor      string
	Network       string
	Verdict       string
	Terminal      bool
	ModelMismatch bool
	// Assisted is the latest retry verdict when a later attempt exists.
	// It is not counted in Pass/Fail, which stay on the first physical attempt.
	Assisted  string
	Repair    string
	TrialID   string
	AttemptID string
	// AgentMillis and EndToEndMillis are nil when that phase was not measured.
	// A pointer to 0 is a real measurement. Missing must not become 0.
	AgentMillis    *int64
	EndToEndMillis *int64
	Interventions  int
}

type Cell struct {
	ProfileVersionID string         `json:"profile_version_id"`
	DisplayName      string         `json:"display_name"`
	Pass             int            `json:"pass"`
	Fail             int            `json:"fail"`
	Inconclusive     int            `json:"inconclusive"`
	Unverified       int            `json:"unverified"`
	Incomplete       int            `json:"incomplete"`
	AssistedPass     int            `json:"assisted_pass"`
	AssistedFail     int            `json:"assisted_fail"`
	RepairPass       int            `json:"repair_pass"`
	AgentMillis      *int64         `json:"agent_millis"`
	EndToEndMillis   *int64         `json:"end_to_end_millis"`
	agentGap         bool           `json:"-"`
	e2eGap           bool           `json:"-"`
	Interventions    int            `json:"interventions"`
	Evidence         []EvidenceLink `json:"evidence"`
}

type EvidenceLink struct {
	TrialID   string `json:"trial_id"`
	AttemptID string `json:"attempt_id"`
	Kind      string `json:"kind"`
}

type Board struct {
	TaskVersionID string   `json:"task_version_id"`
	ComparisonKey string   `json:"comparison_key"`
	Comparable    bool     `json:"comparable"`
	Reasons       []string `json:"reasons"`
	Cells         []Cell   `json:"cells"`
	Exploratory   bool     `json:"exploratory"`
	Method        string   `json:"method"`
	Note          string   `json:"note"`
	Paired        int      `json:"paired"`
	TaskCount     int      `json:"task_count"`
	Executor      string   `json:"executor"`
	Network       string   `json:"network"`
}

func Boards(rows []Input, taskCount int) ([]Board, error) {
	exploratory := taskCount < 20
	note := "数字是原始计数。分母是该任务上的 Trial，不是分数。"
	if exploratory {
		note = "探索性结果：可比任务少于 20。这里只有计数，没有 0 到 100 的分数，也不能据此给模型排名。"
	}
	type group struct {
		board Board
		seen  map[string]int
	}
	order := []string{}
	groups := map[string]*group{}
	for _, row := range rows {
		in := domain.ComparisonInput{
			Mode: row.Mode, TaskDigest: row.TaskDigest, EnvironmentDigest: row.Executor + ":" + row.Network,
			ProtocolDigest: row.Protocol, VerifierDigest: row.TaskDigest, BudgetDigest: "unspecified",
			ProfileDigest: row.ProfileDig, Executor: row.Executor, Network: row.Network, ModelMismatch: row.ModelMismatch,
		}
		cmp, err := domain.ComparisonOf(in)
		if err != nil {
			return nil, err
		}
		key := row.TaskID + ":" + cmp.Key
		g, ok := groups[key]
		if !ok {
			g = &group{board: Board{
				TaskVersionID: row.TaskID, ComparisonKey: cmp.Key, Comparable: cmp.Comparable, Reasons: cmp.Reasons,
				Exploratory: exploratory, Method: "counts-v1", Note: note, TaskCount: taskCount,
				Executor: row.Executor, Network: row.Network,
			}, seen: map[string]int{}}
			groups[key] = g
			order = append(order, key)
		}
		if !cmp.Comparable {
			g.board.Comparable = false
			g.board.Reasons = appendUnique(g.board.Reasons, cmp.Reasons...)
		}
		idx, ok := g.seen[row.ProfileID]
		if !ok {
			idx = len(g.board.Cells)
			g.seen[row.ProfileID] = idx
			g.board.Cells = append(g.board.Cells, Cell{ProfileVersionID: row.ProfileID, DisplayName: row.ProfileName})
		}
		cell := &g.board.Cells[idx]
		addMillis(&cell.AgentMillis, &cell.agentGap, row.AgentMillis)
		addMillis(&cell.EndToEndMillis, &cell.e2eGap, row.EndToEndMillis)
		cell.Interventions += row.Interventions
		if !row.Terminal {
			cell.Incomplete++
			cell.Evidence = append(cell.Evidence, EvidenceLink{TrialID: row.TrialID, AttemptID: row.AttemptID, Kind: "incomplete"})
			continue
		}
		kind := "unverified"
		switch domain.Verdict(row.Verdict) {
		case domain.VerdictPass:
			cell.Pass++
			kind = "pass"
		case domain.VerdictFail:
			cell.Fail++
			kind = "fail"
		case domain.VerdictInconclusive:
			cell.Inconclusive++
			kind = "inconclusive"
		default:
			cell.Unverified++
		}
		cell.Evidence = append(cell.Evidence, EvidenceLink{TrialID: row.TrialID, AttemptID: row.AttemptID, Kind: kind})
		switch domain.Verdict(row.Assisted) {
		case domain.VerdictPass:
			cell.AssistedPass++
		case domain.VerdictFail:
			cell.AssistedFail++
		}
		if domain.Verdict(row.Repair) == domain.VerdictPass {
			cell.RepairPass++
		}
	}
	var out []Board
	for _, key := range order {
		g := groups[key]
		if len(g.board.Cells) >= 2 {
			complete := true
			for _, cell := range g.board.Cells {
				if cell.Incomplete > 0 || cell.Pass+cell.Fail+cell.Inconclusive+cell.Unverified == 0 {
					complete = false
				}
			}
			if complete {
				g.board.Paired = 1
			}
		}
		if g.board.Reasons == nil {
			g.board.Reasons = []string{}
		}
		for i := range g.board.Cells {
			if g.board.Cells[i].Evidence == nil {
				g.board.Cells[i].Evidence = []EvidenceLink{}
			}
		}
		out = append(out, g.board)
	}
	return out, nil
}

func addMillis(sum **int64, gap *bool, next *int64) {
	if next == nil {
		*gap = true
		*sum = nil
		return
	}
	if *gap {
		return
	}
	v := *next
	if *sum != nil {
		v += **sum
	}
	*sum = &v
}

func appendUnique(base []string, extra ...string) []string {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item] = struct{}{}
	}
	for _, item := range extra {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		base = append(base, item)
	}
	return base
}
