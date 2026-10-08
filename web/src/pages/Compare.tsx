import { AlertTriangle, BarChart3, Check, GitCompareArrows, Scale } from 'lucide-react'
import { Link, useSearchParams } from 'react-router-dom'
import { PageBody, PageHeader } from '../components/layout/PageHeader.tsx'
import { StateBadge } from '../components/status.tsx'
import { Alert, Badge, Card, CardBody, CardHeader, Empty, Skeleton, Table, TBody, TD, TH, THead, TR, Tooltip } from '../components/ui/index.ts'
import { cn } from '../lib/cn.ts'
import { duration, shortId } from '../lib/format.ts'
import { errorMessage, useComparison, useExperiment, useExperiments } from '../lib/queries.ts'
import type { Board, BoardCell, Evidence, ExperimentDetail } from '../lib/types.ts'

export function Compare() {
  const experiments = useExperiments()
  const [params, setParams] = useSearchParams()
  const ids = (params.get('ids') ?? '').split(',').filter(Boolean)
  const toggle = (id: string) => {
    const next = ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id]
    setParams(next.length ? { ids: next.join(',') } : {}, { replace: true })
  }
  const list = experiments.data ?? []
  return (
    <>
      <PageHeader title="对比" description="只有执行器、网络和任务版本都一致时才放进同一张表。通过数来自第一次物理执行；人工重试与修复单独计数。没有名次，也没有 0–100 的分数。" />
      <PageBody className="space-y-6">
        <Card>
          <CardHeader title="选择实验" description="可多选；每个实验单独成组展示。" />
          <CardBody>
            {experiments.isLoading ? <Skeleton className="h-8" /> : list.length === 0 ? <p className="text-[13px] text-muted-foreground">还没有实验。</p> : (
              <div className="flex flex-wrap gap-2">
                {list.map((e) => {
                  const on = ids.includes(e.id)
                  return (
                    <button key={e.id} type="button" onClick={() => toggle(e.id)} aria-pressed={on} className={cn('inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-xs font-medium transition-colors', on ? 'border-primary bg-primary text-primary-foreground shadow-xs' : 'bg-card hover:bg-accent')}>
                      {on ? <Check className="size-3.5" /> : null}
                      {e.name || shortId(e.id)}
                      <span className={cn('tabular', on ? 'text-primary-foreground/70' : 'text-muted-foreground')}>{e.trial_count}</span>
                    </button>
                  )
                })}
              </div>
            )}
          </CardBody>
        </Card>
        {ids.length === 0 ? (
          <Card><Empty icon={<GitCompareArrows />} title="选择至少一个实验" description="对比表按任务版本分组，展示每个 Agent 配置的通过、失败、证据不足与耗时。" /></Card>
        ) : ids.map((id) => <ExperimentCompare key={id} id={id} />)}
      </PageBody>
    </>
  )
}

function ExperimentCompare({ id }: { id: string }) {
  const cmp = useComparison(id)
  const exp = useExperiment(id)
  const detail = exp.data
  if (cmp.isLoading || exp.isLoading) return <Skeleton className="h-64 rounded-xl" />
  if (cmp.error) return <Alert tone="danger">{errorMessage(cmp.error)}</Alert>
  const boards = cmp.data?.boards ?? []
  const st = cmp.data?.statistics
  const pl = detail?.labels?.profiles ?? {}
  const name = (pid: string) => (pl[pid] ? pl[pid].display_name || pl[pid].name : shortId(pid))
  return (
    <section className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-base font-semibold"><Link to={`/experiments/${id}`} className="hover:underline">{detail?.experiment.name || `实验 ${shortId(id)}`}</Link></h2>
        {detail ? <StateBadge state={detail.experiment.state} /> : null}
        {(cmp.data?.excluded_flaky ?? []).length ? <Badge tone="warning">已排除 {(cmp.data?.excluded_flaky ?? []).length} 个 flaky 任务</Badge> : null}
      </div>
      {st && st.macro_average && Object.keys(st.macro_average).length ? (
        <Card>
          <CardHeader title="跨任务汇总" icon={<BarChart3 />} description={st.note} actions={<Tooltip content={`方法 ${st.method} · 种子 ${st.seed} · ${st.iterations} 次重采样`}><Badge tone="outline">{st.exploratory ? '探索性' : '按任务等权'}</Badge></Tooltip>} />
          <CardBody className="space-y-4">
            <div className="space-y-2.5">
              {Object.entries(st.macro_average).sort((a, b) => b[1] - a[1]).map(([pid, v]) => {
                const iv = st.intervals?.[pid]
                return (
                  <div key={pid} className="grid grid-cols-[minmax(120px,200px)_1fr_auto] items-center gap-3">
                    <span className="truncate text-[13px]">{name(pid)}</span>
                    <div className="relative h-2 rounded-full bg-muted">
                      {iv ? <div className="absolute inset-y-0 rounded-full bg-primary/25" style={{ left: `${iv.low * 100}%`, width: `${Math.max(0, iv.high - iv.low) * 100}%` }} /> : null}
                      <div className="absolute inset-y-0 left-0 rounded-full bg-primary" style={{ width: `${v * 100}%` }} />
                    </div>
                    <span className="w-28 text-right text-xs tabular">{(v * 100).toFixed(0)}%{iv ? <span className="text-muted-foreground"> [{(iv.low * 100).toFixed(0)}–{(iv.high * 100).toFixed(0)}]</span> : null}</span>
                  </div>
                )
              })}
            </div>
            {st.paired ? (
              <div className="flex flex-wrap gap-2 border-t pt-3 text-xs">
                <span className="text-muted-foreground">配对结果</span>
                <Badge tone="success">都通过 {st.paired.both_pass ?? 0}</Badge>
                <Badge tone="primary">只 A {st.paired.only_a ?? 0}</Badge>
                <Badge tone="info">只 B {st.paired.only_b ?? 0}</Badge>
                <Badge tone="danger">都失败 {st.paired.both_fail ?? 0}</Badge>
                <Badge tone="neutral">未判定 {st.paired.unresolved ?? 0}</Badge>
              </div>
            ) : null}
          </CardBody>
        </Card>
      ) : null}
      {boards.length === 0 ? <Card><Empty icon={<Scale />} title="还没有可对比的数据" description="Trial 结束后才会出现在对比表里。" /></Card> : boards.map((b, i) => <BoardCard key={`${b.task_version_id}-${i}`} board={b} detail={detail} />)}
    </section>
  )
}

function BoardCard({ board, detail }: { board: Board; detail?: ExperimentDetail }) {
  const task = detail?.labels?.tasks?.[board.task_version_id]
  const cells = [...board.cells].sort((a, b) => rate(b) - rate(a))
  return (
    <Card className="overflow-hidden">
      <CardHeader
        title={task ? `${task.task_name} v${task.version}` : board.task_version_id ? `任务 ${shortId(board.task_version_id)}` : '多任务汇总'}
        description={<span className="flex flex-wrap gap-x-3">{board.executor ? <span>执行器 <span className="font-mono">{board.executor}</span></span> : null}{board.network ? <span>网络 <span className="font-mono">{board.network}</span></span> : null}<span>配对完成 {board.paired}</span></span>}
        actions={board.comparable ? <Badge tone="success"><Check />可比</Badge> : <Badge tone="warning"><AlertTriangle />不能合榜</Badge>}
      />
      {!board.comparable ? <Alert tone="warning" className="mx-5 mt-4">{(board.reasons ?? []).join('；') || '条件不一致'}</Alert> : null}
      <Table>
        <THead>
          <TR>
            <TH>Agent 配置</TH>
            <TH className="w-48">首轮结论</TH>
            <TH className="text-right">通过</TH>
            <TH className="text-right">未通过</TH>
            <TH className="hidden text-right md:table-cell">证据不足</TH>
            <TH className="hidden text-right md:table-cell">未完成</TH>
            <TH className="hidden text-right lg:table-cell">协助 / 修复</TH>
            <TH className="hidden text-right lg:table-cell">Agent 耗时</TH>
            <TH className="hidden text-right xl:table-cell">端到端</TH>
          </TR>
        </THead>
        <TBody>
          {cells.map((c, i) => {
            const total = c.pass + c.fail + c.inconclusive + c.unverified + c.incomplete
            return (
              <TR key={`${c.display_name}-${i}`}>
                <TD className="font-medium">{c.display_name || '未命名'}</TD>
                <TD>
                  <div className="flex h-1.5 overflow-hidden rounded-full bg-muted">
                    {total ? <>
                      <div className="bg-success" style={{ width: `${(c.pass / total) * 100}%` }} />
                      <div className="bg-danger" style={{ width: `${(c.fail / total) * 100}%` }} />
                      <div className="bg-warning" style={{ width: `${(c.inconclusive / total) * 100}%` }} />
                    </> : null}
                  </div>
                </TD>
                <TD className="text-right"><Count n={c.pass} kind="pass" evidence={c.evidence} cls="text-success" /></TD>
                <TD className="text-right"><Count n={c.fail} kind="fail" evidence={c.evidence} cls="text-danger" /></TD>
                <TD className="hidden text-right md:table-cell"><Count n={c.inconclusive} kind="inconclusive" evidence={c.evidence} cls="text-warning" /></TD>
                <TD className="hidden text-right md:table-cell"><Count n={c.incomplete + c.unverified} kind="incomplete" evidence={c.evidence} /></TD>
                <TD className="hidden text-right text-xs text-muted-foreground lg:table-cell tabular">{c.assisted_pass}/{c.assisted_fail} · {c.repair_pass || 0}{c.interventions ? ` · 介入 ${c.interventions}` : ''}</TD>
                <TD className="hidden text-right text-xs lg:table-cell tabular">{duration(c.agent_millis)}</TD>
                <TD className="hidden text-right text-xs xl:table-cell tabular">{duration(c.end_to_end_millis)}</TD>
              </TR>
            )
          })}
        </TBody>
      </Table>
      {board.note ? <p className="border-t px-5 py-2.5 text-xs text-muted-foreground">{board.note}</p> : null}
    </Card>
  )
}

function rate(c: BoardCell) {
  const d = c.pass + c.fail + c.inconclusive
  return d ? c.pass / d : -1
}

function Count({ n, kind, evidence, cls }: { n: number; kind: string; evidence?: Evidence[]; cls?: string }) {
  const hit = (evidence ?? []).find((e) => e.kind === kind)
  const body = <span className={cn('tabular font-medium', n === 0 && 'text-muted-foreground font-normal', n > 0 && cls)}>{n}</span>
  if (!hit || n === 0) return body
  return <Tooltip content="打开一个证据 Trial"><Link className="underline decoration-dotted underline-offset-4" to={`/trials/${hit.trial_id}`}>{body}</Link></Tooltip>
}
