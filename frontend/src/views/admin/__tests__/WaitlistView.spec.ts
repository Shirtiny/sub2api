import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import WaitlistView from '../WaitlistView.vue'
import routerSource from '@/router/index.ts?raw'
const { list, approve } = vi.hoisted(() => ({ list: vi.fn(), approve: vi.fn() }))
vi.mock('@/api/admin/waitlist', () => ({ listWaitlist: list, approveWaitlist: approve }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
let page: VueWrapper
function render() {
  page = mount(WaitlistView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    TablePageLayout: { template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>' },
    DataTable: { props: ['data', 'loading', 'columns'], template: `<div class="entries">{{ data }}<div v-for="row in data" :key="row.id"><slot name="cell-status" :row="row" /><slot name="cell-notification" :row="row" /><slot name="cell-actions" :row="row" /></div></div>` },
    ConfirmDialog: { props: ['show', 'confirmDisabled'], emits: ['confirm', 'cancel'], template: `<div v-if="show" class="confirm"><button class="approve-confirm" :disabled="confirmDisabled" @click="$emit('confirm')">Confirm</button><button class="cancel" @click="$emit('cancel')">Cancel</button></div>` },
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
  it('requires confirmation and prevents duplicate approval while notifying', async () => {
    list.mockResolvedValue({ items: [{ id: 1, email: 'one@example.com', approved_at: null }], total: 1 })
    let finish!: () => void
    approve.mockImplementation(() => new Promise<void>(resolve => { finish = resolve }))
    render(); await flushPromises()
    await page.get('.entries button').trigger('click')
    expect(approve).not.toHaveBeenCalled()
    expect(page.find('.confirm').exists()).toBe(true)
    await page.get('.approve-confirm').trigger('click')
    expect(approve).toHaveBeenCalledOnce()
    expect(approve).toHaveBeenCalledWith(1)
    expect(page.get('.entries button').attributes('disabled')).toBeDefined()
    list.mockResolvedValue({ items: [{ id: 1, email: 'one@example.com', approved_at: '2026-09-23T12:00:00Z', approval_notice_sent_at: '2026-09-23T12:01:00Z' }], total: 1 })
    finish(); await flushPromises()
    expect(page.get('[role="status"]').text()).toBe('admin.waitlist.success')
    expect(page.find('.entries button').exists()).toBe(false)
    expect(page.get('.entries').text()).toContain('admin.waitlist.notified')
  })
  it('refreshes durable approval after email failure and offers notification-only retry', async () => {
    list.mockResolvedValueOnce({ items: [{ id: 2, email: 'two@example.com', approved_at: null }], total: 1 })
      .mockResolvedValue({ items: [{ id: 2, email: 'two@example.com', approved_at: '2026-09-23T12:00:00Z', approval_notice_sent_at: null }], total: 1 })
    approve.mockRejectedValueOnce({ reason: 'WAITLIST_APPROVAL_NOTICE_FAILED' }).mockResolvedValueOnce(undefined)
    render(); await flushPromises()
    await page.get('.entries button').trigger('click')
    await page.get('.approve-confirm').trigger('click'); await flushPromises()
    expect(page.get('[role="alert"]').text()).toBe('admin.waitlist.approveFailed')
    expect(page.get('.entries button').text()).toBe('admin.waitlist.retryNotice')
    await page.get('.entries button').trigger('click'); await flushPromises()
    expect(approve).toHaveBeenCalledTimes(2)
    expect(page.find('.confirm').exists()).toBe(false)
    expect(page.find('[role="alert"]').exists()).toBe(false)
  })

})
