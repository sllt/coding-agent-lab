package domain

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	TaskSchema       = "agentlab.task/v1alpha1"
	ProfileSchema    = "agentlab.profile/v1alpha1"
	ExperimentSchema = "agentlab.experiment/v1alpha1"
)

type TaskDraft struct {
	SchemaVersion         string            `yaml:"schema_version" json:"schema_version"`
	Kind                  string            `yaml:"kind" json:"kind"`
	Name                  string            `yaml:"name" json:"name"`
	ProjectRef            string            `yaml:"project_ref" json:"project_ref"`
	Source                TaskSource        `yaml:"source" json:"source"`
	EnvironmentRef        string            `yaml:"environment_ref" json:"environment_ref"`
	Prompt                string            `yaml:"prompt" json:"prompt"`
	ChangePolicy          ChangePolicy      `yaml:"change_policy" json:"change_policy"`
	VerifierRef           string            `yaml:"verifier_ref" json:"verifier_ref"`
	BaselineExpectations  map[string]string `yaml:"baseline_expectations" json:"baseline_expectations"`
	CandidateExpectations map[string]string `yaml:"candidate_expectations" json:"candidate_expectations"`
	Limits                Limits            `yaml:"limits" json:"limits"`
	Protocol              string            `yaml:"protocol" json:"protocol"`
}

type TaskSource struct {
	BaseRef       string `yaml:"base_ref" json:"base_ref"`
	HistoryPolicy string `yaml:"history_policy" json:"history_policy"`
	Directory     string `yaml:"directory" json:"directory"`
	VerifierRoot  string `yaml:"verifier_root" json:"verifier_root"`
}

type ChangePolicy struct {
	AllowedPaths       []string `yaml:"allowed_paths" json:"allowed_paths"`
	ForbidSpecialFiles bool     `yaml:"forbid_special_files" json:"forbid_special_files"`
	ForbidSymlinks     bool     `yaml:"forbid_symlinks" json:"forbid_symlinks"`
}

type Limits struct {
	AgentWallSeconds   int `yaml:"agent_wall_seconds" json:"agent_wall_seconds"`
	PrepareWallSeconds int `yaml:"prepare_wall_seconds" json:"prepare_wall_seconds"`
	VerifyWallSeconds  int `yaml:"verify_wall_seconds" json:"verify_wall_seconds"`
	LogBytes           int `yaml:"log_bytes" json:"log_bytes"`
	ArtifactBytes      int `yaml:"artifact_bytes" json:"artifact_bytes"`
	MaxFiles           int `yaml:"max_files" json:"max_files"`
}

type ProfileDraft struct {
	SchemaVersion string         `yaml:"schema_version" json:"schema_version"`
	Kind          string         `yaml:"kind" json:"kind"`
	Name          string         `yaml:"name" json:"name"`
	Adapter       string         `yaml:"adapter" json:"adapter"`
	CLI           CLIDraft       `yaml:"cli" json:"cli"`
	Model         ModelDraft     `yaml:"model" json:"model"`
	Auth          AuthDraft      `yaml:"auth" json:"auth"`
	Billing       BillingDraft   `yaml:"billing" json:"billing"`
	Execution     ExecutionDraft `yaml:"execution" json:"execution"`
	SessionPolicy string         `yaml:"session_policy" json:"session_policy"`
	RulesPolicy   string         `yaml:"rules_policy" json:"rules_policy"`
}

type CLIDraft struct {
	Executable      string `yaml:"executable" json:"executable"`
	ExpectedVersion string `yaml:"expected_version" json:"expected_version"`
	AutoUpdate      bool   `yaml:"auto_update" json:"auto_update"`
}

type ModelDraft struct {
	DisplayName string         `yaml:"display_name" json:"display_name"`
	RequestedID string         `yaml:"requested_id" json:"requested_id"`
	Parameters  map[string]any `yaml:"parameters" json:"parameters"`
}

type AuthDraft struct {
	Kind          string `yaml:"kind" json:"kind"`
	AccountRef    string `yaml:"account_ref" json:"account_ref"`
	CredentialRef string `yaml:"credential_ref" json:"credential_ref"`
}

type BillingDraft struct {
	Path                  string  `yaml:"path" json:"path"`
	EntitlementVerifiedAt *string `yaml:"entitlement_verified_at" json:"entitlement_verified_at"`
}

type ExecutionDraft struct {
	Executor            string `yaml:"executor" json:"executor"`
	EnvironmentRef      string `yaml:"environment_ref" json:"environment_ref"`
	PermissionPolicyRef string `yaml:"permission_policy_ref" json:"permission_policy_ref"`
	NetworkPolicyRef    string `yaml:"network_policy_ref" json:"network_policy_ref"`
}

type ExperimentDraft struct {
	SchemaVersion      string         `yaml:"schema_version" json:"schema_version"`
	Kind               string         `yaml:"kind" json:"kind"`
	Name               string         `yaml:"name" json:"name"`
	Mode               string         `yaml:"mode" json:"mode"`
	TaskVersionIDs     []string       `yaml:"task_version_ids" json:"task_version_ids"`
	ProfileVersionIDs  []string       `yaml:"profile_version_ids" json:"profile_version_ids"`
	Repetitions        int            `yaml:"repetitions" json:"repetitions"`
	Protocol           string         `yaml:"protocol" json:"protocol"`
	Scheduler          SchedulerDraft `yaml:"scheduler" json:"scheduler"`
	Budget             BudgetDraft    `yaml:"budget" json:"budget"`
	InterventionPolicy string         `yaml:"intervention_policy" json:"intervention_policy"`
	ExportPolicy       ExportDraft    `yaml:"export_policy" json:"export_policy"`
}

type SchedulerDraft struct {
	GlobalConcurrency     int `yaml:"global_concurrency" json:"global_concurrency"`
	PerAccountConcurrency int `yaml:"per_account_concurrency" json:"per_account_concurrency"`
	MaxAutoInfraRetries   int `yaml:"max_auto_infra_retries" json:"max_auto_infra_retries"`
}

type BudgetDraft struct {
	MaxAgentStarts            int    `yaml:"max_agent_starts" json:"max_agent_starts"`
	ObservedCostLimitMicroUSD *int64 `yaml:"observed_cost_limit_microusd" json:"observed_cost_limit_microusd"`
	CostEnforcement           string `yaml:"cost_enforcement" json:"cost_enforcement"`
}

type ExportDraft struct {
	IncludeCredentials       bool `yaml:"include_credentials" json:"include_credentials"`
	IncludeReferenceSolution bool `yaml:"include_reference_solution" json:"include_reference_solution"`
}

func DecodeTask(data []byte) (TaskDraft, error) {
	var d TaskDraft
	if err := decodeStrict(data, &d); err != nil {
		return TaskDraft{}, err
	}
	if d.SchemaVersion != TaskSchema || d.Kind != "TaskDraft" {
		return TaskDraft{}, fmt.Errorf("not a task draft")
	}
	return d, nil
}

func DecodeProfile(data []byte) (ProfileDraft, error) {
	var d ProfileDraft
	if err := decodeStrict(data, &d); err != nil {
		return ProfileDraft{}, err
	}
	if d.SchemaVersion != ProfileSchema || d.Kind != "AgentProfileDraft" {
		return ProfileDraft{}, fmt.Errorf("not a profile draft")
	}
	return d, nil
}

func DecodeExperiment(data []byte) (ExperimentDraft, error) {
	var d ExperimentDraft
	if err := decodeStrict(data, &d); err != nil {
		return ExperimentDraft{}, err
	}
	if d.SchemaVersion != ExperimentSchema || d.Kind != "ExperimentDraft" {
		return ExperimentDraft{}, fmt.Errorf("not an experiment draft")
	}
	return d, nil
}

func decodeStrict(data []byte, dest any) error {
	if len(data) > 1<<20 {
		return fmt.Errorf("draft exceeds 1 MiB")
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	if err := rejectDuplicateKeys(&node, 0); err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(dest); err != nil {
		return err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("draft must contain one document")
	}
	return nil
}

func rejectDuplicateKeys(node *yaml.Node, depth int) error {
	if depth > 32 {
		return fmt.Errorf("draft is too deep")
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, c := range node.Content {
			if err := rejectDuplicateKeys(c, depth+1); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := map[string]struct{}{}
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			if _, ok := seen[k]; ok {
				return fmt.Errorf("duplicate key %s", k)
			}
			seen[k] = struct{}{}
			if err := rejectDuplicateKeys(node.Content[i+1], depth+1); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, c := range node.Content {
			if err := rejectDuplicateKeys(c, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// PublishBlockers lists reasons a draft cannot become an execution snapshot.
func PublishBlockers(v any) []string {
	raw, err := Canonical(v)
	if err != nil {
		return []string{err.Error()}
	}
	var blockers []string
	if bytes.Contains(raw, []byte("REPLACE_")) {
		blockers = append(blockers, "placeholder_field")
	}
	switch d := v.(type) {
	case TaskDraft:
		if d.Limits.AgentWallSeconds <= 0 || d.Limits.LogBytes <= 0 || d.Limits.MaxFiles <= 0 || d.Limits.ArtifactBytes <= 0 {
			blockers = append(blockers, "non_positive_limit")
		}
		if strings.TrimSpace(d.Prompt) == "" || d.VerifierRef == "" {
			blockers = append(blockers, "incomplete_task")
		}
	case ProfileDraft:
		if d.CLI.AutoUpdate {
			blockers = append(blockers, "auto_update_enabled")
		}
		if d.Auth.Kind != "official_login" && d.Auth.Kind != "api_key" && d.Auth.Kind != "manual_import" {
			blockers = append(blockers, "unknown_auth_kind")
		}
	case ExperimentDraft:
		if d.Repetitions <= 0 || len(d.TaskVersionIDs) == 0 || len(d.ProfileVersionIDs) == 0 {
			blockers = append(blockers, "empty_plan")
		}
		if d.Scheduler.GlobalConcurrency <= 0 || d.Scheduler.PerAccountConcurrency <= 0 {
			blockers = append(blockers, "non_positive_concurrency")
		}
		if d.Scheduler.MaxAutoInfraRetries != 0 {
			blockers = append(blockers, "auto_retry_not_allowed")
		}
		if d.Budget.MaxAgentStarts <= 0 {
			blockers = append(blockers, "non_positive_budget")
		}
	}
	return blockers
}

func TrialCount(tasks, profiles, repetitions int) int {
	if tasks < 0 || profiles < 0 || repetitions < 0 {
		return 0
	}
	return tasks * profiles * repetitions
}
