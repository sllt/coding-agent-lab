import { ApiError, requestJSON } from './generated/client.ts'

export { ApiError }

type UnauthorizedHandler = () => void

let onUnauthorized: UnauthorizedHandler | null = null

/** setUnauthorizedHandler registers the callback that returns to login. */
export function setUnauthorizedHandler(fn: UnauthorizedHandler | null) {
  onUnauthorized = fn
}

export const api = {
  csrf: '',
  async request<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
    try {
      return await requestJSON<T>({ method, path, body, headers, csrf: this.csrf })
    } catch (err) {
      const e = err instanceof ApiError ? err : new ApiError(0, 'unknown', err instanceof Error ? err.message : '请求失败')
      // A 401 on anything but the login/session probe means the session
      // expired: drop the CSRF token and go back to the login screen.
      if (e.status === 401 && !path.endsWith('/login') && !path.endsWith('/session') && !path.endsWith('/setup')) {
        this.csrf = ''
        onUnauthorized?.()
      }
      throw e
    }
  },
}
