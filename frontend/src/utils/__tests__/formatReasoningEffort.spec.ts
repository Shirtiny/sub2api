import { describe, expect, it } from 'vitest'
import { formatReasoningEffort } from '../format'

describe('formatReasoningEffort', () => {
  it('formats max and ultra explicitly for usage tables', () => {
    expect(formatReasoningEffort('max')).toBe('Max')
    expect(formatReasoningEffort('MAX')).toBe('Max')
    expect(formatReasoningEffort('ultra')).toBe('Ultra')
    expect(formatReasoningEffort(' ULTRA ')).toBe('Ultra')
  })

  it('keeps existing effort formatting behavior', () => {
    expect(formatReasoningEffort('low')).toBe('Low')
    expect(formatReasoningEffort('medium')).toBe('Medium')
    expect(formatReasoningEffort('high')).toBe('High')
    expect(formatReasoningEffort('x-high')).toBe('XHigh')
    expect(formatReasoningEffort(null)).toBe('-')
  })
})
