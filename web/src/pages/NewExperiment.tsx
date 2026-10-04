import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api.ts'
import { Button, Card, Notice } from '../components/ui.tsx'

type Version = { id: string; label: string }

export function NewExperiment() {
  const navigate = useNavigate()
  const [step, setStep] = useState(1)
  const [tasks, setTasks] = useState<Version[]>([])
  const [profiles, setProfiles] = useState<Version[]>([])
  const [pickedTasks, setPickedTasks] = useState<string[]>([])
  const [pickedProfiles, setPickedProfiles] = useState<string[]>([])
  const [reps, setReps] = useState(1)
  const [preview, setPreview] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    void (async () => {
      const projects = await api.request<{ items: { ID: string }[] | null }>('GET', '/api/v1/projects')
      const taskVersions: Version[] = []
      for (const project of projects.items || []) {
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
    })().catch((err: Error) => setError(err.message))
  }, [])

  const formula = useMemo(() => {
    const total = pickedTasks.length * pickedProfiles.length * Math.max(reps, 1)
    return `${pickedTasks.length} 个任务 × ${pickedProfiles.length} 个配置 × ${reps} 次 = ${total} 个 Trial`
  }, [pickedTasks.length, pickedProfiles.length, reps])

  function toggle(list: string[], id: string, set: (v: string[]) => void) {
    set(list.includes(id) ? list.filter((item) => item !== id) : [...list, id])
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">创建实验 · 第 {step} 步</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {step === 1 ? (
        <Card title="选择已发布任务">
          {tasks.length === 0 ? <Notice>还没有已发布的任务版本。</Notice> : tasks.map((task) => (
            <label key={task.id} className="mb-2 flex gap-2 text-sm"><input type="checkbox" checked={pickedTasks.includes(task.id)} onChange={() => toggle(pickedTasks, task.id, setPickedTasks)} />{task.label}</label>
          ))}
          <Button disabled={pickedTasks.length === 0} onClick={() => setStep(2)}>下一步</Button>
        </Card>
      ) : null}
      {step === 2 ? (
        <Card title="选择已发布配置">
          {profiles.length === 0 ? <Notice>还没有已发布的配置。doctor 没通过的草稿不能选。</Notice> : profiles.map((profile) => (
            <label key={profile.id} className="mb-2 flex gap-2 text-sm"><input type="checkbox" checked={pickedProfiles.includes(profile.id)} onChange={() => toggle(pickedProfiles, profile.id, setPickedProfiles)} />{profile.label}</label>
          ))}
          <div className="mt-2 flex gap-2"><Button tone="quiet" onClick={() => setStep(1)}>上一步</Button><Button disabled={pickedProfiles.length === 0} onClick={() => setStep(3)}>下一步</Button></div>
        </Card>
      ) : null}
      {step === 3 ? (
        <Card title="环境、权限和费用">
          <p className="mb-3 text-sm">{formula}。这是计划规模，不是预计 API 请求数，也不是费用估算。</p>
          <ul className="mb-3 list-disc pl-5 text-sm">
            <li>执行器：native-trusted。本机进程执行，不提供容器级隔离。</li>
            <li>网络：unrestricted。restricted 还没有强制执行。</li>
            <li>全局并发默认 1。同一账号的清理未完成时不会再启动。</li>
            <li>费用观测：未知。未知不会被显示成 0。</li>
            <li>重复次数
              <input className="ml-2 w-16 rounded border px-2" type="number" min={1} value={reps} onChange={(e) => setReps(Number(e.target.value) || 1)} />
            </li>
          </ul>
          <div className="flex gap-2"><Button tone="quiet" onClick={() => setStep(2)}>上一步</Button><Button onClick={() => void api.request<{ formula: string; note: string }>('POST', '/api/v1/experiments/preview', { mode: 'agent_profile', task_version_ids: pickedTasks, profile_version_ids: pickedProfiles, repetitions: reps, protocol: 'single-pass-v1' }).then((data) => { setPreview(`${data.formula}。${data.note}`); setStep(4) })}>预览规模</Button></div>
        </Card>
      ) : null}
      {step === 4 ? (
        <Card title="确认后才冻结计划">
          <p className="mb-2 text-sm">{preview || formula}</p>
          <Notice tone="warn">提交会排队执行。假 CLI 不产生模型费用；真实适配器一旦发布并运行，就会消耗对应账号。</Notice>
          <div className="mt-3 flex gap-2">
            <Button tone="quiet" onClick={() => setStep(3)}>上一步</Button>
            <Button onClick={() => void api.request<{ id: string }>('POST', '/api/v1/experiments', { mode: 'agent_profile', task_version_ids: pickedTasks, profile_version_ids: pickedProfiles, repetitions: reps, protocol: 'single-pass-v1' }, { 'Idempotency-Key': `ui-${Date.now()}` }).then((data) => navigate(`/experiments/${data.id}`)).catch((err: Error) => setError(err.message))}>冻结并排队</Button>
          </div>
        </Card>
      ) : null}
    </div>
  )
}
