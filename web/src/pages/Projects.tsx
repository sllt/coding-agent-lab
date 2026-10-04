import { useEffect, useState } from 'react'
import { api } from '../api.ts'
import { Button, Card, Input, Notice, TextArea } from '../components/ui.tsx'

type Project = { ID: string; Name: string }
type Task = { ID: string; Name: string; RowVersion: number; DraftJSON: string }
type Version = { ID: string; Version: number; Digest: string }

export function Projects() {
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState('')
  const [selected, setSelected] = useState('')
  const [tasks, setTasks] = useState<Task[]>([])
  const [taskName, setTaskName] = useState('')
  const [prompt, setPrompt] = useState('')
  const [source, setSource] = useState('')
  const [root, setRoot] = useState('')
  const [versions, setVersions] = useState<Version[]>([])
  const [editing, setEditing] = useState('')
  const [rowVersion, setRowVersion] = useState(0)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  async function load() {
    const data = await api.request<{ items: Project[] | null }>('GET', '/api/v1/projects')
    setProjects(data.items || [])
  }
  useEffect(() => {
    load().catch((err: Error) => setError(err.message)).finally(() => setLoading(false))
  }, [])

  async function open(id: string) {
    setSelected(id)
    setVersions([])
    const data = await api.request<{ items: Task[] | null }>('GET', `/api/v1/projects/${id}/tasks`)
    setTasks(data.items || [])
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">项目与任务</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {message ? <Notice>{message}</Notice> : null}
      <Card title="登记项目">
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input placeholder="项目名称" value={name} onChange={(e) => setName(e.target.value)} />
          <Button onClick={() => void api.request('POST', '/api/v1/projects', { name, source_spec: { kind: 'local' } }).then(() => { setName(''); return load() }).catch((err: Error) => setError(err.message))}>登记</Button>
        </div>
        <p className="mt-2 text-sm text-[#6b6258]">登记不会运行仓库里的脚本，也不会改你的源目录。</p>
      </Card>
      {loading ? <Notice>正在读取项目。</Notice> : error ? null : projects.length === 0 ? <Notice>还没有项目。</Notice> : (
        <ul className="flex flex-col gap-2">
          {projects.map((project) => (
            <li key={project.ID}>
              <button className="underline" onClick={() => void open(project.ID).catch((err: Error) => setError(err.message))}>{project.Name}</button>
            </li>
          ))}
        </ul>
      )}
      {selected ? (
        <Card title="任务草稿">
          <div className="mb-3 flex flex-col gap-2">
            <Input placeholder="允许根目录的绝对路径" value={root} onChange={(e) => setRoot(e.target.value)} />
            <Button tone="quiet" disabled={!root.trim()} onClick={() => void api.request('POST', `/api/v1/projects/${selected}/roots`, { roots: [root.trim()] }).then(() => setMessage('已登记允许根。发布只会冻结这个根下面的目录。')).catch((err: Error) => setError(err.message))}>登记允许根</Button>
            <p className="text-sm text-[#6b6258]">控制面数据目录不能作为任务来源。发布时会把当时的文件复制进不可变快照，之后改源目录不会改变已发布版本。</p>
          </div>
          <div className="flex flex-col gap-2">
            <Input placeholder="任务名称" value={taskName} onChange={(e) => setTaskName(e.target.value)} />
            <TextArea placeholder="交给 Agent 的说明" value={prompt} onChange={(e) => setPrompt(e.target.value)} />
            <Input placeholder="任务目录，须在允许根内。示范：fixtures/tasks/orders-pagination 的绝对路径" value={source} onChange={(e) => setSource(e.target.value)} />
            <Button onClick={() => {
              const draft = JSON.stringify({ name: taskName, prompt, source_dir: source, verifier_root: source })
              const done = () => { setEditing(''); setMessage(editing ? '已更新草稿。已发布的版本没有被改写。' : '已保存草稿。'); return open(selected) }
              const req = editing
                ? api.request('PATCH', `/api/v1/tasks/${editing}`, { name: taskName, draft_json: draft, row_version: rowVersion })
                : api.request('POST', `/api/v1/projects/${selected}/tasks`, { name: taskName, prompt, source_dir: source, verifier_root: source })
              void req.then(done).catch((err: Error) => setError(err.message))
            }}>{editing ? '更新草稿' : '保存草稿'}</Button>
          </div>
          {tasks.length === 0 ? <p className="mt-3 text-sm">这个项目还没有任务。</p> : (
            <ul className="mt-3 flex flex-col gap-2 text-sm">
              {tasks.map((task) => (
                <li key={task.ID} className="flex flex-wrap items-center gap-2">
                  <span>{task.Name}</span>
                  <Button tone="quiet" onClick={() => {
                    setEditing(task.ID)
                    setRowVersion(task.RowVersion)
                    setTaskName(task.Name)
                    try {
                      const draft = JSON.parse(task.DraftJSON) as { prompt?: string; source_dir?: string }
                      setPrompt(draft.prompt || '')
                      setSource(draft.source_dir || '')
                    } catch {
                      setPrompt('')
                    }
                  }}>编辑草稿</Button>
                  <Button tone="quiet" onClick={() => void api.request('POST', `/api/v1/tasks/${task.ID}/publish`).then(() => api.request<{ items: Version[] }>("GET", `/api/v1/tasks/${task.ID}/versions`)).then((data) => { setVersions(data.items || []); setMessage('已发布新版本。快照已冻结，已经跑过的计划不会被改写。') }).catch((err: Error) => setError(err.message))}>发布版本</Button>
                </li>
              ))}
            </ul>
          )}
          {versions.length > 0 ? <p className="mt-3 text-sm">已发布 {versions.map((v) => `v${v.Version}`).join('、')}。发布后的版本不能再改。</p> : null}
        </Card>
      ) : null}
    </div>
  )
}
