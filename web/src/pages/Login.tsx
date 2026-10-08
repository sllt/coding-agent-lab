import { FlaskConical, KeyRound, Loader2, ShieldCheck } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../api.ts'
import { Alert, Button, Field, Input } from '../components/ui/index.ts'

export function Login({ onReady }: { onReady: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<'' | 'login' | 'setup'>('')
  const valid = username.trim() !== '' && password.length >= 8

  async function run(kind: 'login' | 'setup', e?: FormEvent) {
    e?.preventDefault()
    if (!valid || busy) return
    setBusy(kind)
    setError('')
    try {
      if (kind === 'setup') await api.request('POST', '/api/v1/setup', { username: username.trim(), password })
      const data = await api.request<{ csrf_token: string }>('POST', '/api/v1/login', { username: username.trim(), password })
      api.csrf = data.csrf_token
      onReady()
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) setError(`${err.message}（约 ${err.retryAfter ?? 60} 秒后可重试）`)
      else if (err instanceof ApiError && err.status === 0) setError('无法连接到控制面，请确认 agentlab serve 正在运行')
      else setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="grid min-h-screen lg:grid-cols-[1fr_1.1fr]">
      <div className="relative hidden overflow-hidden border-r bg-sidebar lg:flex lg:flex-col lg:justify-between lg:p-10">
        <div className="flex items-center gap-2.5">
          <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"><FlaskConical className="size-4" /></div>
          <span className="text-sm font-semibold">Agent Lab</span>
        </div>
        <div className="max-w-md space-y-6">
          <h2 className="text-3xl font-semibold leading-tight tracking-tight">在你自己的机器上，<br />用证据比较 Coding Agent。</h2>
          <ul className="space-y-3 text-sm text-muted-foreground">
            <li className="flex gap-2.5"><ShieldCheck className="mt-0.5 size-4 shrink-0 text-success" />独立验证器给结论，Agent 自称的 PASS 不算数</li>
            <li className="flex gap-2.5"><KeyRound className="mt-0.5 size-4 shrink-0 text-primary" />凭据只存环境变量名，密钥不入库、不回显</li>
            <li className="flex gap-2.5"><FlaskConical className="mt-0.5 size-4 shrink-0 text-info" />冻结的任务与配置版本，可复现、可对比</li>
          </ul>
        </div>
        <p className="text-xs text-muted-foreground">只监听本机 loopback</p>
        <div className="pointer-events-none absolute -right-24 -top-24 size-96 rounded-full bg-primary/10 blur-3xl" />
      </div>
      <div className="flex items-center justify-center p-6">
        <form className="w-full max-w-sm space-y-6" onSubmit={(e) => void run('login', e)}>
          <div className="space-y-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">登录</h1>
            <p className="text-sm text-muted-foreground">第一次使用请填写用户名和密码后点击「初始化管理员」。</p>
          </div>
          <div className="space-y-4">
            <Field label="用户名" htmlFor="username">
              <Input id="username" autoComplete="username" autoFocus value={username} onChange={(e) => setUsername(e.target.value)} />
            </Field>
            <Field label="密码" htmlFor="password" hint="至少 8 位">
              <Input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
            </Field>
          </div>
          {error ? <Alert tone="danger">{error}</Alert> : null}
          <div className="space-y-2">
            <Button type="submit" className="w-full" disabled={!valid || busy !== ''}>
              {busy === 'login' ? <Loader2 className="animate-spin" /> : null}登录
            </Button>
            <Button variant="outline" className="w-full" disabled={!valid || busy !== ''} onClick={() => void run('setup')}>
              {busy === 'setup' ? <Loader2 className="animate-spin" /> : null}初始化管理员
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}
