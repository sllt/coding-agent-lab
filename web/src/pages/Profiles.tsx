import { useEffect, useState } from 'react'
import { api } from '../api.ts'
import { Button, Card, Input, Notice } from '../components/ui.tsx'

type Account = { ID: string; AuthKind: string; CredentialRef: string }
type Profile = { ID: string; Name: string }
type Report = { static_passed?: boolean; verified?: boolean; model_call?: string; network?: string; note?: string; blockers?: string[]; supports_headless?: boolean }

const adapters = [
  { id: 'fixture', label: '本地假 CLI' },
  { id: 'cursor', label: 'Cursor' },
  { id: 'grok', label: 'Grok' },
  { id: 'opencode', label: 'OpenCode' },
]

export function Profiles() {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState('本地假 CLI')
  const [account, setAccount] = useState('')
  const [adapter, setAdapter] = useState('fixture')
  const [mode, setMode] = useState('empty')
  const [model, setModel] = useState('fixture-local')
  const [executable, setExecutable] = useState('')
  const [approve, setApprove] = useState(false)
  const [billing, setBilling] = useState<'subscription' | 'metered_api'>('subscription')
  const [entitled, setEntitled] = useState(false)
  const [report, setReport] = useState('')
  const [error, setError] = useState('')

  async function load() {
    const a = await api.request<{ items: Account[] | null }>('GET', '/api/v1/accounts')
    const p = await api.request<{ items: Profile[] | null }>('GET', '/api/v1/profiles')
    setAccounts(a.items || [])
    setProfiles(p.items || [])
    if ((a.items || [])[0]) setAccount((current) => current || (a.items || [])[0].ID)
  }
  useEffect(() => { load().catch((err: Error) => setError(err.message)).finally(() => setLoading(false)) }, [])
  const loadFailed = !loading && error !== '' && accounts.length === 0 && profiles.length === 0

  function chooseAdapter(next: string) {
    setAdapter(next)
    if (next === 'fixture') {
      setModel('fixture-local')
      setExecutable('')
      setApprove(false)
      return
    }
    if (next === 'opencode') setModel('provider/model')
    else setModel('')
    setExecutable(next === 'cursor' ? 'agent' : next)
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">Agent 配置</h1>
      <Notice>这里的配置是工具和账号的组合，不是“一个模型”。doctor 只做静态检查，不会发起付费调用。未探测的无头能力不会写成已支持。</Notice>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      <Card title="账号引用">
        <Button onClick={() => void api.request('POST', '/api/v1/accounts', { auth_kind: 'manual_import', credential_ref: 'ref:local-fixture', concurrency: 1 }).then(load).catch((err: Error) => setError(err.message))}>保存本地假 CLI 引用</Button>
        {loading ? <p className="mt-2 text-sm">正在读取账号。</p> : loadFailed ? null : accounts.length === 0 ? <p className="mt-2 text-sm">还没有账号引用。</p> : (
          <ul className="mt-2 text-sm">{accounts.map((item) => <li key={item.ID}>{item.AuthKind} · {item.CredentialRef}</li>)}</ul>
        )}
      </Card>
      <Card title="新配置">
        <div className="flex flex-col gap-2">
          <Input value={name} onChange={(e) => setName(e.target.value)} />
          <label className="text-sm">适配器
            <select className="mt-1 w-full rounded-md border bg-white px-3 py-2" value={adapter} onChange={(e) => chooseAdapter(e.target.value)}>
              {adapters.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
            </select>
          </label>
          {adapter === 'fixture' ? (
            <label className="text-sm">假 CLI 模式
              <select className="mt-1 w-full rounded-md border bg-white px-3 py-2" value={mode} onChange={(e) => setMode(e.target.value)}>
                <option value="empty">empty · 不改文件</option>
                <option value="success">success · 用参考解校准通过</option>
                <option value="fail">fail · 错误补丁</option>
                <option value="forge">forge · 伪造 PASS</option>
              </select>
            </label>
          ) : (
            <>
              <Input placeholder={adapter === 'opencode' ? '模型，必须是 provider/model' : '模型标识'} value={model} onChange={(e) => setModel(e.target.value)} />
              <Input placeholder="可执行文件，留空则用默认命令名" value={executable} onChange={(e) => setExecutable(e.target.value)} />
            </>
          )}
          {adapter !== 'fixture' ? (
            <>
              <label className="text-sm">计费路径
                <select className="mt-1 w-full rounded-md border bg-white px-3 py-2" value={billing} onChange={(e) => setBilling(e.target.value === 'metered_api' ? 'metered_api' : 'subscription')}>
                  <option value="subscription">订阅</option>
                  <option value="metered_api">按量 API</option>
                </select>
              </label>
              <label className="flex items-start gap-2 text-sm">
                <input type="checkbox" checked={entitled} onChange={(e) => setEntitled(e.target.checked)} />
                <span>记下你确认的计费路径。真实适配器在授权模型探测完成前不能发布，勾选不会把它变成已验证。</span>
              </label>
            </>
          ) : null}
          {adapter === 'cursor' ? (
            <label className="flex items-start gap-2 text-sm">
              <input type="checkbox" checked={approve} onChange={(e) => setApprove(e.target.checked)} />
              <span>批准工具写入（启动参数才会带 --force）。不批准时 Cursor 不能写文件，doctor 会写明这一点。</span>
            </label>
          ) : null}
          <Button disabled={!account || !name.trim()} onClick={() => void api.request('POST', '/api/v1/profiles', {
            name,
            account_id: account,
            profile: {
              adapter,
              model,
              fake_mode: adapter === 'fixture' ? mode : '',
              executor: 'native-trusted',
              network: 'unrestricted',
              display_name: name,
              executable,
              approve_tools: approve,
              billing_path: adapter === 'fixture' ? '' : billing,
              entitlement_verified_at: adapter !== 'fixture' && entitled ? new Date().toISOString() : '',
            },
          }).then(load).catch((err: Error) => setError(err.message))}>创建草稿</Button>
        </div>
        {loading ? <p className="mt-3 text-sm">正在读取配置。</p> : loadFailed ? null : profiles.length === 0 ? <p className="mt-3 text-sm">还没有配置。</p> : (
          <ul className="mt-3 flex flex-col gap-2 text-sm">
            {profiles.map((profile) => (
              <li key={profile.ID} className="flex flex-wrap gap-2">
                <span>{profile.Name}</span>
                <Button tone="quiet" onClick={() => void api.request<Report>('POST', `/api/v1/profiles/${profile.ID}/doctor`, { allow_model_call: false }).then((data) => setReport(`${data.note || ''} 模型调用 ${data.model_call || 'not_run'}，网络 ${data.network || 'unrestricted'}，静态通过 ${data.static_passed ? '是' : '否'}，已验证 ${data.verified ? '是' : '否'}。`)).catch((err: Error) => setError(err.message))}>doctor</Button>
                <Button onClick={() => void api.request('POST', `/api/v1/profiles/${profile.ID}/publish`).then(() => setReport('已发布。未通过 doctor、未验证的真实配置，以及 Docker 或受限网络，都会被拒绝，不会改成本机执行。')).catch((err: Error) => setError(err.message))}>发布</Button>
              </li>
            ))}
          </ul>
        )}
        {report ? <p className="mt-3 text-sm">{report}</p> : null}
      </Card>
    </div>
  )
}
