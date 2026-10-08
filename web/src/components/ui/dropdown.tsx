import * as DropdownPrimitive from '@radix-ui/react-dropdown-menu'
import type { ComponentProps, ReactNode } from 'react'
import { cn } from '../../lib/cn.ts'

export const DropdownMenu = DropdownPrimitive.Root
export const DropdownMenuTrigger = DropdownPrimitive.Trigger

export function DropdownMenuContent({ className, align = 'end', ...rest }: ComponentProps<typeof DropdownPrimitive.Content>) {
  return (
    <DropdownPrimitive.Portal>
      <DropdownPrimitive.Content align={align} sideOffset={6} className={cn('z-50 min-w-44 rounded-lg border bg-popover p-1 text-popover-foreground shadow-lg data-[state=open]:animate-in', className)} {...rest} />
    </DropdownPrimitive.Portal>
  )
}

export function DropdownMenuItem({ className, icon, children, danger, ...rest }: ComponentProps<typeof DropdownPrimitive.Item> & { icon?: ReactNode; danger?: boolean }) {
  return (
    <DropdownPrimitive.Item
      className={cn('flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-[13px] outline-none data-[highlighted]:bg-accent [&_svg]:size-4 [&_svg]:text-muted-foreground', danger && 'text-danger [&_svg]:text-danger', className)}
      {...rest}
    >
      {icon}
      {children}
    </DropdownPrimitive.Item>
  )
}

export function DropdownMenuLabel({ children }: { children: ReactNode }) {
  return <DropdownPrimitive.Label className="px-2 py-1.5 text-xs text-muted-foreground">{children}</DropdownPrimitive.Label>
}

export function DropdownMenuSeparator() {
  return <DropdownPrimitive.Separator className="-mx-1 my-1 h-px bg-border" />
}
