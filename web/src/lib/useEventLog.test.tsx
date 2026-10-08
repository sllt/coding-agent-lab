// @vitest-environment happy-dom
import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useEventLog } from './useEventLog.ts'

const sse = (n: number[]) => n.map((s) => `id: ${s}\ndata: {"sequence":${s},"type":"Message"}\n\n`).join('')

afterEach(() => vi.unstubAllGlobals())

describe('useEventLog', () => {
  it('replays a finished attempt once and never mixes attempts', async () => {
    const fetchMock = vi.fn(async (url: string) => new Response(url.includes('att_a') ? sse([1, 2, 3]) : sse([7])))
    vi.stubGlobal('fetch', fetchMock)
    const { result, rerender } = renderHook(({ id }) => useEventLog(id, false), { initialProps: { id: 'att_a' } })
    await waitFor(() => expect(result.current.log.events.map((e) => e.sequence)).toEqual([1, 2, 3]))
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/attempts/att_a/events?once=1', expect.anything())
    rerender({ id: 'att_b' })
    expect(result.current.log.events).toHaveLength(0)
    await waitFor(() => expect(result.current.log.events.map((e) => e.sequence)).toEqual([7]))
  })
})
