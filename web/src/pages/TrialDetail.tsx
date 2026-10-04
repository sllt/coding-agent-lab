import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, cleanupLabel, executionLabel, label, money, verdictLabel } from '../api.ts'
import { Button, Card, Notice, TextArea } from '../components/ui.tsx'

type Trial = { ID: string; ExecutionState: string; Verdict: string; TaskVersionID: string; ProfileVersionID: string }
type Attempt = { ID: string; Number: number; State: string; Verdict: string; CleanupState: string; Reason: string; CreatedAt: string }
type Usage = { cost_microusd: number | null; confidence: string; note: string; input_tokens: number | null }
type Artifact = { id: string; kind: string; status: string; bytes: number }

export function TrialDetail() {
  const { id = '' } = useParams()
  const [trial, setTrial] = useState<Trial | null>(null)
  const [attempts, setAttempts] = useState<Attempt[]>([])
  const [checks, setChecks] = useState<string[]>([])
  const [events, setEvents] = useState('还没有事件。')
  const [usage, setUsage] = useState<Usage | null>(null)
  const [artifacts, setArtifacts] = useState<Artifact[]>([])
  const [patch, setPatch] = useState('')
  const [review, setReview] = useState('')
  const [error, setError] = useState('')
  const [note, setNote] = useState('')

  function load() {
    return api.request<{ trial: Trial; attempts: Attempt[] | null }>('GET', `/api/v1/trials/${id}`).then(async (data) => {
      setTrial(data.trial)
      const list = data.attempts || []
      setAttempts(list)
      const current = list[list.length - 1]
      if (!current) return
      const checkData = await api.request<{ items: string[] | null; note: string }>('GET', `/api/v1/attempts/${current.ID}/checks`)
      setChecks(checkData.items || [])
      setNote(checkData.note)
      const usageData = await api.request<Usage>('GET', `/api/v1/attempts/${current.ID}/usage`)
      setUsage(usageData)
      const artifactData = await api.request<{ items: Artifact[] | null }>('GET', `/api/v1/attempts/${current.ID}/artifacts`)
      const items = artifactData.items || []
      setArtifacts(items)
      const patchItem = items.find((item) => item.kind === 'patch' && item.status !== 'missing')
      if (patchItem) {
        const file = await fetch(`/api/v1/artifacts/${patchItem.id}/download`, { credentials: 'include' })
        setPatch(await file.text())
      } else {
        setPatch('')
      }
      const res = await fetch(`/api/v1/attempts/${current.ID}/events?once=1`, { credentials: 'include' })
      const text = await res.text()
      setEvents(text || '没有可回放的事件。')
    })
  }
  useEffect(() => { load().catch((err: Error) => setError(err.message)) }, [id])

  const current = attempts[attempts.length - 1]
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">运行详情</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {trial ? (
        <Card>
          <p>阶段：{label(executionLabel, trial.ExecutionState)}</p>
          <p>验收结论：{label(verdictLabel, trial.Verdict)}</p>
          <p>清理：{current ? label(cleanupLabel, current.CleanupState) : '尚未启动'}</p>
          <p className="mt-2 text-sm text-[#6b6258]">任务版本 {trial.TaskVersionID || '未知'} · 配置版本 {trial.ProfileVersionID || '未知'} · 开始 {current?.CreatedAt || '未知'} · 预算 未知</p>
          <p className="mt-2 text-sm text-[#6b6258]">native-trusted：本机进程执行，不提供容器级隔离。Agent 自己说的 PASS 不会变成这里的结论。</p>
        </Card>
      ) : <Notice>正在读取。</Notice>}
      <Card title="差异">
        {patch ? <pre className="max-h-80 overflow-auto whitespace-pre-wrap text-xs">{patch}</pre> : <p className="text-sm">还没有改动包。这里只显示文本，不会执行 Agent 输出。</p>}
      </Card>
      <Card title="制品">
        {artifacts.length === 0 ? <p className="text-sm">还没有制品。</p> : (
          <ul className="text-sm">
            {artifacts.map((item) => (
              <li key={item.id}>{item.kind} · {item.status} · {item.bytes} 字节 {item.status !== 'missing' ? <a className="underline" href={`/api/v1/artifacts/${item.id}/download`}>下载</a> : null}</li>
            ))}
          </ul>
        )}
      </Card>
      <Card title="独立证据">
        {checks.length === 0 ? <p className="text-sm">还没有检查结果。</p> : checks.map((item) => <pre key={item} className="mb-2 overflow-x-auto whitespace-pre-wrap text-xs">{item}</pre>)}
        {note ? <p className="text-sm">{note}</p> : null}
      </Card>
      <Card title="用量">
        <p>费用：{money(usage?.cost_microusd)}</p>
        <p className="text-sm">输入 token：{usage?.input_tokens ?? '未知'}。{usage?.note}</p>
      </Card>
      <Card title="事件">
        <pre className="max-h-80 overflow-auto whitespace-pre-wrap text-xs">{events}</pre>
      </Card>
      <Card title="人工意见">
        <TextArea value={review} onChange={(e) => setReview(e.target.value)} placeholder="意见会单独保存，不能把未通过改成通过。" />
        <Button className="mt-2" disabled={!current || !review} onClick={() => void api.request('POST', `/api/v1/attempts/${current.ID}/reviews`, { kind: 'note', body: review }).then(() => { setReview(''); return load() }).catch((err: Error) => setError(err.message))}>保存意见</Button>
      </Card>
      {attempts.length > 1 ? <Notice>这个 Trial 有 {attempts.length} 次 Attempt。第一次的结论不会被后来的重试改写。</Notice> : null}
    </div>
  )
}
