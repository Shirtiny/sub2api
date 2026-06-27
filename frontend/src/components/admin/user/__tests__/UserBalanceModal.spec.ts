import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const apiMocks = vi.hoisted(() => ({
  updateBalance: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      updateBalance: apiMocks.updateBalance,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: {
    name: 'BaseDialog',
    props: ['show', 'title', 'width'],
    template: '<div v-if="show"><slot /><slot name="footer" /></div>',
  },
}))

import UserBalanceModal from '../UserBalanceModal.vue'

const user = {
  id: 99,
  email: 'user@example.com',
  balance: 10,
}

describe('UserBalanceModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.updateBalance.mockResolvedValue({ ...user, balance: 15 })
  })

  it('keeps deposits visible in user history by default', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: true, user: user as any, operation: 'add' },
    })

    await wrapper.find('input[type="number"]').setValue('5')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.updateBalance).toHaveBeenCalledWith(99, 5, 'add', '', {
      recordUserHistory: true,
    })
  })

  it('defaults direct set balance to the current balance and audit-only history', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: true, user: user as any, operation: 'set' },
    })

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.updateBalance).toHaveBeenCalledWith(99, 10, 'set', '', {
      recordUserHistory: false,
    })
  })

  it('allows setting balance to zero when explicitly entered', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: true, user: user as any, operation: 'set' },
    })

    await wrapper.find('input[type="number"]').setValue('0')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.updateBalance).toHaveBeenCalledWith(99, 0, 'set', '', {
      recordUserHistory: false,
    })
  })
})
