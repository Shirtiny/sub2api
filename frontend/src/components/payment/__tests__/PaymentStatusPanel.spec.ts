import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'

const pollOrderStatus = vi.hoisted(() => vi.fn())
const cancelOrder = vi.hoisted(() => vi.fn())
const verifyOrder = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const showWarning = vi.hoisted(() => vi.fn())
const toCanvas = vi.hoisted(() => vi.fn())
const routerPush = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({ useRouter: () => ({ push: routerPush }) }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: ref('zh-CN'),
    }),
  }
})

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    pollOrderStatus,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showWarning,
  }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    cancelOrder,
    verifyOrder,
  },
}))

vi.mock('qrcode', () => ({
  default: {
    toCanvas,
  },
}))

import PaymentStatusPanel from '../PaymentStatusPanel.vue'

const orderFactory = (status: string) => ({
  id: 42,
  user_id: 9,
  amount: 88,
  pay_amount: 88,
  fee_rate: 0,
  payment_type: 'alipay',
  out_trade_no: 'sub2_20260420abcd1234',
  status,
  order_type: 'balance',
  created_at: '2026-04-20T12:00:00Z',
  expires_at: '2099-01-01T12:30:00Z',
  refund_amount: 0,
})

describe('PaymentStatusPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    pollOrderStatus.mockReset()
    cancelOrder.mockReset()
    verifyOrder.mockReset()
    showError.mockReset()
    showWarning.mockReset()
    toCanvas.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows one shared presale confirmation with subscription and close actions', async () => {
    vi.setSystemTime(new Date('2026-09-25T12:00:00Z'))
    const order = { ...orderFactory('COMPLETED'), order_type: 'subscription', presale_starts_at: '2026-10-01T00:00:00+08:00', presale_expires_at: '2026-11-01T00:00:00+08:00', presale_plan_name: '中杯' }
    pollOrderStatus.mockResolvedValue(order)
    const wrapper = mount(PaymentStatusPanel, {
      props: { orderId: 42, qrCode: '', expiresAt: '2099-01-01T12:30:00Z', paymentType: 'alipay', orderType: 'subscription' },
    })
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(wrapper.findAll('[data-test="presale-success"]')).toHaveLength(1)
    expect(wrapper.text().split('presale.purchased')).toHaveLength(2)
    expect(wrapper.text()).toContain('中杯')
    expect(wrapper.get('details').attributes('open')).toBeUndefined()
    expect(wrapper.emitted('success')).toEqual([[order]])
    await wrapper.get('.btn-primary').trigger('click')
    expect(routerPush).toHaveBeenCalledWith('/subscriptions')
    await wrapper.get('.confirmation-secondary').trigger('click')
    expect(wrapper.emitted('done')).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(9000)
    expect(pollOrderStatus).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it.each([
    ['CANCELLED', 'cancelled'],
    ['PAID', 'success'],
    ['PENDING', null],
    [null, null],
  ])('checks the actual order after cancellation: %s', async (status, expectedOutcome) => {
    cancelOrder.mockResolvedValue({ data: { message: 'ok' } })
    pollOrderStatus.mockResolvedValue(status ? orderFactory(status) : null)
    const wrapper = mount(PaymentStatusPanel, {
      props: { orderId: 42, qrCode: '', expiresAt: '2099-01-01T12:30:00Z', paymentType: 'alipay' },
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(cancelOrder).toHaveBeenCalledWith(42)
    expect(pollOrderStatus).toHaveBeenCalledWith(42)
    if (expectedOutcome) {
      expect(wrapper.emitted('settled')).toEqual([[expectedOutcome]])
      expect(showWarning).not.toHaveBeenCalled()
      if (expectedOutcome === 'success') {
        expect(wrapper.emitted('success')).toEqual([[orderFactory('PAID')]])
      }
    } else {
      expect(wrapper.emitted('settled')).toBeUndefined()
      expect(showWarning).toHaveBeenCalledWith('payment.errors.originalOrderUnsettled')
      await vi.advanceTimersByTimeAsync(3000)
      expect(pollOrderStatus).toHaveBeenCalledTimes(2)
    }
    wrapper.unmount()
  })

  it('treats RECHARGING as a successful terminal state', async () => {
    pollOrderStatus.mockResolvedValue(orderFactory('RECHARGING'))

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'alipay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(pollOrderStatus).toHaveBeenCalledWith(42)
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.emitted('success')).toHaveLength(1)
    expect(wrapper.emitted('success')?.[0]).toEqual([orderFactory('RECHARGING')])
  })

  it('shows reopen button in QR mode when payUrl is also available', async () => {
    const openSpy = vi.spyOn(window, 'open').mockReturnValue({ closed: false } as Window)

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        payUrl: 'https://pay.example.com/session/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'alipay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('payment.qr.openPayWindow')

    await wrapper.get('button.btn.btn-secondary.text-sm').trigger('click')
    expect(openSpy).toHaveBeenCalledWith(
      'https://pay.example.com/session/42',
      'paymentPopup',
      expect.any(String),
    )

    openSpy.mockRestore()
  })

  it('actively verifies a stuck pending order and settles it when upstream confirms payment', async () => {
    pollOrderStatus.mockResolvedValue(orderFactory('PENDING'))
    verifyOrder.mockResolvedValue({
      data: orderFactory('COMPLETED'),
    })

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'wxpay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(pollOrderStatus).toHaveBeenCalledWith(42)
    expect(verifyOrder).toHaveBeenCalledWith('sub2_20260420abcd1234')
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.emitted('success')).toHaveLength(1)
    expect(wrapper.emitted('success')?.[0]).toEqual([orderFactory('COMPLETED')])
  })
})
