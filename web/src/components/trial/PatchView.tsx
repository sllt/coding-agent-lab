import { FileDiff, FileMinus, FilePlus, FileText } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { bytes, shortId } from '../../lib/format.ts'
import type { Artifact } from '../../lib/types.ts'
import { Alert, Badge, Empty, Skeleton } from '../ui/index.ts'

type Change = { path?: string; action?: string; size?: number; binary?: boolean; digest?: string }
type Patch = { base_digest?: string; digest?: string; changes?: Change[] }

export function PatchView({ artifacts }: { artifacts: Artifact[] }) {
  const item = artifacts.find((a) => a.kind === 'patch')
  const usable = item && item.status !== 'missing' && item.status !== 'tampered'
  const q = useQuery({
    queryKey: ['patch', item?.id],
    enabled: !!usable,
    staleTime: Infinity,
    queryFn: async () => {
      const res = await fetch(`/api/v1/artifacts/${item!.id}/download`, { credentials: 'include' })
      if (!res.ok) throw new Error(`下载失败（${res.status}）`)
      return res.text()
    },
  })
  if (!item) return <Empty icon={<FileDiff />} title="没有改动包" description="Agent 完成后会在这里列出它改动的文件。这里只显示文本，不会执行 Agent 输出。" />
  if (item.status === 'tampered') return <Alert tone="danger" className="m-4">改动包内容和登记的摘要不一致，已拒绝显示。</Alert>
  if (item.status === 'missing') return <Alert tone="warning" className="m-4">改动包文件已缺失（可能被保留策略清理）。</Alert>
  if (q.isLoading) return <div className="space-y-2 p-4">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-8" />)}</div>
  if (q.error) return <Alert tone="danger" className="m-4">{String((q.error as Error).message)}</Alert>
  let doc: Patch | null = null
  try { doc = JSON.parse(q.data ?? '') as Patch } catch { doc = null }
  if (!doc?.changes) return <pre className="m-4 max-h-[60vh] overflow-auto whitespace-pre-wrap rounded-md bg-muted/60 p-3 font-mono text-xs">{q.data}</pre>
  const counts = { add: 0, modify: 0, delete: 0 }
  for (const c of doc.changes) {
    if (c.action === 'add' || c.action === 'create') counts.add++
    else if (c.action === 'delete' || c.action === 'remove') counts.delete++
    else counts.modify++
  }
  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 border-b px-4 py-2.5 text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{doc.changes.length} 个文件</span>
        <span className="text-success">+{counts.add} 新增</span>
        <span className="text-warning">~{counts.modify} 修改</span>
        <span className="text-danger">−{counts.delete} 删除</span>
        <span className="ml-auto font-mono">base {shortId(doc.base_digest, 10)} → {shortId(doc.digest, 10)}</span>
        <a className="font-medium text-primary hover:underline" href={`/api/v1/artifacts/${item.id}/download`}>下载</a>
      </div>
      {doc.changes.length === 0 ? <Empty title="Agent 没有改动任何文件" /> : (
        <ul className="divide-y">
          {doc.changes.map((c, i) => {
            const Icon = c.action === 'delete' || c.action === 'remove' ? FileMinus : c.action === 'add' || c.action === 'create' ? FilePlus : FileText
            const color = c.action === 'delete' || c.action === 'remove' ? 'text-danger' : c.action === 'add' || c.action === 'create' ? 'text-success' : 'text-warning'
            return (
              <li key={`${c.path}-${i}`} className="flex items-center gap-3 px-4 py-2 text-[13px]">
                <Icon className={`size-4 shrink-0 ${color}`} />
                <span className="min-w-0 flex-1 truncate font-mono text-xs">{c.path}</span>
                {c.binary ? <Badge tone="outline">二进制</Badge> : null}
                <span className="shrink-0 text-xs text-muted-foreground tabular">{bytes(c.size ?? 0)}</span>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
