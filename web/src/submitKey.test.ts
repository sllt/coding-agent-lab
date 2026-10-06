import assert from 'node:assert/strict'
import test from 'node:test'
import { stableSubmitKey } from './submitKey.ts'

test('double click reuses the key and a new visit does not', () => {
  const payload = '{"mode":"agent_profile"}'
  const first = { payload: '', key: '' }
  const key = stableSubmitKey(first, payload)
  const again = stableSubmitKey({ payload, key }, payload)
  assert.equal(again, key)
  const afterSuccess = stableSubmitKey({ payload: '', key: '' }, payload)
  assert.notEqual(afterSuccess, key)
})

test('a changed payload gets a new key', () => {
  const key = stableSubmitKey({ payload: 'a', key: 'same' }, 'b')
  assert.notEqual(key, 'same')
})
