import { ChevronRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { cn } from '../../lib/cn.ts'

export function PageHeader({ title, description, actions, crumbs, meta, className }: { title: ReactNode; description?: ReactNode; actions?: ReactNode; crumbs?: { to?: string; label: ReactNode }[]; meta?: ReactNode; className?: string }) {
  return (
    <header className={cn('border-b bg-background px-5 pb-4 pt-5 md:px-8', className)}>
      {crumbs?.length ? (
        <nav className="mb-2 flex items-center gap-1 text-xs text-muted-foreground" aria-label="面包屑">
          {crumbs.map((c, i) => (
            <span key={i} className="flex items-center gap-1">
              {i > 0 ? <ChevronRight className="size-3" /> : null}
              {c.to ? <Link to={c.to} className="hover:text-foreground">{c.label}</Link> : <span className="text-foreground">{c.label}</span>}
            </span>
          ))}
        </nav>
      ) : null}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          {description ? <p className="mt-1 max-w-3xl text-[13px] text-muted-foreground">{description}</p> : null}
          {meta ? <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-muted-foreground">{meta}</div> : null}
        </div>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
    </header>
  )
}

export function PageBody({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('mx-auto w-full max-w-[1400px] px-5 py-6 md:px-8', className)}>{children}</div>
}

export function Stat({ label, value, hint, icon, tone, className }: { label: ReactNode; value: ReactNode; hint?: ReactNode; icon?: ReactNode; tone?: 'default' | 'info' | 'success' | 'warning' | 'danger'; className?: string }) {
  const toneCls = {
    default: 'bg-muted text-muted-foreground',
    info: 'bg-info-soft text-info-soft-foreground',
    success: 'bg-success-soft text-success-soft-foreground',
    warning: 'bg-warning-soft text-warning-soft-foreground',
    danger: 'bg-danger-soft text-danger-soft-foreground',
  }[tone ?? 'default']
  return (
    <div className={cn('rounded-xl border bg-card p-4 shadow-xs', className)}>
      <div className="flex items-center justify-between">
        <p className="text-[13px] text-muted-foreground">{label}</p>
        {icon ? <span className={cn('flex size-7 items-center justify-center rounded-md [&_svg]:size-4', toneCls)}>{icon}</span> : null}
      </div>
      <p className="mt-2 text-2xl font-semibold tracking-tight tabular">{value}</p>
      {hint ? <p className="mt-1 text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  )
}
