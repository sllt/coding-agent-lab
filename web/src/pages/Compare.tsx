import { useEffect, useState } from 'react'
import { api } from '../api.ts'
import { Card, Notice } from '../components/ui.tsx'

type Cell = { display_name: string; pass: number; fail: number; inconclusive: number; unverified: number; incomplete: number }
type Board = { task_version_id: string; comparable: boolean; reasons: string[]; cells: Cell[]; exploratory: boolean; note: string; paired: number }

export function Compare() {
  const [experiments, setExperiments] = useState<{ ID: string }[]>([])
  const [boards, setBoards] = useState<Board[]>([])
  const [error, setError] = useState('')
  useEffect(() => {
    api.request<{ items: { ID: string }[] | null }>('GET', '/api/v1/experiments').then((data) => setExperiments(data.items || [])).catch((err: Error) => setError(err.message))
  }, [])

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">对比</h1>
      <Notice>只在执行器、网络和任务版本一致时放在同一张表。这里没有名次，也没有 0 到 100 的分数。</Notice>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {experiments.length === 0 ? <Notice>没有可对比的实验。</Notice> : (
        <ul className="flex flex-col gap-2 text-sm">
          {experiments.map((item) => (
            <li key={item.ID}><button className="underline" onClick={() => void api.request<{ boards: Board[] | null }>('GET', `/api/v1/comparisons?experiment_id=${item.ID}`).then((data) => setBoards(data.boards || [])).catch((err: Error) => setError(err.message))}>{item.ID}</button></li>
          ))}
        </ul>
      )}
      {boards.map((board) => (
        <Card key={board.task_version_id + board.comparable} title={board.comparable ? '可比计数' : '不能合榜'}>
          <p className="mb-2 text-sm">{board.note}</p>
          {!board.comparable ? <p className="mb-2 text-sm">原因：{board.reasons.join('、') || '条件不一致'}</p> : null}
          <p className="mb-2 text-sm">配对完成：{board.paired}。探索性：{board.exploratory ? '是' : '否'}。</p>
          <ul className="text-sm">
            {board.cells.map((cell) => (
              <li key={cell.display_name}>{cell.display_name || '未命名'}：通过 {cell.pass}，未通过 {cell.fail}，证据不足 {cell.inconclusive}，未经验收 {cell.unverified}，未完成 {cell.incomplete}</li>
            ))}
          </ul>
        </Card>
      ))}
    </div>
  )
}
