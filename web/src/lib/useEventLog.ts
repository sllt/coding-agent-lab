import { useEffect, useState } from 'react'
import { appendEvents, emptyLog, parseEvent, parseSSE, type EventLog } from './events.ts'

type Keyed = { id: string; log: EventLog; loading: boolean }

/**
 * useEventLog follows one attempt's event log. A live attempt uses an
 * EventSource (the server sends the backlog, then each new event once, and
 * resumes from Last-Event-ID on reconnect). A finished attempt is a single
 * ?once=1 replay. State is keyed by attempt so switching attempts never shows
 * the previous attempt's events.
 */
export function useEventLog(attemptId: string | undefined, live: boolean) {
  const [state, setState] = useState<Keyed>({ id: '', log: emptyLog, loading: false })
  useEffect(() => {
    if (!attemptId) return
    let stop = false
    let source: EventSource | null = null
    const add = (events: ReturnType<typeof parseSSE>, loading = false) => {
      if (stop) return
      setState((prev) => {
        const base = prev.id === attemptId ? prev.log : emptyLog
        return { id: attemptId, log: appendEvents(base, events), loading }
      })
    }
    const url = `/api/v1/attempts/${attemptId}/events`
    if (!live || typeof EventSource === 'undefined') {
      void fetch(`${url}?once=1`, { credentials: 'include' })
        .then((res) => (res.ok ? res.text() : ''))
        .then((text) => add(parseSSE(text)))
        .catch(() => add([]))
    } else {
      source = new EventSource(url, { withCredentials: true })
      source.onopen = () => add([])
      source.onmessage = (msg) => {
        const ev = parseEvent(msg.data)
        if (ev) add([ev])
      }
      source.addEventListener('gap', () => {
        if (!stop) setState((prev) => (prev.id === attemptId ? { ...prev, log: { ...prev.log, gap: true } } : prev))
      })
    }
    return () => {
      stop = true
      source?.close()
    }
  }, [attemptId, live])
  if (!attemptId) return { log: emptyLog, loading: false }
  if (state.id !== attemptId) return { log: emptyLog, loading: true }
  return { log: state.log, loading: state.loading }
}
