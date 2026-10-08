import { Bot, Check, FlaskConical, History, Info, LibraryBig, Minus, Plus, Search } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api.ts'
import { PageBody, PageHeader } from '../components/layout/PageHeader.tsx'
import { ReadinessBadge } from '../components/status.tsx'
import { Alert, Badge, Button, Card, CardBody, CardHeader, Empty, Field, Input, Segmented, Skeleton, Switch, Textarea } from '../components/ui/index.ts'
import { cn } from '../lib/cn.ts'
import { shortId, timeAgo } from '../lib/format.ts'
import { errorMessage, useAction, useCatalog } from '../lib/queries.ts'
import { adapterLabel, lookup } from '../lib/status.ts'
import type { ProfileVersionInfo, TaskVersionInfo } from '../lib/types.ts'
import { stableSubmitKey } from '../submitKey.ts'

const maxReps = 30
type Mode = 'agent_profile' | 'controlled_model' | 'workflow'
type Protocol = 'single-pass-v1' | 'repair-once-v1'

const modes: { value: Mode; label: string; hint: string }[] = [
  { value: 'agent_profile', label: '配置对比', hint: '不同 Agent 配置跑同一批任务，最常用。' },
  { value: 'controlled_model', label: '同工具换模型', hint: '同一个适配器，至少两个不同模型 ID。实际模型与请求不符会标记为不可比。' },
  { value: 'workflow', label: '工作流对比', hint: '比较工作流差异，结论只作探索参考。' },
]

/** Keep only the newest version of each task/profile unless history is requested. */
function latestOnly<T extends { version: number }>(items: T[], key: (t: T) => string) {
  const best = new Map<string, T>()
  for (const it of items) {
    const k = key(it)
    const cur = best.get(k)
    if (!cur || it.version > cur.version) best.set(k, it)
  }
  return items.filter((it) => best.get(key(it)) === it)
}

export function NewExperiment() {
  const catalog = useCatalog()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [mode, setMode] = useState<Mode>('agent_profile')
  const [protocol, setProtocol] = useState<Protocol>('single-pass-v1')
  const [reps, setReps] = useState(3)
  const [tasks, setTasks] = useState<string[]>([])
  const [profiles, setProfiles] = useState<string[]>([])
  const [history, setHistory] = useState(false)
  const [taskQ, setTaskQ] = useState('')
  const keyRef = useRef({ payload: '', key: '' })

  const allTasks = useMemo(() => catalog.data?.tasks ?? [], [catalog.data])
  const allProfiles = useMemo(() => catalog.data?.profiles ?? [], [catalog.data])
  const taskList = useMemo(() => {
    const list = history ? allTasks : latestOnly(allTasks, (t) => t.task_id)
    const needle = taskQ.trim().toLowerCase()
    return needle ? list.filter((t) => `${t.task_name} ${t.project_name}`.toLowerCase().includes(needle)) : list
  }, [allTasks, history, taskQ])
  const profileList = useMemo(() => (history ? allProfiles : latestOnly(allProfiles, (p) => p.profile_id)), [allProfiles, history])

  const pickedProfiles = allProfiles.filter((p) => profiles.includes(p.version_id))
  const pickedTasks = allTasks.filter((t) => tasks.includes(t.version_id))
  const total = tasks.length * profiles.length * reps
  const issues: string[] = []
  if (tasks.length === 0) issues.push('至少选择一个任务版本')
  if (profiles.length === 0) issues.push('至少选择一个 Agent 配置')
  if (mode === 'controlled_model' && profiles.length > 0) {
    const adapters = new Set(pickedProfiles.map((p) => p.adapter))
    const models = new Set(pickedProfiles.map((p) => p.model))
    if (adapters.size > 1) issues.push('同工具换模型：所选配置必须使用同一个适配器')
    if (models.size < 2) issues.push('同工具换模型：至少需要两个不同的模型 ID')
  }
  if (name.length > 120) issues.push('名称最多 120 个字符')
  const notReady = pickedProfiles.filter((p) => p.readiness !== 'verified' && p.readiness !== 'ready_unverified')

  const payload = useMemo(() => ({
    name: name.trim(),
    description: description.trim(),
    mode,
    task_version_ids: [...tasks].sort(),
    profile_version_ids: [...profiles].sort(),
    repetitions: reps,
    protocol,
  }), [name, description, mode, tasks, profiles, reps, protocol])

  const submit = useAction(async () => {
    const raw = JSON.stringify(payload)
    const key = stableSubmitKey(keyRef.current, raw)
    keyRef.current = { payload: raw, key }
    return api.request<{ id: string; trial_count: number }>('POST', '/api/v1/experiments', payload, { 'Idempotency-Key': key })
  }, {
    success: (d) => `已排队 ${d.trial_count} 个 Trial`,
    invalidate: [['experiments'], ['overview']],
    onSuccess: (d) => { keyRef.current = { payload: '', key: '' }; navigate(`/experiments/${d.id}`) },
  })

  const toggle = (list: string[], set: (v: string[]) => void, id: string) => set(list.includes(id) ? list.filter((x) => x !== id) : [...list, id])

  return (
    <>
      <PageHeader title="新建实验" crumbs={[{ to: '/experiments', label: '实验' }, { label: '新建' }]} description="在一个页面里选好任务、Agent 和协议。提交即冻结计划；同一计划重复提交会回到原实验，不会重复排队。" />
      <PageBody>
        {catalog.error ? <Alert tone="danger" className="mb-4">{errorMessage(catalog.error)}</Alert> : null}
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
          <div className="space-y-6">
            <Card>
              <CardHeader title="基本信息" icon={<FlaskConical />} />
              <CardBody className="grid gap-4 md:grid-cols-2">
                <Field label="实验名称" htmlFor="exp-name" optional hint="方便在列表里辨认；不影响计划摘要。">
                  <Input id="exp-name" maxLength={120} placeholder="例如：Cursor vs OpenCode · 修复类任务" value={name} onChange={(e) => setName(e.target.value)} />
                </Field>
                <Field label="模式">
                  <Segmented<Mode> value={mode} onChange={setMode} options={modes.map((m) => ({ value: m.value, label: m.label }))} />
                  <p className="text-xs text-muted-foreground">{modes.find((m) => m.value === mode)?.hint}</p>
                </Field>
                <Field label="说明" htmlFor="exp-desc" optional className="md:col-span-2">
                  <Textarea id="exp-desc" maxLength={2000} rows={2} placeholder="想验证什么？" value={description} onChange={(e) => setDescription(e.target.value)} />
                </Field>
              </CardBody>
            </Card>

            <Card>
              <CardHeader
                title="任务"
                icon={<LibraryBig />}
                description={`已选 ${tasks.length} 个 · 只列出已发布的冻结版本`}
                actions={(
                  <div className="flex items-center gap-3">
                    <label className="flex items-center gap-2 text-xs text-muted-foreground"><Switch checked={history} onCheckedChange={setHistory} aria-label="显示历史版本" /><History className="size-3.5" />历史版本</label>
                  </div>
                )}
              />
              <CardBody className="space-y-3">
                {allTasks.length > 6 ? (
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                    <Input className="pl-8" placeholder="筛选任务" value={taskQ} onChange={(e) => setTaskQ(e.target.value)} />
                  </div>
                ) : null}
                {catalog.isLoading ? <PickerSkeleton /> : taskList.length === 0 ? (
                  <Empty className="py-8" icon={<LibraryBig />} title="没有已发布的任务" description="先在任务库里创建任务并发布一个版本。" action={<Link to="/tasks" className="text-sm font-medium text-primary hover:underline">前往任务库</Link>} />
                ) : (
                  <div className="grid gap-2 md:grid-cols-2">
                    {taskList.map((t) => <TaskOption key={t.version_id} t={t} selected={tasks.includes(t.version_id)} onToggle={() => toggle(tasks, setTasks, t.version_id)} />)}
                  </div>
                )}
              </CardBody>
            </Card>

            <Card>
              <CardHeader title="Agent 配置" icon={<Bot />} description={`已选 ${profiles.length} 个 · 徽标是最近一次 doctor 的结论`} />
              <CardBody>
                {catalog.isLoading ? <PickerSkeleton /> : profileList.length === 0 ? (
                  <Empty className="py-8" icon={<Bot />} title="没有已发布的配置" description="doctor 未通过的草稿不能发布，也不能选。" action={<Link to="/agents" className="text-sm font-medium text-primary hover:underline">前往 Agents</Link>} />
                ) : (
                  <div className="grid gap-2 md:grid-cols-2">
                    {profileList.map((p) => <ProfileOption key={p.version_id} p={p} selected={profiles.includes(p.version_id)} onToggle={() => toggle(profiles, setProfiles, p.version_id)} />)}
                  </div>
                )}
              </CardBody>
            </Card>

            <Card>
              <CardHeader title="协议与重复" />
              <CardBody className="grid gap-5 md:grid-cols-2">
                <Field label="协议">
                  <div className="grid gap-2">
                    {([['single-pass-v1', '单次通过', 'Agent 跑一次，独立验收给结论。'], ['repair-once-v1', '允许修复一次', '首轮失败后把验收反馈交回 Agent 再修一次，修复成功单独计数。']] as const).map(([v, l, h]) => (
                      <button key={v} type="button" onClick={() => setProtocol(v)} className={cn('rounded-lg border p-3 text-left transition-colors', protocol === v ? 'border-primary bg-primary-soft ring-1 ring-primary/30' : 'hover:bg-accent/60')}>
                        <p className="flex items-center gap-2 text-[13px] font-medium">{l}<code className="font-mono text-[11px] text-muted-foreground">{v}</code></p>
                        <p className="mt-0.5 text-xs text-muted-foreground">{h}</p>
                      </button>
                    ))}
                  </div>
                </Field>
                <Field label="每组重复次数" hint={`1–${maxReps}。重复越多，置信区间越窄；跑 3 次以上才看得出稳定性。`}>
                  <div className="flex items-center gap-2">
                    <Button variant="outline" size="icon" aria-label="减少" disabled={reps <= 1} onClick={() => setReps((r) => Math.max(1, r - 1))}><Minus /></Button>
                    <Input type="number" className="w-20 text-center tabular" min={1} max={maxReps} value={reps} onChange={(e) => { const v = Math.floor(Number(e.target.value)); setReps(Number.isFinite(v) ? Math.min(maxReps, Math.max(1, v)) : 1) }} aria-label="重复次数" />
                    <Button variant="outline" size="icon" aria-label="增加" disabled={reps >= maxReps} onClick={() => setReps((r) => Math.min(maxReps, r + 1))}><Plus /></Button>
                  </div>
                </Field>
              </CardBody>
            </Card>
          </div>

          <aside className="lg:sticky lg:top-6 lg:self-start">
            <Card>
              <CardHeader title="计划摘要" />
              <CardBody className="space-y-4">
                <div className="rounded-lg bg-muted/60 p-4 text-center">
                  <p className="text-xs text-muted-foreground tabular">{tasks.length} 任务 × {profiles.length} 配置 × {reps} 次</p>
                  <p className="mt-1 text-3xl font-semibold tracking-tight tabular">{total}</p>
                  <p className="text-xs text-muted-foreground">个 Trial</p>
                </div>
                <SummaryList title="任务" items={pickedTasks.map((t) => `${t.task_name} v${t.version}`)} />
                <SummaryList title="Agent" items={pickedProfiles.map((p) => `${p.display_name || p.name} v${p.version}`)} />
                {notReady.length > 0 ? <Alert tone="warning">{notReady.map((p) => p.display_name || p.name).join('、')} 最近一次 doctor 未判定为可运行，Trial 可能在准备阶段失败。</Alert> : null}
                {issues.length > 0 ? (
                  <ul className="space-y-1 text-xs text-muted-foreground">{issues.map((i) => <li key={i} className="flex gap-1.5"><Info className="mt-0.5 size-3 shrink-0" />{i}</li>)}</ul>
                ) : null}
                <div className="space-y-1.5 border-t pt-3 text-xs text-muted-foreground">
                  <p>这是计划规模，不是 API 请求数或费用估算。</p>
                  <p>本地假 CLI 不产生费用；真实适配器会消耗对应账号额度。</p>
                </div>
                <Button className="w-full" disabled={issues.length > 0} loading={submit.isPending} onClick={() => submit.mutate(undefined)}>
                  <Check />冻结计划并排队
                </Button>
              </CardBody>
            </Card>
          </aside>
        </div>
      </PageBody>
    </>
  )
}

function PickerSkeleton() {
  return <div className="grid gap-2 md:grid-cols-2">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-16 rounded-lg" />)}</div>
}

function Option({ selected, onToggle, children }: { selected: boolean; onToggle: () => void; children: React.ReactNode }) {
  return (
    <button type="button" role="checkbox" aria-checked={selected} onClick={onToggle} className={cn('group flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors', selected ? 'border-primary bg-primary-soft/60 ring-1 ring-primary/30' : 'hover:border-foreground/20 hover:bg-accent/40')}>
      <span className={cn('mt-0.5 flex size-4 shrink-0 items-center justify-center rounded border transition-colors', selected ? 'border-primary bg-primary text-primary-foreground' : 'border-input bg-card')}>
        {selected ? <Check className="size-3" strokeWidth={3} /> : null}
      </span>
      <div className="min-w-0 flex-1">{children}</div>
    </button>
  )
}

function TaskOption({ t, selected, onToggle }: { t: TaskVersionInfo; selected: boolean; onToggle: () => void }) {
  return (
    <Option selected={selected} onToggle={onToggle}>
      <div className="flex items-center justify-between gap-2">
        <p className="truncate text-[13px] font-medium">{t.task_name}</p>
        <Badge tone="outline" className="font-mono">v{t.version}</Badge>
      </div>
      <p className="mt-0.5 truncate text-xs text-muted-foreground">{t.project_name} · <span className="font-mono">{shortId(t.digest, 10)}</span> · {timeAgo(t.created_at)}</p>
    </Option>
  )
}

function ProfileOption({ p, selected, onToggle }: { p: ProfileVersionInfo; selected: boolean; onToggle: () => void }) {
  return (
    <Option selected={selected} onToggle={onToggle}>
      <div className="flex items-center justify-between gap-2">
        <p className="truncate text-[13px] font-medium">{p.display_name || p.name}</p>
        <Badge tone="outline" className="font-mono">v{p.version}</Badge>
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
        <Badge tone="neutral">{lookup(adapterLabel, p.adapter)}</Badge>
        {p.model ? <Badge tone="neutral" className="font-mono">{p.model}</Badge> : null}
        <ReadinessBadge readiness={p.readiness || null} />
      </div>
    </Option>
  )
}

function SummaryList({ title, items }: { title: string; items: string[] }) {
  return (
    <div>
      <p className="mb-1 text-xs font-medium text-muted-foreground">{title}</p>
      {items.length === 0 ? <p className="text-xs text-muted-foreground/70">未选择</p> : (
        <div className="flex flex-wrap gap-1">{items.map((i) => <Badge key={i} tone="primary">{i}</Badge>)}</div>
      )}
    </div>
  )
}
