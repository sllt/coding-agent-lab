import { useEffect, useState } from 'react'
import { api } from '../api.ts'
import { Button, Card, Input, Notice, TextArea } from '../components/ui.tsx'

type Settings = {
  executor_default: string
  executor_note: string
  network_restricted: string
  credential_display: string
  global_limit: number
  disk_quota_bytes: number
  release_rss: string
  process_rss_bytes: number
  process_rss_note: string
  retention: { log_days: number; patch_days: number; report_days: number }
}
type Audit = { id: string; action: string; resource_id: string; at: string; detail_json: string }

export function Settings() {
  const [settings, setSettings] = useState<Settings | null>(null)
  const [failed, setFailed] = useState(false)
  const [harbor, setHarbor] = useState('{"schema_version":"harbor.subset/v1","name":"示例","instruction":"说明任务","environment":{"image":"未映射"}}')
  const [result, setResult] = useState('')
  const [error, setError] = useState('')
  const [audit, setAudit] = useState<Audit[]>([])
  const [limit, setLimit] = useState('1')
  const [logDays, setLogDays] = useState('30')
  const [patchDays, setPatchDays] = useState('0')
  const [reportDays, setReportDays] = useState('365')
  const [quota, setQuota] = useState('0')

  function applySettings(data: Settings) {
    setSettings(data)
    setFailed(false)
    setLimit(String(data.global_limit || 1))
    setLogDays(String(data.retention?.log_days ?? 30))
    setPatchDays(String(data.retention?.patch_days ?? 0))
    setReportDays(String(data.retention?.report_days ?? 365))
    setQuota(String(data.disk_quota_bytes || 0))
  }

  useEffect(() => {
    api.request<Settings>('GET', '/api/v1/settings').then(applySettings).catch((err: Error) => { setError(err.message); setFailed(true) })
  }, [])

  function mapHarbor() {
    let parsed: unknown
    try {
      parsed = JSON.parse(harbor)
    } catch {
      setResult('')
      setError('Harbor 文本不是合法 JSON。')
      return
    }
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
      setResult('')
      setError('Harbor 文本必须是一个 JSON 对象。')
      return
    }
    setError('')
    void api.request('POST', '/api/v1/imports/harbor', parsed).then((data) => setResult(JSON.stringify(data, null, 2))).catch((err: Error) => setError(err.message))
  }

  function savePolicy() {
    setError('')
    void api.request<Settings>('PATCH', '/api/v1/settings', {
      global_limit: Number(limit),
      disk_quota_bytes: Number(quota),
      retention: { log_days: Number(logDays), patch_days: Number(patchDays), report_days: Number(reportDays) },
    }).then((data) => { applySettings(data); setResult('已保存并发、保留和磁盘配额。补丁天数 0 表示不按时间删除补丁。') }).catch((err: Error) => setError(err.message))
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">设置</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {failed ? <Notice tone="warn">设置没有读到。这不是一份空配置。</Notice> : settings ? (
        <Card title="执行与凭据">
          <p className="text-sm">默认执行器 {settings.executor_default}。{settings.executor_note}</p>
          <p className="mt-2 text-sm">网络限制：{settings.network_restricted}。{settings.credential_display}。</p>
          <p className="mt-2 text-sm">发布用峰值 RSS：{settings.release_rss}。{settings.process_rss_note} 当前进程约 {settings.process_rss_bytes} 字节。</p>
        </Card>
      ) : <Notice>正在读取设置。</Notice>}
      <Card title="并发、保留和配额">
        <div className="grid gap-2 sm:grid-cols-2">
          <label className="text-sm">全局并发（1–8）<Input value={limit} onChange={(e) => setLimit(e.target.value)} /></label>
          <label className="text-sm">磁盘配额字节，0 表示不设水位<Input value={quota} onChange={(e) => setQuota(e.target.value)} /></label>
          <label className="text-sm">日志保留天数<Input value={logDays} onChange={(e) => setLogDays(e.target.value)} /></label>
          <label className="text-sm">补丁保留天数，0 为不删<Input value={patchDays} onChange={(e) => setPatchDays(e.target.value)} /></label>
          <label className="text-sm">报告保留天数<Input value={reportDays} onChange={(e) => setReportDays(e.target.value)} /></label>
        </div>
        <Button className="mt-3" onClick={savePolicy}>保存策略</Button>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button tone="quiet" onClick={() => void api.request('POST', '/api/v1/maintenance/retention', { apply: false }).then((data) => setResult(JSON.stringify(data, null, 2))).catch((err: Error) => setError(err.message))}>预览保留清理</Button>
          <Button tone="quiet" onClick={() => void api.request('POST', '/api/v1/maintenance/retention', { apply: true }).then((data) => setResult(JSON.stringify(data, null, 2))).catch((err: Error) => setError(err.message))}>按策略删除过期日志和报告</Button>
        </div>
        <p className="mt-2 text-sm">删除前先看预览。原始日志和过期报告删了不能从这里恢复。被留下的补丁和检查摘要还在。</p>
      </Card>
      <Card title="隔离清理">
        <Button onClick={() => void api.request('POST', '/api/v1/maintenance/quarantine', {}).then((data) => setResult(JSON.stringify(data, null, 2))).catch((err: Error) => setError(err.message))}>清理隔离工作区</Button>
        <p className="mt-2 text-sm">只删除 runner 标成隔离的工作区副本。补丁、事件和检查摘要留下。不会合并到源目录。</p>
      </Card>
      <Card title="审计">
        <Button tone="quiet" onClick={() => void api.request<{ items: Audit[] | null }>('GET', '/api/v1/audit').then((data) => setAudit(data.items || [])).catch((err: Error) => setError(err.message))}>读取审计</Button>
        {audit.length === 0 ? <p className="mt-2 text-sm">还没有展开审计列表。点上面的按钮读取，空结果会写在这里，不会和读取失败混在一起。</p> : (
          <ul className="mt-2 flex flex-col gap-1 text-xs">
            {audit.map((item) => <li key={item.id}>{item.at} · {item.action} · {item.resource_id}</li>)}
          </ul>
        )}
      </Card>
      <Card title="升级">
        <Button tone="quiet" onClick={() => void api.request('GET', '/api/v1/upgrade').then((data) => setResult(JSON.stringify(data, null, 2))).catch((err: Error) => setError(err.message))}>查看升级路径</Button>
        <p className="mt-2 text-sm">升级前要先备份。没有向下迁移，也不会顺便更新三个 CLI。</p>
      </Card>
      <Card title="备份">
        <Button onClick={() => void api.request<{ path: string; note: string }>('POST', '/api/v1/maintenance/backup').then((data) => setResult(`${data.note} ${data.path}`)).catch((err: Error) => setError(err.message))}>生成一致性备份</Button>
        <p className="mt-2 text-sm">仍有未结束的 Attempt 时不会生成备份。</p>
      </Card>
      <Card title="Harbor 子集">
        <TextArea rows={6} value={harbor} onChange={(e) => setHarbor(e.target.value)} />
        <div className="mt-2 flex flex-wrap gap-2">
          <Button onClick={mapHarbor}>查看映射</Button>
          <Button tone="quiet" onClick={() => void api.request('POST', '/api/v1/exports/harbor', { name: '示例', prompt: '说明任务' }).then((data) => setResult(JSON.stringify(data, null, 2))).catch((err: Error) => setError(err.message))}>导出子集</Button>
        </div>
        <p className="mt-2 text-sm">未映射字段会列出来。这里不会自动合并代码，也不会把子集收成已发布任务。</p>
      </Card>
      {result ? <pre className="overflow-x-auto whitespace-pre-wrap text-xs">{result}</pre> : null}
    </div>
  )
}
