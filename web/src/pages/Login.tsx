import { useState } from 'react'
import { api } from '../api.ts'
import { Button, Card, Input, Notice } from '../components/ui.tsx'

export function Login(props: { onReady: () => void }) {
  const [username, setUsername] = useState('malong')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function setup() {
    setBusy(true)
    setError('')
    try {
      await api.request('POST', '/api/v1/setup', { username, password })
      await login()
    } catch (err) {
      setError(err instanceof Error ? err.message : '初始化失败')
      setBusy(false)
    }
  }

  async function login() {
    setBusy(true)
    setError('')
    try {
      const data = await api.request<{ csrf_token: string }>('POST', '/api/v1/login', { username, password })
      api.csrf = data.csrf_token
      props.onReady()
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col justify-center gap-4 px-4">
      <div>
        <p className="text-sm text-[#6b6258]">个人工作台</p>
        <h1 className="text-3xl">Coding Agent Lab</h1>
      </div>
      <Card title="本机管理员">
        <div className="flex flex-col gap-3">
          <label className="text-sm">用户名<Input value={username} onChange={(e) => setUsername(e.target.value)} /></label>
          <label className="text-sm">密码（至少 8 位）<Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></label>
          {error ? <Notice tone="warn">{error}</Notice> : <Notice>第一次使用先初始化。之后用同一组密码登录。密钥不会出现在这个页面上。</Notice>}
          <div className="flex gap-2">
            <Button disabled={busy || password.length < 8} onClick={() => void setup()}>初始化</Button>
            <Button tone="quiet" disabled={busy || password.length < 8} onClick={() => void login()}>登录</Button>
          </div>
        </div>
      </Card>
    </main>
  )
}
