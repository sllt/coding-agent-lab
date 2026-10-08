import { expect, test } from 'vitest'
import { stableSubmitKey } from './submitKey.ts'

test('double click reuses the key and a new visit does not', () => {
  const payload = '{"mode":"agent_profile"}'
  const first = { payload: '', key: '' }
  const key = stableSubmitKey(first, payload)
  const again = stableSubmitKey({ payload, key }, payload)
  expect(again).toBe(key)
  const afterSuccess = stableSubmitKey({ payload: '', key: '' }, payload)
  expect(afterSuccess).not.toBe(key)
})

test('a changed payload gets a new key', () => {
  const key = stableSubmitKey({ payload: 'a', key: 'same' }, 'b')
  expect(key).not.toBe('same')
})
