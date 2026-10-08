import type { BadgeTone } from '../components/ui/badge.tsx'

export const execLabel: Record<string, string> = {
  queued: '排队中',
  preparing: '准备中',
  running: '运行中',
  collecting: '收集改动',
  verifying: '独立验收',
  completed: '已结束',
  cancelling: '取消中',
  cancelled: '已取消',
  aborted: '已中止',
}

export const execTone: Record<string, BadgeTone> = {
  queued: 'neutral',
  preparing: 'info',
  running: 'info',
  collecting: 'info',
  verifying: 'info',
  completed: 'success',
  cancelling: 'warning',
  cancelled: 'neutral',
  aborted: 'danger',
}

export const activeStates = new Set(['preparing', 'running', 'collecting', 'verifying', 'cancelling'])
export const terminalStates = new Set(['completed', 'cancelled', 'aborted'])

export const verdictLabel: Record<string, string> = {
  pass: '通过',
  fail: '未通过',
  inconclusive: '证据不足',
  unverified: '未验收',
}

export const verdictTone: Record<string, BadgeTone> = {
  pass: 'success',
  fail: 'danger',
  inconclusive: 'warning',
  unverified: 'neutral',
}

export const cleanupLabel: Record<string, string> = {
  pending: '清理中',
  clean: '已清理',
  failed: '清理失败',
  quarantined: '已隔离',
  not_started: '未启动',
}

export const readinessLabel: Record<string, string> = {
  unavailable: '未安装',
  cli_detected: '已检测到 CLI',
  needs_credentials: '缺少凭据',
  ready_unverified: '可运行 · 未验证',
  verified: '已验证',
}

export const readinessTone: Record<string, BadgeTone> = {
  unavailable: 'danger',
  cli_detected: 'warning',
  needs_credentials: 'warning',
  ready_unverified: 'info',
  verified: 'success',
}

export const adapterLabel: Record<string, string> = {
  fixture: '本地假 CLI',
  cursor: 'Cursor',
  grok: 'Grok',
  opencode: 'OpenCode',
}

export const modeLabel: Record<string, string> = {
  agent_profile: '配置对比',
  controlled_model: '同工具换模型',
  workflow: '工作流对比',
}

export function lookup(table: Record<string, string>, v: string | undefined | null, fallback = '未知') {
  if (!v) return fallback
  return table[v] ?? v
}
