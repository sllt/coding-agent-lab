// Incremental attempt event log. The server streams one NDJSON event per SSE
// message; the client appends and de-duplicates by sequence instead of
// re-downloading the whole log on every message.

export type AgentEvent = {
  sequence: number
  type?: string
  origin?: string
  observed_at?: string
  payload?: unknown
  raw: string
}

export type EventLog = { events: AgentEvent[]; last: number; gap: boolean }

export const emptyLog: EventLog = { events: [], last: 0, gap: false }

export function parseEvent(data: string): AgentEvent | null {
  const raw = data.trim()
  if (!raw.startsWith('{')) return null
  try {
    const doc = JSON.parse(raw) as Omit<AgentEvent, 'raw'>
    if (typeof doc.sequence !== 'number') return null
    return { ...doc, raw }
  } catch {
    return null
  }
}

/** appendEvents returns a new log with unseen events added in order. */
export function appendEvents(log: EventLog, incoming: AgentEvent[]): EventLog {
  const fresh = incoming.filter((ev) => ev.sequence > log.last)
  if (fresh.length === 0) return log
  fresh.sort((a, b) => a.sequence - b.sequence)
  const events = log.events.concat(fresh)
  return { events, last: events[events.length - 1].sequence, gap: log.gap }
}

/** parseSSE parses a complete text/event-stream body (the ?once=1 replay). */
export function parseSSE(text: string): AgentEvent[] {
  const out: AgentEvent[] = []
  for (const block of text.split('\n\n')) {
    const data = block
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).trimStart())
      .join('\n')
    const ev = data ? parseEvent(data) : null
    if (ev) out.push(ev)
  }
  return out
}
