import { useQueryClient } from '@tanstack/react-query'
import { FolderGit2, FolderOpen, History, LibraryBig, Pencil, Plus, Rocket, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { api } from '../api.ts'
import { PageBody, PageHeader } from '../components/layout/PageHeader.tsx'
import { Alert, Badge, Button, Card, CardBody, CardHeader, ConfirmButton, CopyText, Dialog, DialogContent, DialogTrigger, Empty, Field, Input, Skeleton, Table, TBody, TD, TH, THead, TR, Textarea } from '../components/ui/index.ts'
import { cn } from '../lib/cn.ts'
import { shortId, timeAgo } from '../lib/format.ts'
import { errorMessage, useAction, useProjects, useTasks, useTaskVersions } from '../lib/queries.ts'
import type { Project, Task } from '../lib/types.ts'

function rootsOf(p?: Project): string[] {
  const spec = p?.source_spec as { allowed_roots?: unknown } | null | undefined
  return Array.isArray(spec?.allowed_roots) ? (spec!.allowed_roots as unknown[]).filter((x): x is string => typeof x === 'string') : []
}

export function Tasks() {
  const projects = useProjects()
  const [picked, setSelected] = useState('')
  const list = projects.data ?? []
  const selected = list.some((p) => p.id === picked) ? picked : list[0]?.id ?? ''
  const project = list.find((p) => p.id === selected)

  return (
    <>
      <PageHeader title="任务库" description="任务按项目归档。发布会把源目录复制成不可变快照；之后改源目录不会影响已发布版本。" actions={<ProjectDialog onCreated={setSelected} />} />
      <PageBody>
        {projects.error ? <Alert tone="danger" className="mb-4">{errorMessage(projects.error)}</Alert> : null}
        <div className="grid gap-6 lg:grid-cols-[260px_minmax(0,1fr)]">
          <Card className="self-start">
            <CardHeader title="项目" description={`${list.length} 个`} />
            {projects.isLoading ? <div className="space-y-2 p-3">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-10" />)}</div> : list.length === 0 ? (
              <Empty className="py-8" icon={<FolderGit2 />} title="还没有项目" description="先登记一个本地项目。" />
            ) : (
              <ul className="p-2">
                {list.map((p) => (
                  <li key={p.id}>
                    <button type="button" onClick={() => setSelected(p.id)} className={cn('flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-left transition-colors', p.id === selected ? 'bg-primary-soft text-foreground ring-1 ring-primary/25' : 'hover:bg-accent')}>
                      <FolderGit2 className={cn('size-4 shrink-0', p.id === selected ? 'text-primary' : 'text-muted-foreground')} />
                      <div className="min-w-0">
                        <p className="truncate text-[13px] font-medium">{p.name}</p>
                        <p className="text-[11px] text-muted-foreground">{rootsOf(p).length ? `${rootsOf(p).length} 个允许根` : '未设置允许根'}</p>
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </Card>
          {project ? <ProjectPanel project={project} /> : !projects.isLoading ? (
            <Card><Empty icon={<LibraryBig />} title="选择或登记一个项目" description="登记不会运行仓库里的脚本，也不会改你的源目录。" action={<ProjectDialog onCreated={setSelected} />} /></Card>
          ) : null}
        </div>
      </PageBody>
    </>
  )
}

function ProjectDialog({ onCreated }: { onCreated: (id: string) => void }) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const create = useAction(() => api.request<Project>('POST', '/api/v1/projects', { name: name.trim(), source_spec: { kind: 'local' } }), {
    success: '项目已登记',
    invalidate: [['projects']],
    onSuccess: (p) => { setOpen(false); setName(''); onCreated(p.id) },
  })
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button size="sm"><Plus />登记项目</Button></DialogTrigger>
      <DialogContent title="登记项目" description="登记不会运行仓库里的脚本，也不会改你的源目录。" footer={<><Button variant="outline" size="sm" onClick={() => setOpen(false)}>取消</Button><Button size="sm" disabled={!name.trim()} loading={create.isPending} onClick={() => create.mutate(undefined)}>登记</Button></>}>
        <Field label="项目名称"><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter' && name.trim()) create.mutate(undefined) }} /></Field>
      </DialogContent>
    </Dialog>
  )
}

function ProjectPanel({ project }: { project: Project }) {
  const tasks = useTasks(project.id)
  const roots = rootsOf(project)
  const [open, setOpen] = useState<string>('')
  return (
    <div className="min-w-0 space-y-4">
      <Card>
        <CardHeader title={project.name} icon={<FolderGit2 />} description={<CopyText value={project.id} />} actions={<RootsDialog project={project} roots={roots} />} />
        <CardBody>
          {roots.length === 0 ? (
            <Alert tone="warning">还没有允许根。任务目录必须位于允许根内才能发布；控制面数据目录不能作为任务来源。</Alert>
          ) : (
            <div className="flex flex-wrap gap-1.5">
              <span className="mr-1 flex items-center gap-1 text-xs text-muted-foreground"><ShieldCheck className="size-3.5" />允许根</span>
              {roots.map((r) => <Badge key={r} tone="neutral" className="font-mono">{r}</Badge>)}
            </div>
          )}
        </CardBody>
      </Card>
      <Card className="overflow-hidden">
        <CardHeader title="任务" description={`${tasks.data?.length ?? 0} 个草稿`} actions={<TaskDialog projectId={project.id} />} />
        {tasks.isLoading ? <div className="space-y-2 p-4">{[0, 1].map((i) => <Skeleton key={i} className="h-12" />)}</div> : (tasks.data ?? []).length === 0 ? (
          <Empty icon={<LibraryBig />} title="这个项目还没有任务" description="任务 = 交给 Agent 的提示词 + 一个源目录 + 独立验证器。" action={<TaskDialog projectId={project.id} />} />
        ) : (
          <Table>
            <THead><TR><TH>任务</TH><TH className="hidden md:table-cell">源目录</TH><TH className="hidden lg:table-cell">更新</TH><TH className="text-right">操作</TH></TR></THead>
            <TBody>
              {(tasks.data ?? []).map((t) => (
                <TaskRow key={t.id} task={t} projectId={project.id} expanded={open === t.id} onToggle={() => setOpen(open === t.id ? '' : t.id)} />
              ))}
            </TBody>
          </Table>
        )}
      </Card>
    </div>
  )
}

function TaskRow({ task, projectId, expanded, onToggle }: { task: Task; projectId: string; expanded: boolean; onToggle: () => void }) {
  const versions = useTaskVersions(expanded ? task.id : undefined)
  const qc = useQueryClient()
  const publish = useAction(() => api.request<{ version: number }>('POST', `/api/v1/tasks/${task.id}/publish`), {
    success: (v) => `已发布 v${v.version}，快照已冻结`,
    invalidate: [['task-versions', task.id], ['catalog']],
    onSuccess: () => { if (!expanded) onToggle() },
  })
  return (
    <>
      <TR>
        <TD>
          <p className="text-[13px] font-medium">{task.name}</p>
          <p className="mt-0.5 line-clamp-1 max-w-md text-xs text-muted-foreground">{task.draft?.prompt || '（无提示词）'}</p>
        </TD>
        <TD className="hidden max-w-64 truncate font-mono text-xs text-muted-foreground md:table-cell" title={task.draft?.source_dir}>{task.draft?.source_dir || '—'}</TD>
        <TD className="hidden whitespace-nowrap text-xs text-muted-foreground lg:table-cell">{timeAgo(task.updated_at)}</TD>
        <TD>
          <div className="flex justify-end gap-1">
            <Button variant="ghost" size="xs" onClick={onToggle}><History />版本</Button>
            <TaskDialog projectId={projectId} task={task} onSaved={() => void qc.invalidateQueries({ queryKey: ['tasks', projectId] })} />
            <ConfirmButton size="xs" variant="secondary" icon={<Rocket />} title={`发布「${task.name}」的新版本？`} description="会把源目录和验证器复制成不可变快照，并运行发布前检查。已发布的版本不能修改；已经跑过的计划不会被改写。" confirmLabel="发布" onConfirm={() => publish.mutateAsync(undefined)}>发布</ConfirmButton>
          </div>
        </TD>
      </TR>
      {expanded ? (
        <TR className="bg-muted/30 hover:bg-muted/30">
          <TD colSpan={4}>
            {versions.isLoading ? <Skeleton className="h-6" /> : (versions.data ?? []).length === 0 ? <p className="text-xs text-muted-foreground">还没有发布过版本。</p> : (
              <div className="flex flex-wrap gap-2">
                {(versions.data ?? []).map((v) => (
                  <div key={v.id} className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1.5 text-xs">
                    <Badge tone="primary" className="font-mono">v{v.version}</Badge>
                    <span className="font-mono text-muted-foreground" title={v.digest}>{shortId(v.digest, 10)}</span>
                    <span className="text-muted-foreground">{timeAgo(v.created_at)}</span>
                  </div>
                ))}
              </div>
            )}
          </TD>
        </TR>
      ) : null}
    </>
  )
}

function RootsDialog({ project, roots }: { project: Project; roots: string[] }) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState(roots.join('\n'))
  const list = text.split('\n').map((s) => s.trim()).filter(Boolean)
  const bad = list.filter((r) => !r.startsWith('/'))
  const save = useAction(() => api.request('POST', `/api/v1/projects/${project.id}/roots`, { roots: list }), {
    success: '允许根已更新',
    invalidate: [['projects']],
    onSuccess: () => setOpen(false),
  })
  return (
    <Dialog open={open} onOpenChange={(o) => { if (o) setText(roots.join('\n')); setOpen(o) }}>
      <DialogTrigger asChild><Button variant="outline" size="sm"><FolderOpen />允许根</Button></DialogTrigger>
      <DialogContent title="允许根" description="发布只会冻结这些目录下面的内容。每行一个绝对路径。" footer={<><Button variant="outline" size="sm" onClick={() => setOpen(false)}>取消</Button><Button size="sm" disabled={bad.length > 0} loading={save.isPending} onClick={() => save.mutate(undefined)}>保存</Button></>}>
        <Field label="绝对路径" error={bad.length ? `不是绝对路径：${bad.join(', ')}` : undefined} hint="控制面数据目录不能作为任务来源。">
          <Textarea rows={4} className="font-mono text-xs" placeholder="/home/me/code/my-repo" value={text} onChange={(e) => setText(e.target.value)} />
        </Field>
      </DialogContent>
    </Dialog>
  )
}

function TaskDialog({ projectId, task, onSaved }: { projectId: string; task?: Task; onSaved?: () => void }) {
  const [open, setOpen] = useState(false)
  const d = task?.draft ?? {}
  const [name, setName] = useState('')
  const [prompt, setPrompt] = useState('')
  const [source, setSource] = useState('')
  const [verifier, setVerifier] = useState('')
  const [base, setBase] = useState('')
  const [wall, setWall] = useState('')
  const reset = () => {
    setName(task?.name ?? '')
    setPrompt(d.prompt ?? '')
    setSource(d.source_dir ?? '')
    setVerifier(d.verifier_root ?? '')
    setBase(d.base_commit ?? '')
    setWall(d.limits?.agent_wall_seconds ? String(d.limits.agent_wall_seconds) : '')
  }
  const wallN = wall ? Math.floor(Number(wall)) : 0
  const draft = {
    name: name.trim(),
    prompt,
    source_dir: source.trim(),
    verifier_root: (verifier || source).trim(),
    base_commit: base.trim(),
    ...(wallN > 0 ? { limits: { agent_wall_seconds: wallN, prepare_wall_seconds: 0, verify_wall_seconds: 0, log_bytes: 0, artifact_bytes: 0, max_files: 0 } } : {}),
  }
  const save = useAction(() => (task
    ? api.request('PATCH', `/api/v1/tasks/${task.id}`, { name: draft.name, draft, row_version: task.row_version })
    : api.request('POST', `/api/v1/projects/${projectId}/tasks`, draft)), {
    success: task ? '草稿已更新，已发布版本未受影响' : '任务草稿已保存',
    invalidate: [['tasks', projectId]],
    onSuccess: () => { setOpen(false); onSaved?.() },
  })
  const valid = name.trim() && prompt.trim() && source.trim().startsWith('/') && (!wall || wallN > 0)
  return (
    <Dialog open={open} onOpenChange={(o) => { if (o) reset(); setOpen(o) }}>
      <DialogTrigger asChild>{task ? <Button variant="ghost" size="xs"><Pencil />编辑</Button> : <Button size="sm"><Plus />新建任务</Button>}</DialogTrigger>
      <DialogContent wide title={task ? `编辑「${task.name}」` : '新建任务'} description="草稿可以反复修改；只有发布出来的版本能用于实验。" footer={<><Button variant="outline" size="sm" onClick={() => setOpen(false)}>取消</Button><Button size="sm" disabled={!valid} loading={save.isPending} onClick={() => save.mutate(undefined)}>{task ? '保存草稿' : '创建草稿'}</Button></>}>
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="任务名称" className="md:col-span-2"><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Field>
          <Field label="提示词" className="md:col-span-2" hint="原样交给 Agent。"><Textarea rows={5} value={prompt} onChange={(e) => setPrompt(e.target.value)} /></Field>
          <Field label="任务目录" hint="绝对路径，须位于允许根内。"><Input className="font-mono text-xs" placeholder="/path/to/fixtures/tasks/orders-pagination" value={source} onChange={(e) => setSource(e.target.value)} /></Field>
          <Field label="验证器目录" optional hint="留空则与任务目录相同。"><Input className="font-mono text-xs" value={verifier} onChange={(e) => setVerifier(e.target.value)} /></Field>
          <Field label="基线提交" optional><Input className="font-mono text-xs" value={base} onChange={(e) => setBase(e.target.value)} /></Field>
          <Field label="Agent 墙钟上限（秒）" optional hint="留空使用系统默认值。"><Input type="number" min={1} value={wall} onChange={(e) => setWall(e.target.value)} /></Field>
        </div>
      </DialogContent>
    </Dialog>
  )
}
