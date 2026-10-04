import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api.ts'
import { Card, Notice } from '../components/ui.tsx'

type Evidence = { trial_id: string; attempt_id: string; kind: string }
type Cell = { display_name: string; pass: number; fail: number; inconclusive: number; unverified: number; incomplete: number; assisted_pass: number; assisted_fail: number; repair_pass: number; agent_millis: number; end_to_end_millis: number; interventions: number; evidence?: Evidence[] }
type Board = { task_version_id: string; comparable: boolean; reasons: string[]; cells: Cell[]; exploratory: boolean; note: string; paired: number; executor: string; network: string }
type Stats = { method?: string; seed?: number; iterations?: number; note?: string; exploratory?: boolean; macro_average?: Record<string, number>; paired?: Record<string, number> }

export function Compare() {
  const [experiments, setExperiments] = useState<{ ID: string }[]>([])
  const [loading, setLoading] = useState(true)
  const [boards, setBoards] = useState<Board[]>([])
  const [stats, setStats] = useState<Stats | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    api.request<{ items: { ID: string }[] | null }>('GET', '/api/v1/experiments')
      .then((data) => setExperiments(data.items || []))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">对比</h1>
      <Notice>只在执行器、网络和任务版本一致时放在同一张表。通过数来自第一次物理执行。人工重试单独计，不会进入首 Attempt 通过率。这里没有名次，也没有 0 到 100 的分数。</Notice>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {loading ? <Notice>正在读取可对比的实验。</Notice> : error ? null : experiments.length === 0 ? <Notice>没有可对比的实验。</Notice> : (
        <ul className="flex flex-col gap-2 text-sm">
          {experiments.map((item) => (
            <li key={item.ID}><button className="underline" onClick={() => void api.request<{ boards: Board[] | null; statistics?: Stats }>('GET', `/api/v1/comparisons?experiment_id=${item.ID}`).then((data) => { setBoards(data.boards || []); setStats(data.statistics || null) }).catch((err: Error) => setError(err.message))}>{item.ID}</button></li>
          ))}
        </ul>
      )}
      {stats ? <Notice>{stats.note} 方法 {stats.method}，种子 {stats.seed}，迭代 {stats.iterations}。配对：都通过 {stats.paired?.both_pass ?? 0}，只 A {stats.paired?.only_a ?? 0}，只 B {stats.paired?.only_b ?? 0}，都失败 {stats.paired?.both_fail ?? 0}，未判定 {stats.paired?.unresolved ?? 0}。</Notice> : null}
      {boards.map((board) => (
        <Card key={board.task_version_id + board.comparable} title={board.comparable ? '可比计数' : '不能合榜'}>
          <p className="mb-2 text-sm">{board.note}</p>
          {!board.comparable ? <p className="mb-2 text-sm">原因：{board.reasons.join('、') || '条件不一致'}</p> : null}
          <p className="mb-2 text-sm">执行器 {board.executor || '未知'} · 网络 {board.network || '未知'}。配对完成：{board.paired}。探索性：{board.exploratory ? '是' : '否'}。</p>
          <ul className="text-sm">
            {board.cells.map((cell) => (
              <li key={cell.display_name}>{cell.display_name || '未命名'}：通过 <Count n={cell.pass} kind="pass" evidence={cell.evidence} />，未通过 <Count n={cell.fail} kind="fail" evidence={cell.evidence} />，证据不足 <Count n={cell.inconclusive} kind="inconclusive" evidence={cell.evidence} />，未经验收 <Count n={cell.unverified} kind="unverified" evidence={cell.evidence} />，未完成 <Count n={cell.incomplete} kind="incomplete" evidence={cell.evidence} />。人工重试通过 {cell.assisted_pass}，人工重试未通过 {cell.assisted_fail}。修复协议通过 {cell.repair_pass || 0}。Agent {cell.agent_millis || 0} ms，端到端 {cell.end_to_end_millis || 0} ms，人工介入 {cell.interventions || 0}。</li>
            ))}
          </ul>
        </Card>
      ))}
    </div>
  )
}

function Count({ n, kind, evidence }: { n: number; kind: string; evidence?: Evidence[] }) {
  const hit = (evidence || []).find((item) => item.kind === kind)
  if (!hit) return <>{n}</>
  return <Link className="underline" to={`/trials/${hit.trial_id}`}>{n}</Link>
}
