export const MEMBERSHIP_LEVEL_THRESHOLDS = [20, 300, 1000, 3000, 5000] as const

export const MAX_MEMBERSHIP_LEVEL = MEMBERSHIP_LEVEL_THRESHOLDS.length
const MAX_INCOMPLETE_PROGRESS = 99
const DISPLAY_POINTS_PER_UNIT = 100
const FLOAT_COMPARISON_TOLERANCE = 1e-9

function normalizeMembershipLevel(level: number): number {
  if (!Number.isFinite(level)) return 0
  return Math.min(Math.max(Math.floor(level), 0), MAX_MEMBERSHIP_LEVEL)
}

export function resolveMembershipLevel(points: number): number {
  if (!Number.isFinite(points)) return 0

  for (let index = MEMBERSHIP_LEVEL_THRESHOLDS.length - 1; index >= 0; index -= 1) {
    if (points > MEMBERSHIP_LEVEL_THRESHOLDS[index]) return index + 1
  }
  return 0
}

export function getMembershipThreshold(level: number): number {
  const normalizedLevel = normalizeMembershipLevel(level)
  return normalizedLevel === 0 ? 0 : MEMBERSHIP_LEVEL_THRESHOLDS[normalizedLevel - 1]
}

export function getNextMembershipThreshold(level: number): number | null {
  const normalizedLevel = normalizeMembershipLevel(level)
  return normalizedLevel >= MAX_MEMBERSHIP_LEVEL
    ? null
    : MEMBERSHIP_LEVEL_THRESHOLDS[normalizedLevel]
}

export function getMembershipProgress(points: number, level: number): number {
  const next = getNextMembershipThreshold(level)
  if (next == null) return 100

  const current = getMembershipThreshold(level)
  const safePoints = Number.isFinite(points) ? points : 0
  const progress = ((safePoints - current) / (next - current)) * 100
  return Math.max(0, Math.min(progress, MAX_INCOMPLETE_PROGRESS))
}

export function getMembershipPointsRemaining(points: number, level: number): number | null {
  const next = getNextMembershipThreshold(level)
  if (next == null) return null

  const safePoints = Number.isFinite(points) ? points : 0
  const remaining = Math.max(next - safePoints, 0)
  const wholeDisplayUnits = Math.floor((remaining + FLOAT_COMPARISON_TOLERANCE) * DISPLAY_POINTS_PER_UNIT)
  return (wholeDisplayUnits + 1) / DISPLAY_POINTS_PER_UNIT
}
