import { ArrowDownToLine, ChevronRight, Search, TerminalSquare } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { cn } from '../../lib/cn.ts'
import type { AgentEvent, EventLog } from '../../lib/events.ts'
import { Alert, Badge, Button, Empty, Input, Skeleton, type BadgeTone } from '../ui/index.ts'

const PAGE = 400

function tone(ev: AgentEvent): BadgeTone {
  const t = (ev.type ?? '').toLowerCase()
  if (t.includes('error') || t.includes('fail') || t.includes('abort')) return 'danger'
  if (t.includes('phase')) return 'primary'
  if (t.includes('tool')) return 'info'
  if (t.includes('usage') || t.includes('cost')) return 'warning'
  if (t.includes('verdict') || t.includes('check')) return 'success'
  return 'neutral'
}

function summary(ev: AgentEvent): string {
  const p = ev.payload as Record<string, unknown> | undefined
  if (!p || typeof p !== 'object') return typeof ev.payload === 'string' ? ev.payload : ''
  for (const k of ['phase', 'message', 'text', 'summary', 'reason', 'tool', 'name', 'line', 'state']) {
    const v = p[k]
    if (typeof v === 'string' && v) return v
  }
  const s = JSON.stringify(p)
  return s.length > 160 ? `${s.slice(0, 160)}…` : s
}

function clock(iso?: string) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString('zh-CN', { hour12: false })
}

export function EventStream({ log, loading, live }: { log: EventLog; loading: boolean; live: boolean }) {
  const [q, setQ] = useState('')
  const [limit, setLimit] = useState(PAGE)
  const [follow, setFollow] = useState(true)
  const [open, setOpen] = useState<Set<number>>(() => new Set())
  const box = useRef<HTMLDivElement>(null)
  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase()
    return needle ? log.events.filter((e) => e.raw.toLowerCase().includes(needle)) : log.events
  }, [log.events, q])
  const shown = filtered.slice(Math.max(0, filtered.length - limit))
  const truncated = log.events.some((e) => e.raw.includes('"truncated":true') || e.raw.includes('event_buffer_8MiB'))
  const calibrated = log.events.some((e) => e.raw.includes('copied solution files'))

  useEffect(() => {
    if (follow && live && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [shown.length, follow, live])

  if (loading && log.events.length === 0) return <div className="space-y-2 p-4">{[0, 1, 2, 3, 4].map((i) => <Skeleton key={i} className="h-6" />)}</div>
  if (log.events.length === 0) return <Empty icon={<TerminalSquare />} title={live ? '等待事件…' : '没有事件'} description={live ? 'Agent 启动后，事件会实时追加在这里。' : '这次 Attempt 没有记录事件。'} />

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b px-3 py-2">
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input className="h-8 pl-8 text-xs" placeholder="在事件中搜索" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <span className="text-xs text-muted-foreground tabular">{filtered.length} 条</span>
        {live ? <Button size="xs" variant={follow ? 'secondary' : 'ghost'} onClick={() => setFollow((f) => !f)}><ArrowDownToLine />跟随</Button> : null}
      </div>
      {calibrated ? <Alert tone="warning" className="m-3 mb-0">这次通过来自参考解校准（事件含 copied solution files），是假 CLI 的校准路径，不是一次模型运行。</Alert> : null}
      {truncated ? <Alert tone="info" className="m-3 mb-0">原始输出带截断标记，下载的事件文件不会把截断后的内容伪装成完整日志。</Alert> : null}
      {log.gap ? <Alert tone="warning" className="m-3 mb-0">事件序号有缺口，部分事件没有送达；刷新页面可以获取完整快照。</Alert> : null}
      <div ref={box} className="min-h-0 flex-1 overflow-auto scrollbar-thin font-mono text-xs" onWheel={() => live && setFollow(false)}>
        {filtered.length > shown.length ? (
          <button type="button" className="w-full border-b py-1.5 text-center text-[11px] text-muted-foreground hover:bg-accent" onClick={() => setLimit((l) => l + PAGE)}>
            还有 {filtered.length - shown.length} 条更早的事件 · 加载更多
          </button>
        ) : null}
        <ol>
          {shown.map((ev) => {
            const isOpen = open.has(ev.sequence)
            return (
              <li key={ev.sequence} className="border-b border-border/60 last:border-0">
                <button
                  type="button"
                  className="flex w-full items-start gap-2 px-3 py-1.5 text-left hover:bg-accent/50"
                  onClick={() => setOpen((s) => { const n = new Set(s); if (n.has(ev.sequence)) n.delete(ev.sequence); else n.add(ev.sequence); return n })}
                  aria-expanded={isOpen}
                >
                  <ChevronRight className={cn('mt-0.5 size-3 shrink-0 text-muted-foreground transition-transform', isOpen && 'rotate-90')} />
                  <span className="w-9 shrink-0 text-right text-muted-foreground tabular">{ev.sequence}</span>
                  <span className="hidden w-16 shrink-0 text-muted-foreground sm:inline">{clock(ev.observed_at)}</span>
                  <Badge tone={tone(ev)} className="shrink-0 font-mono text-[10px]">{ev.type ?? 'event'}</Badge>
                  <span className="min-w-0 flex-1 truncate font-sans text-foreground/90">{summary(ev)}</span>
                  {ev.origin ? <span className="hidden shrink-0 text-[10px] text-muted-foreground md:inline">{ev.origin}</span> : null}
                </button>
                {isOpen ? <pre className="mx-3 mb-2 overflow-x-auto whitespace-pre-wrap break-all rounded-md bg-muted/60 p-2.5 text-[11px] leading-relaxed">{pretty(ev.raw)}</pre> : null}
              </li>
            )
          })}
        </ol>
      </div>
    </div>
  )
}

function pretty(raw: string) {
  try { return JSON.stringify(JSON.parse(raw), null, 2) } catch { return raw }
}
