import { FlaskConical, Plus, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { PageBody, PageHeader } from '../components/layout/PageHeader.tsx'
import { StateBadge, VerdictBar } from '../components/status.tsx'
import { Alert, Card, Empty, Input, Segmented, Skeleton, Table, TBody, TD, TH, THead, TR } from '../components/ui/index.ts'
import { dateTime, protocolName, shortId, timeAgo } from '../lib/format.ts'
import { errorMessage, useExperiments } from '../lib/queries.ts'
import { lookup, modeLabel, terminalStates } from '../lib/status.ts'
import type { Experiment } from '../lib/types.ts'

type Filter = 'all' | 'active' | 'done'

export function Experiments() {
  const { data, isLoading, error } = useExperiments()
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const navigate = useNavigate()
  const items = useMemo(() => data ?? [], [data])
  const active = items.filter((e) => !terminalStates.has(e.state)).length
  const shown = useMemo(() => {
    const needle = q.trim().toLowerCase()
    return items.filter((e) => {
      if (filter === 'active' && terminalStates.has(e.state)) return false
      if (filter === 'done' && !terminalStates.has(e.state)) return false
      if (!needle) return true
      return [e.id, e.name, e.description, e.mode].some((s) => (s ?? '').toLowerCase().includes(needle))
    })
  }, [items, q, filter])

  return (
    <>
      <PageHeader
        title="实验"
        description="每个实验冻结一份计划：任务版本 × Agent 配置 × 重复次数。提交后计划不可修改。"
        actions={<Link to="/experiments/new" className="inline-flex h-8 items-center gap-1.5 rounded-md bg-primary px-3 text-[13px] font-medium text-primary-foreground shadow-xs hover:bg-primary/90"><Plus className="size-4" />新建实验</Link>}
      />
      <PageBody className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Segmented<Filter> value={filter} onChange={setFilter} options={[
            { value: 'all', label: '全部', count: items.length },
            { value: 'active', label: '进行中', count: active },
            { value: 'done', label: '已结束', count: items.length - active },
          ]} />
          <div className="relative w-full sm:w-72">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input className="pl-8" placeholder="按名称、ID 搜索" value={q} onChange={(e) => setQ(e.target.value)} aria-label="搜索实验" />
          </div>
        </div>
        {error ? <Alert tone="danger">{errorMessage(error)}</Alert> : null}
        <Card className="overflow-hidden">
          {isLoading ? <div className="space-y-2 p-4">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-11" />)}</div> : shown.length === 0 ? (
            items.length === 0
              ? <Empty icon={<FlaskConical />} title="还没有实验" description="选好任务和 Agent 配置，就能排队跑第一组 Trial。" action={<Link to="/experiments/new" className="text-sm font-medium text-primary hover:underline">新建实验</Link>} />
              : <Empty icon={<Search />} title="没有匹配的实验" description="换个关键词或筛选条件试试。" />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>实验</TH>
                  <TH>状态</TH>
                  <TH className="hidden md:table-cell">模式 · 协议</TH>
                  <TH className="w-56">进度与结论</TH>
                  <TH className="hidden text-right lg:table-cell">创建</TH>
                </TR>
              </THead>
              <TBody>
                {shown.map((exp) => <Row key={exp.id} exp={exp} onOpen={() => navigate(`/experiments/${exp.id}`)} />)}
              </TBody>
            </Table>
          )}
        </Card>
      </PageBody>
    </>
  )
}

function Row({ exp, onOpen }: { exp: Experiment; onOpen: () => void }) {
  const sc = exp.state_counts ?? {}
  const vc = exp.verdict_counts ?? {}
  const done = (sc.completed ?? 0) + (sc.cancelled ?? 0) + (sc.aborted ?? 0)
  const plan = exp.plan ?? {}
  return (
    <TR className="cursor-pointer hover:bg-accent/50" onClick={onOpen}>
      <TD>
        <Link to={`/experiments/${exp.id}`} className="block min-w-0" onClick={(e) => e.stopPropagation()}>
          <p className="truncate text-[13px] font-medium">{exp.name || `实验 ${shortId(exp.id)}`}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            <span className="font-mono">{shortId(exp.id, 12)}</span>
            <span className="mx-1.5">·</span>
            {plan.task_version_ids?.length ?? 0} 任务 × {plan.profile_version_ids?.length ?? 0} 配置 × {plan.repetitions ?? 1} 次
          </p>
        </Link>
      </TD>
      <TD><StateBadge state={exp.state} /></TD>
      <TD className="hidden text-xs text-muted-foreground md:table-cell">{lookup(modeLabel, exp.mode)}<br /><span className="font-mono">{protocolName(plan.protocol ?? exp.protocol)}</span></TD>
      <TD>
        <VerdictBar counts={vc} total={exp.trial_count} />
        <div className="mt-1 flex justify-between text-[11px] text-muted-foreground tabular">
          <span><span className="text-success">{vc.pass ?? 0}</span> / <span className="text-danger">{vc.fail ?? 0}</span> / <span className="text-warning">{vc.inconclusive ?? 0}</span></span>
          <span>{done}/{exp.trial_count}</span>
        </div>
      </TD>
      <TD className="hidden text-right text-xs text-muted-foreground lg:table-cell" title={dateTime(exp.created_at)}>{timeAgo(exp.created_at)}</TD>
    </TR>
  )
}
