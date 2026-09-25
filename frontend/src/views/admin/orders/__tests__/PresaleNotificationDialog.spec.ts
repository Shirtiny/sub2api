import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import PresaleNotificationDialog from '../PresaleNotificationDialog.vue'

const mocks = vi.hoisted(() => ({ preview: vi.fn(), test: vi.fn(), next: vi.fn() }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { getPresaleNotice: mocks.preview, testPresaleNotice: mocks.test, sendNextPresaleNotice: mocks.next } }))
enableAutoUnmount(afterEach)
const data = () => ({ month: '2026-10', subject: '2026 年 10 月订阅预售', html: '<p>Preview only</p>', version: 'review-1', ready: true, draft: false, published_plans: 2, counts: { eligible: 3, pending: 3, sent: 0, skipped: 0, uncertain: 0, sending: 0 } })
const create = () => mount(PresaleNotificationDialog, { props: { show: true }, global: { stubs: {
  BaseDialog: { props: ['show'], template: '<div v-if="show"><button class="close" @click="$emit(\'close\')">Close</button><slot/><slot name="footer"/></div>' },
  ConfirmDialog: { props: ['show'], template: '<div v-if="show" class="confirmation"><button class="confirm" @click="$emit(\'confirm\')">Confirm</button><button @click="$emit(\'cancel\')">Cancel</button></div>' },
  Icon: true
} } })
beforeEach(() => { vi.clearAllMocks(); mocks.preview.mockResolvedValue({ data: data() }); mocks.test.mockResolvedValue({ data: { status: 'sent' } }); mocks.next.mockResolvedValue({ data: { status: 'complete', done: true } }) })

describe('presale notification dialog', () => {
  it('only previews by default, with restricted recipients excluded and a sandboxed iframe', async () => {
    const w = create(); await flushPromises()
    expect(mocks.preview).toHaveBeenCalledWith({ include_restricted: false, locale: 'zh' })
    expect(mocks.test).not.toHaveBeenCalled(); expect(mocks.next).not.toHaveBeenCalled()
    expect(w.get('iframe').attributes('sandbox')).toBe('')
    expect(w.get('iframe').attributes('srcdoc')).toBe('<p>Preview only</p>')
  })
  it('refreshes the reviewed audience before a separate formal confirmation', async () => {
    const w = create(); await flushPromises()
    await w.get('[data-test="include-restricted"]').setValue(true); await flushPromises()
    expect(mocks.preview).toHaveBeenLastCalledWith({ include_restricted: true, locale: 'zh' })
    await w.get('[data-test="send-notice"]').trigger('click')
    expect(mocks.next).not.toHaveBeenCalled()
    await w.get('.confirm').trigger('click'); await flushPromises()
    expect(mocks.next).toHaveBeenCalledTimes(1)
    expect(mocks.next).toHaveBeenCalledWith({ month: '2026-10', version: 'review-1', locale: 'zh', include_restricted: true, confirmed: true })
  })
  it('sends a single test only to the entered email without starting a broadcast', async () => {
    const w = create(); await flushPromises()
    await w.get('input[type="email"]').setValue('owner@example.com'); await w.get('form').trigger('submit'); await flushPromises()
    expect(mocks.test).toHaveBeenCalledTimes(1)
    expect(mocks.test.mock.calls[0][0].email).toBe('owner@example.com')
    expect(mocks.next).not.toHaveBeenCalled(); expect(w.text()).toContain('presaleNotice.testSuccess')
  })
  it('does not allow a formal send before publication', async () => {
    mocks.preview.mockResolvedValue({ data: { ...data(), ready: false, draft: true, published_plans: 0 } })
    const w = create(); await flushPromises()
    expect(w.get('[data-test="send-notice"]').attributes()).toHaveProperty('disabled')
    expect(w.text()).toContain('presaleNotice.draft')
    await w.get('input[type="email"]').setValue('owner@example.com'); await w.get('form').trigger('submit'); await flushPromises()
    expect(mocks.test).toHaveBeenCalledTimes(1)
  })
  it('stops after an uncertain response instead of retrying and requires a new preview', async () => {
    mocks.next.mockRejectedValue({ message: 'Unconfirmed delivery' })
    const w = create(); await flushPromises(); await w.get('[data-test="send-notice"]').trigger('click'); await w.get('.confirm').trigger('click'); await flushPromises()
    expect(mocks.next).toHaveBeenCalledTimes(1); expect(w.text()).toContain('Unconfirmed delivery')
    expect(w.find('iframe').exists()).toBe(false)
  })
  it('discards stale audience preview responses', async () => {
    let resolveOld!: (value: unknown) => void
    mocks.preview.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const w = create(); await w.get('[data-test="include-restricted"]').setValue(true); await flushPromises()
    expect(w.get('iframe').attributes('srcdoc')).toBe('<p>Preview only</p>')
    resolveOld({ data: { ...data(), html: 'OLD AUDIENCE' } }); await flushPromises()
    expect(w.get('iframe').attributes('srcdoc')).toBe('<p>Preview only</p>')
  })
})
