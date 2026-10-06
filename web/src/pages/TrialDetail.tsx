import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, cleanupLabel, executionLabel, label, money, verdictLabel } from '../api.ts'
import { Button, Card, Input, Notice, TextArea } from '../components/ui.tsx'

type Trial = { ID: string; ExecutionState: string; Verdict: string; TaskVersionID: string; ProfileVersionID: string }
type Attempt = { ID: string; Number: number; State: string; Verdict: string; CleanupState: string; Reason: string; CreatedAt: string; RuntimeJSON?: string }
type Usage = { cost_microusd: number | null; confidence: string; note: string; input_tokens: number | null; output_tokens: number | null }
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
  const [reviews, setReviews] = useState<string[]>([])
  const [review, setReview] = useState('')
  const [blind, setBlind] = useState(true)
  const [showAllEvents, setShowAllEvents] = useState(false)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [loading, setLoading] = useState(true)

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
      const reviewData = await api.request<{ items: string[] | null }>('GET', `/api/v1/attempts/${current.ID}/reviews`)
      setReviews(reviewData.items || [])
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
    })
  }
  useEffect(() => { load().catch((err: Error) => setError(err.message)).finally(() => setLoading(false)) }, [id])

  const current = attempts[attempts.length - 1]
  const live = current && !['completed', 'cancelled', 'aborted'].includes(current.State)

  useEffect(() => {
    if (!current) return
    let stop = false
    let source: EventSource | null = null
    const replay = () => fetch(`/api/v1/attempts/${current.ID}/events?once=1`, { credentials: 'include' })
      .then((res) => res.text())
      .then((text) => { if (!stop) setEvents(text || '没有可回放的事件。') })
      .catch(() => { if (!stop) setEvents('没有可回放的事件。') })
    void replay()
    if (typeof EventSource !== 'undefined') {
      source = new EventSource(`/api/v1/attempts/${current.ID}/events`, { withCredentials: true })
      source.onmessage = () => { void replay() }
      source.addEventListener('gap', () => { if (!stop) setEvents((prev) => prev + '\n事件序号有缺口，需要完整快照。') })
    }
    const timer = window.setInterval(() => { void replay(); if (live) void load().catch(() => undefined) }, 2000)
    return () => {
      stop = true
      source?.close()
      window.clearInterval(timer)
    }
  }, [current?.ID, live])

  const calibrated = events.includes('copied solution files')
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">运行详情</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {loading && !trial ? <Notice>正在读取。</Notice> : null}
      {trial ? (
        <Card>
          <p>阶段：{label(executionLabel, trial.ExecutionState)}</p>
          <p>当前 Trial 行：{label(verdictLabel, trial.Verdict)}。对比和实验摘要使用第一次物理执行的结论。</p>
          <p>清理：{current ? label(cleanupLabel, current.CleanupState) : '尚未启动'}</p>
          <p className="mt-2 text-sm text-[#6b6258]">任务版本 {trial.TaskVersionID || '未知'} · 配置版本 {trial.ProfileVersionID || '未知'} · 开始 {current?.CreatedAt || '未知'} · 费用预算 未知。{phaseLine(current?.RuntimeJSON)}</p>
          <p className="mt-2 text-sm text-[#6b6258]">native-trusted：本机进程，HOME 是这次运行的空目录。同一用户仍能读到控制面文件，这不是容器隔离。Agent 自己说的 PASS 不会变成这里的结论。</p>
        </Card>
      ) : null}
      {calibrated ? <Notice tone="warn">这次通过来自参考解校准：事件里有 copied solution files。这是假 CLI 的校准路径，不是一次未标明的模型运行。</Notice> : null}
      <Card title="每次 Attempt">
        {attempts.length === 0 ? <p className="text-sm">还没有 Attempt。</p> : (
          <ul className="flex flex-col gap-2 text-sm">
            {attempts.map((attempt) => (
              <li key={attempt.ID}>第 {attempt.Number} 次 · {label(executionLabel, attempt.State)} · {label(verdictLabel, attempt.Verdict)} · 清理 {label(cleanupLabel, attempt.CleanupState)}{attempt.Reason ? ` · ${attempt.Reason}` : ''}</li>
            ))}
          </ul>
        )}
        {attempts.length > 1 ? <p className="mt-2 text-sm">第一次的结论单独保留。后来的人工重试不会改写它，也不会进入首 Attempt 通过率。</p> : null}
      </Card>
      <Card title="差异">
        {patch ? <PatchView raw={patch} /> : <p className="text-sm">还没有改动包。这里只显示文本，不会执行 Agent 输出。</p>}
      </Card>
      <Card title="制品">
        {artifacts.length === 0 ? <p className="text-sm">还没有制品。</p> : (
          <ul className="text-sm">
            {artifacts.map((item) => (
              <li key={item.id}>{item.kind} · {item.status} · {item.bytes} 字节 {item.status !== 'missing' ? <a className="underline" href={`/api/v1/artifacts/${item.id}/download`} target="_blank" rel="noreferrer">下载</a> : null}</li>
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
        <p className="text-sm">输入 token：{usage?.input_tokens ?? '未知'}。输出 token：{usage?.output_tokens ?? '未知'}。{usage?.note}</p>
      </Card>
      <Card title="事件">
        {events.includes('event_buffer_8MiB') || events.includes('"truncated":true') ? <p className="mb-2 text-sm">原始输出带截断标记。下载的事件文件不是被裁掉之后假装完整的文本。</p> : null}
        <EventView text={events} showAll={showAllEvents} />
        {eventDocuments(events).length > 80 ? <Button tone="quiet" className="mt-2" onClick={() => setShowAllEvents((v) => !v)}>{showAllEvents ? '只看一段' : '展开全部'}</Button> : null}
      </Card>
      <Card title="人工意见">
        {reviews.length === 0 ? <p className="mb-2 text-sm">还没有意见。</p> : (
          <ul className="mb-2 flex flex-col gap-2 text-sm">
            {reviews.map((item) => <li key={item} className="whitespace-pre-wrap rounded bg-[#efeae2] px-2 py-1">{item}</li>)}
          </ul>
        )}
        <TextArea value={review} onChange={(e) => setReview(e.target.value)} placeholder="意见会单独保存，不能把未通过改成通过。" />
        <label className="mt-2 flex items-start gap-2 text-sm"><input type="checkbox" checked={blind} onChange={(e) => setBlind(e.target.checked)} /><span>盲评：不附带模型名称。同一条 rubric，上下文预算 8192 字节。证据不够就写不够。</span></label>
        <Button className="mt-2" disabled={!current || !review} onClick={() => void api.request('POST', `/api/v1/attempts/${current.ID}/reviews`, { kind: 'note', body: review, blind, rubric: '可读性、维护成本和潜在缺陷。证据不足就写不足，不要求打分。', context_budget_bytes: 8192 }).then(() => { setReview(''); return load() }).catch((err: Error) => setError(err.message))}>保存意见</Button>
      </Card>
      <Card title="人工重试">
        <Input placeholder="必须写明原因" value={reason} onChange={(e) => setReason(e.target.value)} />
        <Button className="mt-2" disabled={!trial || !reason.trim()} onClick={() => void api.request('POST', `/api/v1/trials/${trial!.ID}/attempts`, { reason: reason.trim() }).then(() => { setReason(''); return load() }).catch((err: Error) => setError(err.message))}>按这个原因再跑一次</Button>
        <p className="mt-2 text-sm text-[#6b6258]">新 Attempt 不会改写上一次的结论。</p>
        <Button tone="quiet" className="mt-2" disabled={!trial} onClick={() => void api.request<{ note: string }>('POST', `/api/v1/trials/${trial!.ID}/adopt`).then((data) => setNote(data.note)).catch((err: Error) => setError(err.message))}>只记录采纳这个补丁</Button>
      </Card>
    </div>
  )
}

function phaseLine(raw?: string) {
  if (!raw) return '阶段耗时还没有。'
  try {
    const doc = JSON.parse(raw) as { phases?: { agent_ms?: number; end_to_end_ms?: number; verify_ms?: number; queue_ms?: number } }
    const phases = doc.phases
    if (!phases) return '阶段耗时还没有。'
    return `排队 ${phases.queue_ms ?? '未知'} ms，Agent ${phases.agent_ms ?? '未知'} ms，验收 ${phases.verify_ms ?? '未知'} ms，端到端 ${phases.end_to_end_ms ?? '未知'} ms。`
  } catch {
    return '阶段耗时还没有。'
  }
}

function PatchView({ raw }: { raw: string }) {
  try {
    const doc = JSON.parse(raw) as { changes?: { path?: string; action?: string; size?: number; binary?: boolean }[] }
    if (!doc.changes || doc.changes.length === 0) {
      return <pre className="max-h-80 overflow-auto whitespace-pre-wrap text-xs">{raw}</pre>
    }
    return (
      <ul className="flex flex-col gap-2 text-xs">
        {doc.changes.map((change) => (
          <li key={change.path} className="rounded bg-[#efeae2] px-2 py-1">
            <p>{change.action} {change.path} · {change.size ?? 0} 字节{change.binary ? ' · 二进制' : ''}</p>
          </li>
        ))}
      </ul>
    )
  } catch {
    return <pre className="max-h-80 overflow-auto whitespace-pre-wrap text-xs">{raw}</pre>
  }
}

function eventDocuments(text: string) {
  const out: string[] = []
  let data: string[] = []
  const flush = () => {
    if (data.length === 0) return
    const payload = data.join('\n').trim()
    if (payload) out.push(payload)
    data = []
  }
  for (const line of text.split('\n')) {
    if (line === '') {
      flush()
      continue
    }
    if (line.startsWith('data:')) {
      data.push(line.slice(5).trim())
      continue
    }
    if (line.startsWith('id:') || line.startsWith('event:') || line.startsWith(':')) continue
    flush()
    const raw = line.trim()
    if (raw.startsWith('{')) out.push(raw)
  }
  flush()
  return out
}

function EventView({ text, showAll }: { text: string; showAll: boolean }) {
  const docs = eventDocuments(text)
  const visible = showAll ? docs : docs.slice(0, 80)
  const groups = new Map<string, string[]>()
  let phase = '未分阶段'
  for (const line of visible) {
    try {
      const doc = JSON.parse(line) as { type?: string; payload?: { phase?: string } }
      if (doc.type === 'PhaseChanged' && doc.payload?.phase) phase = doc.payload.phase
    } catch {
      phase = phase
    }
    const list = groups.get(phase) || []
    list.push(line)
    groups.set(phase, list)
  }
  return (
    <div className="flex max-h-80 flex-col gap-2 overflow-auto">
      {[...groups.entries()].map(([name, rows]) => (
        <details key={name} open>
          <summary className="text-sm">{name} · {rows.length} 行</summary>
          <pre className="whitespace-pre-wrap text-xs">{rows.join('\n')}</pre>
        </details>
      ))}
    </div>
  )
}
