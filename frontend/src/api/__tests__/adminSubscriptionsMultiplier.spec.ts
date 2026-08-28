import { beforeEach, describe, expect, it, vi } from 'vitest'

import { updateMultiplier } from '@/api/admin/subscriptions'

const { put } = vi.hoisted(() => ({ put: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { put }
}))

describe('admin subscription multiplier API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    put.mockResolvedValue({ data: { id: 1038, custom_multiplier: 1 } })
  })

  it('updates the selected plan and multiplier without sending term changes', async () => {
    const result = await updateMultiplier(1038, { plan_id: 6, multiplier: 1 })

    expect(put).toHaveBeenCalledWith(
      '/admin/subscriptions/1038/multiplier',
      { plan_id: 6, multiplier: 1 },
      { headers: expect.objectContaining({ 'Idempotency-Key': expect.stringMatching(/^subscription-multiplier-/) }) }
    )
    expect(result.custom_multiplier).toBe(1)
  })
})
