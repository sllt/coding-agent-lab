import type { HTMLAttributes, ReactNode } from 'react'
import { cn } from '../../lib/cn.ts'

export function Card({ className, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('rounded-xl border bg-card text-card-foreground shadow-xs', className)} {...rest} />
}

export function CardHeader({ title, description, actions, className, icon }: { title: ReactNode; description?: ReactNode; actions?: ReactNode; className?: string; icon?: ReactNode }) {
  return (
    <div className={cn('flex items-start justify-between gap-3 border-b px-5 py-3.5', className)}>
      <div className="flex min-w-0 items-start gap-2.5">
        {icon ? <span className="mt-0.5 text-muted-foreground [&_svg]:size-4">{icon}</span> : null}
        <div className="min-w-0">
          <h3 className="text-sm font-semibold leading-6">{title}</h3>
          {description ? <p className="text-[13px] text-muted-foreground">{description}</p> : null}
        </div>
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  )
}

export function CardBody({ className, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('px-5 py-4', className)} {...rest} />
}
