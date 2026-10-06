import { useEffect, useMemo, useRef, useState } from 'react'
import { stableSubmitKey } from '../submitKey.ts'
import { useNavigate } from 'react-router-dom'
import { api } from '../api.ts'
import { Button, Card, Notice } from '../components/ui.tsx'

type Version = { id: string; label: string }

const maxReps = 30

export function NewExperiment() {
  const navigate = useNavigate()
  const [step, setStep] = useState(1)
  const [loading, setLoading] = useState(true)
  const [tasks, setTasks] = useState<Version[]>([])
  const [profiles, setProfiles] = useState<Version[]>([])
  const [pickedTasks, setPickedTasks] = useState<string[]>([])
  const [pickedProfiles, setPickedProfiles] = useState<string[]>([])
  const [reps, setReps] = useState(1)
  const [preview, setPreview] = useState('')
  const [mode, setMode] = useState('agent_profile')
  const [protocol, setProtocol] = useState('single-pass-v1')
  const [error, setError] = useState('')
  const [loadFailed, setLoadFailed] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const keyRef = useRef({ payload: '', key: '' })

  useEffect(() => {
    void (async () => {
      const projects = await api.request<{ items: { ID: string }[] | null; next_cursor?: string }>('GET', '/api/v1/projects')
      let cursor = projects.next_cursor || ''
      const projectItems = [...(projects.items || [])]
      while (cursor) {
        const page = await api.request<{ items: { ID: string }[] | null; next_cursor?: string }>('GET', `/api/v1/projects?cursor=${encodeURIComponent(cursor)}`)
        projectItems.push(...(page.items || []))
        cursor = page.next_cursor || ''
      }
      const taskVersions: Version[] = []
      for (const project of projectItems) {
        const tasks = await api.request<{ items: { ID: string; Name: string }[] | null }>('GET', `/api/v1/projects/${project.ID}/tasks`)
        for (const task of tasks.items || []) {
          const versions = await api.request<{ items: { ID: string; Version: number }[] | null }>('GET', `/api/v1/tasks/${task.ID}/versions`)
          for (const version of versions.items || []) taskVersions.push({ id: version.ID, label: `${task.Name} v${version.Version}` })
        }
      }
      const profileList = await api.request<{ items: { ID: string; Name: string }[] | null }>('GET', '/api/v1/profiles')
      const profileVersions: Version[] = []
      for (const profile of profileList.items || []) {
        const versions = await api.request<{ items: { ID: string; Version: number }[] | null }>('GET', `/api/v1/profiles/${profile.ID}/versions`)
        for (const version of versions.items || []) profileVersions.push({ id: version.ID, label: `${profile.Name} v${version.Version}` })
      }
      setTasks(taskVersions)
      setProfiles(profileVersions)
      setLoadFailed(false)
    })().catch((err: Error) => { setError(err.message); setLoadFailed(true) }).finally(() => setLoading(false))
  }, [])

  const formula = useMemo(() => {
    const total = pickedTasks.length * pickedProfiles.length * Math.max(reps, 1)
    return `${pickedTasks.length} 个任务 × ${pickedProfiles.length} 个配置 × ${reps} 次 = ${total} 个 Trial`
  }, [pickedTasks.length, pickedProfiles.length, reps])

  const payload = useMemo(() => ({
    mode,
    task_version_ids: [...pickedTasks].sort(),
    profile_version_ids: [...pickedProfiles].sort(),
    repetitions: reps,
    protocol,
  }), [mode, pickedTasks, pickedProfiles, reps, protocol])

  function toggle(list: string[], id: string, set: (v: string[]) => void) {
    set(list.includes(id) ? list.filter((item) => item !== id) : [...list, id])
  }

  function setRepetitions(value: number) {
    if (!Number.isFinite(value) || value < 1) {
      setReps(1)
      return
    }
    setReps(Math.min(maxReps, Math.floor(value)))
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">创建实验 · 第 {step} 步</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {loading ? <Notice>正在读取已发布的任务和配置。</Notice> : null}
      {step === 1 ? (
        <Card title="选择已发布任务">
          {!loading && loadFailed ? null : !loading && tasks.length === 0 ? <Notice>还没有已发布的任务版本。</Notice> : tasks.map((task) => (
            <label key={task.id} className="mb-2 flex gap-2 text-sm"><input type="checkbox" checked={pickedTasks.includes(task.id)} onChange={() => toggle(pickedTasks, task.id, setPickedTasks)} />{task.label}</label>
          ))}
          <Button disabled={pickedTasks.length === 0} onClick={() => setStep(2)}>下一步</Button>
        </Card>
      ) : null}
      {step === 2 ? (
        <Card title="选择已发布配置">
          {!loading && loadFailed ? null : !loading && profiles.length === 0 ? <Notice>还没有已发布的配置。doctor 没通过的草稿不能选。</Notice> : profiles.map((profile) => (
            <label key={profile.id} className="mb-2 flex gap-2 text-sm"><input type="checkbox" checked={pickedProfiles.includes(profile.id)} onChange={() => toggle(pickedProfiles, profile.id, setPickedProfiles)} />{profile.label}</label>
          ))}
          <div className="mt-2 flex gap-2"><Button tone="quiet" onClick={() => setStep(1)}>上一步</Button><Button disabled={pickedProfiles.length === 0} onClick={() => setStep(3)}>下一步</Button></div>
        </Card>
      ) : null}
      {step === 3 ? (
        <Card title="环境、权限和费用">
          <p className="mb-3 text-sm">{formula}。这是计划规模，不是预计 API 请求数，也不是费用估算。</p>
          <ul className="mb-3 list-disc pl-5 text-sm">
            <li>执行器必须在配置里明确选择 native-trusted。Docker 还没有经过验证，发布时会被拒绝，不会改成本机执行。</li>
            <li>网络必须是 unrestricted。restricted 和 offline 不能发布。</li>
            <li>全局并发可在设置里改成 1 到 8。大于 1 时不同账号会重叠运行。同一账号的清理未完成时不会再启动。</li>
            <li>
              <label className="mr-3">模式
                <select className="ml-2 rounded border px-2 py-1" value={mode} onChange={(e) => setMode(e.target.value)}>
                  <option value="agent_profile">配置对比</option>
                  <option value="controlled_model">同一工具只换模型</option>
                  <option value="workflow">工作流对比</option>
                </select>
              </label>
              <label>协议
                <select className="ml-2 rounded border px-2 py-1" value={protocol} onChange={(e) => setProtocol(e.target.value)}>
                  <option value="single-pass-v1">single-pass-v1</option>
                  <option value="repair-once-v1">repair-once-v1 · 失败后再修一次</option>
                </select>
              </label>
            </li>
            {mode === 'controlled_model' ? <li>受控模型模式要求同一个适配器、至少两个不同的模型 ID。服务端若发现实际模型 ID 和请求不一致，会标成不可比。</li> : null}
            <li>费用观测：未知。未知不会被显示成 0。</li>
            <li>重复次数（1 到 {maxReps}）
              <input className="ml-2 w-16 rounded border px-2" type="number" min={1} max={maxReps} value={reps} onChange={(e) => setRepetitions(Number(e.target.value))} />
            </li>
          </ul>
          <div className="flex gap-2"><Button tone="quiet" onClick={() => setStep(2)}>上一步</Button><Button onClick={() => void api.request<{ formula: string; note: string }>('POST', '/api/v1/experiments/preview', payload).then((data) => { setPreview(`${data.formula}。${data.note}`); setStep(4) }).catch((err: Error) => setError(err.message))}>预览规模</Button></div>
        </Card>
      ) : null}
      {step === 4 ? (
        <Card title="确认后才冻结计划">
          <p className="mb-2 text-sm">{preview || formula}</p>
          <Notice tone="warn">提交会排队执行。假 CLI 不产生模型费用；真实适配器一旦发布并运行，就会消耗对应账号。同一份计划重复提交会回到原计划，不会再插一份。</Notice>
          <div className="mt-3 flex gap-2">
            <Button tone="quiet" onClick={() => setStep(3)}>上一步</Button>
            <Button disabled={submitting} onClick={() => {
              if (submitting) return
              const key = stableSubmitKey(keyRef.current, JSON.stringify(payload))
              keyRef.current = { payload: JSON.stringify(payload), key }
              setSubmitting(true)
              void api.request<{ id: string }>('POST', '/api/v1/experiments', payload, { 'Idempotency-Key': key }).then((data) => {
                keyRef.current = { payload: '', key: '' }
                navigate(`/experiments/${data.id}`)
              }).catch((err: Error) => { setError(err.message); setSubmitting(false) })
            }}>冻结并排队</Button>
          </div>
        </Card>
      ) : null}
    </div>
  )
}

