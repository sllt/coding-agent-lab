import { useEffect, useState } from 'react'
import { api } from '../api.ts'
import { Button, Card, Notice, TextArea } from '../components/ui.tsx'

type Settings = { executor_default: string; executor_note: string; network_restricted: string; credential_display: string }

export function Settings() {
  const [settings, setSettings] = useState<Settings | null>(null)
  const [harbor, setHarbor] = useState('{"schema_version":"harbor.subset/v1","name":"示例","instruction":"说明任务","environment":{"image":"未映射"}}')
  const [result, setResult] = useState('')
  const [error, setError] = useState('')
  useEffect(() => { api.request<Settings>('GET', '/api/v1/settings').then(setSettings).catch((err: Error) => setError(err.message)) }, [])

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

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl">设置</h1>
      {error ? <Notice tone="warn">{error}</Notice> : null}
      {settings ? (
        <Card title="执行与凭据">
          <p className="text-sm">默认执行器 {settings.executor_default}。{settings.executor_note}</p>
          <p className="mt-2 text-sm">网络限制：{settings.network_restricted}。{settings.credential_display}。</p>
        </Card>
      ) : <Notice>正在读取设置。</Notice>}
      <Card title="备份">
        <Button onClick={() => void api.request<{ path: string; note: string }>('POST', '/api/v1/maintenance/backup').then((data) => setResult(`${data.note} ${data.path}`)).catch((err: Error) => setError(err.message))}>生成一致性备份</Button>
        <p className="mt-2 text-sm">仍有未结束的 Attempt 时不会生成备份。</p>
      </Card>
      <Card title="Harbor 子集">
        <TextArea rows={6} value={harbor} onChange={(e) => setHarbor(e.target.value)} />
        <Button className="mt-2" onClick={mapHarbor}>查看映射</Button>
        <p className="mt-2 text-sm">未映射字段会列出来。这里不会自动合并代码。</p>
      </Card>
      {result ? <pre className="overflow-x-auto whitespace-pre-wrap text-xs">{result}</pre> : null}
    </div>
  )
}
