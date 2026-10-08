import { CheckCircle2, CircleDashed, ShieldCheck, XCircle, AlertTriangle } from 'lucide-react'
import { duration } from '../../lib/format.ts'
import { Badge, Empty, Skeleton } from '../ui/index.ts'

type Check = { ID?: string; Required?: boolean; Kind?: string; Outcome?: string; FailureClass?: string; CaseCount?: number; MinCases?: number; MissingCaseIDs?: string[] | null; SkippedCaseIDs?: string[] | null; Assurance?: string; ExitCode?: number | null; DurationMS?: number }

const outcome: Record<string, { label: string; tone: 'success' | 'danger' | 'warning' | 'neutral'; icon: typeof CheckCircle2 }> = {
  pass: { label: '通过', tone: 'success', icon: CheckCircle2 },
  fail: { label: '失败', tone: 'danger', icon: XCircle },
  error: { label: '出错', tone: 'danger', icon: AlertTriangle },
  missing: { label: '缺失', tone: 'warning', icon: AlertTriangle },
  skipped: { label: '跳过', tone: 'neutral', icon: CircleDashed },
}

export function ChecksView({ items, note, loading }: { items: string[]; note?: string; loading: boolean }) {
  if (loading) return <div className="space-y-2 p-4">{[0, 1].map((i) => <Skeleton key={i} className="h-14" />)}</div>
  if (items.length === 0) return <Empty icon={<ShieldCheck />} title="还没有检查结果" description="独立验证器在 Agent 结束后运行，结果写在这里。" />
  return (
    <div>
      {note ? <p className="flex items-center gap-1.5 border-b px-4 py-2.5 text-xs text-muted-foreground"><ShieldCheck className="size-3.5 text-success" />{note}</p> : null}
      <ul className="divide-y">
        {items.map((raw, i) => {
          let c: Check | null = null
          try { c = JSON.parse(raw) as Check } catch { c = null }
          if (!c) return <li key={i} className="p-4"><pre className="whitespace-pre-wrap font-mono text-xs">{raw}</pre></li>
          const o = outcome[c.Outcome ?? ''] ?? { label: c.Outcome ?? '未知', tone: 'neutral' as const, icon: CircleDashed }
          return (
            <li key={i} className="px-4 py-3">
              <div className="flex flex-wrap items-center gap-2">
                <o.icon className={`size-4 ${o.tone === 'success' ? 'text-success' : o.tone === 'danger' ? 'text-danger' : o.tone === 'warning' ? 'text-warning' : 'text-muted-foreground'}`} />
                <span className="font-mono text-[13px] font-medium">{c.ID || `check-${i + 1}`}</span>
                <Badge tone={o.tone}>{o.label}</Badge>
                {c.Required ? <Badge tone="outline">必需</Badge> : <Badge tone="outline">可选</Badge>}
                {c.Kind ? <Badge tone="neutral">{c.Kind}</Badge> : null}
                <span className="ml-auto text-xs text-muted-foreground tabular">
                  {c.ExitCode !== undefined && c.ExitCode !== null ? `exit ${c.ExitCode}` : '未启动进程'}
                  {c.DurationMS ? ` · ${duration(c.DurationMS)}` : ''}
                </span>
              </div>
              <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 pl-6 text-xs text-muted-foreground">
                {c.CaseCount !== undefined ? <span>用例 {c.CaseCount}{c.MinCases ? ` / 至少 ${c.MinCases}` : ''}</span> : null}
                {c.FailureClass ? <span>失败类型 <span className="font-mono">{c.FailureClass}</span></span> : null}
                {c.Assurance ? <span>保证级别 {c.Assurance}</span> : null}
                {c.MissingCaseIDs?.length ? <span className="text-warning">缺少 {c.MissingCaseIDs.join(', ')}</span> : null}
                {c.SkippedCaseIDs?.length ? <span>跳过 {c.SkippedCaseIDs.join(', ')}</span> : null}
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
