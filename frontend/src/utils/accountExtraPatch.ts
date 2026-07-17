export interface AccountExtraPatch {
  set: Record<string, unknown>
  delete: string[]
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value)

const extraValueEqual = (left: unknown, right: unknown): boolean => {
  if (Object.is(left, right)) return true
  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) return false
    return left.every((value, index) => extraValueEqual(value, right[index]))
  }
  if (!isRecord(left) || !isRecord(right)) return false

  const leftKeys = Object.keys(left)
  const rightKeys = Object.keys(right)
  if (leftKeys.length !== rightKeys.length) return false
  return leftKeys.every(
    key => Object.prototype.hasOwnProperty.call(right, key) && extraValueEqual(left[key], right[key])
  )
}

// Builds a top-level patch. Runtime-owned keys absent from the edited value are
// deleted only when they were present in the exact snapshot the form loaded.
export const buildAccountExtraPatch = (
  current: Record<string, unknown> | null | undefined,
  edited: Record<string, unknown> | null | undefined
): AccountExtraPatch => {
  const before = current ?? {}
  const after = edited ?? {}
  const set: Record<string, unknown> = {}

  for (const [key, value] of Object.entries(after)) {
    const exists = Object.prototype.hasOwnProperty.call(before, key)
    if (key === 'aether_ws' && exists && isRecord(before[key]) && isRecord(value)) {
      const nestedSet: Record<string, unknown> = {}
      for (const [nestedKey, nestedValue] of Object.entries(value)) {
        if (
          !Object.prototype.hasOwnProperty.call(before[key], nestedKey) ||
          !extraValueEqual(before[key][nestedKey], nestedValue)
        ) {
          nestedSet[nestedKey] = nestedValue
        }
      }
      if (Object.keys(nestedSet).length > 0) set[key] = nestedSet
      continue
    }
    if (!exists || !extraValueEqual(before[key], value)) {
      set[key] = value
    }
  }

  const deleted = Object.keys(before)
    .filter(key => !Object.prototype.hasOwnProperty.call(after, key))
    .sort()

  return { set, delete: deleted }
}

export const isAccountExtraPatchEmpty = (patch: AccountExtraPatch): boolean =>
  Object.keys(patch.set).length === 0 && patch.delete.length === 0
