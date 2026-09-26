import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getConcurrencyRules, updateConcurrencyRules, usersAPI } from '@/api/admin/users'

const { get, put } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, put } }))

describe('admin concurrency rules API', () => {
  const rules = { balance_tiers: [{ min_balance: 0, concurrency: 7 }, { min_balance: 12.5, concurrency: 11 }] }
  beforeEach(() => vi.clearAllMocks())

  it('loads through the shared client with cancellation support', async () => {
    get.mockResolvedValueOnce({ data: rules })
    const signal = new AbortController().signal
    expect(await getConcurrencyRules(signal)).toEqual(rules)
    expect(get).toHaveBeenCalledWith('/admin/users/concurrency-rules', { signal })
    expect(usersAPI.getConcurrencyRules).toBe(getConcurrencyRules)
  })

  it('PUTs the exact balance_tiers payload and returns configured rules', async () => {
    put.mockResolvedValueOnce({ data: rules })
    expect(await updateConcurrencyRules(rules)).toEqual(rules)
    expect(put).toHaveBeenCalledWith('/admin/users/concurrency-rules', rules)
    expect(usersAPI.updateConcurrencyRules).toBe(updateConcurrencyRules)
  })

  it('propagates failures rather than returning defaults', async () => {
    get.mockRejectedValueOnce(new Error('load failed'))
    put.mockRejectedValueOnce(new Error('save failed'))
    await expect(getConcurrencyRules()).rejects.toThrow('load failed')
    await expect(updateConcurrencyRules(rules)).rejects.toThrow('save failed')
  })
})
