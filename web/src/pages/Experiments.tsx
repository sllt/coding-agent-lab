import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, executionLabel, label } from '../api.ts'
import { Card, Notice } from '../components/ui.tsx'

type Experiment = { ID: string; State: string; ProtocolJSON: string; TrialCount: number }

export function Experiments() {
  const [items, setItems] = useState<Experiment[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => {
    let stop = false
    async function tick() {
      try {
        const data = await api.request<{ items: Experiment[] | null }>('GET', '/api/v1/experiments')
        if (!stop) {
          setItems(data.items || [])
          setError('')
        }
      } catch (err) {
        if (!stop) setError(err instanceof Error ? err.message : '读取失败')
      } finally {
        if (!stop) setLoading(false)
      }
    }
    void tick()
    const timer = window.setInterval(() => void tick(), 3000)
    return () => { stop = true; window.clearInterval(timer) }
  }, [])
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-2xl">实验</h1>
        <Link className="text-sm underline" to="/experiments/new">创建</Link>
      </div>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {loading ? <Notice>正在读取实验。</Notice> : items.length === 0 ? <Notice>还没有实验。空列表不会填演示分数。</Notice> : (
        <Card>
          <ul className="flex flex-col gap-3 text-sm">
            {items.map((item) => (
              <li key={item.ID}>
                <Link className="underline" to={`/experiments/${item.ID}`}>{item.ID}</Link>
                <span className="ml-2">{item.ProtocolJSON} · {item.TrialCount} 个 Trial · {label(executionLabel, item.State)}</span>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  )
}
