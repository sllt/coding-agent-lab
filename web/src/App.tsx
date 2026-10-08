import { useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { lazy, Suspense, useEffect, useState } from 'react'
import { BrowserRouter, Link, Route, Routes } from 'react-router-dom'
import { toast } from 'sonner'
import { api, setUnauthorizedHandler } from './api.ts'
import { AppShell } from './components/layout/AppShell.tsx'
import { PageBody } from './components/layout/PageHeader.tsx'
import { Empty, Skeleton } from './components/ui/index.ts'
import { Dashboard } from './pages/Dashboard.tsx'
import { Experiments } from './pages/Experiments.tsx'
import { Login } from './pages/Login.tsx'

// Heavier pages load on demand so the first paint only ships the shell,
// the dashboard and the experiment list.
const Agents = lazy(() => import('./pages/Agents.tsx').then((m) => ({ default: m.Agents })))
const Compare = lazy(() => import('./pages/Compare.tsx').then((m) => ({ default: m.Compare })))
const ExperimentDetail = lazy(() => import('./pages/ExperimentDetail.tsx').then((m) => ({ default: m.ExperimentDetail })))
const NewExperiment = lazy(() => import('./pages/NewExperiment.tsx').then((m) => ({ default: m.NewExperiment })))
const Settings = lazy(() => import('./pages/Settings.tsx').then((m) => ({ default: m.Settings })))
const Tasks = lazy(() => import('./pages/Tasks.tsx').then((m) => ({ default: m.Tasks })))
const TrialDetail = lazy(() => import('./pages/TrialDetail.tsx').then((m) => ({ default: m.TrialDetail })))

type AuthState = 'checking' | 'in' | 'out'

export default function App() {
  const [auth, setAuth] = useState<AuthState>('checking')
  const qc = useQueryClient()

  useEffect(() => {
    setUnauthorizedHandler(() => {
      setAuth((prev) => {
        if (prev === 'in') toast.warning('会话已过期，请重新登录')
        return 'out'
      })
      qc.clear()
    })
    return () => setUnauthorizedHandler(null)
  }, [qc])

  useEffect(() => {
    api.request<{ csrf_token: string }>('GET', '/api/v1/session')
      .then((data) => { api.csrf = data.csrf_token; setAuth('in') })
      .catch(() => setAuth('out'))
  }, [])

  if (auth === 'checking') {
    return (
      <div className="flex min-h-screen items-center justify-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" />正在连接本机控制面
      </div>
    )
  }
  if (auth === 'out') return <Login onReady={() => setAuth('in')} />

  const logout = () => {
    void api.request('POST', '/api/v1/logout').finally(() => {
      api.csrf = ''
      qc.clear()
      setAuth('out')
    })
  }

  return (
    <BrowserRouter>
      <AppShell onLogout={logout}>
        <Suspense fallback={<PageBody className="space-y-4"><Skeleton className="h-16" /><Skeleton className="h-64" /></PageBody>}>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/experiments" element={<Experiments />} />
          <Route path="/experiments/new" element={<NewExperiment />} />
          <Route path="/experiments/:id" element={<ExperimentDetail />} />
          <Route path="/trials/:id" element={<TrialDetail />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/agents" element={<Agents />} />
          <Route path="/compare" element={<Compare />} />
          <Route path="/settings" element={<Settings />} />
          {/* Old paths keep working. */}
          <Route path="/projects" element={<Tasks />} />
          <Route path="/profiles" element={<Agents />} />
          <Route path="*" element={<PageBody><Empty title="没有这个页面" description="这个地址不在工作台里。" action={<Link className="text-sm text-primary hover:underline" to="/">回到工作台</Link>} /></PageBody>} />
        </Routes>
        </Suspense>
      </AppShell>
    </BrowserRouter>
  )
}
