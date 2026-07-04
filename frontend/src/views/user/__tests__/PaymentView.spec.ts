import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'

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
  paymentAPI: {
    getCheckoutInfo,
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

  it('defaults to subscription tab and lists it before top up', async () => {
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
    expect(vm.tabs.map((tab) => tab.key)).toEqual(['subscription', 'recharge'])
    expect(vm.activeTab).toBe('subscription')
  })



  it('selects the source plan when renewal route points at an active custom subscription group', async () => {
    routeState.query = { tab: 'subscription', group: '99' }
    activeSubscriptionsState.push({
      id: 44,
      group_id: 99,
      status: 'active',
      expires_at: '2099-01-01T00:00:00Z',
      group: {
        id: 99,
        name: 'Starter-custom-user',
        is_custom_subscription_group: true,
        custom_source_plan_id: 7,
        custom_source_group_id: 3,
        custom_multiplier: 3,
      },
    })
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())

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
      selectedPlan: { id: number } | null
      selectedSubscriptionMultiplier: number
    }
    expect(fetchActiveSubscriptions).toHaveBeenCalled()
    expect(vm.activeTab).toBe('subscription')
    expect(vm.selectedPlan?.id).toBe(7)
    expect(vm.selectedSubscriptionMultiplier).toBe(3)
  })


  it('displays custom subscription multiplier instead of base rate in purchase details', async () => {
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
      custom_display_name: '[4x]Starter#1',
      group: {
        id: 3,
        name: '[4x]Starter#1',
        rate_multiplier: 1,
        is_custom_subscription_group: false,
      },
    }
    activeSubscriptionsState.push(customSub)
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())

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
      selectedPlanRateDisplay: string
      activeSubscriptionRateDisplay: (subscription: typeof customSub) => string
    }
    expect(vm.selectedPlanRateDisplay).toBe('4x')
    expect(vm.activeSubscriptionRateDisplay(customSub)).toBe('4x')
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
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('clears stale recovery state when JSAPI never becomes available', async () => {
    vi.useFakeTimers()
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-missing-bridge'))
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      'payment.errors.wechatJsapiUnavailable payment.errors.wechatOpenInWeChatHint',
    )
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
    expect(wrapper.html()).not.toContain('payment-status-panel-stub')
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
    }))
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
    }))
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
    }))
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
      custom_display_name: '[4x]Starter#1',
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
    expect(vm.selectedMultiplierConflictsActiveCustom).toBe(true)
    expect(vm.canSubmitSubscription).toBe(false)
  })

  it('calculates Cafe coupon plan-card amount from one coupon info lookup', async () => {
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

    expect(getCafeCouponInfo).toHaveBeenCalledTimes(1)
    expect(getCafeCouponInfo).toHaveBeenCalledWith({ code: 'CAFEPREVIEW' })
    expect(previewCafeCoupon).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="plan-card-7"]').attributes('data-coupon-pay-amount')).toBe('200')
  })

  it('uses coupon info for the selected subscription confirmation amount before preview returns', async () => {
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

    const vm = wrapper.vm as unknown as {
      checkout: { plans: Array<Record<string, unknown>> }
      selectPlan: (plan: Record<string, unknown>, multiplier?: number) => void
      effectiveSelectedCouponPrice: number | null
      subscriptionCouponPayableAmount: number | null
      subscriptionButtonAmount: number
    }
    vm.selectPlan(vm.checkout.plans[0], 2)
    await flushPromises()

    expect(previewCafeCoupon).not.toHaveBeenCalled()
    expect(vm.effectiveSelectedCouponPrice).toBe(200)
    expect(vm.subscriptionCouponPayableAmount).toBe(200)
    expect(vm.subscriptionButtonAmount).toBe(200)
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
    }))
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
