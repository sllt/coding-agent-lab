import { Archive, ArrowUpCircle, Cpu, Database, FileClock, FileDown, FileUp, HardDrive, Lock, ScrollText, Shield, ShieldAlert, ShieldCheck, SlidersHorizontal, Trash2, Webhook, Wrench } from 'lucide-react'
import { useState } from 'react'
import { api } from '../api.ts'
import { PageBody, PageHeader } from '../components/layout/PageHeader.tsx'
import { Alert, Badge, Button, Card, CardBody, CardHeader, ConfirmButton, Empty, Field, Input, KeyValue, Skeleton, Table, TBody, TD, TH, THead, TR, Tabs, TabsContent, TabsList, TabsTrigger, Textarea } from '../components/ui/index.ts'
import { bytes, dateTime, shortId, timeAgo } from '../lib/format.ts'
import { errorMessage, get, useAction, useAudit, useSettings } from '../lib/queries.ts'
import { useQuery } from '@tanstack/react-query'
import type { Settings as SettingsT } from '../lib/types.ts'

type RetentionPlan = { log_files: string[] | null; report_files: string[] | null; kept_patches: string[] | null; irreversible: string; apply: boolean }
type Upgrade = { product: string; version: string; migrations: string[] | null; backup_required: boolean; down_migration: boolean; auto_cli_update: boolean; note: string }

export function Settings() {
  const settings = useSettings()
  const [tab, setTab] = useState('policy')
  const s = settings.data
  return (
    <>
      <PageHeader title="系统" description="并发与保留策略、维护操作、安全边界与审计记录。破坏性操作都需要二次确认。" />
      <PageBody className="space-y-4">
        {settings.error ? <Alert tone="danger" title="设置没有读到">这不是一份空配置：{errorMessage(settings.error)}</Alert> : null}
        {s?.maintenance ? (
          <Alert tone="warning" title="维护模式">新的 Trial 不会被调度。{(s.maintenance_reasons ?? []).join('；')}{s.store_error ? ` · ${s.store_error}` : ''}</Alert>
        ) : null}
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList className="mb-4">
            <TabsTrigger value="policy"><SlidersHorizontal />策略</TabsTrigger>
            <TabsTrigger value="maintenance"><Wrench />维护</TabsTrigger>
            <TabsTrigger value="security"><Shield />安全</TabsTrigger>
            <TabsTrigger value="audit"><ScrollText />审计</TabsTrigger>
            <TabsTrigger value="harbor"><FileUp />Harbor</TabsTrigger>
            <TabsTrigger value="upgrade"><ArrowUpCircle />版本</TabsTrigger>
          </TabsList>
          <TabsContent value="policy">{s ? <Policy s={s} /> : <Skeleton className="h-80 rounded-xl" />}</TabsContent>
          <TabsContent value="maintenance"><Maintenance /></TabsContent>
          <TabsContent value="security">{s ? <Security s={s} /> : <Skeleton className="h-80 rounded-xl" />}</TabsContent>
          <TabsContent value="audit"><Audit enabled={tab === 'audit'} /></TabsContent>
          <TabsContent value="harbor"><Harbor /></TabsContent>
          <TabsContent value="upgrade"><UpgradeInfo enabled={tab === 'upgrade'} s={s} /></TabsContent>
        </Tabs>
      </PageBody>
    </>
  )
}

function Policy({ s }: { s: SettingsT }) {
  const [limit, setLimit] = useState(String(s.global_limit))
  const [wall, setWall] = useState(s.agent_wall_seconds ? String(s.agent_wall_seconds) : '')
  const [quotaGB, setQuotaGB] = useState(s.disk_quota_bytes ? String(+(s.disk_quota_bytes / 1024 ** 3).toFixed(2)) : '')
  const [logDays, setLogDays] = useState(String(s.retention.log_days))
  const [patchDays, setPatchDays] = useState(String(s.retention.patch_days))
  const [reportDays, setReportDays] = useState(String(s.retention.report_days))
  const lim = Number(limit)
  const limitErr = !Number.isInteger(lim) || lim < 1 || lim > 8 ? '1 到 8 之间的整数' : ''
  const nonNeg = (v: string) => v === '' || (Number.isFinite(Number(v)) && Number(v) >= 0)
  const valid = !limitErr && [wall, quotaGB, logDays, patchDays, reportDays].every(nonNeg)
  const save = useAction(() => api.request('PATCH', '/api/v1/settings', {
    global_limit: lim,
    agent_wall_seconds: wall ? Math.floor(Number(wall)) : 0,
    disk_quota_bytes: quotaGB ? Math.round(Number(quotaGB) * 1024 ** 3) : 0,
    retention: { log_days: Number(logDays) || 0, patch_days: Number(patchDays) || 0, report_days: Number(reportDays) || 0 },
  }), { success: '策略已保存', invalidate: [['settings']] })
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader title="调度" icon={<Cpu />} />
        <CardBody className="grid gap-4 sm:grid-cols-2">
          <Field label="全局并发" error={limitErr || undefined} hint="大于 1 时，不同账号的 Trial 会重叠运行；同一账号仍受自身并发约束。">
            <Input type="number" min={1} max={8} value={limit} onChange={(e) => setLimit(e.target.value)} />
          </Field>
          <Field label="默认 Agent 墙钟（秒）" optional hint="任务与配置都没设上限时使用。留空 = 环境变量或内置默认值。">
            <Input type="number" min={0} value={wall} onChange={(e) => setWall(e.target.value)} />
          </Field>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="保留与配额" icon={<HardDrive />} />
        <CardBody className="grid gap-4 sm:grid-cols-2">
          <Field label="原始日志保留（天）"><Input type="number" min={0} value={logDays} onChange={(e) => setLogDays(e.target.value)} /></Field>
          <Field label="报告保留（天）"><Input type="number" min={0} value={reportDays} onChange={(e) => setReportDays(e.target.value)} /></Field>
          <Field label="补丁保留（天）" hint="0 表示不按时间删除补丁。"><Input type="number" min={0} value={patchDays} onChange={(e) => setPatchDays(e.target.value)} /></Field>
          <Field label="磁盘配额（GB）" optional hint="留空 / 0 表示不设水位。"><Input type="number" min={0} step="0.5" value={quotaGB} onChange={(e) => setQuotaGB(e.target.value)} /></Field>
        </CardBody>
      </Card>
      <div className="flex items-center justify-end gap-3 xl:col-span-2">
        <span className="text-xs text-muted-foreground">保存会写入审计日志。</span>
        <Button disabled={!valid} loading={save.isPending} onClick={() => save.mutate(undefined)}>保存策略</Button>
      </div>
    </div>
  )
}

function Maintenance() {
  const [plan, setPlan] = useState<RetentionPlan | null>(null)
  const [backupPath, setBackupPath] = useState('')
  const [quarantine, setQuarantine] = useState<{ removed: string[] | null; kept_evidence: string[] | null; note: string } | null>(null)
  const backup = useAction(() => api.request<{ path: string; note: string }>('POST', '/api/v1/maintenance/backup'), { success: (d) => d.note, onSuccess: (d) => setBackupPath(d.path) })
  const preview = useAction(() => api.request<RetentionPlan>('POST', '/api/v1/maintenance/retention', { apply: false }), { onSuccess: setPlan })
  const apply = useAction(() => api.request<RetentionPlan>('POST', '/api/v1/maintenance/retention', { apply: true }), { success: '已按策略删除过期文件', onSuccess: setPlan, invalidate: [['audit']] })
  const clean = useAction(() => api.request<{ removed: string[] | null; kept_evidence: string[] | null; note: string }>('POST', '/api/v1/maintenance/quarantine', {}), { success: '已清理隔离工作区', onSuccess: setQuarantine })
  const deletable = (plan?.log_files?.length ?? 0) + (plan?.report_files?.length ?? 0)
  return (
    <div className="grid gap-4 xl:grid-cols-3">
      <Card className="flex flex-col">
        <CardHeader title="一致性备份" icon={<Archive />} />
        <CardBody className="flex-1 space-y-3 text-[13px] text-muted-foreground">
          <p>在所有活跃 Attempt 结束后生成 SQLite 在线备份。仍有运行中的任务时会拒绝，而不是生成半截备份。</p>
          {backupPath ? <Alert tone="success">已写入 <code className="font-mono text-xs">{backupPath}</code></Alert> : null}
        </CardBody>
        <div className="border-t px-5 py-3"><ConfirmButton icon={<Database />} title="生成一致性备份？" description="期间会短暂进入维护状态，新的 Trial 暂不调度；备份结束后自动恢复。" confirmLabel="开始备份" onConfirm={() => backup.mutateAsync(undefined)}>生成备份</ConfirmButton></div>
      </Card>
      <Card className="flex flex-col">
        <CardHeader title="保留清理" icon={<FileClock />} />
        <CardBody className="flex-1 space-y-3 text-[13px]">
          <p className="text-muted-foreground">先预览，再删除。被报告引用的补丁和检查摘要会保留。</p>
          {plan ? (
            <div className="space-y-2 rounded-lg border p-3 text-xs">
              <div className="flex flex-wrap gap-2">
                <Badge tone={plan.log_files?.length ? 'warning' : 'neutral'}>日志 {plan.log_files?.length ?? 0}</Badge>
                <Badge tone={plan.report_files?.length ? 'warning' : 'neutral'}>报告 {plan.report_files?.length ?? 0}</Badge>
                <Badge tone="success">保留补丁 {plan.kept_patches?.length ?? 0}</Badge>
                {plan.apply ? <Badge tone="danger">已执行</Badge> : <Badge tone="outline">预览</Badge>}
              </div>
              {[...(plan.log_files ?? []), ...(plan.report_files ?? [])].slice(0, 6).map((f) => <p key={f} className="truncate font-mono text-[11px] text-muted-foreground">{f}</p>)}
              {deletable > 6 ? <p className="text-muted-foreground">…还有 {deletable - 6} 个</p> : null}
              <p className="text-muted-foreground">{plan.irreversible}</p>
            </div>
          ) : null}
        </CardBody>
        <div className="flex gap-2 border-t px-5 py-3">
          <Button size="sm" variant="outline" loading={preview.isPending} onClick={() => preview.mutate(undefined)}>预览</Button>
          <ConfirmButton danger variant="danger-outline" icon={<Trash2 />} disabled={!plan || plan.apply || deletable === 0} typeToConfirm="删除" title={`删除 ${deletable} 个过期文件？`} description={plan?.irreversible ?? ''} confirmLabel="永久删除" onConfirm={() => apply.mutateAsync(undefined)}>按策略删除</ConfirmButton>
        </div>
      </Card>
      <Card className="flex flex-col">
        <CardHeader title="隔离工作区" icon={<ShieldAlert />} />
        <CardBody className="flex-1 space-y-3 text-[13px] text-muted-foreground">
          <p>清理被标记为隔离的 Attempt 的工作区副本。补丁、事件和检查摘要不受影响，结论也不会改变。</p>
          {quarantine ? <Alert tone="success">已删除 {quarantine.removed?.length ?? 0} 个工作区，保留证据 {quarantine.kept_evidence?.length ?? 0} 项。</Alert> : null}
        </CardBody>
        <div className="border-t px-5 py-3"><ConfirmButton danger variant="danger-outline" icon={<Trash2 />} typeToConfirm="清理" title="清理隔离工作区？" description="工作区里没被收成制品的文件不能恢复。这不会把结论改成通过，也不会合并到源目录。" confirmLabel="清理" onConfirm={() => clean.mutateAsync(undefined)}>清理隔离区</ConfirmButton></div>
      </Card>
    </div>
  )
}

function Security({ s }: { s: SettingsT }) {
  const sb = s.sandbox
  const sbTone = sb.disabled ? 'warning' : sb.supported ? 'success' : 'danger'
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader title="文件系统沙箱" icon={sb.supported && !sb.disabled ? <ShieldCheck className="text-success" /> : <ShieldAlert className="text-warning" />} actions={<Badge tone={sbTone}>{sb.disabled ? '已手动关闭' : sb.supported ? `Landlock ABI ${sb.abi}` : '不可用'}</Badge>} />
        <CardBody className="space-y-3 text-[13px]">
          <p className="text-muted-foreground">真实适配器在 Landlock 下运行：只能写本次工作区、临时 HOME 与 /tmp，读不到控制面数据目录（数据库、其他 Attempt、webhook 密钥）。内核不支持 Landlock 时照常运行并记录 mode=none；已请求沙箱但应用失败时，Agent 不会启动（fail closed）。</p>
          {sb.reason ? <p className="text-xs text-muted-foreground">{sb.reason}</p> : null}
          {sb.disabled ? <Alert tone="warning">AGENTLAB_SANDBOX=off：真实 Agent 将以无沙箱的本机进程运行，每次 Attempt 都会记录这一点。</Alert> : null}
          <Alert tone="neutral">Landlock 只限制文件系统。网络与进程隔离尚未实现；restricted / offline 网络的配置会被拒绝发布。</Alert>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="控制面" icon={<Lock />} />
        <CardBody>
          <KeyValue items={[
            { label: '默认执行器', value: <span className="font-mono text-xs">{s.executor_default}</span> },
            { label: '执行器规则', value: <span className="text-xs text-muted-foreground">{s.executor_note}</span> },
            { label: '网络限制', value: <Badge tone="warning">{s.network_restricted}</Badge> },
            { label: '凭据', value: s.credential_display },
            { label: 'Webhook', value: s.webhook_configured ? <Badge tone="success"><Webhook />已配置签名密钥</Badge> : <Badge tone="neutral">未配置</Badge> },
            { label: '维护模式', value: s.maintenance ? <Badge tone="warning">开启</Badge> : <Badge tone="success">关闭</Badge> },
            { label: '进程内存', value: <span className="text-xs">{bytes(s.process_rss_bytes)} <span className="text-muted-foreground">· {s.process_rss_note}</span></span> },
          ]} />
        </CardBody>
      </Card>
    </div>
  )
}

function Audit({ enabled }: { enabled: boolean }) {
  const audit = useAudit(enabled)
  const items = audit.data ?? []
  return (
    <Card className="overflow-hidden">
      <CardHeader title="审计日志" description="最近 100 条" actions={<Button size="xs" variant="ghost" loading={audit.isFetching} onClick={() => void audit.refetch()}>刷新</Button>} />
      {audit.isLoading ? <div className="space-y-2 p-4">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-8" />)}</div> : items.length === 0 ? <Empty icon={<ScrollText />} title="还没有审计记录" /> : (
        <Table>
          <THead><TR><TH>时间</TH><TH>操作</TH><TH>对象</TH><TH className="hidden md:table-cell">操作者</TH></TR></THead>
          <TBody>
            {items.map((a) => (
              <TR key={a.id}>
                <TD className="whitespace-nowrap text-xs text-muted-foreground" title={dateTime(a.at)}>{timeAgo(a.at)}</TD>
                <TD><Badge tone={a.action.includes('apply') || a.action.includes('model_call') ? 'warning' : 'neutral'} className="font-mono">{a.action}</Badge></TD>
                <TD className="font-mono text-xs">{shortId(a.resource_id, 18)}</TD>
                <TD className="hidden text-xs text-muted-foreground md:table-cell">{a.actor_id}</TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
    </Card>
  )
}

function Harbor() {
  const [text, setText] = useState('')
  const [result, setResult] = useState('')
  const [name, setName] = useState('')
  const [prompt, setPrompt] = useState('')
  let parsed: unknown = null
  let parseErr = ''
  if (text.trim()) {
    try { parsed = JSON.parse(text) } catch { parseErr = '不是合法的 JSON' }
    if (!parseErr && (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed))) parseErr = '必须是一个 JSON 对象'
  }
  const imp = useAction(() => api.request('POST', '/api/v1/imports/harbor', parsed), { success: '已解析 Harbor 子集', onSuccess: (d) => setResult(JSON.stringify(d, null, 2)) })
  const exp = useAction(() => api.request('POST', '/api/v1/exports/harbor', { name: name.trim(), prompt }), { success: '已生成 Harbor 子集', onSuccess: (d) => setResult(JSON.stringify(d, null, 2)) })
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader title="导入" icon={<FileUp />} description="只解析受支持的 Harbor 子集，不会执行其中的脚本。" />
        <CardBody className="space-y-3">
          <Field label="Harbor JSON" error={parseErr || undefined}><Textarea rows={8} className="font-mono text-xs" value={text} onChange={(e) => setText(e.target.value)} placeholder='{"name": "...", "instruction": "..."}' /></Field>
          <Button size="sm" disabled={!text.trim() || !!parseErr} loading={imp.isPending} onClick={() => imp.mutate(undefined)}>解析</Button>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="导出" icon={<FileDown />} />
        <CardBody className="space-y-3">
          <Field label="名称"><Input value={name} onChange={(e) => setName(e.target.value)} /></Field>
          <Field label="提示词"><Textarea rows={4} value={prompt} onChange={(e) => setPrompt(e.target.value)} /></Field>
          <Button size="sm" disabled={!name.trim() || !prompt.trim()} loading={exp.isPending} onClick={() => exp.mutate(undefined)}>导出</Button>
        </CardBody>
      </Card>
      {result ? (
        <Card className="xl:col-span-2">
          <CardHeader title="结果" />
          <pre className="max-h-96 overflow-auto p-5 font-mono text-xs scrollbar-thin">{result}</pre>
        </Card>
      ) : null}
    </div>
  )
}

function UpgradeInfo({ enabled, s }: { enabled: boolean; s?: SettingsT }) {
  const q = useQuery({ queryKey: ['upgrade'], queryFn: get<Upgrade>('/api/v1/upgrade'), enabled })
  const u = q.data
  void s
  return (
    <Card>
      <CardHeader title="版本与迁移" icon={<ArrowUpCircle />} />
      <CardBody className="space-y-4">
        {q.isLoading || !u ? <Skeleton className="h-24" /> : (
          <>
            <KeyValue items={[
              { label: '产品', value: u.product },
              { label: '版本', value: <span className="font-mono text-xs">{u.version}</span> },
              { label: '已应用迁移', value: <div className="flex flex-wrap gap-1">{(u.migrations ?? []).map((m) => <Badge key={m} tone="neutral" className="font-mono">{m}</Badge>)}</div> },
              { label: '升级前备份', value: u.backup_required ? '必需' : '可选' },
              { label: '向下迁移', value: u.down_migration ? '支持' : '不支持' },
              { label: '自动更新 CLI', value: u.auto_cli_update ? '是' : '否' },
            ]} />
            <Alert tone="info">{u.note}</Alert>
          </>
        )}
      </CardBody>
    </Card>
  )
}
