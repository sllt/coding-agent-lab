import { useEffect, useState } from 'react'
import { api } from '../api.ts'
import { Button, Card, Input, Notice } from '../components/ui.tsx'

type Account = { ID: string; AuthKind: string; CredentialRef: string }
type Profile = { ID: string; Name: string }
type Report = { static_passed?: boolean; model_call?: string; network?: string; note?: string; blockers?: string[] }

export function Profiles() {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [name, setName] = useState('本地假 CLI')
  const [account, setAccount] = useState('')
  const [mode, setMode] = useState('empty')
  const [report, setReport] = useState('')
  const [error, setError] = useState('')

  async function load() {
    const a = await api.request<{ items: Account[] | null }>('GET', '/api/v1/accounts')
    const p = await api.request<{ items: Profile[] | null }>('GET', '/api/v1/profiles')
    setAccounts(a.items || [])
    setProfiles(p.items || [])
    if ((a.items || [])[0]) setAccount((a.items || [])[0].ID)
  }
  useEffect(() => { load().catch((err: Error) => setError(err.message)) }, [])

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">Agent 配置</h1>
      <Notice>这里的配置是工具和账号的组合，不是“一个模型”。doctor 只做静态检查，不会发起付费调用。</Notice>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      <Card title="账号引用">
        <Button onClick={() => void api.request('POST', '/api/v1/accounts', { auth_kind: 'manual_import', credential_ref: 'ref:local-fixture', concurrency: 1 }).then(load).catch((err: Error) => setError(err.message))}>保存本地假 CLI 引用</Button>
        {accounts.length === 0 ? <p className="mt-2 text-sm">还没有账号引用。</p> : (
          <ul className="mt-2 text-sm">{accounts.map((item) => <li key={item.ID}>{item.AuthKind} · {item.CredentialRef}</li>)}</ul>
        )}
      </Card>
      <Card title="新配置">
        <div className="flex flex-col gap-2">
          <Input value={name} onChange={(e) => setName(e.target.value)} />
          <label className="text-sm">假 CLI 模式
            <select className="mt-1 w-full rounded-md border bg-white px-3 py-2" value={mode} onChange={(e) => setMode(e.target.value)}>
              <option value="empty">empty · 不改文件</option>
              <option value="success">success · 用参考解校准通过</option>
              <option value="fail">fail · 错误补丁</option>
              <option value="forge">forge · 伪造 PASS</option>
            </select>
          </label>
          <Button disabled={!account} onClick={() => void api.request('POST', '/api/v1/profiles', { name, account_id: account, profile: { adapter: 'fixture', model: 'fixture-local', fake_mode: mode, executor: 'native-trusted', network: 'unrestricted', display_name: name } }).then(load).catch((err: Error) => setError(err.message))}>创建草稿</Button>
        </div>
        {profiles.length === 0 ? <p className="mt-3 text-sm">还没有配置。</p> : (
          <ul className="mt-3 flex flex-col gap-2 text-sm">
            {profiles.map((profile) => (
              <li key={profile.ID} className="flex flex-wrap gap-2">
                <span>{profile.Name}</span>
                <Button tone="quiet" onClick={() => void api.request<Report>('POST', `/api/v1/profiles/${profile.ID}/doctor`, { allow_model_call: false }).then((data) => setReport(`${data.note || ''} 模型调用 ${data.model_call || 'not_run'}，网络 ${data.network || 'unrestricted'}`)).catch((err: Error) => setError(err.message))}>doctor</Button>
                <Button onClick={() => void api.request('POST', `/api/v1/profiles/${profile.ID}/publish`).then(() => setReport('已发布。占位字段和未通过 doctor 的配置不能发布。')).catch((err: Error) => setError(err.message))}>发布</Button>
              </li>
            ))}
          </ul>
        )}
        {report ? <p className="mt-3 text-sm">{report}</p> : null}
      </Card>
    </div>
  )
}
