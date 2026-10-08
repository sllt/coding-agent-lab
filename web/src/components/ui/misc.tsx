import { Check, Copy } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { cn } from '../../lib/cn.ts'
import { Tooltip } from './tooltip.tsx'

export function CopyText({ value, display, className }: { value: string; display?: ReactNode; className?: string }) {
  const [done, setDone] = useState(false)
  return (
    <Tooltip content={done ? '已复制' : '复制'}>
      <button
        type="button"
        className={cn('group inline-flex max-w-full items-center gap-1 rounded font-mono text-xs text-muted-foreground hover:text-foreground', className)}
        onClick={(e) => {
          e.preventDefault()
          e.stopPropagation()
          void navigator.clipboard?.writeText(value).then(() => {
            setDone(true)
            window.setTimeout(() => setDone(false), 1200)
          }).catch(() => toast.error('无法访问剪贴板'))
        }}
      >
        <span className="truncate">{display ?? value}</span>
        {done ? <Check className="size-3 shrink-0 text-success" /> : <Copy className="size-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />}
      </button>
    </Tooltip>
  )
}

export function Separator({ className, vertical }: { className?: string; vertical?: boolean }) {
  return <div className={cn(vertical ? 'mx-1 h-4 w-px' : 'my-1 h-px w-full', 'bg-border', className)} role="separator" />
}

export function KeyValue({ items, className }: { items: { label: ReactNode; value: ReactNode; mono?: boolean }[]; className?: string }) {
  return (
    <dl className={cn('grid grid-cols-[minmax(84px,auto)_1fr] gap-x-4 gap-y-2 text-[13px]', className)}>
      {items.map((item, i) => (
        <div key={i} className="contents">
          <dt className="text-muted-foreground">{item.label}</dt>
          <dd className={cn('min-w-0 break-words', item.mono && 'font-mono text-xs leading-5')}>{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}
