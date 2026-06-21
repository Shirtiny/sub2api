import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import PromoCodesView from '../PromoCodesView.vue'
import type { AdminCafeCoupon, PromoCode } from '@/types'

const {
  listPromoCodes,
  listCafeCoupons,
  voidCafeCoupon,
  updateCafeCouponStatus,
  resetCafeCouponClaimPeriod,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listPromoCodes: vi.fn(),
  listCafeCoupons: vi.fn(),
  voidCafeCoupon: vi.fn(),
  updateCafeCouponStatus: vi.fn(),
  resetCafeCouponClaimPeriod: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    promo: {
      list: listPromoCodes,
      getById: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      getUsages: vi.fn(),
      listCafeCoupons,
      getCafeCoupon: vi.fn(),
      voidCafeCoupon,
      updateCafeCouponStatus,
      resetCafeCouponClaimPeriod
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess,
    showError
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn().mockResolvedValue(true)
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return Object.entries(params).reduce(
          (message, [paramKey, value]) => message.replace(`{${paramKey}}`, String(value)),
          key
        )
      }
    })
  }
})

const promoCode: PromoCode = {
  id: 1,
  code: 'REGISTER-1',
  bonus_amount: 5,
  max_uses: 0,
  used_count: 0,
  status: 'active',
  expires_at: null,
  notes: null,
  created_at: '2026-06-11T00:00:00Z',
  updated_at: '2026-06-11T00:00:00Z'
}

const cafeCoupon: AdminCafeCoupon = {
  id: 7,
  code: 'CAFE-ISSUED',
  user_id: 42,
  membership_level: 2,
  type: 'cash',
  value: 8,
  period: 'month',
  period_start: '2026-06-01T00:00:00Z',
  period_end: '2026-06-30T23:59:59Z',
  expires_at: '2026-06-30T23:59:59Z',
  status: 'issued',
  order_id: null,
  applied_at: null,
  created_at: '2026-06-11T00:00:00Z',
  updated_at: '2026-06-11T00:00:00Z',
  user: {
    id: 42,
    username: 'cafe-user',
    email: 'cafe@example.com',
    role: 'user',
    balance: 0,
    concurrency: 1,
    status: 'active',
    allowed_groups: [],
    created_at: '2026-06-11T00:00:00Z',
    updated_at: '2026-06-11T00:00:00Z'
  } as AdminCafeCoupon['user']
}

const DataTableStub = {
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <table>
      <thead>
        <tr>
          <th v-for="column in columns" :key="column.key">{{ column.key }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in data" :key="row.id">
          <td v-for="column in columns" :key="column.key">
            <slot :name="'cell-' + column.key" :row="row" :value="row[column.key]">
              {{ row[column.key] }}
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  `
}

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  setup(props: { options: Array<{ value: unknown; label: string }> }, { emit }: { emit: (event: string, ...args: unknown[]) => void }) {
    const onChange = (event: Event) => {
      const raw = (event.target as HTMLSelectElement).value
      const option = props.options.find((item) => String(item.value ?? '') === raw)
      const value = option ? option.value : raw
      emit('update:modelValue', value)
      emit('change', value, option ?? null)
    }
    return { onChange }
  },
  template: `
    <select :value="modelValue ?? ''" @change="onChange">
      <option v-for="option in options" :key="String(option.value ?? '')" :value="option.value ?? ''">
        {{ option.label }}
      </option>
    </select>
  `
}

const ConfirmDialogStub = {
  props: ['show', 'title', 'message'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-test="confirm-dialog">
      <span>{{ title }}</span>
      <button data-test="confirm" @click="$emit('confirm')">confirm</button>
      <button data-test="cancel" @click="$emit('cancel')">cancel</button>
    </div>
  `
}

const BaseDialogStub = {
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
}

const mountView = () => mount(PromoCodesView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      },
      DataTable: DataTableStub,
      Pagination: true,
      ConfirmDialog: ConfirmDialogStub,
      BaseDialog: BaseDialogStub,
      Select: SelectStub,
      Icon: true,
      Teleport: true
    }
  }
})

describe('admin PromoCodesView Café coupon tab', () => {
  beforeEach(() => {
    localStorage.clear()
    listPromoCodes.mockReset()
    listCafeCoupons.mockReset()
    voidCafeCoupon.mockReset()
    updateCafeCouponStatus.mockReset()
    resetCafeCouponClaimPeriod.mockReset()
    showSuccess.mockReset()
    showError.mockReset()

    listPromoCodes.mockResolvedValue({
      items: [promoCode],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listCafeCoupons.mockResolvedValue({
      items: [cafeCoupon],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    voidCafeCoupon.mockResolvedValue({ ...cafeCoupon, status: 'void' })
    updateCafeCouponStatus.mockResolvedValue({ ...cafeCoupon, status: 'applied' })
    resetCafeCouponClaimPeriod.mockResolvedValue({ ...cafeCoupon, status: 'void' })
  })

  it('keeps registration promo codes as the default tab', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listPromoCodes).toHaveBeenCalledTimes(1)
    expect(listCafeCoupons).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('REGISTER-1')
  })

  it('loads Café coupons and voids issued unused coupons from the Café tab', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="promo-tab-cafe"]').trigger('click')
    await flushPromises()

    expect(listCafeCoupons).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({
        sort_by: 'created_at',
        sort_order: 'desc'
      }),
      expect.any(Object)
    )
    expect(wrapper.text()).toContain('CAFE-ISSUED')
    expect(wrapper.text()).toContain('cafe@example.com')

    await wrapper.get('[data-test="void-cafe-coupon"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()

    expect(voidCafeCoupon).toHaveBeenCalledWith(7)
    expect(showSuccess).toHaveBeenCalledWith('admin.promo.cafeCoupon.voided')
    expect(listCafeCoupons).toHaveBeenCalledTimes(2)
  })

  it('uses backend-compatible Café coupon membership levels', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="promo-tab-cafe"]').trigger('click')
    await flushPromises()

    const selects = wrapper.findAll('select')
    const membershipOptions = selects[2].findAll('option').map((option) => option.element.value)
    expect(membershipOptions).toEqual(['', '0', '1', '2', '3'])
    expect(membershipOptions).not.toContain('4')

    await selects[2].setValue('0')
    await flushPromises()

    expect(listCafeCoupons).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({ membership_level: 0 }),
      expect.any(Object)
    )
  })

  it('updates Café coupon status and resets claim period', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="promo-tab-cafe"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="status-cafe-coupon"]').trigger('click')
    await flushPromises()
    const selects = wrapper.findAll('select')
    const statusOptions = selects[selects.length - 1].findAll('option').map((option) => option.element.value)
    expect(statusOptions).toEqual(['issued', 'void'])
    await selects[selects.length - 1].setValue('void')
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()

    expect(updateCafeCouponStatus).toHaveBeenCalledWith(7, { status: 'void' })
    expect(showSuccess).toHaveBeenCalledWith('admin.promo.cafeCoupon.statusUpdated')

    await wrapper.get('[data-test="reset-cafe-coupon"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()

    expect(resetCafeCouponClaimPeriod).toHaveBeenCalledWith(7)
    expect(showSuccess).toHaveBeenCalledWith('admin.promo.cafeCoupon.claimPeriodReset')
  })
})
