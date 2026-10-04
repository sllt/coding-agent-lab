package domain

import "fmt"

// Usage is a nullable observation. Nil means unknown, never zero.
type Usage struct {
	AttemptID         string  `json:"attempt_id"`
	Source            string  `json:"source"`
	SourceEventID     string  `json:"source_event_id"`
	BillingPath       string  `json:"billing_path"`
	InputTokens       *int64  `json:"input_tokens"`
	OutputTokens      *int64  `json:"output_tokens"`
	CachedInputTokens *int64  `json:"cached_input_tokens"`
	CostMicroUSD      *int64  `json:"cost_microusd"`
	Currency          *string `json:"currency"`
	Confidence        string  `json:"confidence"`
	IsCumulative      bool    `json:"is_cumulative"`
}

// FoldUsage deduplicates by source event id. Cumulative snapshots keep the
// last value in that scope. Incremental events add. Missing stays nil.
func FoldUsage(records []Usage) (Usage, error) {
	var out Usage
	seen := map[string]struct{}{}
	var cumulative []Usage
	for _, r := range records {
		if r.SourceEventID != "" {
			k := r.Source + "\x00" + r.SourceEventID
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
		}
		if r.IsCumulative {
			cumulative = append(cumulative, r)
			continue
		}
		out.InputTokens = addOpt(out.InputTokens, r.InputTokens)
		out.OutputTokens = addOpt(out.OutputTokens, r.OutputTokens)
		out.CachedInputTokens = addOpt(out.CachedInputTokens, r.CachedInputTokens)
		out.CostMicroUSD = addOpt(out.CostMicroUSD, r.CostMicroUSD)
		if r.Currency != nil {
			out.Currency = r.Currency
		}
	}
	if len(cumulative) > 0 && (out.InputTokens != nil || out.OutputTokens != nil || out.CostMicroUSD != nil) {
		return Usage{}, fmt.Errorf("cannot mix cumulative snapshots with incremental events")
	}
	if len(cumulative) > 0 {
		last := cumulative[len(cumulative)-1]
		out.InputTokens = last.InputTokens
		out.OutputTokens = last.OutputTokens
		out.CachedInputTokens = last.CachedInputTokens
		out.CostMicroUSD = last.CostMicroUSD
		out.Currency = last.Currency
		out.IsCumulative = true
		out.Confidence = last.Confidence
		out.BillingPath = last.BillingPath
	}
	if out.Confidence == "" {
		out.Confidence = "unknown"
	}
	if out.BillingPath == "" {
		out.BillingPath = "unknown"
	}
	return out, nil
}

func addOpt(a, b *int64) *int64 {
	if b == nil {
		return a
	}
	if a == nil {
		v := *b
		return &v
	}
	v := *a + *b
	return &v
}

func Int64Ptr(v int64) *int64 { return &v }
