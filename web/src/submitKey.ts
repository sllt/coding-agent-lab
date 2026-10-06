export type SubmitKey = { payload: string; key: string }

// stableSubmitKey keeps one key for the same payload until the caller clears it
// after a successful response. A lost response retries with the same key.
export function stableSubmitKey(current: SubmitKey, payload: string) {
  if (current.payload === payload && current.key) return current.key
  return newSubmitKey()
}

export function newSubmitKey() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `ui-${Date.now()}-${Math.random().toString(16).slice(2)}`
}
