// API shapes (snake_case, as served by the control plane).

export type ExecState = 'queued' | 'preparing' | 'running' | 'collecting' | 'verifying' | 'completed' | 'cancelling' | 'cancelled' | 'aborted'
export type Verdict = 'pass' | 'fail' | 'inconclusive' | 'unverified'
export type Readiness = 'unavailable' | 'cli_detected' | 'needs_credentials' | 'ready_unverified' | 'verified'

export type Project = { id: string; name: string; source_spec: unknown; created_at: string }

export type TaskDraft = {
  name?: string
  prompt?: string
  source_dir?: string
  verifier_root?: string
  base_commit?: string
  limits?: { agent_wall_seconds?: number; prepare_wall_seconds?: number; verify_wall_seconds?: number } | null
}
export type Task = { id: string; project_id: string; name: string; draft: TaskDraft | null; row_version: number; created_at: string; updated_at: string }
export type TaskVersion = { id: string; task_id: string; version: number; digest: string; created_at: string; precheck?: unknown; snapshot?: unknown }

export type Account = { id: string; auth_kind: string; credential_ref: string; concurrency: number; blocked_reason: string; blocked_until: string; created_at: string }

export type ProfileDraft = {
  adapter?: string
  model?: string
  fake_mode?: string
  executor?: string
  network?: string
  display_name?: string
  executable?: string
  approve_tools?: boolean
  billing_path?: string
  credential_env?: string[]
  inherit_home?: boolean
  wall_seconds?: number
}
export type Profile = { id: string; name: string; account_id: string; draft: ProfileDraft | null; row_version: number }

export type TaskVersionInfo = { version_id: string; task_id: string; task_name: string; project_id: string; project_name: string; version: number; digest: string; created_at: string }
export type ProfileVersionInfo = { version_id: string; profile_id: string; name: string; version: number; adapter: string; model: string; display_name: string; readiness: Readiness | ''; digest: string; created_at: string }

export type DoctorReport = {
  adapter: string
  executable: string
  cli_version: string
  model_id: string
  model_call: string
  network: string
  executor: string
  static_passed: boolean
  verified: boolean
  readiness: Readiness
  runnable: boolean
  supports_headless: boolean
  credential_sources: string[] | null
  blockers: string[] | null
  hints: string[] | null
  smoke?: { passed: boolean; exit_code: number; json_lines: number; duration_ms: number; reason?: string; auth_suspected: boolean } | null
  checked_at: string
  note: string
}

export type Experiment = {
  id: string
  name: string
  description: string
  mode: string
  protocol: unknown
  plan: { task_version_ids?: string[]; profile_version_ids?: string[]; repetitions?: number; protocol?: string; mode?: string } | null
  plan_digest: string
  state: ExecState
  created_by: string
  created_at: string
  trial_count: number
  state_counts?: Record<string, number>
  verdict_counts?: Record<string, number>
}

export type Trial = {
  id: string
  experiment_id: string
  task_version_id: string
  profile_version_id: string
  repeat_index: number
  execution_state: ExecState
  verdict: Verdict
  current_attempt_id: string
  cancel_requested: boolean
  enqueue_seq: number
  queued_at: string
  cleanup_state?: string
  attempt_count?: number
}

export type Runtime = {
  executor?: string
  network?: string
  adapter?: string
  isolation?: string
  limitation?: string
  home_inherited?: boolean
  credential_env?: string[]
  budget?: { wall_seconds?: number; wall_source?: string; [k: string]: unknown }
  budget_digest?: string
  model_resolution_mismatch?: boolean
  requested_model?: string
  resolved_model?: string
  phases?: { agent_ms?: number; end_to_end_ms?: number; verify_ms?: number; queue_ms?: number }
  sandbox?: { mode: string; abi?: number; reason?: string; detail?: string; read_only?: string[]; read_write?: string[]; exposed?: string[]; limits?: string }
  pid?: number
}

export type Attempt = {
  id: string
  trial_id: string
  number: number
  account_id: string
  runtime: Runtime | null
  state: ExecState
  reason: string
  cleanup_state: string
  verdict: Verdict
  agent_started: boolean
  created_at: string
  finished_at: string
}

export type Summary = { planned: number; terminal: number; incomplete: number; pass: number; fail: number; inconclusive: number; unverified: number; assisted_pass: number; repair_pass?: number; note: string }
export type ExportItem = { id: string; bytes: number; status: string; download_path: string }

export type ExperimentDetail = {
  experiment: Experiment
  trials: Trial[] | null
  summary: Summary
  exports: ExportItem[] | null
  labels: { tasks: Record<string, TaskVersionInfo>; profiles: Record<string, ProfileVersionInfo> }
}

export type Overview = { running: number; queued: number; blocked_accounts: number; state_counts: Record<string, number>; note: string; recent: Trial[] | null }

export type Usage = { input_tokens: number | null; output_tokens: number | null; cached_input_tokens: number | null; cost_microusd: number | null; currency: string; confidence: string; billing_path: string; note: string }
export type Artifact = { id: string; kind: string; digest: string; bytes: number; status: string }

export type Evidence = { trial_id: string; attempt_id: string; kind: string }
export type BoardCell = { display_name: string; profile_version_id?: string; pass: number; fail: number; inconclusive: number; unverified: number; incomplete: number; assisted_pass: number; assisted_fail: number; repair_pass: number; agent_millis: number | null; end_to_end_millis: number | null; interventions: number; evidence?: Evidence[] }
export type Board = { task_version_id: string; comparable: boolean; reasons: string[] | null; cells: BoardCell[]; exploratory: boolean; note: string; paired: number; executor: string; network: string }
export type Statistics = { method?: string; seed?: number; iterations?: number; task_count?: number; note?: string; exploratory?: boolean; macro_average?: Record<string, number> | null; intervals?: Record<string, { low: number; high: number }> | null; paired?: Record<string, number> | null }

export type Settings = {
  executor_default: string
  executor_note: string
  network_restricted: string
  maintenance: boolean
  maintenance_reasons: string[] | null
  store_error: string
  webhook_configured: boolean
  credential_display: string
  global_limit: number
  agent_wall_seconds: number
  retention: { log_days: number; patch_days: number; report_days: number }
  disk_quota_bytes: number
  release_rss: string
  process_rss_bytes: number
  process_rss_note: string
  sandbox: { supported: boolean; abi: number; reason: string; disabled: boolean }
}

export type AuditEvent = { id: string; actor_id: string; action: string; resource_id: string; at: string; detail_json: string }
