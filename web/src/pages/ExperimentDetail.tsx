import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, cleanupLabel, executionLabel, label, verdictLabel } from '../api.ts'
import { Button, Notice } from '../components/ui.tsx'

type Trial = { ID: string; ExecutionState: string; Verdict: string; RepeatIndex: number; CurrentAttemptID: string; cleanup_state: string }
type Summary = { planned: number; terminal: number; incomplete: number; pass: number; fail: number; inconclusive: number; unverified: number; note: string }

export function ExperimentDetail() {
  const { id = '' } = useParams()
  const [trials, setTrials] = useState<Trial[]>([])
  const [summary, setSummary] = useState<Summary | null>(null)
  const [error, setError] = useState('')
  const [note, setNote] = useState('')

  function load() {
    return api.request<{ trials: Trial[] | null; summary: Summary }>('GET', `/api/v1/experiments/${id}`).then((data) => {
      setTrials(data.trials || [])
      setSummary(data.summary)
    })
  }
  useEffect(() => { load().catch((err: Error) => setError(err.message)) }, [id])

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">实验详情</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {summary ? <Notice>计划 {summary.planned}，已结束 {summary.terminal}，未完成 {summary.incomplete}。通过 {summary.pass}，未通过 {summary.fail}，证据不足 {summary.inconclusive}，未经验收 {summary.unverified}。{summary.note}</Notice> : null}
      {note ? <Notice>{note}</Notice> : null}
      <Button tone="quiet" onClick={() => void api.request<{ artifact_id: string }>('POST', `/api/v1/experiments/${id}/export`).then((data) => setNote(`已生成导出 ${data.artifact_id}。导出不含凭据、隐藏测试和参考解。`)).catch((err: Error) => setError(err.message))}>导出报告</Button>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[640px] border-collapse text-left text-sm">
          <thead><tr className="border-b"><th className="py-2">Trial</th><th>阶段</th><th>结论</th><th>清理</th><th>次数</th><th></th></tr></thead>
          <tbody>
            {trials.map((trial) => (
              <tr key={trial.ID} className="border-b border-[#1c1915]/10">
                <td className="py-2"><Link className="underline" to={`/trials/${trial.ID}`}>{trial.ID.slice(0, 18)}</Link></td>
                <td>{label(executionLabel, trial.ExecutionState)}</td>
                <td>{label(verdictLabel, trial.Verdict)}</td>
                <td>{label(cleanupLabel, trial.cleanup_state)}</td>
                <td>{trial.RepeatIndex}</td>
                <td><Button tone="danger" onClick={() => void api.request<{ note: string; cleanup_state: string }>('POST', `/api/v1/trials/${trial.ID}/cancel`).then((data) => { setNote(`${data.note} 清理：${label(cleanupLabel, data.cleanup_state)}`); return load() }).catch((err: Error) => setError(err.message))}>取消</Button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
