export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export const api = {
  csrf: '',
  async request<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
    const head: Record<string, string> = { ...headers }
    if (body !== undefined) head['Content-Type'] = 'application/json'
    if (this.csrf && method !== 'GET') head['X-CSRF-Token'] = this.csrf
    const res = await fetch(path, {
      method,
      headers: head,
      credentials: 'include',
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const text = await res.text()
    const json = text ? JSON.parse(text) as { data?: T; error?: { message?: string } } : {}
    if (!res.ok) throw new ApiError(res.status, json.error?.message || '请求失败')
    return json.data as T
  },
}

export const executionLabel: Record<string, string> = {
  queued: '排队',
  preparing: '准备',
  running: '运行中',
  collecting: '收集改动',
  verifying: '独立验收',
  completed: '已结束',
  cancelling: '正在取消',
  cancelled: '已取消',
  aborted: '已中止',
}

export const verdictLabel: Record<string, string> = {
  pass: '通过',
  fail: '未通过',
  inconclusive: '证据不足',
  unverified: '未经验收',
}

export const cleanupLabel: Record<string, string> = {
  pending: '清理中',
  clean: '已清理',
  failed: '清理失败',
  quarantined: '已隔离',
  not_started: '尚未启动',
}

export function label(table: Record<string, string>, value: string) {
  return table[value] || value || '未知'
}

export function money(cost: number | null | undefined) {
  if (cost === null || cost === undefined) return '未知'
  return `${cost} microusd`
}
