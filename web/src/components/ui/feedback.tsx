import { AlertCircle, AlertTriangle, CheckCircle2, Info } from 'lucide-react'
import type { ReactNode } from 'react'
import { cn } from '../../lib/cn.ts'

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-muted', className)} />
}

export function Empty({ icon, title, description, action, className }: { icon?: ReactNode; title: ReactNode; description?: ReactNode; action?: ReactNode; className?: string }) {
  return (
    <div className={cn('flex flex-col items-center justify-center gap-2 px-6 py-12 text-center', className)}>
      {icon ? <div className="mb-1 flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground [&_svg]:size-5">{icon}</div> : null}
      <p className="text-sm font-medium">{title}</p>
      {description ? <p className="max-w-sm text-[13px] text-muted-foreground">{description}</p> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  )
}

const alertTone = {
  info: { cls: 'border-info/25 bg-info-soft text-info-soft-foreground', icon: Info },
  warning: { cls: 'border-warning/30 bg-warning-soft text-warning-soft-foreground', icon: AlertTriangle },
  danger: { cls: 'border-danger/25 bg-danger-soft text-danger-soft-foreground', icon: AlertCircle },
  success: { cls: 'border-success/25 bg-success-soft text-success-soft-foreground', icon: CheckCircle2 },
  neutral: { cls: 'border-border bg-muted/60 text-muted-foreground', icon: Info },
}

export function Alert({ tone = 'info', title, children, className, action }: { tone?: keyof typeof alertTone; title?: ReactNode; children?: ReactNode; className?: string; action?: ReactNode }) {
  const { cls, icon: Icon } = alertTone[tone]
  return (
    <div role={tone === 'danger' ? 'alert' : 'status'} className={cn('flex gap-2.5 rounded-lg border px-3.5 py-2.5 text-[13px] leading-relaxed', cls, className)}>
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden />
      <div className="min-w-0 flex-1">
        {title ? <p className="font-medium">{title}</p> : null}
        {children ? <div className={cn(title && 'mt-0.5 opacity-90')}>{children}</div> : null}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  )
}

export function Progress({ value, className, tone = 'primary' }: { value: number; className?: string; tone?: 'primary' | 'success' }) {
  const v = Math.max(0, Math.min(100, value))
  return (
    <div className={cn('h-1.5 w-full overflow-hidden rounded-full bg-muted', className)} role="progressbar" aria-valuenow={Math.round(v)} aria-valuemin={0} aria-valuemax={100}>
      <div className={cn('h-full rounded-full transition-[width] duration-500', tone === 'success' ? 'bg-success' : 'bg-primary')} style={{ width: `${v}%` }} />
    </div>
  )
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="rounded border bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">{children}</kbd>
}
