const rtf = new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' })

export function timeAgo(iso: string | undefined | null, now = Date.now()): string {
  if (!iso) return '—'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return '—'
  const s = Math.round((t - now) / 1000)
  const abs = Math.abs(s)
  if (abs < 45) return '刚刚'
  if (abs < 3600) return rtf.format(Math.round(s / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(s / 3600), 'hour')
  if (abs < 86400 * 30) return rtf.format(Math.round(s / 86400), 'day')
  return dateTime(iso)
}

export function dateTime(iso: string | undefined | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}

export function duration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || Number.isNaN(ms)) return '—'
  if (ms < 1000) return `${Math.round(ms)} ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)} s`
  const m = Math.floor(s / 60)
  const rest = Math.round(s % 60)
  if (m < 60) return `${m}m ${rest}s`
  return `${Math.floor(m / 60)}h ${m % 60}m`
}

export function bytes(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`
}

export function shortId(id: string | undefined | null, n = 8): string {
  if (!id) return '—'
  const i = id.indexOf('_')
  const body = i >= 0 ? id.slice(i + 1) : id
  return body.slice(0, n)
}

export function percent(part: number, total: number): string {
  if (!total) return '—'
  return `${Math.round((part / total) * 100)}%`
}

export function num(n: number | null | undefined): string {
  if (n === null || n === undefined) return '未知'
  return n.toLocaleString('zh-CN')
}

export function protocolName(p: unknown): string {
  if (typeof p === 'string' && p) return p
  if (p && typeof p === 'object' && 'name' in p && typeof (p as { name: unknown }).name === 'string') return (p as { name: string }).name
  return 'single-pass-v1'
}
