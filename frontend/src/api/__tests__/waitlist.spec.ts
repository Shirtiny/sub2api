import { beforeEach, describe, expect, it, vi } from 'vitest'
import { joinWaitlist } from '../waitlist'
import { approveWaitlist, listWaitlist } from '../admin/waitlist'
const client = vi.hoisted(() => ({ post: vi.fn(), get: vi.fn() }))
vi.mock('../client', () => ({ default: client }))
beforeEach(() => { vi.clearAllMocks() })
describe('waiting-list API', () => {
  it('posts only the email and optional challenge token', async () => {
    client.post.mockResolvedValue({ data: { accepted: true } })
    await joinWaitlist('a@example.com', 'proof')
    expect(client.post).toHaveBeenCalledWith('/waitlist', { email: 'a@example.com', turnstile_token: 'proof' }, { timeout: 60000 })
  })
  it.each([undefined, {}, { accepted: false }])('does not treat an unacknowledged result as success', async data => {
    client.post.mockResolvedValue({ data })
    await expect(joinWaitlist('a@example.com')).rejects.toThrow('not acknowledged')
  })
  it('uses the authenticated admin endpoint for collection access', async () => {
    const data = { items: [], total: 0 }
    client.get.mockResolvedValue({ data })
    const signal = new AbortController().signal
    expect(await listWaitlist(2, 20, signal)).toEqual(data)
    expect(client.get).toHaveBeenCalledWith('/admin/waitlist', { params: { page: 2, page_size: 20 }, signal })
  })
  it('approves only through the administrator endpoint', async () => {
    client.post.mockResolvedValue({ data: { approved: true } })
    await approveWaitlist(7)
    expect(client.post).toHaveBeenCalledWith('/admin/waitlist/7/approve', {}, { timeout: 60000 })
  })
  it.each([undefined, {}, { approved: false }])('does not claim an unacknowledged approval succeeded', async data => {
    client.post.mockResolvedValue({ data })
    await expect(approveWaitlist(7)).rejects.toThrow('not acknowledged')
  })

})
