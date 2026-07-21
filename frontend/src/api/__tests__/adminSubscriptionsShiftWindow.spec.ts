import { beforeEach, describe, expect, it, vi } from 'vitest'

import { bulkShiftWindow } from '@/api/admin/subscriptions'
import type { BulkShiftWindowRequest } from '@/types'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

const baseRequest: Omit<BulkShiftWindowRequest, 'dry_run'> = {
  daily: false,
  weekly: true,
  monthly: false,
  offset_hours: 14,
  filters: { status: 'active' }
}

const headerOf = (callIndex: number): string | undefined =>
  post.mock.calls[callIndex]?.[2]?.headers?.['Idempotency-Key']

describe('bulkShiftWindow idempotency key', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    post.mockResolvedValue({
      data: { matched: 105, updated: 105, skipped_future: 0, dry_run: false }
    })
  })

  it('sends an Idempotency-Key on a real write', async () => {
    await bulkShiftWindow({ ...baseRequest, dry_run: false })

    expect(headerOf(0)).toMatch(/^subscription-shift-window-/)
  })

  it('omits the header on the read-only dry-run preview', async () => {
    await bulkShiftWindow({ ...baseRequest, dry_run: true })

    expect(headerOf(0)).toBeUndefined()
  })

  // 复用 key 会让后端在 24 小时幂等 TTL 内把第二次合法提交静默吞掉。
  it('mints a fresh key for every write so a deliberate second shift still applies', async () => {
    await bulkShiftWindow({ ...baseRequest, dry_run: false })
    await bulkShiftWindow({ ...baseRequest, dry_run: false })

    const first = headerOf(0)
    const second = headerOf(1)

    expect(first).toBeDefined()
    expect(second).toBeDefined()
    expect(first).not.toBe(second)
  })

  it('still posts the request body unchanged', async () => {
    await bulkShiftWindow({ ...baseRequest, dry_run: false })

    expect(post).toHaveBeenCalledWith(
      '/admin/subscriptions/bulk-shift-window',
      { ...baseRequest, dry_run: false },
      expect.anything()
    )
  })
})
