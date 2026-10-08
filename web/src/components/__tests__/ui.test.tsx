// @vitest-environment happy-dom
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ReadinessBadge, StateBadge, VerdictBadge, VerdictBar } from '../status.tsx'
import { ConfirmButton, TooltipProvider } from '../ui/index.ts'

afterEach(cleanup)

describe('status badges', () => {
  it('labels execution states and verdicts in Chinese', () => {
    render(<TooltipProvider><StateBadge state="verifying" /><VerdictBadge verdict="inconclusive" /><ReadinessBadge readiness="needs_credentials" /></TooltipProvider>)
    expect(screen.getByText('独立验收')).toBeInTheDocument()
    expect(screen.getByText('证据不足')).toBeInTheDocument()
    expect(screen.getByText('缺少凭据')).toBeInTheDocument()
  })

  it('says "未检测" when no doctor has run', () => {
    render(<ReadinessBadge readiness={null} />)
    expect(screen.getByText('未检测')).toBeInTheDocument()
  })

  it('draws verdict segments proportional to counts', () => {
    const { container } = render(<VerdictBar counts={{ pass: 3, fail: 1 }} total={4} />)
    const segs = (container.firstElementChild as HTMLElement).children
    expect(segs).toHaveLength(2)
    expect((segs[0] as HTMLElement).style.width).toBe('75%')
  })
})

describe('ConfirmButton', () => {
  it('runs nothing until the confirm word is typed', async () => {
    const onConfirm = vi.fn()
    render(<TooltipProvider><ConfirmButton title="删除？" description="不可恢复" typeToConfirm="删除" confirmLabel="永久删除" onConfirm={onConfirm}>按策略删除</ConfirmButton></TooltipProvider>)
    fireEvent.click(screen.getByRole('button', { name: '按策略删除' }))
    const confirm = await screen.findByRole('button', { name: '永久删除' })
    expect(confirm).toBeDisabled()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '删除' } })
    expect(confirm).toBeEnabled()
    fireEvent.click(confirm)
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1))
  })

  it('cancel closes without running the action', async () => {
    const onConfirm = vi.fn()
    render(<TooltipProvider><ConfirmButton title="取消 Trial？" description="x" onConfirm={onConfirm}>取消</ConfirmButton></TooltipProvider>)
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(Array.from(dialog.querySelectorAll('button')).find((b) => b.textContent === '取消')!)
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(onConfirm).not.toHaveBeenCalled()
  })
})
