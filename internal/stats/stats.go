// Package stats aggregates first-attempt outcomes at task level.
// It does not emit a 0–100 score.
package stats

import (
	"math"
	"sort"
)

const MethodVersion = "task-cluster-bootstrap/v1"

type Row struct {
	TaskID     string
	ProfileID  string
	Pass       int
	Fail       int
	Unresolved int
}

type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

type Report struct {
	Method       string              `json:"method"`
	Seed         int64               `json:"seed"`
	Iterations   int                 `json:"iterations"`
	TaskCount    int                 `json:"task_count"`
	Exploratory  bool                `json:"exploratory"`
	MacroAverage map[string]float64  `json:"macro_average"`
	Intervals    map[string]Interval `json:"intervals"`
	Paired       map[string]int      `json:"paired"`
	Note         string              `json:"note"`
}

// Aggregate computes an equal-weight task mean and a paired cluster bootstrap.
// The resample unit is the task. Repetitions inside one task do not become
// extra tasks.
func Aggregate(rows []Row, seed int64, iterations int) Report {
	if iterations <= 0 {
		iterations = 200
	}
	if iterations > 2000 {
		iterations = 2000
	}
	if seed == 0 {
		seed = 20261004
	}
	tasks := map[string]map[string]Row{}
	order := []string{}
	profiles := map[string]struct{}{}
	for _, row := range rows {
		if _, ok := tasks[row.TaskID]; !ok {
			tasks[row.TaskID] = map[string]Row{}
			order = append(order, row.TaskID)
		}
		tasks[row.TaskID][row.ProfileID] = row
		profiles[row.ProfileID] = struct{}{}
	}
	sort.Strings(order)
	profileIDs := make([]string, 0, len(profiles))
	for id := range profiles {
		profileIDs = append(profileIDs, id)
	}
	sort.Strings(profileIDs)
	macro := map[string]float64{}
	for _, id := range profileIDs {
		value := macroOf(order, tasks, id)
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			macro[id] = value
		}
	}
	report := Report{
		Method: MethodVersion, Seed: seed, Iterations: iterations,
		TaskCount: len(order), Exploratory: len(order) < 20,
		MacroAverage: macro, Intervals: map[string]Interval{}, Paired: map[string]int{},
		Note: "任务等权平均。重采样单位是任务，不是单次 Trial。少于 20 个任务时只作探索性计数，不宣布谁更强。",
	}
	if len(order) < 2 || len(profileIDs) == 0 {
		report.Note = "任务不足两个，只保留原始计数，不给区间。"
		report.Paired = pairCounts(order, tasks, profileIDs)
		return report
	}
	rng := seed
	next := func() float64 {
		rng = rng*6364136223846793005 + 1
		return float64(uint64(rng)>>11) / (1 << 53)
	}
	samples := map[string][]float64{}
	for _, id := range profileIDs {
		samples[id] = make([]float64, 0, iterations)
	}
	for i := 0; i < iterations; i++ {
		picked := make([]string, len(order))
		for j := range picked {
			picked[j] = order[int(next()*float64(len(order)))%len(order)]
		}
		for _, id := range profileIDs {
			value := macroOf(picked, tasks, id)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}
			samples[id] = append(samples[id], value)
		}
	}
	for _, id := range profileIDs {
		if len(samples[id]) == 0 {
			continue
		}
		sort.Float64s(samples[id])
		report.Intervals[id] = Interval{Low: quantile(samples[id], 0.025), High: quantile(samples[id], 0.975)}
	}
	report.Paired = pairCounts(order, tasks, profileIDs)
	return report
}

func macroOf(taskIDs []string, tasks map[string]map[string]Row, profile string) float64 {
	if len(taskIDs) == 0 {
		return math.NaN()
	}
	sum := 0.0
	n := 0
	for _, taskID := range taskIDs {
		row, ok := tasks[taskID][profile]
		if !ok {
			continue
		}
		den := row.Pass + row.Fail
		if den == 0 {
			continue
		}
		sum += float64(row.Pass) / float64(den)
		n++
	}
	if n == 0 {
		return math.NaN()
	}
	return sum / float64(n)
}

func pairCounts(taskIDs []string, tasks map[string]map[string]Row, profiles []string) map[string]int {
	out := map[string]int{"both_pass": 0, "only_a": 0, "only_b": 0, "both_fail": 0, "unresolved": 0}
	if len(profiles) < 2 {
		return out
	}
	a, b := profiles[0], profiles[1]
	for _, taskID := range taskIDs {
		left, lok := tasks[taskID][a]
		right, rok := tasks[taskID][b]
		if !lok || !rok || left.Unresolved > 0 || right.Unresolved > 0 || (left.Pass+left.Fail) == 0 || (right.Pass+right.Fail) == 0 {
			out["unresolved"]++
			continue
		}
		lp := left.Pass > 0 && left.Fail == 0
		rp := right.Pass > 0 && right.Fail == 0
		lf := left.Fail > 0 && left.Pass == 0
		rf := right.Fail > 0 && right.Pass == 0
		switch {
		case lp && rp:
			out["both_pass"]++
		case lp && rf:
			out["only_a"]++
		case lf && rp:
			out["only_b"]++
		case lf && rf:
			out["both_fail"]++
		default:
			out["unresolved"]++
		}
	}
	return out
}

func quantile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(math.Floor(p * float64(len(sorted)-1)))
	return sorted[idx]
}
