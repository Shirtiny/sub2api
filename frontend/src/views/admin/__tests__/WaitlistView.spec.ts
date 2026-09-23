import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import WaitlistView from '../WaitlistView.vue'
import routerSource from '@/router/index.ts?raw'
const list = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/waitlist', () => ({ listWaitlist: list }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
let page: VueWrapper
function render() {
  page = mount(WaitlistView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    TablePageLayout: { template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>' },
    DataTable: { props: ['data', 'loading', 'columns'], template: '<div class="entries">{{ data }}</div>' },
    Pagination: { props: ['page', 'total'], emits: ['update:page'], template: '<button class="next" @click="$emit(\'update:page\', 2)">Next</button>' }
  } } })
}
afterEach(() => { page?.unmount(); vi.clearAllMocks() })
describe('administrator waiting list', () => {
  it('loads and paginates collected entries', async () => {
    list.mockResolvedValue({ items: [{ id: 1, email: 'one@example.com', created_at: '2026-09-23T00:00:00Z' }], total: 21 })
    render(); await flushPromises()
    expect(list).toHaveBeenCalledWith(1, 20, expect.any(AbortSignal))
    expect(page.get('.entries').text()).toContain('one@example.com')
    list.mockResolvedValue({ items: [{ id: 2, email: 'two@example.com' }], total: 21 })
    await page.get('.next').trigger('click'); await flushPromises()
    expect(list).toHaveBeenLastCalledWith(2, 20, expect.any(AbortSignal))
    expect(page.get('.entries').text()).toContain('two@example.com')
  })
  it('shows a retryable failure rather than claiming the list is empty', async () => {
    list.mockRejectedValue(new Error('unavailable'))
    render(); await flushPromises()
    expect(page.get('[role="alert"]').text()).toBe('admin.waitlist.failed')
    expect(page.get('button').attributes('disabled')).toBeUndefined()
  })
  it('cancels an outstanding request on navigation away', async () => {
    list.mockReturnValue(new Promise(() => {}))
    render()
    const signal = list.mock.calls[0][2] as AbortSignal
    page.unmount()
    expect(signal.aborted).toBe(true)
  })
  it('marks the page as administrator-only', () => {
    const route = routerSource.split("path: '/admin/waitlist'")[1].split("path: '/admin/announcements'")[0]
    expect(route).toContain('requiresAuth: true')
    expect(route).toContain('requiresAdmin: true')
  })
})
