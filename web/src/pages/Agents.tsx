import { Bot, KeyRound, Lightbulb, Plus, Rocket, Stethoscope, Terminal, UserRound, XCircle, Zap } from 'lucide-react'
import { useState } from 'react'
import { api } from '../api.ts'
import { PageBody, PageHeader } from '../components/layout/PageHeader.tsx'
import { ReadinessBadge } from '../components/status.tsx'
import { Alert, Badge, Button, Card, CardBody, Checkbox, ConfirmButton, CopyText, Dialog, DialogContent, DialogTrigger, Empty, Field, Input, KeyValue, Select, Skeleton, Switch } from '../components/ui/index.ts'
import { cn } from '../lib/cn.ts'
import { dateTime, duration, shortId, timeAgo } from '../lib/format.ts'
import { errorMessage, useAccounts, useAction, useProfiles } from '../lib/queries.ts'
import { adapterLabel, lookup } from '../lib/status.ts'
import type { Account, DoctorReport, Profile, ProfileVersionInfo } from '../lib/types.ts'

const adapters = [
  { id: 'fixture', label: '本地假 CLI', hint: '不调用模型，用来校准流程与验证器。', exe: '', model: 'fixture-local' },
  { id: 'cursor', label: 'Cursor', hint: 'Cursor Agent CLI（agent）。', exe: 'agent', model: '' },
  { id: 'grok', label: 'Grok', hint: 'Grok CLI。', exe: 'grok', model: '' },
  { id: 'opencode', label: 'OpenCode', hint: '模型必须写成 provider/model。', exe: 'opencode', model: 'provider/model' },
]

export function Agents() {
  const profiles = useProfiles()
  const accounts = useAccounts()
  const [reports, setReports] = useState<Record<string, DoctorReport>>({})
  const items = profiles.data?.items ?? []
  const latest = profiles.data?.latest ?? {}

  return (
    <>
      <PageHeader
        title="Agents"
        description="配置 = 工具 + 账号 + 模型 + 运行参数，不是「一个模型」。发布前先跑 doctor；doctor 默认只做静态检查，不会发起付费调用。"
        actions={<><AccountDialog /><ProfileDialog accounts={accounts.data ?? []} /></>}
      />
      <PageBody className="space-y-6">
        {profiles.error ? <Alert tone="danger">{errorMessage(profiles.error)}</Alert> : null}
        <section>
          <h2 className="mb-3 text-sm font-semibold">配置</h2>
          {profiles.isLoading ? <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-56 rounded-xl" />)}</div> : items.length === 0 ? (
            <Card><Empty icon={<Bot />} title="还没有 Agent 配置" description={(accounts.data ?? []).length === 0 ? '先添加一个账号引用，再创建配置。' : '创建一个配置草稿，跑 doctor，然后发布。'} action={<ProfileDialog accounts={accounts.data ?? []} />} /></Card>
          ) : (
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {items.map((p) => <ProfileCard key={p.id} profile={p} latest={latest[p.id]} report={reports[p.id]} onReport={(r) => setReports((s) => ({ ...s, [p.id]: r }))} account={(accounts.data ?? []).find((a) => a.id === p.account_id)} />)}
            </div>
          )}
        </section>
        <section>
          <h2 className="mb-3 text-sm font-semibold">账号引用</h2>
          <Card className="overflow-hidden">
            {accounts.isLoading ? <div className="p-4"><Skeleton className="h-10" /></div> : (accounts.data ?? []).length === 0 ? (
              <Empty icon={<KeyRound />} title="还没有账号引用" description="账号只保存引用（如环境变量名），密钥本身不入库、不回显。" action={<AccountDialog />} />
            ) : (
              <ul className="divide-y">
                {(accounts.data ?? []).map((a) => (
                  <li key={a.id} className="flex flex-wrap items-center gap-3 px-5 py-3">
                    <span className="flex size-8 items-center justify-center rounded-full bg-muted"><UserRound className="size-4 text-muted-foreground" /></span>
                    <div className="min-w-0 flex-1">
                      <p className="text-[13px] font-medium">{a.auth_kind || 'manual_import'}</p>
                      <CopyText value={a.id} display={shortId(a.id, 14)} />
                    </div>
                    <Badge tone="neutral">{a.credential_ref}</Badge>
                    <Badge tone="outline">并发 {a.concurrency}</Badge>
                    {a.blocked_reason ? <Badge tone="danger" title={a.blocked_until ? `至 ${dateTime(a.blocked_until)}` : undefined}>已阻塞 · {a.blocked_reason}</Badge> : <Badge tone="success">可用</Badge>}
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </section>
      </PageBody>
    </>
  )
}

function ProfileCard({ profile, latest, report, onReport, account }: { profile: Profile; latest?: ProfileVersionInfo; report?: DoctorReport; onReport: (r: DoctorReport) => void; account?: Account }) {
  const d = profile.draft ?? {}
  const [open, setOpen] = useState(false)
  const doctor = useAction((allow: boolean) => api.request<DoctorReport>('POST', `/api/v1/profiles/${profile.id}/doctor`, { allow_model_call: allow }), {
    onSuccess: (r) => { onReport(r); setOpen(true) },
  })
  const publish = useAction(() => api.request<{ version: number }>('POST', `/api/v1/profiles/${profile.id}/publish`), {
    success: (v) => `已发布 v${v.version}`,
    invalidate: [['profiles'], ['catalog']],
  })
  const readiness = report?.readiness ?? latest?.readiness ?? null
  return (
    <Card className="flex flex-col">
      <div className="flex items-start gap-3 p-5 pb-3">
        <div className={cn('flex size-10 shrink-0 items-center justify-center rounded-lg', d.adapter === 'fixture' ? 'bg-muted text-muted-foreground' : 'bg-primary-soft text-primary')}>
          {d.adapter === 'fixture' ? <Terminal className="size-5" /> : <Bot className="size-5" />}
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{d.display_name || profile.name}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">{lookup(adapterLabel, d.adapter)}{d.model ? <> · <span className="font-mono">{d.model}</span></> : null}</p>
          <ReadinessBadge readiness={readiness || null} className="mt-2" />
        </div>
      </div>
      <CardBody className="flex-1 space-y-3 pt-0">
        <KeyValue className="text-xs" items={[
          { label: '已发布', value: latest ? <span>v{latest.version} · {timeAgo(latest.created_at)}</span> : <span className="text-muted-foreground">尚未发布</span> },
          ...(d.adapter === 'fixture' ? [{ label: '假 CLI 模式', value: <span className="font-mono">{d.fake_mode || 'empty'}</span> }] : [{ label: '可执行文件', value: <span className="font-mono">{d.executable || '默认'}</span> }]),
          ...(d.credential_env?.length ? [{ label: '凭据变量', value: <span className="font-mono">{d.credential_env.join(', ')}</span> }] : []),
          { label: '账号', value: account ? `${account.auth_kind} · 并发 ${account.concurrency}` : shortId(profile.account_id) },
        ]} />
        <div className="flex flex-wrap gap-1.5">
          {d.inherit_home ? <Badge tone="warning">继承 HOME</Badge> : null}
          {d.approve_tools ? <Badge tone="info">批准工具写入</Badge> : null}
          {d.wall_seconds ? <Badge tone="outline">墙钟 {d.wall_seconds}s</Badge> : null}
          {d.billing_path ? <Badge tone="outline">{d.billing_path === 'metered_api' ? '按量 API' : '订阅'}</Badge> : null}
        </div>
        {report && open ? <DoctorPanel report={report} onClose={() => setOpen(false)} /> : report ? <button type="button" className="text-xs text-primary hover:underline" onClick={() => setOpen(true)}>查看 doctor 报告</button> : null}
      </CardBody>
      <div className="flex items-center gap-2 border-t px-5 py-3">
        <Button size="sm" variant="outline" loading={doctor.isPending && doctor.variables === false} onClick={() => doctor.mutate(false)}><Stethoscope />doctor</Button>
        {d.adapter !== 'fixture' ? (
          <ConfirmButton size="sm" variant="ghost" icon={<Zap />} title="运行一次真实冒烟测试？" description="会用这个配置真正调用一次模型，可能产生少量费用，并记录到审计日志。用于把就绪度从「可运行 · 未验证」提升到「已验证」。" confirmLabel="运行冒烟测试" onConfirm={() => doctor.mutateAsync(true)}>冒烟</ConfirmButton>
        ) : null}
        <div className="flex-1" />
        <ConfirmButton size="sm" variant="default" icon={<Rocket />} title={`发布「${profile.name}」？`} description="发布会冻结当前草稿为新版本。doctor 未通过、未验证的真实配置、Docker 或受限网络都会被拒绝，不会被悄悄改成本机执行。" confirmLabel="发布" onConfirm={() => publish.mutateAsync(undefined)}>发布</ConfirmButton>
      </div>
    </Card>
  )
}

function DoctorPanel({ report, onClose }: { report: DoctorReport; onClose: () => void }) {
  return (
    <div className="space-y-2.5 rounded-lg border bg-muted/30 p-3 text-xs animate-in">
      <div className="flex items-center justify-between">
        <p className="font-medium">doctor 报告 <span className="font-normal text-muted-foreground">· {timeAgo(report.checked_at)}</span></p>
        <button type="button" className="text-muted-foreground hover:text-foreground" onClick={onClose} aria-label="收起"><XCircle className="size-3.5" /></button>
      </div>
      <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-muted-foreground">
        <span>CLI 版本 <span className="font-mono text-foreground">{report.cli_version || '—'}</span></span>
        <span>静态检查 <span className={report.static_passed ? 'text-success' : 'text-danger'}>{report.static_passed ? '通过' : '未通过'}</span></span>
        <span>无头模式 {report.supports_headless ? '支持' : '未确认'}</span>
        <span>模型调用 <span className="font-mono">{report.model_call || 'not_run'}</span></span>
      </div>
      {report.credential_sources?.length ? <p className="text-muted-foreground">凭据来源：<span className="text-foreground">{report.credential_sources.join('、')}</span></p> : null}
      {report.smoke ? (
        <div className={cn('rounded-md px-2.5 py-1.5', report.smoke.passed ? 'bg-success-soft text-success-soft-foreground' : 'bg-danger-soft text-danger-soft-foreground')}>
          冒烟测试{report.smoke.passed ? '通过' : '未通过'} · exit {report.smoke.exit_code} · {report.smoke.json_lines} 行 JSON · {duration(report.smoke.duration_ms)}
          {report.smoke.reason ? <p className="mt-0.5 opacity-90">{report.smoke.reason}</p> : null}
          {report.smoke.auth_suspected ? <p className="mt-0.5 font-medium">疑似认证问题：请检查登录状态或凭据变量。</p> : null}
        </div>
      ) : null}
      {report.blockers?.length ? (
        <ul className="space-y-1">{report.blockers.map((b) => <li key={b} className="flex gap-1.5 text-danger"><XCircle className="mt-0.5 size-3 shrink-0" />{b}</li>)}</ul>
      ) : null}
      {report.hints?.length ? (
        <ul className="space-y-1">{report.hints.map((h) => <li key={h} className="flex gap-1.5 text-muted-foreground"><Lightbulb className="mt-0.5 size-3 shrink-0 text-warning" />{h}</li>)}</ul>
      ) : null}
      {report.note ? <p className="text-muted-foreground">{report.note}</p> : null}
    </div>
  )
}

function AccountDialog() {
  const [open, setOpen] = useState(false)
  const [kind, setKind] = useState<'env' | 'local'>('env')
  const [envName, setEnvName] = useState('')
  const [concurrency, setConcurrency] = useState(1)
  const envOk = /^[A-Z_][A-Z0-9_]*$/.test(envName)
  const create = useAction(() => api.request('POST', '/api/v1/accounts', {
    auth_kind: kind === 'env' ? 'env_reference' : 'manual_import',
    credential_ref: kind === 'env' ? `env:${envName}` : 'ref:local-fixture',
    concurrency,
  }), { success: '账号引用已保存', invalidate: [['accounts']], onSuccess: () => { setOpen(false); setEnvName('') } })
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button size="sm" variant="outline"><KeyRound />添加账号</Button></DialogTrigger>
      <DialogContent title="添加账号引用" description="只保存引用，不保存密钥。真实值在启动 Agent 时从控制面进程的环境变量读取。" footer={<><Button variant="outline" size="sm" onClick={() => setOpen(false)}>取消</Button><Button size="sm" disabled={kind === 'env' && !envOk} loading={create.isPending} onClick={() => create.mutate(undefined)}>保存</Button></>}>
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-2">
            {([['env', '环境变量', '真实 CLI：引用一个环境变量名'], ['local', '本地 / CLI 登录', '假 CLI，或使用 CLI 自己的登录状态']] as const).map(([v, l, h]) => (
              <button key={v} type="button" onClick={() => setKind(v)} className={cn('rounded-lg border p-3 text-left', kind === v ? 'border-primary bg-primary-soft ring-1 ring-primary/30' : 'hover:bg-accent')}>
                <p className="text-[13px] font-medium">{l}</p><p className="mt-0.5 text-xs text-muted-foreground">{h}</p>
              </button>
            ))}
          </div>
          {kind === 'env' ? (
            <Field label="环境变量名" error={envName && !envOk ? '只能包含大写字母、数字和下划线' : undefined} hint="例如 CURSOR_API_KEY。">
              <Input autoFocus className="font-mono" placeholder="CURSOR_API_KEY" value={envName} onChange={(e) => setEnvName(e.target.value.toUpperCase())} />
            </Field>
          ) : null}
          <Field label="同账号并发" hint="同一账号的清理未完成时不会再启动下一个。">
            <Input type="number" min={1} max={8} value={concurrency} onChange={(e) => setConcurrency(Math.min(8, Math.max(1, Math.floor(Number(e.target.value) || 1))))} />
          </Field>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ProfileDialog({ accounts }: { accounts: Account[] }) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [account, setAccount] = useState('')
  const [adapter, setAdapter] = useState('fixture')
  const [mode, setMode] = useState('success')
  const [model, setModel] = useState('fixture-local')
  const [exe, setExe] = useState('')
  const [credEnv, setCredEnv] = useState('')
  const [inheritHome, setInheritHome] = useState(false)
  const [approve, setApprove] = useState(false)
  const [wall, setWall] = useState('')
  const [billing, setBilling] = useState('subscription')
  const acct = account || accounts[0]?.id || ''
  const real = adapter !== 'fixture'
  const envs = credEnv.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean)
  const badEnv = envs.filter((e) => !/^[A-Z_][A-Z0-9_]*$/.test(e))
  const wallN = wall ? Math.floor(Number(wall)) : 0

  const pick = (id: string) => {
    const a = adapters.find((x) => x.id === id)!
    setAdapter(id)
    setModel(a.model)
    setExe(a.exe)
    if (id === 'fixture') { setApprove(false); setInheritHome(false); setCredEnv('') }
  }
  const create = useAction(() => api.request('POST', '/api/v1/profiles', {
    name: name.trim(),
    account_id: acct,
    profile: {
      adapter, model, fake_mode: real ? '' : mode, executor: 'native-trusted', network: 'unrestricted',
      display_name: name.trim(), executable: real ? exe.trim() : '', approve_tools: approve,
      billing_path: real ? billing : '', credential_env: envs, inherit_home: inheritHome,
      ...(wallN > 0 ? { wall_seconds: wallN } : {}),
    },
  }), { success: '配置草稿已创建，下一步跑 doctor', invalidate: [['profiles']], onSuccess: () => { setOpen(false); setName('') } })

  const valid = name.trim() && acct && badEnv.length === 0 && (!real || model.trim()) && (adapter !== 'opencode' || model.includes('/'))
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button size="sm"><Plus />新建配置</Button></DialogTrigger>
      <DialogContent wide title="新建 Agent 配置" description="执行器固定为 native-trusted（本机进程 + Landlock 文件系统沙箱），网络为 unrestricted。" footer={<><Button variant="outline" size="sm" onClick={() => setOpen(false)}>取消</Button><Button size="sm" disabled={!valid} loading={create.isPending} onClick={() => create.mutate(undefined)}>创建草稿</Button></>}>
        {accounts.length === 0 ? <Alert tone="warning" className="mb-4">还没有账号引用。请先关闭此窗口并「添加账号」。</Alert> : null}
        <div className="space-y-5">
          <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
            {adapters.map((a) => (
              <button key={a.id} type="button" onClick={() => pick(a.id)} className={cn('rounded-lg border p-3 text-left transition-colors', adapter === a.id ? 'border-primary bg-primary-soft ring-1 ring-primary/30' : 'hover:bg-accent')}>
                <p className="text-[13px] font-medium">{a.label}</p>
                <p className="mt-0.5 text-[11px] leading-snug text-muted-foreground">{a.hint}</p>
              </button>
            ))}
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="显示名称"><Input autoFocus value={name} placeholder={real ? '例如 Cursor · sonnet-4' : '本地假 CLI'} onChange={(e) => setName(e.target.value)} /></Field>
            <Field label="账号">
              <Select value={acct} onChange={(e) => setAccount(e.target.value)} disabled={accounts.length === 0}>
                {accounts.map((a) => <option key={a.id} value={a.id}>{a.auth_kind} · {shortId(a.id)}</option>)}
              </Select>
            </Field>
            {real ? (
              <>
                <Field label="模型" hint={adapter === 'opencode' ? '必须是 provider/model' : '模型标识，会与实际解析出的模型核对'}><Input className="font-mono" value={model} onChange={(e) => setModel(e.target.value)} /></Field>
                <Field label="可执行文件" optional hint="留空则用默认命令名"><Input className="font-mono" value={exe} onChange={(e) => setExe(e.target.value)} /></Field>
                <Field label="凭据环境变量" optional error={badEnv.length ? `无效的变量名：${badEnv.join(', ')}` : undefined} hint="逗号分隔。只转发这些变量，值不入库，日志里会被打码。"><Input className="font-mono" placeholder="CURSOR_API_KEY" value={credEnv} onChange={(e) => setCredEnv(e.target.value.toUpperCase())} /></Field>
                <Field label="计费路径">
                  <Select value={billing} onChange={(e) => setBilling(e.target.value)}>
                    <option value="subscription">订阅</option>
                    <option value="metered_api">按量 API</option>
                  </Select>
                </Field>
              </>
            ) : (
              <Field label="假 CLI 模式" className="md:col-span-2">
                <Select value={mode} onChange={(e) => setMode(e.target.value)}>
                  <option value="success">success · 用参考解校准通过</option>
                  <option value="empty">empty · 不改文件</option>
                  <option value="fail">fail · 错误补丁</option>
                  <option value="forge">forge · 伪造 PASS（验证器应判失败）</option>
                </Select>
              </Field>
            )}
            <Field label="Agent 墙钟上限（秒）" optional hint="不同配置的上限不一致会导致结果不可比。"><Input type="number" min={1} value={wall} onChange={(e) => setWall(e.target.value)} /></Field>
          </div>
          {real ? (
            <div className="space-y-3 rounded-lg border p-4">
              <label className="flex items-start gap-3">
                <Switch checked={inheritHome} onCheckedChange={setInheritHome} className="mt-0.5" />
                <span className="text-[13px]"><span className="font-medium">继承真实 HOME</span><span className="block text-xs text-muted-foreground">让 CLI 读到你本机的登录文件。会削弱隔离：沙箱会放开 HOME 的读写，每次 Attempt 都会记录这一点。</span></span>
              </label>
              {adapter === 'cursor' ? (
                <label className="flex items-start gap-3">
                  <Checkbox checked={approve} onCheckedChange={(v) => setApprove(v === true)} className="mt-0.5" />
                  <span className="text-[13px]"><span className="font-medium">批准工具写入</span><span className="block text-xs text-muted-foreground">启动参数会带 --force。不批准时 Cursor 不能写文件，doctor 会写明这一点。</span></span>
                </label>
              ) : null}
            </div>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
