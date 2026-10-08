import { AlertTriangle } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Button } from './button.tsx'
import { Dialog, DialogContent, DialogTrigger } from './dialog.tsx'
import { Input } from './input.tsx'

/**
 * ConfirmButton asks before running a destructive or irreversible action.
 * When `typeToConfirm` is set the user must type that word first.
 */
export function ConfirmButton({ children, title, description, confirmLabel = '确认', onConfirm, variant = 'outline', size = 'sm', danger = false, typeToConfirm, disabled, icon }: {
  children: ReactNode
  title: ReactNode
  description: ReactNode
  confirmLabel?: string
  onConfirm: () => Promise<unknown> | void
  variant?: 'outline' | 'default' | 'danger' | 'danger-outline' | 'ghost' | 'secondary'
  size?: 'sm' | 'xs' | 'default'
  danger?: boolean
  typeToConfirm?: string
  disabled?: boolean
  icon?: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [typed, setTyped] = useState('')
  const ready = !typeToConfirm || typed.trim() === typeToConfirm
  return (
    <Dialog open={open} onOpenChange={(next) => { setOpen(next); if (!next) setTyped('') }}>
      <DialogTrigger asChild>
        <Button variant={variant} size={size} disabled={disabled}>{icon}{children}</Button>
      </DialogTrigger>
      <DialogContent
        title={<span className="flex items-center gap-2">{danger ? <AlertTriangle className="size-4 text-danger" /> : null}{title}</span>}
        footer={(
          <>
            <Button variant="outline" size="sm" onClick={() => setOpen(false)}>取消</Button>
            <Button
              variant={danger ? 'danger' : 'default'}
              size="sm"
              loading={busy}
              disabled={!ready}
              onClick={async () => {
                setBusy(true)
                try {
                  await onConfirm()
                  setOpen(false)
                } finally {
                  setBusy(false)
                }
              }}
            >
              {confirmLabel}
            </Button>
          </>
        )}
      >
        <div className="space-y-3 text-sm text-muted-foreground">
          <div>{description}</div>
          {typeToConfirm ? (
            <div className="space-y-1.5">
              <p className="text-foreground">输入 <code className="rounded bg-muted px-1 py-0.5 text-xs">{typeToConfirm}</code> 以确认</p>
              <Input value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus />
            </div>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
