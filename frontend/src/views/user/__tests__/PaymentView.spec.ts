import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import type { PaymentOrder } from '@/types/payment'

const routeState = vi.hoisted(() => ({
  path: '/purchase',
  query: {} as Record<string, unknown>,
}))

const routerReplace = vi.hoisted(() => vi.fn())
const routerPush = vi.hoisted(() => vi.fn())
const routerResolve = vi.hoisted(() => vi.fn(() => ({ href: '/payment/stripe?mock=1' })))
const createOrder = vi.hoisted(() => vi.fn())
const previewCafeCoupon = vi.hoisted(() => vi.fn())
const getCafeCouponInfo = vi.hoisted(() => vi.fn())
const refreshUser = vi.hoisted(() => vi.fn())
const fetchActiveSubscriptions = vi.hoisted(() => vi.fn().mockResolvedValue(undefined))
const activeSubscriptionsState = vi.hoisted(() => [] as any[])
const showError = vi.hoisted(() => vi.fn())
const showInfo = vi.hoisted(() => vi.fn())
const showWarning = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
const getCheckoutInfo = vi.hoisted(() => vi.fn())
const getOrder = vi.hoisted(() => vi.fn())
const cancelOrder = vi.hoisted(() => vi.fn())
const createPaymentOrderIdempotencyKey = vi.hoisted(() => vi.fn(() => 'payment-order-test-key'))
const getPresaleQuote = vi.hoisted(() => vi.fn())
vi.mock('@/api/presale', () => ({ presaleAPI: { quote: getPresaleQuote } }))

const bridgeInvoke = vi.hoisted(() => vi.fn())

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({
      replace: routerReplace,
      push: routerPush,
      resolve: routerResolve,
    }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'payment.cafeCoupon.errors.CAFE_COUPON_NOT_FOUND': '券码无效',
    'payment.cafeCoupon.invalid': '券码无效',
    'payment.cafeCoupon.applied': 'Café券已应用',
    'payment.cafeCoupon.appliedDiscount': '券成功应用，折扣 {value}%',
    'payment.cafeCoupon.appliedCash': '券成功应用，抵扣 {amount}',
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
      locale: { value: 'zh' },
    }),
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: {
      username: 'demo-user',
      balance: 0,
    },
    refreshUser,
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    createOrder,
    previewCafeCoupon,
    getCafeCouponInfo,
  }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: activeSubscriptionsState,
    fetchActiveSubscriptions,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showInfo,
    showWarning,
    showSuccess,
  }),
}))

vi.mock('@/api/payment', () => ({
  createPaymentOrderIdempotencyKey,
  paymentAPI: {
    getCheckoutInfo,
    getOrder,
    cancelOrder,
  },
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => true,
}))

function checkoutInfoFixture() {
  return {
    data: {
      methods: {
        wxpay: {
          daily_limit: 0,
          daily_used: 0,
          daily_remaining: 0,
          single_min: 0,
          single_max: 0,
          fee_rate: 0,
          available: true,
        },
      },
      global_min: 0,
      global_max: 0,
      plans: [],
      balance_disabled: false,
      balance_recharge_multiplier: 1,
      recharge_fee_rate: 0,
      help_text: '',
      help_image_url: '',
      stripe_publishable_key: '',
    },
  }
}

function checkoutInfoWithPlansFixture() {
  return {
    data: {
      ...checkoutInfoFixture().data,
      plans: [
        {
          id: 7,
          group_id: 3,
          name: 'Starter',
          description: '',
          price: 128,
          original_price: 0,
          validity_days: 30,
          validity_unit: 'day',
          rate_multiplier: 1,
          daily_limit_usd: null,
          weekly_limit_usd: null,
          monthly_limit_usd: null,
          features: [],
          group_platform: 'openai',
          sort_order: 1,
          for_sale: true,
          group_name: 'OpenAI',
          custom_multiplier_enabled: true,
          custom_multiplier_min: 2,
          custom_multiplier_max: 5,
        },
      ],
    },
  }
}

function jsapiOrderFixture(resumeToken: string) {
  return {
    order_id: 123,
    amount: 88,
    pay_amount: 88,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    payment_type: 'wxpay',
    out_trade_no: 'sub2_jsapi_123',
    result_type: 'jsapi_ready' as const,
    resume_token: resumeToken,
    jsapi: {
      appId: 'wx123',
      timeStamp: '1712345678',
      nonceStr: 'nonce',
      package: 'prepay_id=wx123',
      signType: 'RSA',
      paySign: 'signed',
    },
  }
}

function serverPresaleOrder(overrides: Partial<PaymentOrder> = {}): PaymentOrder {
  return {
    id: 123, user_id: 1, amount: 128, pay_amount: 128, fee_rate: 0,
    payment_type: 'wxpay', out_trade_no: 'sub2_jsapi_123', status: 'PENDING',
    order_type: 'subscription', plan_id: 7, created_at: '2026-09-23T00:00:00Z',
    expires_at: '2099-01-01T00:00:00Z', refund_amount: 0,
    presale_starts_at: '2026-09-30T16:00:00Z', ...overrides,
  }
}

function saveRecovery(orderType: 'balance' | 'subscription' = 'subscription') {
  window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
    orderId: 123, amount: 128, qrCode: 'weixin://original', expiresAt: '2099-01-01T00:00:00Z',
    paymentType: 'wxpay', payUrl: '', outTradeNo: 'sub2_jsapi_123', clientSecret: '',
    payAmount: 128, orderType, paymentMode: 'qrcode', resumeToken: 'original-resume', createdAt: Date.now(),
  }))
}

interface PresaleCheckoutVM {
  presalePaid: boolean
  canSubmitSubscription: boolean
  paymentPhase: string
  paymentState: { orderId: number }
  confirmSubscribe: () => Promise<void>
  onPaymentSuccess: (order: PaymentOrder) => void
  onPaymentDone: () => void
}

async function mountPresaleCheckout() {
  routeState.path = '/presale'
  routeState.query = { plan: '7' }
  getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())
  const wrapper = shallowMount(PaymentView, {
    props: { presalePlanId: 7, presaleMonth: '2026-10' },
    global: { stubs: { Teleport: true, Transition: false, RouterLink: true } },
  })
  await flushPromises()
  return { wrapper, vm: wrapper.vm as unknown as PresaleCheckoutVM }
}

function oauthOrderFixture() {
  return {
    order_id: 456,
    amount: 128,
    pay_amount: 128,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    payment_type: 'wxpay',
    result_type: 'oauth_required' as const,
    oauth: {
      authorize_url: '/api/v1/auth/oauth/wechat/payment/start?context_token=signed-context-token',
      appid: 'wx123',
      scope: 'snsapi_base',
      redirect_url: '/auth/wechat/payment/callback',
    },
  }
}

const SubscriptionPlanCardCouponPreviewStub = {
  name: 'SubscriptionPlanCard',
  props: ['plan', 'activeSubscriptions', 'couponPayAmount'],
  emits: ['multiplier-change', 'select'],
  mounted() {
    this.$emit('multiplier-change', this.plan, 2)
  },
  template: `
    <div
      :data-testid="'plan-card-' + plan.id"
      :data-coupon-pay-amount="couponPayAmount == null ? '' : String(couponPayAmount)"
    />
  `,
}

describe('PaymentView WeChat JSAPI flow', () => {
  beforeEach(() => {
    getPresaleQuote.mockReset().mockResolvedValue({ data: { month: '2026-10', starts_at: '2026-10-01T00:00:00+08:00', expires_at: '2026-11-01T00:00:00+08:00', full_refund_before: '2026-09-28T00:00:00+08:00', renewal: false } })
    routeState.path = '/purchase'
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-123',
    }
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    previewCafeCoupon.mockReset()
    getCafeCouponInfo.mockReset().mockResolvedValue({ valid: true, coupon: { code: 'CAFEPREVIEW', type: 'cash', value: 56 } })
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    showSuccess.mockReset()
    activeSubscriptionsState.splice(0)
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture())
    cancelOrder.mockReset().mockResolvedValue({ data: { message: 'cancelled' } })
    getOrder.mockReset().mockResolvedValue({ data: serverPresaleOrder({ status: 'CANCELLED' }) })
    createPaymentOrderIdempotencyKey.mockReset().mockReturnValue('payment-order-test-key')
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = {
      invoke: bridgeInvoke,
    }
  })

  it('honors the recharge tab from route query before coupon preview', async () => {
    routeState.query = { tab: 'recharge' }
    getCheckoutInfo.mockResolvedValue(checkoutInfoFixture())

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('recharge')
  })

  it('defaults to balance recharge without subscription tabs', async () => {
    routeState.query = {}

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      activeTab: string
      tabs: Array<{ key: string; label: string }>
    }
    expect(vm.activeTab).toBe('recharge')
    expect(vm.tabs).toBeUndefined()
  })

  it('does not re-enable subscriptions when balance recharge is disabled', async () => {
    routeState.query = {}
    getCheckoutInfo.mockResolvedValue({
      data: {
        ...checkoutInfoWithPlansFixture().data,
        balance_disabled: true,
      },
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as { tabs: Array<{ key: string; label: string }> }
    expect(vm.tabs).toBeUndefined()
    expect(wrapper.text()).toContain('payment.usagePolicyWarning')
  })

  it('keeps the usage warning visible while confirming a selected plan', async () => {
    routeState.query = {}
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      checkout: { plans: Array<unknown> }
      selectPlan: (plan: unknown) => void
      selectedPlan: unknown
    }
    vm.selectPlan(vm.checkout.plans[0])
    await flushPromises()

    expect(vm.selectedPlan).not.toBeNull()
    expect(wrapper.text()).toContain('payment.usagePolicyWarning')
  })

  it('redirects legacy subscription renewal links to the presale page', async () => {
    routeState.query = { tab: 'subscription', plan: '7' }
    const wrapper = shallowMount(PaymentView)
    await flushPromises()
    expect(routerReplace).toHaveBeenCalledWith({ path: '/presale', query: { plan: '7' } })
    expect(getCheckoutInfo).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('displays billing rate instead of custom subscription multiplier in purchase details', async () => {
    routeState.query = { tab: 'subscription', plan: '7', multiplier: '4' }
    const customSub = {
      id: 88,
      user_id: 1,
      group_id: 3,
      status: 'active',
      expires_at: '2099-01-01T00:00:00Z',
      custom_multiplier: 4,
      custom_source_plan_id: 7,
      custom_source_group_id: 3,
      custom_expires_at: '2099-01-01T00:00:00Z',
      custom_display_name: 'Starter-4x',
      group: {
        id: 3,
        name: 'Starter-4x',
        rate_multiplier: 1.25,
        is_custom_subscription_group: false,
      },
    }
    activeSubscriptionsState.push(customSub)
    const checkout = checkoutInfoWithPlansFixture()
    checkout.data.plans[0].rate_multiplier = 1.25
    getCheckoutInfo.mockResolvedValue(checkout)

    const wrapper = shallowMount(PaymentView, {
      props: { presalePlanId: 7, presaleMonth: '2026-10' },
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      selectedPlanRateDisplay: string
      activeSubscriptionRateDisplay: (subscription: typeof customSub) => string
    }
    expect(vm.selectedPlanRateDisplay).toBe('1.25x')
    expect(vm.activeSubscriptionRateDisplay(customSub)).toBe('1.25x')
  })

  it('resets payment state and redirects to /payment/result after JSAPI reports success', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-123'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:ok' })
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(routerPush).toHaveBeenCalledWith({
      path: '/payment/result',
      query: {
        order_id: '123',
        out_trade_no: 'sub2_jsapi_123',
        resume_token: 'resume-token-123',
      },
    })
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('resets payment state when JSAPI reports cancellation', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-cancel'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:cancel' })
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(showInfo).toHaveBeenCalledWith('payment.qr.cancelled')
    expect(cancelOrder).toHaveBeenCalledWith(123)
    expect(getOrder).toHaveBeenCalledWith(123)
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('keeps recovery when JSAPI is unavailable and cancellation cannot be confirmed', async () => {
    vi.useFakeTimers()
    cancelOrder.mockRejectedValue(new Error('offline'))
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-missing-bridge'))
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    await flushPromises()

    expect(showWarning).toHaveBeenCalledWith('payment.errors.originalOrderUnsettled')
    expect(createOrder).toHaveBeenCalledTimes(1)
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('resume-token-missing-bridge')
    expect(wrapper.html()).toContain('payment-status-panel-stub')
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('clears a stale recovery snapshot before handling wechat resume callback params', async () => {
    createOrder.mockRejectedValueOnce(new Error('resume failed'))
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 999,
      amount: 66,
      qrCode: 'stale-qr',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/stale',
      outTradeNo: 'stale-out-trade-no',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 66,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: '',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }))

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      wechat_resume_token: 'resume-token-123',
    }), 'payment-order-test-key')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('uses the signed OAuth context token for token-only WeChat callbacks', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-subscription-7',
      payment_type: 'wxpay_direct',
      order_type: 'subscription',
      plan_id: '7',
    }
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())
    createOrder.mockResolvedValue(oauthOrderFixture())

    const originalLocation = window.location
    const locationState = {
      href: 'http://localhost/purchase',
      origin: 'http://localhost',
    }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState,
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      payment_type: 'wxpay',
      order_type: 'subscription',
      plan_id: 7,
      multiplier: 2,
      wechat_resume_token: 'resume-subscription-7',
    }), 'payment-order-test-key')
    expect(locationState.href).toContain('/api/v1/auth/oauth/wechat/payment/start?')
    const parsedAuthorizeUrl = new URL(locationState.href, 'http://localhost')
    expect(parsedAuthorizeUrl.searchParams.get('context_token')).toBe('signed-context-token')
    expect(parsedAuthorizeUrl.searchParams.get('redirect')).toBeNull()
    expect(parsedAuthorizeUrl.searchParams.get('payment_type')).toBeNull()
    expect(parsedAuthorizeUrl.searchParams.get('order_type')).toBeNull()
    expect(parsedAuthorizeUrl.searchParams.get('plan_id')).toBeNull()
    expect(parsedAuthorizeUrl.searchParams.get('multiplier')).toBeNull()
    expect(parsedAuthorizeUrl.searchParams.get('amount')).toBeNull()
    expect(parsedAuthorizeUrl.searchParams.get('cafe_coupon_code')).toBeNull()

    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    })
  })

  it('preserves café coupon code through token-only WeChat resume payloads', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-coupon-7',
      payment_type: 'wxpay_direct',
      order_type: 'subscription',
      plan_id: '7',
      cafe_coupon_code: 'CAFE-KEEP123',
    }
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())
    createOrder.mockResolvedValue({
      order_id: 779,
      amount: 128,
      pay_amount: 128,
      fee_rate: 0,
      expires_at: '2099-01-01T00:10:00.000Z',
      payment_type: 'wxpay',
      qr_code: 'weixin://wxpay/bizpayurl?pr=coupon-resume',
      out_trade_no: 'sub2_coupon_779',
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      payment_type: 'wxpay',
      order_type: 'subscription',
      plan_id: 7,
      wechat_resume_token: 'resume-coupon-7',
      cafe_coupon_code: 'CAFE-KEEP123',
    }), 'payment-order-test-key')
    expect(previewCafeCoupon).not.toHaveBeenCalled()
  })

  it('shows Cafe coupon discount in recharge amount summary', async () => {
    getCheckoutInfo.mockResolvedValue({
      data: {
        ...checkoutInfoFixture().data,
        recharge_fee_rate: 1,
      },
    })
    previewCafeCoupon.mockResolvedValueOnce({
      valid: true,
      discount_amount: 50,
      pay_amount: 808,
      coupon: { code: 'CAFE50', type: 'cash', value: 50 },
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      activeTab: string
      amount: number
      cafeCouponCode: string
      previewCafeCoupon: () => Promise<void>
      rechargeCouponPayableAmount: number | null
      rechargeCouponDiscountAmount: number | null
      feeAmount: number
      rechargeTotalAmount: number
      rechargeButtonAmount: number
    }
    vm.activeTab = 'recharge'
    vm.amount = 858
    vm.cafeCouponCode = 'CAFE50'
    await vm.previewCafeCoupon()
    await flushPromises()

    expect(vm.rechargeCouponPayableAmount).toBe(808)
    expect(vm.rechargeCouponDiscountAmount).toBe(50)
    expect(vm.feeAmount).toBe(8.08)
    expect(vm.rechargeTotalAmount).toBe(816.08)
    expect(vm.rechargeButtonAmount).toBe(816.08)
  })

  it('shows Cafe coupon discount in subscription amount summary', async () => {
    getCheckoutInfo.mockResolvedValue({
      data: {
        ...checkoutInfoWithPlansFixture().data,
        recharge_fee_rate: 1,
      },
    })
    previewCafeCoupon.mockResolvedValueOnce({
      valid: true,
      discount_amount: 56,
      pay_amount: 200,
      coupon: { code: 'CAFESUB', type: 'cash', value: 56 },
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      checkout: { plans: Array<Record<string, unknown>> }
      selectPlan: (plan: Record<string, unknown>) => void
      cafeCouponCode: string
      previewCafeCoupon: () => Promise<void>
      subscriptionCouponPayableAmount: number | null
      subscriptionCouponDiscountAmount: number | null
      subFeeAmount: number
      subTotalAmount: number
      subscriptionButtonAmount: number
    }
    vm.selectPlan(vm.checkout.plans[0])
    vm.cafeCouponCode = 'CAFESUB'
    await vm.previewCafeCoupon()
    await flushPromises()

    expect(previewCafeCoupon).toHaveBeenCalledWith(expect.objectContaining({
      code: 'CAFESUB',
      amount: 256,
      order_type: 'subscription',
      plan_id: 7,
      multiplier: 2,
    }))
    expect(vm.subscriptionCouponPayableAmount).toBe(200)
    expect(vm.subscriptionCouponDiscountAmount).toBe(56)
    expect(vm.subFeeAmount).toBe(2)
    expect(vm.subTotalAmount).toBe(202)
    expect(vm.subscriptionButtonAmount).toBe(202)
  })

  it('does not show a success toast for route Cafe coupon before validation', async () => {
    routeState.query = { tab: 'subscription', cafe_coupon_code: 'CAFE-NOTFOUND' }
    getCheckoutInfo.mockResolvedValue({
      data: {
        ...checkoutInfoWithPlansFixture().data,
        balance_disabled: true,
      },
    })
    getCafeCouponInfo.mockResolvedValueOnce({ valid: false, coupon: null })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
          SubscriptionPlanCard: SubscriptionPlanCardCouponPreviewStub,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(showSuccess).not.toHaveBeenCalled()
    expect(previewCafeCoupon).not.toHaveBeenCalled()
  })

  it('honors an explicit route multiplier instead of silently using the active custom multiplier', async () => {
    routeState.query = { tab: 'subscription', plan: '7', multiplier: '2' }
    activeSubscriptionsState.push({
      id: 101,
      user_id: 1,
      group_id: 3,
      status: 'active',
      starts_at: '2026-01-01T00:00:00.000Z',
      expires_at: '2099-01-01T00:00:00.000Z',
      custom_multiplier: 4,
      custom_source_plan_id: 7,
      custom_source_group_id: 3,
      custom_expires_at: '2099-01-01T00:00:00.000Z',
      custom_display_name: 'Starter-4x',
      daily_usage_usd: 0,
      weekly_usage_usd: 0,
      monthly_usage_usd: 0,
      daily_window_start: null,
      weekly_window_start: null,
      monthly_window_start: null,
      created_at: '2026-01-01T00:00:00.000Z',
      updated_at: '2026-01-01T00:00:00.000Z',
      group: { id: 3, name: 'Starter' },
    })
    getCheckoutInfo.mockResolvedValue({
      data: {
        ...checkoutInfoWithPlansFixture().data,
        balance_disabled: true,
      },
    })

    const wrapper = shallowMount(PaymentView, {
      props: { presalePlanId: 7, presaleMonth: '2026-10' },
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
          SubscriptionPlanCard: SubscriptionPlanCardCouponPreviewStub,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      selectedSubscriptionMultiplier: number
      effectiveSelectedMultiplier: number
      effectiveSelectedPlanPrice: number
      selectedMultiplierConflictsActiveCustom: boolean
      canSubmitSubscription: boolean
    }
    expect(vm.selectedSubscriptionMultiplier).toBe(2)
    expect(vm.effectiveSelectedMultiplier).toBe(2)
    expect(vm.effectiveSelectedPlanPrice).toBe(256)
    expect(vm.selectedMultiplierConflictsActiveCustom).toBe(false)
    expect(vm.canSubmitSubscription).toBe(true)
  })

  it('does not restore the removed subscription catalog for a coupon deep link', async () => {
    routeState.query = { cafe_coupon_code: 'CAFEPREVIEW' }
    getCheckoutInfo.mockResolvedValue({
      data: {
        ...checkoutInfoWithPlansFixture().data,
        balance_disabled: true,
      },
    })
    getCafeCouponInfo.mockResolvedValueOnce({
      valid: true,
      coupon: { code: 'CAFEPREVIEW', type: 'cash', value: 56 },
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
          SubscriptionPlanCard: SubscriptionPlanCardCouponPreviewStub,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(wrapper.find('[data-testid="plan-card-7"]').exists()).toBe(false)
    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('recharge')
    wrapper.unmount()
  })

  it('uses the card-selected multiplier in the confirmed price, quota and order without an editable input', async () => {
    routeState.query = { multiplier: '4' }
    const checkout = checkoutInfoWithPlansFixture()
    getCheckoutInfo.mockResolvedValue({ data: { ...checkout.data, plans: [{ ...checkout.data.plans[0], daily_limit_usd: 10 }] } })
    createOrder.mockResolvedValue({ ...jsapiOrderFixture('presale-resume'), result_type: 'order_created', qr_code: 'weixin://wxpay/presale', payment_mode: 'qrcode', status: 'PENDING' })
    const wrapper = shallowMount(PaymentView, { props: { presalePlanId: 7, presaleMonth: '2026-10', presaleMultiplier: 3 }, global: { stubs: { Teleport: true, Transition: false } } })
    await flushPromises()
    const vm = wrapper.vm as unknown as { effectiveSelectedMultiplier: number; effectiveSelectedPlanPrice: number; effectiveSelectedDailyLimit: number; confirmSubscribe: () => Promise<void> }
    expect(vm.effectiveSelectedMultiplier).toBe(3)
    expect(vm.effectiveSelectedPlanPrice).toBe(384)
    expect(vm.effectiveSelectedDailyLimit).toBe(30)
    expect(wrapper.find('input[type="number"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('payment.admin.customMultiplierEnabled')
    await vm.confirmSubscribe(); await flushPromises()
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({ order_type: 'subscription', plan_id: 7, presale_month: '2026-10', multiplier: 3, amount: 384 }), 'payment-order-test-key')
    wrapper.unmount()
  })

  it('limits next-month renewal choices to the current plan bounds', async () => {
    const checkout = checkoutInfoWithPlansFixture()
    checkout.data.plans[0].custom_multiplier_max = 3
    getCheckoutInfo.mockResolvedValue(checkout)
    activeSubscriptionsState.push({ status: 'active', expires_at: '2099-01-01T00:00:00Z', custom_source_plan_id: 7, custom_multiplier: 4 })
    const wrapper = shallowMount(PaymentView, { props: { presalePlanId: 7, presaleMonth: '2026-10', presaleMultiplier: 4 }, global: { stubs: { Teleport: true, Transition: false } } })
    await flushPromises()
    const vm = wrapper.vm as unknown as { effectiveSelectedMultiplier: number; effectiveSelectedPlanPrice: number; selectedMultiplierConflictsActiveCustom: boolean }
    expect(vm.effectiveSelectedMultiplier).toBe(3)
    expect(vm.effectiveSelectedPlanPrice).toBe(384)
    expect(vm.selectedMultiplierConflictsActiveCustom).toBe(false)
    wrapper.unmount()
  })

  it.each([[2, 4], [4, 2], [3, 1]])('submits a future %sx to %sx renewal without a current-term conflict', async (current, selected) => {
    routeState.query = {}
    const checkout = checkoutInfoWithPlansFixture()
    checkout.data.plans[0].custom_multiplier_min = 1
    getCheckoutInfo.mockResolvedValue(checkout)
    activeSubscriptionsState.push({ status: 'active', expires_at: '2099-01-01T00:00:00Z', custom_source_plan_id: 7, custom_multiplier: current })
    createOrder.mockResolvedValue({ ...jsapiOrderFixture('presale-renewal'), result_type: 'order_created', qr_code: 'weixin://wxpay/presale', payment_mode: 'qrcode', status: 'PENDING' })
    const wrapper = shallowMount(PaymentView, { props: { presalePlanId: 7, presaleMonth: '2026-10', presaleMultiplier: selected }, global: { stubs: { Teleport: true, Transition: false } } })
    await flushPromises()
    const vm = wrapper.vm as unknown as { effectiveSelectedMultiplier: number; effectiveSelectedPlanPrice: number; selectedMultiplierConflictsActiveCustom: boolean; confirmSubscribe: () => Promise<void> }
    expect(vm.effectiveSelectedMultiplier).toBe(selected)
    expect(vm.effectiveSelectedPlanPrice).toBe(128 * selected)
    expect(vm.selectedMultiplierConflictsActiveCustom).toBe(false)
    expect(wrapper.text()).not.toContain('payment.customMultiplierConflict')
    await vm.confirmSubscribe(); await flushPromises()
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({ order_type: 'subscription', plan_id: 7, multiplier: selected, amount: 128 * selected, presale_month: '2026-10' }), 'payment-order-test-key')
    expect(activeSubscriptionsState[0].custom_multiplier).toBe(current)
    wrapper.unmount()
  })

  it('checks eligibility before purchase without rendering the removed explanation or consent card', async () => {
    routeState.query = {}
    const quote = deferred<{ data: { month: string } }>()
    getPresaleQuote.mockReturnValue(quote.promise)
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())
    createOrder.mockResolvedValue({ ...jsapiOrderFixture('presale-resume'), result_type: 'order_created', qr_code: 'weixin://wxpay/presale', payment_mode: 'qrcode', status: 'PENDING' })
    const wrapper = shallowMount(PaymentView, { props: { presalePlanId: 7, presaleMonth: '2026-10' }, global: { stubs: { Teleport: true, Transition: false } } })
    await flushPromises()
    const vm = wrapper.vm as unknown as { canSubmitSubscription: boolean; confirmSubscribe: () => Promise<void> }
    expect(vm.canSubmitSubscription).toBe(false)
    await vm.confirmSubscribe(); expect(createOrder).not.toHaveBeenCalled()
    quote.resolve({ data: { month: '2026-10' } }); await flushPromises()
    expect(vm.canSubmitSubscription).toBe(true)
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    for (const key of ['newSubscription', 'renewalCopy', 'termCopy', 'timezone', 'refundFullCopy', 'refundFeeCopy', 'refundDailyCopy', 'consent']) {
      expect(wrapper.text()).not.toContain(`presale.${key}`)
    }
    await vm.confirmSubscribe(); await flushPromises()
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({ order_type: 'subscription', plan_id: 7, presale_month: '2026-10' }), 'payment-order-test-key')
    wrapper.unmount()
  })

  it.each(['closed', 'month-changed'])('still blocks checkout when presale eligibility is %s', async (reason) => {
    if (reason === 'closed') getPresaleQuote.mockRejectedValue(new Error('closed'))
    else getPresaleQuote.mockResolvedValue({ data: { month: '2026-11' } })
    const { wrapper, vm } = await mountPresaleCheckout()
    expect(vm.canSubmitSubscription).toBe(false)
    await vm.confirmSubscribe()
    expect(createOrder).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([
    { order_type: 'balance' as const, presale_starts_at: undefined },
    { plan_id: 8 },
    { presale_starts_at: '2026-11-01T00:00:00+08:00' },
  ])('does not restore another checkout into a presale: %j', async (other) => {
    saveRecovery()
    getOrder.mockResolvedValue({ data: serverPresaleOrder(other) })
    const { wrapper, vm } = await mountPresaleCheckout()
    expect(getOrder).toHaveBeenCalledWith(123)
    expect(vm.paymentPhase).toBe('select')
    expect(vm.paymentState.orderId).toBe(0)
    expect(vm.presalePaid).toBe(false)
    expect(getPresaleQuote).toHaveBeenCalledWith(7)
    expect(createOrder).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('original-resume')
    wrapper.unmount()
  })

  it('restores the matching order without rejecting its own reserved month', async () => {
    saveRecovery()
    getOrder.mockResolvedValue({ data: serverPresaleOrder() })
    getPresaleQuote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED' })
    const { wrapper, vm } = await mountPresaleCheckout()
    expect(vm.paymentState.orderId).toBe(123)
    expect(vm.paymentPhase).toBe('paying')
    expect(getPresaleQuote).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    vm.onPaymentSuccess(serverPresaleOrder({ status: 'COMPLETED' }))
    await flushPromises()
    expect(vm.presalePaid).toBe(true)
    expect(wrapper.text()).toContain('presale.purchased')
    vm.onPaymentDone()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('does not report balance, another plan/month or a stale order event as presale success', async () => {
    saveRecovery()
    getOrder.mockResolvedValue({ data: serverPresaleOrder() })
    const { wrapper, vm } = await mountPresaleCheckout()
    for (const other of [
      serverPresaleOrder({ order_type: 'balance', presale_starts_at: undefined }),
      serverPresaleOrder({ plan_id: 8 }),
      serverPresaleOrder({ presale_starts_at: '2026-11-01T00:00:00+08:00' }),
      serverPresaleOrder({ id: 999 }),
    ]) {
      vm.onPaymentSuccess(other)
      expect(vm.presalePaid).toBe(false)
    }
    wrapper.unmount()
  })

  it('does not trust recovery when the authenticated order cannot be read', async () => {
    saveRecovery()
    getOrder.mockRejectedValue(new Error('offline'))
    const { wrapper, vm } = await mountPresaleCheckout()
    expect(vm.paymentState.orderId).toBe(0)
    expect(vm.canSubmitSubscription).toBe(false)
    expect(createOrder).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('original-resume')
    wrapper.unmount()
  })

  it('waits for confirmed cancellation before creating a presale QR fallback', async () => {
    const cancelled = deferred<{ data: { message: string } }>()
    cancelOrder.mockReturnValue(cancelled.promise)
    createOrder.mockResolvedValueOnce(jsapiOrderFixture('original-resume')).mockResolvedValueOnce({
      ...jsapiOrderFixture('qr-resume'), order_id: 124, result_type: 'order_created', payment_mode: 'qrcode', qr_code: 'weixin://fallback',
    })
    bridgeInvoke.mockImplementation((_action, _payload, callback) => callback({ err_msg: 'get_brand_wcpay_request:fail' }))
    const { wrapper, vm } = await mountPresaleCheckout()
    const submitting = vm.confirmSubscribe()
    await flushPromises()
    expect(cancelOrder).toHaveBeenCalledWith(123)
    expect(createOrder).toHaveBeenCalledTimes(1)
    expect(vm.paymentState.orderId).toBe(123)
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('original-resume')
    cancelled.resolve({ data: { message: 'cancelled' } })
    await submitting
    expect(getOrder).toHaveBeenCalledWith(123)
    expect(getOrder.mock.invocationCallOrder[0]).toBeLessThan(createOrder.mock.invocationCallOrder[1])
    expect(createOrder.mock.calls[1][0]).toMatchObject({ order_type: 'subscription', plan_id: 7, presale_month: '2026-10', is_mobile: false })
    expect(vm.paymentState.orderId).toBe(124)
    expect(vm.paymentPhase).toBe('paying')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('qr-resume')
    wrapper.unmount()
  })

  it.each(['PAID', 'COMPLETED', 'PENDING'] as const)('does not retry if cancellation leaves the original order %s', async (status) => {
    getOrder.mockResolvedValue({ data: serverPresaleOrder({ status }) })
    createOrder.mockResolvedValue(jsapiOrderFixture('original-resume'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => callback({ err_msg: 'get_brand_wcpay_request:fail' }))
    const { wrapper, vm } = await mountPresaleCheckout()
    await vm.confirmSubscribe()
    expect(createOrder).toHaveBeenCalledTimes(1)
    expect(vm.paymentState.orderId).toBe(123)
    expect(vm.paymentPhase).toBe('paying')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('original-resume')
    expect(showWarning).toHaveBeenCalledWith('payment.errors.originalOrderUnsettled')
    wrapper.unmount()
  })

  it('preserves the original order when cancellation fails after JSAPI dismissal', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('original-resume'))
    cancelOrder.mockRejectedValue(new Error('response lost'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => callback({ err_msg: 'get_brand_wcpay_request:cancel' }))
    const { wrapper, vm } = await mountPresaleCheckout()
    await vm.confirmSubscribe()
    expect(vm.paymentState.orderId).toBe(123)
    expect(vm.paymentPhase).toBe('paying')
    expect(showInfo).not.toHaveBeenCalledWith('payment.qr.cancelled')
    expect(createOrder).toHaveBeenCalledTimes(1)
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('original-resume')
    wrapper.unmount()
  })

  it('closes a settled presale without exposing unrelated plan selection', async () => {
    const { wrapper, vm } = await mountPresaleCheckout()
    vm.onPaymentDone()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('ignores stale Cafe coupon responses after switching order context', async () => {
    getCheckoutInfo.mockResolvedValue({
      data: {
        ...checkoutInfoWithPlansFixture().data,
        recharge_fee_rate: 1,
      },
    })
    const first = deferred<{ valid: boolean; discount_amount: number; pay_amount: number; coupon: { code: string; type: 'cash'; value: number } }>()
    const second = deferred<{ valid: boolean; discount_amount: number; pay_amount: number; coupon: { code: string; type: 'cash'; value: number } }>()
    previewCafeCoupon
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      activeTab: string
      amount: number
      checkout: { plans: Array<Record<string, unknown>> }
      selectPlan: (plan: Record<string, unknown>) => void
      cafeCouponCode: string
      previewCafeCoupon: () => Promise<void>
      rechargeCouponPayableAmount: number | null
      subscriptionCouponPayableAmount: number | null
    }
    vm.activeTab = 'recharge'
    vm.amount = 100
    vm.cafeCouponCode = 'CAFE-STALE'
    const firstApply = vm.previewCafeCoupon()
    await flushPromises()

    vm.activeTab = 'subscription'
    vm.selectPlan(vm.checkout.plans[0])
    const secondApply = vm.previewCafeCoupon()
    await flushPromises()
    expect(previewCafeCoupon).toHaveBeenCalledTimes(2)

    first.resolve({
      valid: true,
      discount_amount: 20,
      pay_amount: 80,
      coupon: { code: 'CAFE-STALE', type: 'cash', value: 20 },
    })
    await firstApply
    await flushPromises()
    expect(vm.rechargeCouponPayableAmount).toBeNull()
    expect(vm.subscriptionCouponPayableAmount).toBeNull()

    second.resolve({
      valid: true,
      discount_amount: 56,
      pay_amount: 200,
      coupon: { code: 'CAFE-STALE', type: 'cash', value: 56 },
    })
    await secondApply
    await flushPromises()
    expect(vm.subscriptionCouponPayableAmount).toBe(200)
  })

  it('does not create an order when typed café coupon is invalid', async () => {
    routeState.query = {}
    previewCafeCoupon.mockRejectedValueOnce({ reason: 'CAFE_COUPON_NOT_FOUND', message: 'cafe coupon not found' })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      amount: number
      cafeCouponCode: string
      handleSubmitRecharge: () => Promise<void>
      cafeCouponError: string
    }
    vm.amount = 100
    vm.cafeCouponCode = 'CAFE-NOTFOUND'

    await vm.handleSubmitRecharge()
    await flushPromises()

    expect(previewCafeCoupon).toHaveBeenCalledWith(expect.objectContaining({
      code: 'CAFE-NOTFOUND',
      amount: 100,
      order_type: 'balance',
    }))
    expect(createOrder).not.toHaveBeenCalled()
    expect(vm.cafeCouponError).toBe('券码无效')
    const firstCouponError = vm.cafeCouponError

    await vm.handleSubmitRecharge()
    await flushPromises()

    expect(previewCafeCoupon).toHaveBeenCalledTimes(1)
    expect(createOrder).not.toHaveBeenCalled()
    expect(vm.cafeCouponError).toBe(firstCouponError)

  })

  it('falls back to QR flow when mobile WeChat payment is unavailable', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-h5',
      payment_type: 'wxpay_direct',
    }
    createOrder
      .mockRejectedValueOnce({ reason: 'WECHAT_H5_NOT_AUTHORIZED' })
      .mockResolvedValueOnce({
        order_id: 778,
        amount: 88,
        pay_amount: 88,
        fee_rate: 0,
        expires_at: '2099-01-01T00:10:00.000Z',
        payment_type: 'wxpay',
        qr_code: 'weixin://wxpay/bizpayurl?pr=fallback-native',
        out_trade_no: 'sub2_qr_778',
      })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenNthCalledWith(1, expect.objectContaining({
      payment_type: 'wxpay',
      is_mobile: true,
      wechat_resume_token: 'resume-token-h5',
    }), 'payment-order-test-key')
    expect(createOrder).toHaveBeenNthCalledWith(2, expect.objectContaining({
      payment_type: 'wxpay',
      is_mobile: false,
      payment_source: 'hosted_redirect',
    }))
    expect(showWarning).toHaveBeenCalledWith('payment.errors.mobilePaymentFallbackToQr')
    expect(showError).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('weixin://wxpay/bizpayurl?pr=fallback-native')
  })
})
