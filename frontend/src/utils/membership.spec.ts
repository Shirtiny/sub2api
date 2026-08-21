import { describe, expect, it } from 'vitest'

import {
  getMembershipPointsRemaining,
  getMembershipProgress,
  getMembershipThreshold,
  getNextMembershipThreshold,
  MAX_MEMBERSHIP_LEVEL,
  resolveMembershipLevel,
} from './membership'

describe('membership level helpers', () => {
  it.each([
    [0, 0],
    [20, 0],
    [20.01, 1],
    [300, 1],
    [300.01, 2],
    [1000, 2],
    [1000.01, 3],
    [3000, 3],
    [3000.01, 4],
    [5000, 4],
    [5000.01, 5],
  ])('resolves %s points to LV.%s', (points, level) => {
    expect(resolveMembershipLevel(points)).toBe(level)
  })

  it('exposes current and next thresholds through LV.5', () => {
    expect(MAX_MEMBERSHIP_LEVEL).toBe(5)
    expect(getMembershipThreshold(4)).toBe(3000)
    expect(getNextMembershipThreshold(4)).toBe(5000)
    expect(getMembershipThreshold(5)).toBe(5000)
    expect(getNextMembershipThreshold(5)).toBeNull()
  })

  it('keeps strict-threshold progress below 100% and shows a non-zero remainder', () => {
    expect(getMembershipProgress(5000, 4)).toBe(99)
    expect(getMembershipPointsRemaining(5000, 4)).toBe(0.01)
    expect(getMembershipProgress(4999.999, 4)).toBe(99)
    expect(getMembershipPointsRemaining(4999.999, 4)).toBe(0.01)
    expect(getMembershipPointsRemaining(4000, 4)).toBe(1000.01)
  })

  it('reports completion only after reaching LV.5', () => {
    expect(getMembershipProgress(5000.01, 5)).toBe(100)
    expect(getMembershipPointsRemaining(5000.01, 5)).toBeNull()
  })
})
