/**
 * Classifies request latency for the usage table's compact health indicator.
 * TTFB thresholds are intentionally tighter than total-duration thresholds.
 */
export type LatencySeverity = 'good' | 'warn' | 'slow' | 'critical'

export const FIRST_BYTE_THRESHOLDS_MS = {
  warn: 10_000,
  slow: 30_000,
  critical: 60_000,
} as const

export const DURATION_THRESHOLDS_MS = {
  warn: 60_000,
  slow: 180_000,
  critical: 300_000,
} as const

interface Thresholds {
  warn: number
  slow: number
  critical: number
}

const classify = (ms: number, thresholds: Thresholds): LatencySeverity => {
  if (ms >= thresholds.critical) return 'critical'
  if (ms >= thresholds.slow) return 'slow'
  if (ms >= thresholds.warn) return 'warn'
  return 'good'
}

export const firstByteSeverity = (ms: number): LatencySeverity =>
  classify(ms, FIRST_BYTE_THRESHOLDS_MS)

export const durationSeverity = (ms: number): LatencySeverity =>
  classify(ms, DURATION_THRESHOLDS_MS)

export const LATENCY_TEXT_CLASSES: Record<LatencySeverity, string> = {
  good: 'text-emerald-600 dark:text-emerald-400',
  warn: 'text-amber-600 dark:text-amber-400',
  slow: 'text-orange-600 dark:text-orange-400',
  critical: 'text-red-600 dark:text-red-400',
}

export const LATENCY_BAR_CLASSES: Record<LatencySeverity, string> = {
  good: 'bg-emerald-500',
  warn: 'bg-amber-400',
  slow: 'bg-orange-500',
  critical: 'bg-red-500',
}

export const LATENCY_BAR_FROM_CLASSES: Record<LatencySeverity, string> = {
  good: 'from-emerald-500',
  warn: 'from-amber-400',
  slow: 'from-orange-500',
  critical: 'from-red-500',
}

export const LATENCY_BAR_TO_CLASSES: Record<LatencySeverity, string> = {
  good: 'to-emerald-500',
  warn: 'to-amber-400',
  slow: 'to-orange-500',
  critical: 'to-red-500',
}

/** Format a latency value without letting long requests overflow the table. */
export const formatLatencyDuration = (ms: number | null | undefined): string => {
  if (ms == null || !Number.isFinite(ms) || ms < 0) return '-'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`
  const totalSec = Math.round(ms / 1000)
  if (totalSec < 3600) return `${Math.floor(totalSec / 60)}m ${totalSec % 60}s`
  return `${Math.floor(totalSec / 3600)}h ${Math.floor((totalSec % 3600) / 60)}m`
}

/** Calculate output-token throughput using Aether's sync/stream timing windows. */
export const calculateOutputTokensPerSecond = (
  outputTokens: number | null | undefined,
  durationMs: number | null | undefined,
  firstByteMs: number | null | undefined = null,
  stream = false,
): number | null => {
  if (typeof outputTokens !== 'number' || !Number.isFinite(outputTokens) || outputTokens <= 0) return null
  if (typeof durationMs !== 'number' || !Number.isFinite(durationMs) || durationMs <= 0) return null

  let rateDurationMs = durationMs
  if (stream) {
    if (typeof firstByteMs !== 'number' || !Number.isFinite(firstByteMs) || firstByteMs < 0) return null
    if (firstByteMs >= durationMs) return null
    rateDurationMs = durationMs - firstByteMs
  }

  const tps = (outputTokens * 1000) / rateDurationMs
  if (!Number.isFinite(tps) || tps > Number.MAX_SAFE_INTEGER) return null
  if (stream && rateDurationMs / durationMs < 0.1 && tps > 5_000) return null
  return Math.round(tps)
}
