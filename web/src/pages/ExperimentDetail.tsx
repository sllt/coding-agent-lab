import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, cleanupLabel, executionLabel, label, verdictLabel } from '../api.ts'
import { Button, Notice } from '../components/ui.tsx'

type Trial = { ID: string; ExecutionState: string; Verdict: string; RepeatIndex: number; CurrentAttemptID: string; cleanup_state: string }
type Summary = { planned: number; terminal: number; incomplete: number; pass: number; fail: number; inconclusive: number; unverified: number; assisted_pass: number; repair_pass?: number; note: string }
type ExportItem = { id: string; bytes: number; status: string; download_path: string }

export function ExperimentDetail() {
  const { id = '' } = useParams()
  const [trials, setTrials] = useState<Trial[]>([])
  const [summary, setSummary] = useState<Summary | null>(null)
  const [exports, setExports] = useState<ExportItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [filter, setFilter] = useState('all')

  useEffect(() => {
    let stop = false
    async function tick() {
      try {
        const data = await api.request<{ trials: Trial[] | null; summary: Summary; exports: ExportItem[] | null }>('GET', `/api/v1/experiments/${id}`)
        if (stop) return
        setTrials(data.trials || [])
        setSummary(data.summary)
        setExports(data.exports || [])
        setError('')
      } catch (err) {
        if (!stop) setError(err instanceof Error ? err.message : '读取失败')
      } finally {
        if (!stop) setLoading(false)
      }
    }
    void tick()
    const timer = window.setInterval(() => void tick(), 3000)
    return () => { stop = true; window.clearInterval(timer) }
  }, [id])

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">实验详情</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {loading && !summary ? <Notice>正在读取实验。</Notice> : null}
      {summary ? <Notice>计划 {summary.planned}，已结束 {summary.terminal}，未完成 {summary.incomplete}。首次通过 {summary.pass}，未通过 {summary.fail}，证据不足 {summary.inconclusive}，未经验收 {summary.unverified}。人工重试通过 {summary.assisted_pass ?? 0}。修复协议通过 {summary.repair_pass ?? 0}。{summary.note}</Notice> : null}
      {note ? <Notice>{note}</Notice> : null}
      <div className="flex flex-wrap items-center gap-3">
        <Button tone="quiet" onClick={() => void api.request('POST', `/api/v1/experiments/${id}/export`).then(() => setNote('已生成导出。可以在下面下载。导出不含凭据、隐藏测试和参考解。')).then(() => api.request<{ exports: ExportItem[] | null }>('GET', `/api/v1/experiments/${id}`)).then((data) => setExports(data.exports || [])).catch((err: Error) => setError(err.message))}>导出报告</Button>
        {exports.map((item) => (
          <a key={item.id} className="text-sm underline" href={item.download_path}>下载导出（{item.bytes} 字节）</a>
        ))}
      </div>
      <label className="text-sm">筛选阶段
        <select className="ml-2 rounded border px-2 py-1" value={filter} onChange={(e) => setFilter(e.target.value)}>
          <option value="all">全部</option>
          <option value="queued">排队</option>
          <option value="running">运行中</option>
          <option value="completed">已结束</option>
          <option value="cancelled">已取消</option>
          <option value="aborted">已中止</option>
        </select>
      </label>
      <ul className="flex flex-col gap-3">
        {trials.filter((trial) => filter === 'all' || trial.ExecutionState === filter).map((trial) => (
          <li key={trial.ID} className="rounded-lg border border-[#1c1915]/10 bg-white/80 p-3">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0 text-sm">
                <Link className="underline" to={`/trials/${trial.ID}`}>{trial.ID}</Link>
                <p className="mt-1">阶段 {label(executionLabel, trial.ExecutionState)} · 结论 {label(verdictLabel, trial.Verdict)}</p>
                <p>清理 {label(cleanupLabel, trial.cleanup_state)} · 第 {trial.RepeatIndex} 次</p>
              </div>
              <Button tone="danger" onClick={() => void api.request<{ note: string; cleanup_state: string }>('POST', `/api/v1/trials/${trial.ID}/cancel`).then((data) => setNote(`${data.note} 清理：${label(cleanupLabel, data.cleanup_state)}`)).catch((err: Error) => setError(err.message))}>取消</Button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}
