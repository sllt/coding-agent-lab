import { Bot, FlaskConical, GitCompareArrows, LayoutDashboard, LibraryBig, LogOut, Menu, Monitor, Moon, Plus, Settings2, Sun, X } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { Link, NavLink } from 'react-router-dom'
import { cn } from '../../lib/cn.ts'
import { applyTheme, storedTheme, type Theme } from '../../lib/theme.ts'
import { useOverview } from '../../lib/queries.ts'
import { Button, DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger, Tooltip } from '../ui/index.ts'

const nav = [
  { to: '/', label: '工作台', icon: LayoutDashboard, end: true },
  { to: '/experiments', label: '实验', icon: FlaskConical },
  { to: '/tasks', label: '任务库', icon: LibraryBig },
  { to: '/agents', label: 'Agents', icon: Bot },
  { to: '/compare', label: '对比', icon: GitCompareArrows },
  { to: '/settings', label: '系统', icon: Settings2 },
]

export function Logo({ className }: { className?: string }) {
  return (
    <div className={cn('flex items-center gap-2', className)}>
      <div className="flex size-7 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
        <FlaskConical className="size-4" strokeWidth={2.25} />
      </div>
      <div className="leading-tight">
        <p className="text-[13px] font-semibold tracking-tight">Agent Lab</p>
        <p className="text-[11px] text-muted-foreground">Coding Agent 实验台</p>
      </div>
    </div>
  )
}

function Sidebar({ onNavigate, onLogout }: { onNavigate?: () => void; onLogout: () => void }) {
  const overview = useOverview()
  const running = overview.data?.running ?? 0
  const queued = overview.data?.queued ?? 0
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 items-center px-4"><Logo /></div>
      <div className="px-3 pb-2">
        <NewExperimentLink onClick={onNavigate} />
      </div>
      <nav className="flex flex-1 flex-col gap-0.5 px-3 py-2" aria-label="主导航">
        {nav.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            onClick={onNavigate}
            className={({ isActive }) => cn(
              'group flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] font-medium transition-colors',
              isActive ? 'bg-card text-foreground shadow-xs ring-1 ring-border' : 'text-muted-foreground hover:bg-accent hover:text-foreground',
            )}
          >
            <item.icon className="size-4 shrink-0" />
            <span className="flex-1">{item.label}</span>
            {item.to === '/experiments' && running + queued > 0 ? (
              <span className="flex items-center gap-1 rounded-full bg-info-soft px-1.5 text-[11px] font-semibold text-info-soft-foreground tabular">
                <span className="size-1.5 animate-pulse-soft rounded-full bg-info" />{running + queued}
              </span>
            ) : null}
          </NavLink>
        ))}
      </nav>
      <div className="border-t border-sidebar-border p-3">
        <div className="flex items-center justify-between gap-2">
          <ThemeMenu />
          <Tooltip content="退出登录">
            <Button variant="ghost" size="icon-sm" onClick={onLogout} aria-label="退出登录"><LogOut /></Button>
          </Tooltip>
        </div>
        <p className="mt-2 px-1 text-[11px] leading-4 text-muted-foreground">仅本机 loopback · 证据优先，不打分</p>
      </div>
    </div>
  )
}

function ThemeMenu() {
  const [theme, setTheme] = useState<Theme>(storedTheme)
  useEffect(() => {
    applyTheme(theme)
    if (theme !== 'system') return
    const mq = matchMedia('(prefers-color-scheme: dark)')
    const on = () => applyTheme('system')
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [theme])
  const Icon = theme === 'dark' ? Moon : theme === 'light' ? Sun : Monitor
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="gap-2 px-2 text-xs"><Icon />{theme === 'dark' ? '深色' : theme === 'light' ? '浅色' : '跟随系统'}</Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuLabel>外观</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem icon={<Sun />} onSelect={() => setTheme('light')}>浅色</DropdownMenuItem>
        <DropdownMenuItem icon={<Moon />} onSelect={() => setTheme('dark')}>深色</DropdownMenuItem>
        <DropdownMenuItem icon={<Monitor />} onSelect={() => setTheme('system')}>跟随系统</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function AppShell({ children, onLogout }: { children: ReactNode; onLogout: () => void }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="min-h-screen lg:grid lg:grid-cols-[232px_1fr]">
      <div className="hidden border-r border-sidebar-border bg-sidebar lg:block">
        <aside className="sticky top-0 h-screen">
          <Sidebar onLogout={onLogout} />
        </aside>
      </div>
      <div className="sticky top-0 z-30 flex h-12 items-center justify-between border-b bg-background/80 px-3 backdrop-blur lg:hidden">
        <Button variant="ghost" size="icon-sm" onClick={() => setOpen(true)} aria-label="打开导航"><Menu /></Button>
        <Logo />
        <span className="w-8" />
      </div>
      {open ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div className="absolute inset-0 bg-black/40" onClick={() => setOpen(false)} />
          <aside className="absolute inset-y-0 left-0 w-64 border-r bg-sidebar shadow-xl animate-in">
            <Button variant="ghost" size="icon-sm" className="absolute right-2 top-3" onClick={() => setOpen(false)} aria-label="关闭导航"><X /></Button>
            <Sidebar onNavigate={() => setOpen(false)} onLogout={onLogout} />
          </aside>
        </div>
      ) : null}
      <main className="min-w-0">{children}</main>
    </div>
  )
}

export function NewExperimentLink({ onClick, className }: { onClick?: () => void; className?: string }) {
  return (
    <Link to="/experiments/new" onClick={onClick} className={cn('flex h-8 w-full items-center justify-center gap-1.5 rounded-md bg-primary text-[13px] font-medium text-primary-foreground shadow-xs transition-colors hover:bg-primary/90', className)}>
      <Plus className="size-4" />新建实验
    </Link>
  )
}
