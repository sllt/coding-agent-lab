import { Activity, ArrowRight, Bot, CheckCircle2, Clock, FlaskConical, LibraryBig, ShieldAlert } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { PageBody, PageHeader, Stat } from '../components/layout/PageHeader.tsx'
import { StateBadge, VerdictBadge, VerdictBar } from '../components/status.tsx'
import { Alert, Card, CardHeader, Empty, Progress, Skeleton, Table, TBody, TD, TH, THead, TR } from '../components/ui/index.ts'
import { protocolName, shortId, timeAgo } from '../lib/format.ts'
import { errorMessage, useCatalog, useExperiments, useOverview } from '../lib/queries.ts'
import { execLabel, lookup, modeLabel, terminalStates } from '../lib/status.ts'

export function Dashboard() {
  const overview = useOverview()
  const experiments = useExperiments()
  const catalog = useCatalog()
  const navigate = useNavigate()
  const data = overview.data
  const counts = data?.state_counts ?? {}
  const total = Object.values(counts).reduce((a, b) => a + b, 0)
  const completed = counts.completed ?? 0
  const recentExps = (experiments.data ?? []).slice(0, 5)
  const noTasks = catalog.data && catalog.data.tasks.length === 0
  const noProfiles = catalog.data && catalog.data.profiles.length === 0

  return (
    <>
      <PageHeader title="工作台" description="本机所有实验的运行状况。数字都来自真实运行；没有运行时这里保持空白，不会填入演示分数。" />
      <PageBody className="space-y-6">
        {overview.error ? <Alert tone="danger" title="读取失败">{errorMessage(overview.error)}</Alert> : null}

        {(noTasks || noProfiles) ? (
          <Card className="overflow-hidden">
            <div className="flex flex-col gap-4 p-5 md:flex-row md:items-center md:justify-between">
              <div>
                <p className="text-sm font-semibold">开始第一个实验</p>
                <p className="mt-1 text-[13px] text-muted-foreground">需要至少一个已发布的任务版本和一个已发布的 Agent 配置。</p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Onboard to="/tasks" icon={<LibraryBig />} done={!noTasks} label="发布任务" />
                <Onboard to="/agents" icon={<Bot />} done={!noProfiles} label="发布 Agent 配置" />
                <Onboard to="/experiments/new" icon={<FlaskConical />} done={false} label="创建实验" />
              </div>
            </div>
          </Card>
        ) : null}

        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {overview.isLoading ? Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-[106px] rounded-xl" />) : (
            <>
              <Stat label="运行中" value={data?.running ?? 0} icon={<Activity />} tone={data?.running ? 'info' : 'default'} hint="准备、运行、收集、验收中的 Trial" />
              <Stat label="排队" value={data?.queued ?? 0} icon={<Clock />} hint="按提交顺序（FIFO）调度" />
              <Stat label="已结束" value={completed} icon={<CheckCircle2 />} tone="success" hint={total ? `共 ${total} 个 Trial` : '还没有 Trial'} />
              <Stat label="账号阻塞" value={data?.blocked_accounts ?? 0} icon={<ShieldAlert />} tone={data?.blocked_accounts ? 'danger' : 'default'} hint="认证失败等原因被暂停的账号" />
            </>
          )}
        </div>

        {total > 0 ? (
          <Card>
            <CardHeader title="Trial 状态分布" description={`共 ${total} 个`} />
            <div className="space-y-3 px-5 py-4">
              <div className="flex h-2.5 overflow-hidden rounded-full bg-muted">
                {Object.entries(counts).map(([state, n]) => (
                  <div key={state} title={`${lookup(execLabel, state)} ${n}`} className={stateColor(state)} style={{ width: `${(n / total) * 100}%` }} />
                ))}
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                {Object.entries(counts).map(([state, n]) => (
                  <span key={state} className="flex items-center gap-1.5"><span className={`size-2 rounded-sm ${stateColor(state)}`} />{lookup(execLabel, state)} <span className="tabular text-foreground">{n}</span></span>
                ))}
              </div>
            </div>
          </Card>
        ) : null}

        <div className="grid gap-6 xl:grid-cols-[1.25fr_1fr]">
          <Card>
            <CardHeader title="最近实验" actions={<Link to="/experiments" className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">全部<ArrowRight className="size-3" /></Link>} />
            {experiments.isLoading ? <div className="space-y-2 p-5">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-10" />)}</div> : recentExps.length === 0 ? (
              <Empty icon={<FlaskConical />} title="还没有实验" description="选择任务和 Agent 配置，冻结计划后排队执行。" action={<Link to="/experiments/new" className="text-sm font-medium text-primary hover:underline">新建实验</Link>} />
            ) : (
              <ul className="divide-y">
                {recentExps.map((exp) => {
                  const done = (exp.state_counts?.completed ?? 0) + (exp.state_counts?.cancelled ?? 0) + (exp.state_counts?.aborted ?? 0)
                  return (
                    <li key={exp.id}>
                      <Link to={`/experiments/${exp.id}`} className="flex items-center gap-4 px-5 py-3 transition-colors hover:bg-accent/50">
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <p className="truncate text-[13px] font-medium">{exp.name || `实验 ${shortId(exp.id)}`}</p>
                            <StateBadge state={exp.state} />
                          </div>
                          <p className="mt-0.5 text-xs text-muted-foreground">{lookup(modeLabel, exp.mode)} · {protocolName(exp.plan?.protocol ?? exp.protocol)} · {timeAgo(exp.created_at)}</p>
                        </div>
                        <div className="hidden w-40 shrink-0 sm:block">
                          <VerdictBar counts={exp.verdict_counts ?? {}} total={exp.trial_count} />
                          <p className="mt-1 text-right text-[11px] text-muted-foreground tabular">{done}/{exp.trial_count} 完成</p>
                        </div>
                      </Link>
                    </li>
                  )
                })}
              </ul>
            )}
          </Card>

          <Card>
            <CardHeader title="最近 Trial" description="新到旧" />
            {overview.isLoading ? <div className="space-y-2 p-5">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-8" />)}</div> : (data?.recent ?? []).length === 0 ? (
              <Empty title="还没有运行" description="Trial 开始后会出现在这里。" />
            ) : (
              <Table>
                <THead><TR><TH>Trial</TH><TH>状态</TH><TH>结论</TH></TR></THead>
                <TBody>
                  {(data?.recent ?? []).slice(0, 8).map((t) => (
                    <TR key={t.id} className="cursor-pointer hover:bg-accent/50" onClick={() => navigate(`/trials/${t.id}`)}>
                      <TD><span className="font-mono text-xs">{shortId(t.id)}</span><span className="ml-2 text-xs text-muted-foreground">#{t.repeat_index}</span></TD>
                      <TD><StateBadge state={t.execution_state} /></TD>
                      <TD>{terminalStates.has(t.execution_state) ? <VerdictBadge verdict={t.verdict} /> : <span className="text-xs text-muted-foreground">—</span>}</TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            )}
          </Card>
        </div>
        {total > 0 ? (
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span>整体完成度</span>
            <Progress value={(completed / total) * 100} tone="success" className="max-w-xs" />
            <span className="tabular">{Math.round((completed / total) * 100)}%</span>
          </div>
        ) : null}
      </PageBody>
    </>
  )
}

function stateColor(state: string) {
  switch (state) {
    case 'completed': return 'bg-success'
    case 'queued': return 'bg-muted-foreground/30'
    case 'aborted': return 'bg-danger'
    case 'cancelled': return 'bg-muted-foreground/50'
    case 'cancelling': return 'bg-warning'
    default: return 'bg-info'
  }
}

function Onboard({ to, icon, label, done }: { to: string; icon: React.ReactNode; label: string; done: boolean }) {
  return (
    <Link to={to} className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-[13px] font-medium transition-colors hover:bg-accent [&_svg]:size-4 ${done ? 'text-muted-foreground line-through decoration-success' : ''}`}>
      {done ? <CheckCircle2 className="text-success" /> : <span className="text-primary">{icon}</span>}
      {label}
    </Link>
  )
}
