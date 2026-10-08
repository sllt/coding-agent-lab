import { describe, expect, it } from 'vitest'
import { appendEvents, emptyLog, parseEvent, parseSSE } from './events.ts'

describe('event log', () => {
  it('appends only unseen sequences in order', () => {
    const a = parseEvent('{"sequence":1,"type":"Ready"}')!
    const b = parseEvent('{"sequence":2,"type":"PhaseChanged"}')!
    let log = appendEvents(emptyLog, [b, a])
    expect(log.events.map((e) => e.sequence)).toEqual([1, 2])
    log = appendEvents(log, [b])
    expect(log.events).toHaveLength(2)
    expect(log.last).toBe(2)
  })

  it('parses a replay body and ignores comments and partial lines', () => {
    const body = 'id: a:1\ndata: {"sequence":1}\n\n: heartbeat\n\nid: a:2\ndata: {"sequence":2,"type":"x"}\n\ndata: {"seq'
    expect(parseSSE(body).map((e) => e.sequence)).toEqual([1, 2])
  })
})
