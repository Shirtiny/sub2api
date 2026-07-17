import { describe, expect, it } from 'vitest'
import { buildAccountExtraPatch, isAccountExtraPatchEmpty } from '@/utils/accountExtraPatch'

describe('accountExtraPatch', () => {
  it('sends only changed keys and preserves runtime-owned keys', () => {
    const patch = buildAccountExtraPatch(
      {
        mode: 'off',
        passive_usage_sampled_at: '2026-07-15T00:00:00Z',
        nested: { b: 2, a: 1 }
      },
      {
        mode: 'passthrough',
        passive_usage_sampled_at: '2026-07-15T00:00:00Z',
        nested: { a: 1, b: 2 }
      }
    )

    expect(patch).toEqual({ set: { mode: 'passthrough' }, delete: [] })
  })

  it('represents explicit deletion without echoing unchanged fields', () => {
    expect(buildAccountExtraPatch(
      { legacy: true, keep: [1, { value: 'x' }] },
      { keep: [1, { value: 'x' }] }
    )).toEqual({ set: {}, delete: ['legacy'] })
  })

  it('sends only changed Aether WS nested keys', () => {
    expect(buildAccountExtraPatch(
      { aether_ws: { enabled: false, future_option: 'keep' } },
      { aether_ws: { enabled: true, future_option: 'keep' } }
    )).toEqual({
      set: { aether_ws: { enabled: true } },
      delete: []
    })
  })

  it('recognizes an empty patch', () => {
    expect(isAccountExtraPatchEmpty(buildAccountExtraPatch({ keep: true }, { keep: true }))).toBe(true)
  })
})
