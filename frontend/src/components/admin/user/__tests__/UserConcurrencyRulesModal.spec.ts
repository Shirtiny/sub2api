import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import UserConcurrencyRulesModal from '../UserConcurrencyRulesModal.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { BalanceConcurrencyRules } from '@/api/admin/users'

const { getRules, updateRules, showSuccess } = vi.hoisted(() => ({
  getRules: vi.fn(), updateRules: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/api/admin', () => ({
  adminAPI: { users: { getConcurrencyRules: getRules, updateConcurrencyRules: updateRules } }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess }) }))
vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))

enableAutoUnmount(afterEach)
const defaults: BalanceConcurrencyRules = {
  balance_tiers: [{ min_balance: 0, concurrency: 1 }, { min_balance: 20, concurrency: 2 }, { min_balance: 100, concurrency: 3 }]
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
function render(show = true) {
  return mount(UserConcurrencyRulesModal, {
    props: { show },
    global: { stubs: { Teleport: true, Icon: true } }
  })
}
const minInputs = (wrapper: ReturnType<typeof render>) => wrapper.findAll('[data-testid="tier-min"]')
const concurrencyInputs = (wrapper: ReturnType<typeof render>) => wrapper.findAll('[data-testid="tier-concurrency"]')
const saveDisabled = (wrapper: ReturnType<typeof render>) => wrapper.get<HTMLButtonElement>('[data-testid="rules-save"]').element.disabled

describe('UserConcurrencyRulesModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getRules.mockReset().mockResolvedValue(defaults)
    updateRules.mockReset().mockResolvedValue(defaults)
  })

  it('loads only on open and never enables saving defaults while loading', async () => {
    const pending = deferred<BalanceConcurrencyRules>()
    getRules.mockReturnValueOnce(pending.promise)
    const wrapper = render(false)
    expect(getRules).not.toHaveBeenCalled()
    await wrapper.setProps({ show: true })
    expect(wrapper.get('[role="dialog"]').attributes('aria-modal')).toBe('true')
    expect(wrapper.get('[role="status"]').text()).toBe('common.loading')
    expect(saveDisabled(wrapper)).toBe(true)
    expect(minInputs(wrapper)).toHaveLength(0)
    pending.resolve(defaults)
    await flushPromises()
    expect(saveDisabled(wrapper)).toBe(false)
    expect(minInputs(wrapper)).toHaveLength(3)
    expect(minInputs(wrapper)[0].attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('[data-testid="tier-upper"]').map(row => row.text())).toEqual([
      '$20', '$100', 'admin.users.concurrencyRules.noUpperBound'
    ])
    expect(wrapper.findAll('[data-testid="tier-remove"]')).toHaveLength(2)
  })

  it('blocks save after failed load and offers retry without substituting defaults', async () => {
    getRules.mockRejectedValueOnce({ message: 'offline' })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('offline')
    expect(saveDisabled(wrapper)).toBe(true)
    expect(minInputs(wrapper)).toHaveLength(0)
    await wrapper.get('[data-testid="rules-save"]').trigger('click')
    expect(updateRules).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="rules-retry"]').trigger('click')
    await flushPromises()
    expect(getRules).toHaveBeenCalledTimes(2)
    expect(saveDisabled(wrapper)).toBe(false)
  })

  it.each([
    { balance_tiers: [] },
    { balance_tiers: [{ min_balance: 1, concurrency: 1 }] },
    { balance_tiers: [{ min_balance: 0, concurrency: 0 }] },
    { balance_tiers: [{ min_balance: 0, concurrency: 1 }, { min_balance: Infinity, concurrency: 2 }] },
    { balance_tiers: Array.from({ length: 21 }, (_, min_balance) => ({ min_balance, concurrency: 1 })) }
  ])('rejects malformed loaded rules without enabling save: %j', async ({ balance_tiers }) => {
    getRules.mockResolvedValueOnce({ balance_tiers })
    const wrapper = render()
    await flushPromises()
    expect(saveDisabled(wrapper)).toBe(true)
    expect(wrapper.get('[data-testid="rules-retry"]').exists()).toBe(true)
  })

  it.each(['', '0', '-1', '100', '1000000000001', '1e309'])('rejects missing, overlapping, out-of-order or invalid lower bounds: %s', async value => {
    const wrapper = render()
    await flushPromises()
    await minInputs(wrapper)[1].setValue(value)
    expect(saveDisabled(wrapper)).toBe(true)
    await wrapper.get('form').trigger('submit')
    expect(updateRules).not.toHaveBeenCalled()
  })

  it.each(['', '0', '-1', '1.5', '1001', '1e309'])('rejects invalid concurrency: %s', async value => {
    const wrapper = render()
    await flushPromises()
    await concurrencyInputs(wrapper)[0].setValue(value)
    expect(concurrencyInputs(wrapper)[0].attributes('aria-invalid')).toBe('true')
    expect(saveDisabled(wrapper)).toBe(true)
    await wrapper.get('form').trigger('submit')
    expect(updateRules).not.toHaveBeenCalled()
  })

  it('adds and removes tiers, derives upper bounds, and accepts fractional balances and inclusive maxima', async () => {
    const wrapper = render()
    await flushPromises()
    await minInputs(wrapper)[1].setValue('0.125')
    expect(wrapper.findAll('[data-testid="tier-upper"]')[0].text()).toBe('$0.125')
    await wrapper.get('[data-testid="tier-add"]').trigger('click')
    expect(saveDisabled(wrapper)).toBe(true)
    await minInputs(wrapper)[3].setValue('1000000000000')
    await concurrencyInputs(wrapper)[3].setValue('1000')
    expect(saveDisabled(wrapper)).toBe(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateRules).toHaveBeenCalledWith({ balance_tiers: [
      { min_balance: 0, concurrency: 1 }, { min_balance: 0.125, concurrency: 2 },
      { min_balance: 100, concurrency: 3 }, { min_balance: 1e12, concurrency: 1000 }
    ] })
    expect(defaults.balance_tiers[1].min_balance).toBe(20)
  })

  it('allows one tier covering every nonnegative balance and caps the editor at 20 tiers', async () => {
    getRules.mockResolvedValueOnce({ balance_tiers: Array.from({ length: 20 }, (_, min_balance) => ({ min_balance, concurrency: 1 })) })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get<HTMLButtonElement>('[data-testid="tier-add"]').element.disabled).toBe(true)
    for (let i = 19; i > 0; i--) await wrapper.get('[data-testid="tier-remove"]').trigger('click')
    expect(minInputs(wrapper)).toHaveLength(1)
    expect(wrapper.get('[data-testid="tier-upper"]').text()).toBe('admin.users.concurrencyRules.noUpperBound')
    expect(wrapper.find('[data-testid="tier-remove"]').exists()).toBe(false)
    expect(saveDisabled(wrapper)).toBe(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateRules).toHaveBeenCalledWith({ balance_tiers: [{ min_balance: 0, concurrency: 1 }] })
  })

  it('guards header, backdrop, Escape, cancel, edits, and duplicate saves while saving', async () => {
    const pending = deferred<BalanceConcurrencyRules>()
    updateRules.mockReturnValueOnce(pending.promise)
    const wrapper = render()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    expect(saveDisabled(wrapper)).toBe(true)
    expect(wrapper.get('fieldset').attributes('disabled')).toBeDefined()
    expect(wrapper.get<HTMLButtonElement>('[data-testid="rules-cancel"]').element.disabled).toBe(true)
    await wrapper.get('[aria-label="Close modal"]').trigger('click')
    await wrapper.get('[role="dialog"]').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(updateRules).toHaveBeenCalledTimes(1)
    pending.resolve(defaults)
    await flushPromises()
    expect(wrapper.emitted('success')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(showSuccess).toHaveBeenCalledWith('admin.users.concurrencyRules.saved')
  })

  it('keeps edits and the dialog open after a save error, and allows retry', async () => {
    updateRules.mockRejectedValueOnce({ message: 'write failed' })
    const wrapper = render()
    await flushPromises()
    await concurrencyInputs(wrapper)[1].setValue('8')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('write failed')
    expect(concurrencyInputs(wrapper)[1].element).toHaveProperty('value', '8')
    expect(saveDisabled(wrapper)).toBe(false)
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('success')).toBeUndefined()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateRules).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('success')).toHaveLength(1)
  })

  it('ignores a stale load after closing and reopening', async () => {
    const stale = deferred<BalanceConcurrencyRules>()
    getRules.mockReturnValueOnce(stale.promise)
    const wrapper = render()
    const signal = getRules.mock.calls[0][0] as AbortSignal
    await wrapper.setProps({ show: false })
    expect(signal.aborted).toBe(true)
    await wrapper.setProps({ show: true })
    await flushPromises()
    stale.resolve({ balance_tiers: [{ min_balance: 0, concurrency: 99 }] })
    await flushPromises()
    expect(minInputs(wrapper)).toHaveLength(3)
    expect(concurrencyInputs(wrapper)[0].element).toHaveProperty('value', '1')
  })
})
