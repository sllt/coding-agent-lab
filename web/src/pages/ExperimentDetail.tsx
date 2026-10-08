import { useQueryClient } from '@tanstack/react-query'
import { Ban, Download, FileJson, GitCompareArrows, Grid3x3, List, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '../api.ts'
import { PageBody, PageHeader } from '../components/layout/PageHeader.tsx'
import { StateBadge, TrialSquare, VerdictBadge, VerdictBar } from '../components/status.tsx'
import { Alert, Badge, Button, Card, ConfirmButton, CopyText, Empty, Segmented, Skeleton, Table, TBody, TD, TH, THead, TR, Tabs, TabsContent, TabsList, TabsTrigger, Tooltip } from '../components/ui/index.ts'
import { bytes, dateTime, percent, protocolName, shortId, timeAgo } from '../lib/format.ts'
import { errorMessage, useAction, useExperiment } from '../lib/queries.ts'
import { adapterLabel, cleanupLabel, execLabel, lookup, modeLabel, terminalStates, verdictLabel } from '../lib/status.ts'
import type { ExperimentDetail as Detail, Trial } from '../lib/types.ts'

export function ExperimentDetail() {
  const { id = '' } = useParams()
  const { data, isLoading, error, refetch, isFetching } = useExperiment(id)
  const qc = useQueryClient()

  const exportNow = useAction(() => api.request<{ artifact_id: string; bytes: number; download_path: string }>('POST', `/api/v1/experiments/${id}/export`), {
    success: (d) => `已生成导出（${bytes(d.bytes)}）`,
    invalidate: [['experiment', id]],
  })

  if (isLoading) return <PageBody className="space-y-4"><Skeleton className="h-20" /><Skeleton className="h-24" /><Skeleton className="h-80" /></PageBody>
  if (error || !data) return <PageBody><Alert tone="danger" title="无法读取实验">{errorMessage(error)}</Alert></PageBody>

  const { experiment: exp, summary } = data
  const trials = data.trials ?? []
  const live = trials.filter((t) => !terminalStates.has(t.execution_state))
  const plan = exp.plan ?? {}

  async function cancelAll() {
    const results = await Promise.allSettled(live.map((t) => api.request('POST', `/api/v1/trials/${t.id}/cancel`)))
    const failed = results.filter((r) => r.status === 'rejected').length
    if (failed) toast.error(`${failed} 个 Trial 取消请求失败`)
    else toast.success(`已为 ${live.length} 个 Trial 记录取消意图`, { description: '这不表示进程已经停止；清理完成后状态才会变为已取消。' })
    void qc.invalidateQueries({ queryKey: ['experiment', id] })
    void qc.invalidateQueries({ queryKey: ['overview'] })
  }

  return (
    <>
      <PageHeader
        crumbs={[{ to: '/experiments', label: '实验' }, { label: exp.name || shortId(exp.id) }]}
        title={<span className="flex flex-wrap items-center gap-2.5">{exp.name || `实验 ${shortId(exp.id)}`}<StateBadge state={exp.state} /></span>}
        description={exp.description || undefined}
        meta={(
          <>
            <CopyText value={exp.id} />
            <span>{lookup(modeLabel, exp.mode)}</span>
            <span className="font-mono">{protocolName(plan.protocol ?? exp.protocol)}</span>
            <span title={dateTime(exp.created_at)}>创建于 {timeAgo(exp.created_at)}</span>
            <Tooltip content="计划摘要：冻结时的任务、配置、协议与重复次数"><span className="font-mono">plan {shortId(exp.plan_digest, 10)}</span></Tooltip>
          </>
        )}
        actions={(
          <>
            <Button variant="ghost" size="sm" onClick={() => void refetch()} aria-label="刷新"><RefreshCw className={isFetching ? 'animate-spin' : ''} /></Button>
            <Link to={`/compare?ids=${exp.id}`} className="inline-flex h-8 items-center gap-1.5 rounded-md border bg-card px-3 text-[13px] font-medium shadow-xs hover:bg-accent"><GitCompareArrows className="size-4" />对比</Link>
            <Button variant="outline" size="sm" loading={exportNow.isPending} onClick={() => exportNow.mutate(undefined)}><FileJson />导出 JSON</Button>
            {live.length > 0 ? (
              <ConfirmButton danger variant="danger-outline" icon={<Ban />} title={`取消 ${live.length} 个未结束的 Trial？`} description="会为每个排队或运行中的 Trial 记录取消意图。运行中的进程需要等清理完成才会停止，已产生的费用不会退回。" confirmLabel="全部取消" onConfirm={cancelAll}>
                取消未结束
              </ConfirmButton>
            ) : null}
          </>
        )}
      />
      <PageBody className="space-y-6">
        <SummaryStrip data={data} />
        {summary.note ? <p className="-mt-3 text-xs text-muted-foreground">{summary.note}</p> : null}

        <Tabs defaultValue="matrix">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <TabsList>
              <TabsTrigger value="matrix"><Grid3x3 />矩阵</TabsTrigger>
              <TabsTrigger value="list"><List />Trial 列表 <span className="ml-1 text-muted-foreground tabular">{trials.length}</span></TabsTrigger>
              <TabsTrigger value="exports"><Download />导出 <span className="ml-1 text-muted-foreground tabular">{data.exports?.length ?? 0}</span></TabsTrigger>
            </TabsList>
            <Legend />
          </div>
          <TabsContent value="matrix"><Matrix data={data} /></TabsContent>
          <TabsContent value="list"><TrialList data={data} /></TabsContent>
          <TabsContent value="exports">
            <Card>
              {(data.exports ?? []).length === 0 ? <Empty icon={<FileJson />} title="还没有导出" description="导出不含凭据、隐藏测试和参考解。每次导出是一份独立文件。" action={<Button size="sm" variant="outline" loading={exportNow.isPending} onClick={() => exportNow.mutate(undefined)}>生成导出</Button>} /> : (
                <Table>
                  <THead><TR><TH>导出</TH><TH>大小</TH><TH>状态</TH><TH className="text-right">下载</TH></TR></THead>
                  <TBody>
                    {(data.exports ?? []).map((e) => (
                      <TR key={e.id}>
                        <TD className="font-mono text-xs">{e.id}</TD>
                        <TD className="tabular">{bytes(e.bytes)}</TD>
                        <TD><Badge tone={e.status === 'present' ? 'success' : 'neutral'}>{e.status === 'present' ? '可下载' : e.status}</Badge></TD>
                        <TD className="text-right"><a className="inline-flex items-center gap-1 text-[13px] font-medium text-primary hover:underline" href={e.download_path}><Download className="size-3.5" />下载</a></TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              )}
            </Card>
          </TabsContent>
        </Tabs>
      </PageBody>
    </>
  )
}

function SummaryStrip({ data }: { data: Detail }) {
  const s = data.summary
  const planned = s.planned || data.experiment.trial_count
  const items = [
    { label: '计划', value: planned, cls: '' },
    { label: '已结束', value: s.terminal, cls: '', sub: percent(s.terminal, planned) },
    { label: '通过', value: s.pass, cls: 'text-success', sub: s.terminal ? percent(s.pass, s.terminal) : undefined },
    { label: '未通过', value: s.fail, cls: 'text-danger' },
    { label: '证据不足', value: s.inconclusive, cls: 'text-warning' },
    { label: '未验收', value: s.unverified, cls: 'text-muted-foreground' },
  ]
  return (
    <Card className="p-4">
      <div className="grid grid-cols-3 gap-4 md:grid-cols-6">
        {items.map((i) => (
          <div key={i.label}>
            <p className="text-xs text-muted-foreground">{i.label}</p>
            <p className={`mt-1 text-xl font-semibold tabular ${i.cls}`}>{i.value}{i.sub ? <span className="ml-1.5 text-xs font-normal text-muted-foreground">{i.sub}</span> : null}</p>
          </div>
        ))}
      </div>
      <VerdictBar className="mt-4 h-2" counts={{ pass: s.pass, fail: s.fail, inconclusive: s.inconclusive, unverified: s.unverified }} total={planned} />
      {(s.assisted_pass || s.repair_pass) ? (
        <p className="mt-2 text-xs text-muted-foreground">其中人工协助通过 {s.assisted_pass}{s.repair_pass ? ` · 修复后通过 ${s.repair_pass}` : ''}（单独计数，不并入首轮通过率）</p>
      ) : null}
    </Card>
  )
}

function Legend() {
  const items = [['bg-success', '通过'], ['bg-danger', '未通过'], ['bg-warning', '证据不足'], ['bg-info/70', '进行中'], ['bg-muted-foreground/20', '排队'], ['bg-muted-foreground/35', '取消/中止']]
  return (
    <div className="flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground">
      {items.map(([c, l]) => <span key={l} className="flex items-center gap-1"><span className={`size-2.5 rounded-[3px] ${c}`} />{l}</span>)}
    </div>
  )
}

function Matrix({ data }: { data: Detail }) {
  const navigate = useNavigate()
  const trials = useMemo(() => data.trials ?? [], [data.trials])
  const plan = data.experiment.plan ?? {}
  const taskIds = plan.task_version_ids?.length ? plan.task_version_ids : [...new Set(trials.map((t) => t.task_version_id))]
  const profileIds = plan.profile_version_ids?.length ? plan.profile_version_ids : [...new Set(trials.map((t) => t.profile_version_id))]
  const cell = useMemo(() => {
    const m = new Map<string, Trial[]>()
    for (const t of trials) {
      const k = `${t.task_version_id}|${t.profile_version_id}`
      m.set(k, [...(m.get(k) ?? []), t].sort((a, b) => a.repeat_index - b.repeat_index))
    }
    return m
  }, [trials])
  const tl = data.labels?.tasks ?? {}
  const pl = data.labels?.profiles ?? {}
  if (trials.length === 0) return <Card><Empty title="还没有 Trial" /></Card>

  const colStats = (pid: string) => {
    const ts = trials.filter((t) => t.profile_version_id === pid)
    const done = ts.filter((t) => t.execution_state === 'completed')
    return { pass: done.filter((t) => t.verdict === 'pass').length, done: done.length, total: ts.length }
  }

  return (
    <Card className="overflow-hidden">
      <div className="overflow-x-auto scrollbar-thin">
        <table className="w-full min-w-[560px] border-collapse text-[13px]">
          <thead>
            <tr className="border-b bg-muted/40">
              <th className="sticky left-0 z-10 w-64 bg-muted/40 px-4 py-3 text-left text-xs font-medium text-muted-foreground backdrop-blur">任务 \ Agent</th>
              {profileIds.map((pid) => {
                const p = pl[pid]
                const st = colStats(pid)
                return (
                  <th key={pid} className="min-w-44 border-l px-4 py-3 text-left align-top font-normal">
                    <p className="truncate font-medium">{p ? p.display_name || p.name : shortId(pid)}</p>
                    <p className="mt-0.5 truncate text-[11px] text-muted-foreground">{p ? `${lookup(adapterLabel, p.adapter)}${p.model ? ` · ${p.model}` : ''} · v${p.version}` : '已删除的版本'}</p>
                    <p className="mt-1.5 text-xs tabular"><span className="font-semibold text-success">{st.pass}</span><span className="text-muted-foreground">/{st.done} 通过</span></p>
                  </th>
                )
              })}
            </tr>
          </thead>
          <tbody>
            {taskIds.map((tid) => {
              const t = tl[tid]
              return (
                <tr key={tid} className="border-b last:border-0">
                  <td className="sticky left-0 z-10 bg-card px-4 py-3 align-top">
                    <p className="truncate font-medium">{t ? t.task_name : shortId(tid)}</p>
                    <p className="mt-0.5 truncate text-[11px] text-muted-foreground">{t ? `${t.project_name} · v${t.version}` : ''}</p>
                  </td>
                  {profileIds.map((pid) => {
                    const ts = cell.get(`${tid}|${pid}`) ?? []
                    const pass = ts.filter((x) => x.execution_state === 'completed' && x.verdict === 'pass').length
                    const done = ts.filter((x) => terminalStates.has(x.execution_state)).length
                    return (
                      <td key={pid} className="border-l px-4 py-3 align-top">
                        <div className="flex flex-wrap gap-1">
                          {ts.map((x) => (
                            <button key={x.id} type="button" className="rounded-[3px] outline-offset-2 focus-visible:outline-2 focus-visible:outline-ring" onClick={() => navigate(`/trials/${x.id}`)} aria-label={`打开 Trial ${x.repeat_index}`}>
                              <TrialSquare state={x.execution_state} verdict={x.verdict} title={`#${x.repeat_index} · ${lookup(execLabel, x.execution_state)}${x.execution_state === 'completed' ? ` · ${lookup(verdictLabel, x.verdict)}` : ''}`} />
                            </button>
                          ))}
                        </div>
                        <p className="mt-1.5 text-[11px] text-muted-foreground tabular">{done ? `${pass}/${done} 通过` : '未开始'}{done < ts.length && done ? ` · 剩 ${ts.length - done}` : ''}</p>
                      </td>
                    )
                  })}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

type ListFilter = 'all' | 'live' | 'pass' | 'fail' | 'other'

function TrialList({ data }: { data: Detail }) {
  const navigate = useNavigate()
  const [f, setF] = useState<ListFilter>('all')
  const trials = data.trials ?? []
  const tl = data.labels?.tasks ?? {}
  const pl = data.labels?.profiles ?? {}
  const match = (t: Trial) => matchWith(f, t)
  const count = (k: ListFilter) => trials.filter((t) => matchWith(k, t)).length
  function matchWith(k: ListFilter, t: Trial) {
    const done = t.execution_state === 'completed'
    switch (k) {
      case 'live': return !terminalStates.has(t.execution_state)
      case 'pass': return done && t.verdict === 'pass'
      case 'fail': return done && t.verdict === 'fail'
      case 'other': return terminalStates.has(t.execution_state) && !(done && (t.verdict === 'pass' || t.verdict === 'fail'))
      default: return true
    }
  }
  const shown = trials.filter(match)
  return (
    <div className="space-y-3">
      <Segmented<ListFilter> value={f} onChange={setF} options={[
        { value: 'all', label: '全部', count: trials.length },
        { value: 'live', label: '未结束', count: count('live') },
        { value: 'pass', label: '通过', count: count('pass') },
        { value: 'fail', label: '未通过', count: count('fail') },
        { value: 'other', label: '其他', count: count('other') },
      ]} />
      <Card className="overflow-hidden">
        {shown.length === 0 ? <Empty title="没有符合条件的 Trial" /> : (
          <Table>
            <THead><TR><TH>Trial</TH><TH>任务</TH><TH>Agent</TH><TH>状态</TH><TH>结论</TH><TH className="hidden lg:table-cell">尝试</TH><TH className="hidden lg:table-cell">清理</TH></TR></THead>
            <TBody>
              {shown.map((t) => {
                const task = tl[t.task_version_id]
                const prof = pl[t.profile_version_id]
                return (
                  <TR key={t.id} className="cursor-pointer hover:bg-accent/50" onClick={() => navigate(`/trials/${t.id}`)}>
                    <TD><span className="font-mono text-xs">{shortId(t.id)}</span><span className="ml-1.5 text-xs text-muted-foreground">#{t.repeat_index}</span></TD>
                    <TD className="max-w-48 truncate">{task ? `${task.task_name} v${task.version}` : shortId(t.task_version_id)}</TD>
                    <TD className="max-w-48 truncate">{prof ? prof.display_name || prof.name : shortId(t.profile_version_id)}</TD>
                    <TD><StateBadge state={t.execution_state} />{t.cancel_requested && !terminalStates.has(t.execution_state) ? <Badge tone="warning" className="ml-1">已请求取消</Badge> : null}</TD>
                    <TD>{t.execution_state === 'completed' ? <VerdictBadge verdict={t.verdict} /> : <span className="text-xs text-muted-foreground">—</span>}</TD>
                    <TD className="hidden tabular lg:table-cell">{t.attempt_count ?? 0}</TD>
                    <TD className="hidden text-xs text-muted-foreground lg:table-cell">{lookup(cleanupLabel, t.cleanup_state, '—')}</TD>
                  </TR>
                )
              })}
            </TBody>
          </Table>
        )}
      </Card>
    </div>
  )
}
