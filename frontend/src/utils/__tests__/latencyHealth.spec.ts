import { describe, expect, it } from 'vitest'

import {
  calculateOutputTokensPerSecond,
  durationSeverity,
  firstByteSeverity,
  formatLatencyDuration,
} from '../latencyHealth'

describe('latencyHealth', () => {
  it('classifies TTFB at 10s/30s/60s boundaries', () => {
    expect(firstByteSeverity(0)).toBe('good')
    expect(firstByteSeverity(9_999)).toBe('good')
    expect(firstByteSeverity(10_000)).toBe('warn')
    expect(firstByteSeverity(29_999)).toBe('warn')
    expect(firstByteSeverity(30_000)).toBe('slow')
    expect(firstByteSeverity(59_999)).toBe('slow')
    expect(firstByteSeverity(60_000)).toBe('critical')
  })

  it('classifies total duration at 1min/3min/5min boundaries', () => {
    expect(durationSeverity(0)).toBe('good')
    expect(durationSeverity(59_999)).toBe('good')
    expect(durationSeverity(60_000)).toBe('warn')
    expect(durationSeverity(179_999)).toBe('warn')
    expect(durationSeverity(180_000)).toBe('slow')
    expect(durationSeverity(299_999)).toBe('slow')
    expect(durationSeverity(300_000)).toBe('critical')
  })

  it('calculates sync TPS from the complete request duration', () => {
    expect(calculateOutputTokensPerSecond(90, 4_000)).toBe(23)
    expect(calculateOutputTokensPerSecond(10, 4_000)).toBe(3)
    expect(calculateOutputTokensPerSecond(9, 3_000)).toBe(3)
  })

  it('calculates stream TPS from the generation window after first byte', () => {
    expect(calculateOutputTokensPerSecond(50, 1_000, 500, true)).toBe(100)
    expect(calculateOutputTokensPerSecond(90, 4_000, 1_000, true)).toBe(30)
  })

  it('returns a safe placeholder for invalid timing or token values', () => {
    expect(calculateOutputTokensPerSecond(null, 1_000)).toBeNull()
    expect(calculateOutputTokensPerSecond(-1, 1_000)).toBeNull()
    expect(calculateOutputTokensPerSecond(0, 1_000)).toBeNull()
    expect(calculateOutputTokensPerSecond(1, 0)).toBeNull()
    expect(calculateOutputTokensPerSecond(1, Number.NaN)).toBeNull()
    expect(calculateOutputTokensPerSecond(Number.POSITIVE_INFINITY, 1_000)).toBeNull()
    expect(calculateOutputTokensPerSecond(Number.MAX_SAFE_INTEGER, 1)).toBeNull()
    expect(calculateOutputTokensPerSecond(1, 1_000, null, true)).toBeNull()
    expect(calculateOutputTokensPerSecond(1, 1_000, -1, true)).toBeNull()
    expect(calculateOutputTokensPerSecond(1, 1_000, 1_000, true)).toBeNull()
    expect(calculateOutputTokensPerSecond(1, 1_000, 1_001, true)).toBeNull()
    expect(calculateOutputTokensPerSecond(1, 1_000, Number.NaN, true)).toBeNull()
    expect(calculateOutputTokensPerSecond(300, 1_000, 950, true)).toBeNull()
  })

  it('formats long durations compactly', () => {
    expect(formatLatencyDuration(999)).toBe('999ms')
    expect(formatLatencyDuration(60_000)).toBe('1m 0s')
    expect(formatLatencyDuration(3_661_000)).toBe('1h 1m')
    expect(formatLatencyDuration(Number.NaN)).toBe('-')
  })
})
