import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, setUnauthorizedHandler } from './api.ts'

function respond(status: number, body: unknown, headers: Record<string, string> = {}) {
  return vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status, headers }))
}

afterEach(() => {
  vi.unstubAllGlobals()
  setUnauthorizedHandler(null)
  api.csrf = ''
})

describe('api.request', () => {
  it('carries the real HTTP status and error code', async () => {
    vi.stubGlobal('fetch', respond(409, { error: { code: 'busy', message: '已有备份正在进行' }, request_id: 'r1' }))
    const err = await api.request('POST', '/api/v1/maintenance/backup').catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(409)
    expect(err.code).toBe('busy')
    expect(err.requestId).toBe('r1')
  })

  it('does not guess 401 from the message text', async () => {
    vi.stubGlobal('fetch', respond(500, { error: { code: 'internal', message: 'upstream said 401' } }))
    const err = await api.request('GET', '/api/v1/overview').catch((e) => e)
    expect(err.status).toBe(500)
  })

  it('returns to login on 401 and clears the csrf token', async () => {
    const back = vi.fn()
    setUnauthorizedHandler(back)
    api.csrf = 'tok'
    vi.stubGlobal('fetch', respond(401, { error: { code: 'unauthenticated', message: '需要登录' } }))
    await api.request('GET', '/api/v1/overview').catch(() => undefined)
    expect(back).toHaveBeenCalledOnce()
    expect(api.csrf).toBe('')
  })

  it('reports Retry-After on lockout', async () => {
    vi.stubGlobal('fetch', respond(429, { error: { code: 'locked', message: '稍后再试' } }, { 'Retry-After': '61' }))
    const err = await api.request('POST', '/api/v1/login', { username: 'a', password: 'b' }).catch((e) => e)
    expect(err.status).toBe(429)
    expect(err.retryAfter).toBe(61)
  })
})
