import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import PresaleNotificationDialog from '../PresaleNotificationDialog.vue'

const mocks = vi.hoisted(() => ({ preview: vi.fn(), test: vi.fn(), next: vi.fn(), save: vi.fn() }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { getPresaleNotice: mocks.preview, testPresaleNotice: mocks.test, sendNextPresaleNotice: mocks.next, updatePresaleNoticeConfig: mocks.save } }))
enableAutoUnmount(afterEach)
const data = () => ({ month: '2026-10', coupon_code: '', coupon_included: false, subject: '2026 年 10 月订阅预售', html: '<p>Preview only</p>', version: 'review-1', ready: true, draft: false, published_plans: 2, counts: { eligible: 3, pending: 3, sent: 0, skipped: 0, uncertain: 0, sending: 0 } })
const create = () => mount(PresaleNotificationDialog, { props: { show: true }, global: { stubs: {
  BaseDialog: { props: ['show'], template: '<div v-if="show"><button class="close" @click="$emit(\'close\')">Close</button><slot/><slot name="footer"/></div>' },
  ConfirmDialog: { props: ['show'], template: '<div v-if="show" class="confirmation"><button class="confirm" @click="$emit(\'confirm\')">Confirm</button><button @click="$emit(\'cancel\')">Cancel</button></div>' },
  Icon: true
} } })
beforeEach(() => { vi.resetAllMocks(); mocks.preview.mockResolvedValue({ data: data() }); mocks.test.mockResolvedValue({ data: { status: 'sent' } }); mocks.next.mockResolvedValue({ data: { status: 'complete', done: true } }); mocks.save.mockResolvedValue({ data: { month: '2026-10', coupon_code: 'CAFE-PUBLIC-AUTUMN40' } }) })

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
  it('requires saving the code and reviewing fresh content before test or formal mail', async () => {
    const w = create(); await flushPromises()
    await w.get('#presale-notice-coupon').setValue('CAFE-PUBLIC-AUTUMN40')
    expect(w.get('[data-test="send-notice"]').attributes()).toHaveProperty('disabled')
    await w.get('input[type="email"]').setValue('owner@example.com'); await w.get('form').trigger('submit')
    expect(mocks.test).not.toHaveBeenCalled()
    mocks.preview.mockResolvedValue({ data: { ...data(), coupon_code: 'CAFE-PUBLIC-AUTUMN40', coupon_included: true, version: 'review-coupon', html: '<p>CAFE-PUBLIC-AUTUMN40</p>' } })
    await w.get('[data-test="save-coupon"]').trigger('click'); await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith({ month: '2026-10', coupon_code: 'CAFE-PUBLIC-AUTUMN40' })
    expect(w.get('iframe').attributes('srcdoc')).toContain('CAFE-PUBLIC-AUTUMN40')
    expect(mocks.test).not.toHaveBeenCalled(); expect(mocks.next).not.toHaveBeenCalled()
    await w.get('form').trigger('submit'); await flushPromises()
    expect(mocks.test.mock.calls[0][0].version).toBe('review-coupon')
  })
  it('loads a saved selection and can clear it without sending', async () => {
    mocks.preview.mockResolvedValueOnce({ data: { ...data(), coupon_code: 'CAFE-PUBLIC-AUTUMN40', coupon_included: true } })
    const w = create(); await flushPromises()
    expect((w.get('#presale-notice-coupon').element as HTMLInputElement).value).toBe('CAFE-PUBLIC-AUTUMN40')
    await w.get('#presale-notice-coupon').setValue('')
    await w.get('[data-test="save-coupon"]').trigger('click'); await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith({ month: '2026-10', coupon_code: '' })
    expect(w.get('iframe').attributes('srcdoc')).not.toContain('CAFE-PUBLIC')
    expect(mocks.test).not.toHaveBeenCalled(); expect(mocks.next).not.toHaveBeenCalled()
  })
  it('keeps invalid edits visible and blocks sending when save fails', async () => {
    mocks.save.mockRejectedValue({ message: 'Invalid public coupon' })
    const w = create(); await flushPromises()
    await w.get('#presale-notice-coupon').setValue('INVALID')
    await w.get('[data-test="save-coupon"]').trigger('click'); await flushPromises()
    expect(w.text()).toContain('Invalid public coupon')
    expect((w.get('#presale-notice-coupon').element as HTMLInputElement).value).toBe('INVALID')
    expect(w.get('[data-test="send-notice"]').attributes()).toHaveProperty('disabled')
  })
  it('preserves unsaved coupon edits when audience or language changes', async () => {
    const w = create(); await flushPromises()
    await w.get('#presale-notice-coupon').setValue('CAFE-PUBLIC-DRAFT')
    await w.get('[data-test="include-restricted"]').setValue(true); await flushPromises()
    await w.get('select').setValue('en'); await flushPromises()
    expect((w.get('#presale-notice-coupon').element as HTMLInputElement).value).toBe('CAFE-PUBLIC-DRAFT')
    expect(w.get('[data-test="send-notice"]').attributes()).toHaveProperty('disabled')
    expect(mocks.save).not.toHaveBeenCalled()
  })
  it('shows when a saved code is no longer included in the email', async () => {
    mocks.preview.mockResolvedValue({ data: { ...data(), coupon_code: 'CAFE-PUBLIC-EXPIRED', coupon_included: false } })
    const w = create(); await flushPromises()
    expect(w.text()).toContain('presaleNotice.couponUnavailable')
    expect(w.text()).not.toContain('presaleNotice.couponUnsaved')
  })
  it('does not overwrite a reopened dialog with a stale save response', async () => {
    let finishSave!: (value: unknown) => void
    mocks.save.mockImplementationOnce(() => new Promise(resolve => { finishSave = resolve }))
    const w = create(); await flushPromises()
    await w.get('#presale-notice-coupon').setValue('CAFE-PUBLIC-OLD')
    await w.get('[data-test="save-coupon"]').trigger('click')
    await w.setProps({ show: false }); await w.setProps({ show: true }); await flushPromises()
    const calls = mocks.preview.mock.calls.length
    finishSave({ data: { month: '2026-10', coupon_code: 'CAFE-PUBLIC-OLD' } }); await flushPromises()
    expect(mocks.preview).toHaveBeenCalledTimes(calls)
    expect((w.get('#presale-notice-coupon').element as HTMLInputElement).value).toBe('')
  })
})
