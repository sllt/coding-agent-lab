import { Ban, Box, Clock, FileDiff, GitPullRequestArrow, MessageSquareText, RotateCcw, ShieldCheck, ShieldOff, TerminalSquare } from 'lucide-react'
import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { api } from '../api.ts'
import { PageBody, PageHeader } from '../components/layout/PageHeader.tsx'
import { StateBadge, VerdictBadge } from '../components/status.tsx'
import { ChecksView } from '../components/trial/ChecksView.tsx'
import { EventStream } from '../components/trial/EventStream.tsx'
import { PatchView } from '../components/trial/PatchView.tsx'
import { Alert, Badge, Button, Card, CardBody, CardHeader, ConfirmButton, CopyText, Dialog, DialogContent, DialogTrigger, Empty, Field, KeyValue, Skeleton, Switch, Tabs, TabsContent, TabsList, TabsTrigger, Textarea, Tooltip } from '../components/ui/index.ts'
import { cn } from '../lib/cn.ts'
import { bytes, dateTime, duration, num, shortId, timeAgo } from '../lib/format.ts'
import { errorMessage, useAction, useAttemptArtifacts, useAttemptChecks, useAttemptReviews, useAttemptUsage, useExperiment, useTrial } from '../lib/queries.ts'
import { adapterLabel, cleanupLabel, execLabel, lookup, terminalStates } from '../lib/status.ts'
import { useEventLog } from '../lib/useEventLog.ts'
import type { Attempt, Runtime, Trial } from '../lib/types.ts'

export function TrialDetail() {
  const { id = '' } = useParams()
  const { data, isLoading, error } = useTrial(id)
  const [pick, setPick] = useState<{ trial: string; attempt: string }>({ trial: '', attempt: '' })
  const picked = pick.trial === id ? pick.attempt : ''
  const setPicked = (attempt: string) => setPick({ trial: id, attempt })
  const attempts = data?.attempts ?? []
  const current = attempts.find((a) => a.id === picked) ?? attempts[attempts.length - 1]
  const live = !!current && !terminalStates.has(current.state)
  const { log, loading: logLoading } = useEventLog(current?.id, live)
  const checks = useAttemptChecks(current?.id)
  const artifacts = useAttemptArtifacts(current?.id)
  const exp = useExperiment(data?.trial.experiment_id ?? '')

  if (isLoading) return <PageBody className="space-y-4"><Skeleton className="h-16" /><div className="grid gap-4 xl:grid-cols-[260px_1fr_320px]"><Skeleton className="h-96" /><Skeleton className="h-96" /><Skeleton className="h-96" /></div></PageBody>
  if (error || !data) return <PageBody><Alert tone="danger" title="无法读取 Trial">{errorMessage(error)}</Alert></PageBody>

  const trial = data.trial
  const task = exp.data?.labels?.tasks?.[trial.task_version_id]
  const prof = exp.data?.labels?.profiles?.[trial.profile_version_id]
  const expName = exp.data?.experiment.name || `实验 ${shortId(trial.experiment_id)}`
  const checkItems = checks.data?.items ?? []
  const arts = artifacts.data ?? []

  return (
    <>
      <PageHeader
        crumbs={[{ to: '/experiments', label: '实验' }, { to: `/experiments/${trial.experiment_id}`, label: expName }, { label: `Trial ${shortId(trial.id)}` }]}
        title={<span className="flex flex-wrap items-center gap-2.5">{task ? task.task_name : '运行详情'}<span className="font-normal text-muted-foreground">×</span>{prof ? prof.display_name || prof.name : shortId(trial.profile_version_id)}<StateBadge state={trial.execution_state} />{trial.execution_state === 'completed' ? <VerdictBadge verdict={trial.verdict} /> : null}</span>}
        meta={(
          <>
            <CopyText value={trial.id} />
            <span>第 {trial.repeat_index} 次重复</span>
            <span title={dateTime(trial.queued_at)}>入队 {timeAgo(trial.queued_at)}</span>
            {trial.cancel_requested && !terminalStates.has(trial.execution_state) ? <Badge tone="warning">已请求取消</Badge> : null}
          </>
        )}
        actions={<TrialActions trial={trial} />}
      />
      <PageBody className="max-w-[1600px]">
        <div className="grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)_330px]">
          {/* Left: attempts + identity */}
          <div className="space-y-4">
            <Card>
              <CardHeader title="Attempts" description={attempts.length > 1 ? '第一次的结论单独保留，进入首轮通过率' : undefined} />
              {attempts.length === 0 ? <Empty className="py-8" title="还没有 Attempt" description="调度器占位后才会创建。" /> : (
                <ul className="p-2">
                  {attempts.map((a) => (
                    <li key={a.id}>
                      <button type="button" onClick={() => setPicked(a.id)} className={cn('w-full rounded-md px-3 py-2 text-left transition-colors', a.id === current?.id ? 'bg-primary-soft ring-1 ring-primary/25' : 'hover:bg-accent')}>
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-[13px] font-medium">第 {a.number} 次{a.number === 1 ? <span className="ml-1 text-[11px] font-normal text-muted-foreground">首轮</span> : null}</span>
                          {a.state === 'completed' ? <VerdictBadge verdict={a.verdict} /> : <StateBadge state={a.state} />}
                        </div>
                        <p className="mt-1 text-[11px] text-muted-foreground">{timeAgo(a.created_at)} · {lookup(cleanupLabel, a.cleanup_state, '—')}</p>
                        {a.reason ? <p className="mt-1 line-clamp-2 text-[11px] text-muted-foreground">{a.reason}</p> : null}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
            <Card>
              <CardHeader title="冻结版本" />
              <CardBody>
                <KeyValue items={[
                  { label: '任务', value: task ? <span>{task.task_name} <span className="text-muted-foreground">v{task.version}</span></span> : <CopyText value={trial.task_version_id} display={shortId(trial.task_version_id, 12)} /> },
                  { label: '项目', value: task?.project_name ?? '—' },
                  { label: 'Agent', value: prof ? <span>{prof.display_name || prof.name} <span className="text-muted-foreground">v{prof.version}</span></span> : <CopyText value={trial.profile_version_id} display={shortId(trial.profile_version_id, 12)} /> },
                  { label: '适配器', value: prof ? lookup(adapterLabel, prof.adapter) : '—' },
                  { label: '模型', value: prof?.model || '—', mono: true },
                ]} />
              </CardBody>
            </Card>
          </div>

          {/* Center: evidence */}
          <Card className="flex min-h-[560px] flex-col overflow-hidden xl:h-[calc(100vh-220px)]">
            <Tabs defaultValue="events" className="flex min-h-0 flex-1 flex-col">
              <div className="border-b px-3 pt-1">
                <TabsList className="w-full border-b-0">
                  <TabsTrigger value="events"><TerminalSquare />事件{live ? <span className="ml-1 size-1.5 animate-pulse-soft rounded-full bg-info" /> : <span className="ml-1 text-muted-foreground tabular">{log.events.length}</span>}</TabsTrigger>
                  <TabsTrigger value="diff"><FileDiff />改动</TabsTrigger>
                  <TabsTrigger value="checks"><ShieldCheck />独立验收<span className="ml-1 text-muted-foreground tabular">{checkItems.length}</span></TabsTrigger>
                  <TabsTrigger value="artifacts"><Box />制品<span className="ml-1 text-muted-foreground tabular">{arts.length}</span></TabsTrigger>
                </TabsList>
              </div>
              <TabsContent value="events" className="mt-0 min-h-0 flex-1"><EventStream log={log} loading={logLoading} live={live} /></TabsContent>
              <TabsContent value="diff" className="mt-0 min-h-0 flex-1 overflow-auto scrollbar-thin"><PatchView artifacts={arts} /></TabsContent>
              <TabsContent value="checks" className="mt-0 min-h-0 flex-1 overflow-auto scrollbar-thin"><ChecksView items={checkItems} note={checks.data?.note} loading={checks.isLoading} /></TabsContent>
              <TabsContent value="artifacts" className="mt-0 min-h-0 flex-1 overflow-auto scrollbar-thin">
                {arts.length === 0 ? <Empty icon={<Box />} title="还没有制品" /> : (
                  <ul className="divide-y">
                    {arts.map((a) => (
                      <li key={a.id} className="flex items-center gap-3 px-4 py-2.5 text-[13px]">
                        <Badge tone="neutral" className="font-mono">{a.kind}</Badge>
                        <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{shortId(a.digest, 16)}</span>
                        <span className="text-xs text-muted-foreground tabular">{bytes(a.bytes)}</span>
                        <Badge tone={a.status === 'present' ? 'success' : a.status === 'tampered' ? 'danger' : 'warning'}>{a.status === 'present' ? '完整' : a.status === 'tampered' ? '摘要不符' : '缺失'}</Badge>
                        {a.status === 'present' ? <a className="text-xs font-medium text-primary hover:underline" href={`/api/v1/artifacts/${a.id}/download`}>下载</a> : null}
                      </li>
                    ))}
                  </ul>
                )}
              </TabsContent>
            </Tabs>
          </Card>

          {/* Right: verdict, timing, cost, runtime, reviews */}
          <div className="space-y-4">
            {current ? <VerdictCard attempt={current} checks={checkItems} /> : null}
            {current ? <TimingCard runtime={current.runtime} /> : null}
            {current ? <UsageCard attemptId={current.id} /> : null}
            {current ? <RuntimeCard runtime={current.runtime} /> : null}
            {current ? <Reviews attemptId={current.id} /> : null}
          </div>
        </div>
      </PageBody>
    </>
  )
}

function TrialActions({ trial }: { trial: Trial }) {
  const [reason, setReason] = useState('')
  const [open, setOpen] = useState(false)
  const inv = [['trial', trial.id], ['experiment', trial.experiment_id], ['overview']]
  const retry = useAction(() => api.request('POST', `/api/v1/trials/${trial.id}/attempts`, { reason: reason.trim() }), {
    success: '已记录重试原因，等待调度',
    invalidate: inv,
    onSuccess: () => { setReason(''); setOpen(false) },
  })
  const adopt = useAction(() => api.request<{ note: string }>('POST', `/api/v1/trials/${trial.id}/adopt`), { success: (d) => d.note })
  const cancel = useAction(() => api.request<{ note: string }>('POST', `/api/v1/trials/${trial.id}/cancel`), { success: (d) => d.note, invalidate: inv })
  const terminal = terminalStates.has(trial.execution_state)
  return (
    <>
      {!terminal ? (
        <ConfirmButton danger variant="danger-outline" icon={<Ban />} title="取消这个 Trial？" description="会记录取消意图。运行中的进程需要等清理完成才会停止；已产生的费用不会退回。" confirmLabel="取消 Trial" onConfirm={() => cancel.mutateAsync(undefined)} disabled={trial.cancel_requested}>
          {trial.cancel_requested ? '取消中' : '取消'}
        </ConfirmButton>
      ) : null}
      {terminal && trial.execution_state === 'completed' ? (
        <ConfirmButton icon={<GitPullRequestArrow />} title="记录采纳这个补丁？" description="只在审计里记录「采纳」，不会写入源目录，也不会自动合并。" confirmLabel="记录采纳" onConfirm={() => adopt.mutateAsync(undefined)}>采纳补丁</ConfirmButton>
      ) : null}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild><Button size="sm" disabled={!terminal}><RotateCcw />人工重试</Button></DialogTrigger>
        <DialogContent
          title="人工重试"
          description="新 Attempt 不会改写第一次的结论，也不会计入首轮通过率。重试会在审计里记作一次人工干预。"
          footer={<><Button variant="outline" size="sm" onClick={() => setOpen(false)}>取消</Button><Button size="sm" disabled={!reason.trim()} loading={retry.isPending} onClick={() => retry.mutate(undefined)}>按此原因重试</Button></>}
        >
          <Field label="重试原因" hint="必填。写清楚为什么要重跑，例如「网络抖动导致 npm install 失败」。">
            <Textarea autoFocus rows={3} value={reason} onChange={(e) => setReason(e.target.value)} />
          </Field>
        </DialogContent>
      </Dialog>
    </>
  )
}

function VerdictCard({ attempt, checks }: { attempt: Attempt; checks: string[] }) {
  const parsed = checks.map((c) => { try { return JSON.parse(c) as { Outcome?: string; Required?: boolean } } catch { return {} } })
  const req = parsed.filter((c) => c.Required)
  const reqPass = req.filter((c) => c.Outcome === 'pass').length
  const done = attempt.state === 'completed'
  const bg = !done ? 'from-info/10' : attempt.verdict === 'pass' ? 'from-success/12' : attempt.verdict === 'fail' ? 'from-danger/12' : 'from-warning/12'
  return (
    <Card className={cn('overflow-hidden bg-gradient-to-b to-card', bg)}>
      <CardBody className="space-y-3">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium text-muted-foreground">第 {attempt.number} 次 · 独立验收结论</p>
          <StateBadge state={attempt.state} />
        </div>
        {done ? <VerdictBadge verdict={attempt.verdict} className="h-7 px-3 text-sm" /> : <p className="text-sm text-muted-foreground">{lookup(execLabel, attempt.state)}，尚无结论</p>}
        {req.length ? <p className="text-xs text-muted-foreground">必需检查 <span className="font-medium text-foreground tabular">{reqPass}/{req.length}</span> 通过 · 共 {parsed.length} 项</p> : null}
        {attempt.reason ? <p className="rounded-md bg-muted/60 px-2.5 py-1.5 text-xs text-muted-foreground">{attempt.reason}</p> : null}
        <p className="text-[11px] leading-relaxed text-muted-foreground">Agent 自己说的 PASS 不会变成这里的结论；人工意见也不能改写它。</p>
      </CardBody>
    </Card>
  )
}

function TimingCard({ runtime }: { runtime: Runtime | null }) {
  const p = runtime?.phases
  const rows = [
    { k: '排队', v: p?.queue_ms, c: 'bg-muted-foreground/40' },
    { k: 'Agent', v: p?.agent_ms, c: 'bg-primary' },
    { k: '验收', v: p?.verify_ms, c: 'bg-success' },
  ]
  const total = rows.reduce((a, r) => a + (r.v ?? 0), 0)
  return (
    <Card>
      <CardHeader title="耗时" icon={<Clock />} actions={<span className="text-xs text-muted-foreground tabular">端到端 {duration(p?.end_to_end_ms)}</span>} />
      <CardBody className="space-y-3">
        {total > 0 ? (
          <div className="flex h-2 overflow-hidden rounded-full bg-muted">
            {rows.map((r) => (r.v ? <div key={r.k} className={r.c} style={{ width: `${(r.v / total) * 100}%` }} /> : null))}
          </div>
        ) : null}
        <div className="grid grid-cols-3 gap-2">
          {rows.map((r) => (
            <div key={r.k}>
              <p className="flex items-center gap-1 text-[11px] text-muted-foreground"><span className={`size-1.5 rounded-full ${r.c}`} />{r.k}</p>
              <p className="text-[13px] font-medium tabular">{duration(r.v)}</p>
            </div>
          ))}
        </div>
      </CardBody>
    </Card>
  )
}

function UsageCard({ attemptId }: { attemptId: string }) {
  const { data } = useAttemptUsage(attemptId)
  const cost = data?.cost_microusd
  return (
    <Card>
      <CardHeader title="用量" actions={data?.confidence ? <Badge tone={data.confidence === 'exact' ? 'success' : 'neutral'}>{data.confidence === 'unknown' ? '未知' : data.confidence}</Badge> : null} />
      <CardBody className="space-y-2">
        <div className="grid grid-cols-3 gap-2">
          <Metric label="输入" value={num(data?.input_tokens)} />
          <Metric label="缓存" value={num(data?.cached_input_tokens)} />
          <Metric label="输出" value={num(data?.output_tokens)} />
        </div>
        <div className="flex items-baseline justify-between border-t pt-2">
          <span className="text-xs text-muted-foreground">费用</span>
          <span className="text-[13px] font-medium tabular">{cost === null || cost === undefined ? '未知' : `$${(cost / 1e6).toFixed(4)}`}</span>
        </div>
        <p className="text-[11px] text-muted-foreground">{data?.note ?? '空值表示未知，不是零。'}{data?.billing_path ? ` · ${data.billing_path}` : ''}</p>
      </CardBody>
    </Card>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div><p className="text-[11px] text-muted-foreground">{label}</p><p className="text-[13px] font-medium tabular">{value}</p></div>
}

function RuntimeCard({ runtime }: { runtime: Runtime | null }) {
  if (!runtime) return null
  const sb = runtime.sandbox
  const sandboxed = sb?.mode === 'landlock'
  return (
    <Card>
      <CardHeader title="运行环境" icon={sandboxed ? <ShieldCheck className="text-success" /> : <ShieldOff className="text-warning" />} />
      <CardBody className="space-y-3">
        <KeyValue items={[
          { label: '执行器', value: runtime.executor ?? '—', mono: true },
          { label: '隔离', value: runtime.isolation ?? '—', mono: true },
          { label: '网络', value: runtime.network ?? '—', mono: true },
          { label: 'HOME', value: runtime.home_inherited ? <Badge tone="warning">继承真实 HOME</Badge> : <span className="text-xs">本次运行的空目录</span> },
          ...(runtime.credential_env?.length ? [{ label: '凭据变量', value: <span className="font-mono text-xs">{runtime.credential_env.join(', ')}</span> }] : []),
          ...(runtime.budget?.wall_seconds ? [{ label: '墙钟上限', value: `${runtime.budget.wall_seconds}s${runtime.budget.wall_source ? ` · ${runtime.budget.wall_source}` : ''}` }] : []),
          ...(runtime.requested_model ? [{ label: '模型', value: <span className="font-mono text-xs">{runtime.requested_model}{runtime.resolved_model && runtime.resolved_model !== runtime.requested_model ? ` → ${runtime.resolved_model}` : ''}</span> }] : []),
        ]} />
        {runtime.model_resolution_mismatch ? <Alert tone="warning">实际模型与请求不一致，这个 Trial 在对比中标记为不可比。</Alert> : null}
        {sb ? (
          <div className="rounded-lg border p-3">
            <div className="flex items-center justify-between">
              <p className="text-xs font-medium">文件系统沙箱</p>
              <Badge tone={sandboxed ? 'success' : sb.mode === 'disabled' ? 'warning' : 'neutral'}>{sandboxed ? `Landlock ABI ${sb.abi ?? '?'}` : sb.mode}</Badge>
            </div>
            {sb.reason ? <p className="mt-1.5 text-[11px] text-muted-foreground">{sb.reason}</p> : null}
            {sb.read_write?.length ? <PathList label="可写" paths={sb.read_write} /> : null}
            {sb.exposed?.length ? <PathList label="系统目录中可见的数据" paths={sb.exposed} warn /> : null}
            {sb.limits ? <p className="mt-2 text-[11px] text-muted-foreground">{sb.limits}</p> : null}
          </div>
        ) : null}
        {runtime.limitation ? <p className="text-[11px] leading-relaxed text-muted-foreground">{runtime.limitation}</p> : null}
      </CardBody>
    </Card>
  )
}

function PathList({ label, paths, warn }: { label: string; paths: string[]; warn?: boolean }) {
  return (
    <div className="mt-2">
      <p className={cn('text-[11px]', warn ? 'text-warning' : 'text-muted-foreground')}>{label}</p>
      <ul className="mt-0.5 space-y-0.5">{paths.map((p) => <li key={p} className="truncate font-mono text-[11px]" title={p}>{p}</li>)}</ul>
    </div>
  )
}

function Reviews({ attemptId }: { attemptId: string }) {
  const { data } = useAttemptReviews(attemptId)
  const [body, setBody] = useState('')
  const [blind, setBlind] = useState(true)
  const save = useAction(() => api.request('POST', `/api/v1/attempts/${attemptId}/reviews`, { kind: 'note', body: body.trim(), blind, context_budget_bytes: 8192 }), {
    success: '意见已保存',
    invalidate: [['reviews', attemptId]],
    onSuccess: () => setBody(''),
  })
  const items = (data?.items ?? []).map((raw) => { try { return JSON.parse(raw) as { body?: string; blind?: boolean } } catch { return { body: raw } } })
  return (
    <Card>
      <CardHeader title="人工意见" icon={<MessageSquareText />} description={data?.note} />
      <CardBody className="space-y-3">
        {items.length ? (
          <ul className="space-y-2">
            {items.map((r, i) => (
              <li key={i} className="rounded-md bg-muted/60 px-3 py-2 text-[13px]">
                <p className="whitespace-pre-wrap">{r.body}</p>
                {r.blind ? <p className="mt-1 text-[11px] text-muted-foreground">盲评 · 未附带模型名称</p> : null}
              </li>
            ))}
          </ul>
        ) : null}
        <Textarea rows={3} placeholder="可读性、维护成本、潜在缺陷…证据不足就写不足。意见不能把未通过改成通过。" value={body} onChange={(e) => setBody(e.target.value)} />
        <div className="flex items-center justify-between">
          <Tooltip content="不附带模型名称；上下文预算 8192 字节">
            <label className="flex items-center gap-2 text-xs text-muted-foreground"><Switch checked={blind} onCheckedChange={setBlind} />盲评</label>
          </Tooltip>
          <Button size="sm" disabled={!body.trim()} loading={save.isPending} onClick={() => save.mutate(undefined)}>保存意见</Button>
        </div>
      </CardBody>
    </Card>
  )
}

