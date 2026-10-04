package domain

import "fmt"

const (
	ModeAgentProfile    = "agent_profile"
	ModeControlledModel = "controlled_model"
	ModeWorkflow        = "workflow"
)

type ExecutionIdentity struct {
	TaskDigest        string `json:"task_digest"`
	ProfileDigest     string `json:"profile_digest"`
	EnvironmentDigest string `json:"environment_digest"`
	ProtocolDigest    string `json:"protocol_digest"`
	VerifierDigest    string `json:"verifier_digest"`
}

func ExecutionFingerprint(id ExecutionIdentity) (string, error) {
	if id.TaskDigest == "" || id.ProfileDigest == "" || id.EnvironmentDigest == "" || id.ProtocolDigest == "" {
		return "", fmt.Errorf("execution identity is incomplete")
	}
	return Digest(map[string]any{
		"schema":             "agentlab.execution_fingerprint/v1",
		"task_digest":        id.TaskDigest,
		"profile_digest":     id.ProfileDigest,
		"environment_digest": id.EnvironmentDigest,
		"protocol_digest":    id.ProtocolDigest,
		"verifier_digest":    id.VerifierDigest,
	})
}

type ComparisonInput struct {
	Mode              string
	TaskDigest        string
	EnvironmentDigest string
	ProtocolDigest    string
	VerifierDigest    string
	BudgetDigest      string
	ProfileDigest     string
	ModelID           string
	Executor          string
	Network           string
	ModelMismatch     bool
}

type Comparison struct {
	Key        string
	Comparable bool
	Reasons    []string
}

func ComparisonOf(in ComparisonInput) (Comparison, error) {
	body := map[string]any{
		"schema":             "agentlab.comparison_key/v1",
		"mode":               in.Mode,
		"task_digest":        in.TaskDigest,
		"environment_digest": in.EnvironmentDigest,
		"verifier_digest":    in.VerifierDigest,
		"budget_digest":      in.BudgetDigest,
		"executor":           in.Executor,
		"network":            in.Network,
	}
	switch in.Mode {
	case ModeAgentProfile:
		body["protocol_digest"] = in.ProtocolDigest
	case ModeControlledModel:
		body["protocol_digest"] = in.ProtocolDigest
		body["profile_digest"] = in.ProfileDigest
	case ModeWorkflow:
		body["profile_digest"] = in.ProfileDigest
	default:
		return Comparison{}, fmt.Errorf("unknown experiment mode %s", in.Mode)
	}
	key, err := Digest(body)
	if err != nil {
		return Comparison{}, err
	}
	out := Comparison{Key: key, Comparable: true}
	if in.ModelMismatch {
		out.Comparable = false
		out.Reasons = append(out.Reasons, "model_resolution_mismatch")
	}
	if in.Network == "restricted" {
		out.Comparable = false
		out.Reasons = append(out.Reasons, "network_restriction_not_enforced")
	}
	return out, nil
}

// SameBoard reports whether two runs may share a default comparison.
func SameBoard(a, b ComparisonInput) (bool, []string, error) {
	left, err := ComparisonOf(a)
	if err != nil {
		return false, nil, err
	}
	right, err := ComparisonOf(b)
	if err != nil {
		return false, nil, err
	}
	reasons := append([]string{}, left.Reasons...)
	reasons = append(reasons, right.Reasons...)
	if left.Key != right.Key {
		reasons = append(reasons, "comparison_key_mismatch")
	}
	if a.Executor != b.Executor {
		reasons = append(reasons, "executor_mismatch")
	}
	return left.Key == right.Key && len(reasons) == 0, reasons, nil
}
