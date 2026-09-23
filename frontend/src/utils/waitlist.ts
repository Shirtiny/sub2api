/** Validate syntax only; this does not verify mailbox ownership or delivery. */
export function normalizeWaitlistEmail(value: string): string | null {
  const email = value.trim().toLowerCase()
  const parts = email.split('@')
  if (email.length > 254 || parts.length !== 2) return null
  const [local, domain] = parts
  if (!local || local.length > 64 || !/^[a-z0-9!#$%&'*+/=?^_`{|}~.-]+$/.test(local)
    || local.startsWith('.') || local.endsWith('.') || local.includes('..')) return null
  const labels = domain.split('.')
  if (labels.length < 2 || labels.some(label => !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))
    || !/[a-z]/.test(labels[labels.length - 1])) return null
  return email
}
