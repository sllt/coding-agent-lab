import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, executionLabel, label, verdictLabel } from '../api.ts'
import { Card, Notice } from '../components/ui.tsx'

type Recent = { ID: string; ExecutionState: string; Verdict: string }
type OverviewData = { running: number; queued: number; blocked_accounts: number; note: string; recent: Recent[] | null }

export function Overview() {
  const [data, setData] = useState<OverviewData | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let stop = false
    async function tick() {
      try {
        const next = await api.request<OverviewData>('GET', '/api/v1/overview')
        if (!stop) {
          setData(next)
          setError('')
        }
      } catch (err) {
        if (!stop) setError(err instanceof Error ? err.message : '读取失败')
      }
    }
    void tick()
    const timer = window.setInterval(() => void tick(), 3000)
    return () => { stop = true; window.clearInterval(timer) }
  }, [])
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">概览</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {!data && !error ? <Notice>正在读取本机状态。</Notice> : null}
      {data ? (
        <>
          <div className="grid gap-3 sm:grid-cols-3">
            <Card title="运行中"><p className="text-3xl">{data.running}</p></Card>
            <Card title="排队"><p className="text-3xl">{data.queued}</p></Card>
            <Card title="账号阻塞"><p className="text-3xl">{data.blocked_accounts}</p></Card>
          </div>
          <Notice>{data.note}</Notice>
          <Card title="最近证据">
            {(data.recent || []).length === 0 ? <p className="text-sm">还没有运行。这里不会填入演示分数。</p> : (
              <ul className="flex flex-col gap-2 text-sm">
                {(data.recent || []).slice(0, 5).map((trial) => (
                  <li key={trial.ID}>
                    <Link className="underline" to={`/trials/${trial.ID}`}>{trial.ID.slice(0, 18)}</Link>
                    <span className="ml-2">{label(executionLabel, trial.ExecutionState)} · {label(verdictLabel, trial.Verdict)}</span>
                  </li>
                ))}
              </ul>
            )}
          </Card>
          <Link className="text-sm underline" to="/experiments/new">创建实验</Link>
        </>
      ) : null}
    </div>
  )
}
