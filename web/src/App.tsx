import { useEffect, useState } from 'react'
import { NavLink, Route, BrowserRouter, Routes } from 'react-router-dom'
import { api } from './api.ts'
import { Notice } from './components/ui.tsx'
import { Compare } from './pages/Compare.tsx'
import { ExperimentDetail } from './pages/ExperimentDetail.tsx'
import { Experiments } from './pages/Experiments.tsx'
import { Login } from './pages/Login.tsx'
import { NewExperiment } from './pages/NewExperiment.tsx'
import { Overview } from './pages/Overview.tsx'
import { Profiles } from './pages/Profiles.tsx'
import { Projects } from './pages/Projects.tsx'
import { Settings } from './pages/Settings.tsx'
import { TrialDetail } from './pages/TrialDetail.tsx'

const links = [
  ['/', '概览'],
  ['/projects', '项目与任务'],
  ['/experiments', '实验'],
  ['/compare', '对比'],
  ['/profiles', 'Agent 配置'],
  ['/settings', '设置'],
]

export default function App() {
  const [ready, setReady] = useState(false)
  const [authed, setAuthed] = useState(false)
  useEffect(() => {
    api.request<{ csrf_token: string }>('GET', '/api/v1/session')
      .then((data) => { api.csrf = data.csrf_token; setAuthed(true) })
      .catch(() => setAuthed(false))
      .finally(() => setReady(true))
  }, [])
  if (!ready) return <main className="p-6"><Notice>正在连接本机控制面。</Notice></main>
  if (!authed) return <Login onReady={() => setAuthed(true)} />
  return (
    <BrowserRouter>
      <div className="min-h-screen md:grid md:grid-cols-[220px_1fr]">
        <nav className="flex gap-3 overflow-x-auto border-b border-[#1c1915]/10 bg-[#ebe4d8] px-4 py-3 md:flex-col md:border-b-0 md:border-r">
          <span className="hidden text-sm md:block">Coding Agent Lab</span>
          {links.map(([to, name]) => (
            <NavLink key={to} to={to} end={to === '/'} className={({ isActive }) => isActive ? 'text-sm underline' : 'text-sm'}>{name}</NavLink>
          ))}
          <button className="text-left text-sm" onClick={() => void api.request('POST', '/api/v1/logout').then(() => { api.csrf = ''; setAuthed(false) })}>退出</button>
        </nav>
        <main className="px-4 py-6 md:px-8">
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/projects" element={<Projects />} />
            <Route path="/experiments" element={<Experiments />} />
            <Route path="/experiments/new" element={<NewExperiment />} />
            <Route path="/experiments/:id" element={<ExperimentDetail />} />
            <Route path="/trials/:id" element={<TrialDetail />} />
            <Route path="/compare" element={<Compare />} />
            <Route path="/profiles" element={<Profiles />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
