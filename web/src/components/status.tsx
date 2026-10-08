import { CheckCircle2, CircleDashed, CircleSlash, Clock, Loader2, ShieldCheck, ShieldQuestion, XCircle, AlertTriangle, HelpCircle } from 'lucide-react'
import { Badge, Dot, Tooltip } from './ui/index.ts'
import { cn } from '../lib/cn.ts'
import { activeStates, execLabel, execTone, lookup, readinessLabel, readinessTone, verdictLabel, verdictTone } from '../lib/status.ts'

export function StateBadge({ state, className }: { state: string; className?: string }) {
  const active = activeStates.has(state)
  return (
    <Badge tone={execTone[state] ?? 'neutral'} className={className}>
      {active ? <Loader2 className="animate-spin" /> : state === 'queued' ? <Clock /> : state === 'aborted' ? <XCircle /> : state === 'cancelled' ? <CircleSlash /> : <CheckCircle2 />}
      {lookup(execLabel, state)}
    </Badge>
  )
}

export function VerdictBadge({ verdict, className }: { verdict: string; className?: string }) {
  const Icon = verdict === 'pass' ? CheckCircle2 : verdict === 'fail' ? XCircle : verdict === 'inconclusive' ? AlertTriangle : CircleDashed
  return (
    <Badge tone={verdictTone[verdict] ?? 'neutral'} className={className}>
      <Icon />
      {lookup(verdictLabel, verdict)}
    </Badge>
  )
}

export function ReadinessBadge({ readiness, className }: { readiness?: string | null; className?: string }) {
  if (!readiness) return <Badge tone="outline" className={className}><HelpCircle />未检测</Badge>
  const Icon = readiness === 'verified' ? ShieldCheck : readiness === 'ready_unverified' ? ShieldQuestion : readiness === 'unavailable' ? XCircle : AlertTriangle
  return <Badge tone={readinessTone[readiness] ?? 'neutral'} className={className}><Icon />{lookup(readinessLabel, readiness)}</Badge>
}

/** Small square per trial, coloured by verdict once terminal. */
export function TrialSquare({ state, verdict, size = 'md', title }: { state: string; verdict: string; size?: 'sm' | 'md'; title?: string }) {
  const terminal = state === 'completed' || state === 'cancelled' || state === 'aborted'
  const color = !terminal
    ? activeStates.has(state) ? 'bg-info/70 animate-pulse-soft' : 'bg-muted-foreground/20'
    : state !== 'completed' ? 'bg-muted-foreground/35'
    : verdict === 'pass' ? 'bg-success' : verdict === 'fail' ? 'bg-danger' : verdict === 'inconclusive' ? 'bg-warning' : 'bg-muted-foreground/40'
  return (
    <Tooltip content={title}>
      <span className={cn('inline-block rounded-[3px]', size === 'sm' ? 'size-2.5' : 'size-4', color)} aria-label={title} />
    </Tooltip>
  )
}

/** Horizontal stacked bar of verdicts for a set of trials. */
export function VerdictBar({ counts, total, className }: { counts: Record<string, number>; total: number; className?: string }) {
  const parts: { key: string; cls: string }[] = [
    { key: 'pass', cls: 'bg-success' },
    { key: 'fail', cls: 'bg-danger' },
    { key: 'inconclusive', cls: 'bg-warning' },
    { key: 'unverified', cls: 'bg-muted-foreground/30' },
  ]
  if (!total) return <div className={cn('h-1.5 rounded-full bg-muted', className)} />
  return (
    <div className={cn('flex h-1.5 overflow-hidden rounded-full bg-muted', className)}>
      {parts.map((p) => {
        const n = counts[p.key] ?? 0
        if (!n) return null
        return <div key={p.key} className={p.cls} style={{ width: `${(n / total) * 100}%` }} />
      })}
    </div>
  )
}

export function LiveDot({ active }: { active: boolean }) {
  return <Dot tone={active ? 'info' : 'neutral'} pulse={active} />
}
