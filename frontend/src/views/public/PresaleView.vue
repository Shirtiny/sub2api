<template>
  <div class="presale-page">
    <HomeHeader
      :compact="compactHeader"
      :site-name="siteName"
      :doc-url="docUrl"
      :entry-path="entryPath"
      :is-authenticated="auth.isAuthenticated"
      :is-dark="isDark"
      :presale-open="catalog?.enabled ?? false"
      @toggle-theme="toggleTheme"
    />
    <main>
      <section class="presale-hero">
        <div class="hero-copy">
          <p class="eyebrow">{{ t('presale.eyebrow') }}</p>
          <h1>{{ t('presale.title') }}<em>{{ t('presale.titleAccent') }}</em></h1>
          <p class="intro">{{ t('presale.intro') }}</p>
          <div class="hero-actions"><a href="#presale-plans" class="btn btn-primary">{{ t('presale.browse') }} <Icon name="arrowRight" size="sm" /></a><RouterLink :to="balancePath" class="quiet-link">{{ t('presale.balance') }} <span aria-hidden="true">↗</span></RouterLink></div>
        </div>
        <div class="month-art" aria-hidden="true">
          <svg class="month-setting" viewBox="0 0 520 440" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round">
            <g class="coffee-outline">
              <path d="M371 280H449L443 310C440 326 430 334 410 334S380 326 377 310Z"/>
              <path d="M449 286H455C477 286 474 315 445 315M363 343C382 352 441 352 459 343M382 339H440"/>
            </g>
            <g class="coffee-steam">
              <path d="M398 264C386 250 415 240 402 223"/>
              <path d="M418 263C406 247 435 232 421 214"/>
            </g>
          </svg>
          <div class="month-ticket">
            <span class="ticket-kicker">NEXT / {{ catalog?.period.month.split('-')[0] || '—' }}</span>
            <div class="ticket-date">
              <strong>{{ catalog?.period.month.split('-')[1] || '—' }}</strong>
              <span class="ticket-month-abbr" lang="en">{{ monthAbbreviation }}</span>
            </div>
            <span class="ticket-month">{{ date(catalog?.period.starts_at, true) }}</span>
            <div class="ticket-perforation"/>
            <div class="ticket-footer"><span>{{ t('presale.calendar') }}</span><span>01 — {{ daysInPeriod }}</span></div>
          </div>
          <span class="art-note">RESERVED FOR YOUR NEXT CHAPTER</span>
        </div>
      </section>
      <section v-if="catalog" class="timeline" :aria-label="t('presale.calendar')">
        <div v-for="(step, index) in timeline" :key="step.label" class="timeline-step"><span class="step-index">0{{ index + 1 }}</span><div><p class="step-label">{{ step.label }}</p><p class="step-date">{{ step.date }}</p><p class="step-copy">{{ step.copy }}</p></div></div>
      </section>
      <section id="presale-plans" class="plans-section">
        <div class="section-heading"><div><p class="eyebrow">{{ t('presale.collection') }}</p><h2>{{ t('presale.plansTitle') }}</h2></div><p>{{ t('presale.plansIntro') }}</p></div>
        <div v-if="loading" class="empty-state" role="status">{{ t('common.loading') }}</div>
        <div v-else-if="error" class="empty-state" role="alert"><p>{{ error }}</p><button class="btn btn-secondary mt-5" @click="load">{{ t('presale.retry') }}</button></div>
        <div v-else-if="!catalog?.enabled" class="empty-state"><p class="eyebrow">COMING NEXT</p><h3>{{ t('presale.empty') }}</h3><p>{{ t('presale.emptyCopy') }}</p><RouterLink :to="balancePath" class="quiet-link">{{ t('presale.balance') }} →</RouterLink></div>
        <div v-else class="plan-grid">
          <article v-for="(plan, index) in catalog.plans" :key="plan.id" class="presale-plan" :style="{ '--plan-index': index }">
            <div class="plan-top"><span class="plan-index">{{ String(index + 1).padStart(2, '0') }} / {{ plan.group_platform?.toUpperCase() }}</span><span v-if="plan.presale_badge" class="plan-badge">{{ plan.presale_badge }}</span></div>
            <h3>{{ plan.name }}</h3><p class="plan-description">{{ plan.description }}</p>
            <div class="plan-price"><span v-if="plan.original_price && plan.original_price > plan.price" class="old-price">￥{{ scaledPlanValue(plan, plan.original_price).toLocaleString(locale) }}</span><strong><span class="price-currency">￥</span>{{ scaledPlanValue(plan, plan.price).toLocaleString(locale) }}</strong><span>{{ t('presale.perMonth') }}</span></div>
            <dl class="plan-quotas"><template v-for="quota in quotas(plan)" :key="quota.label"><div><dt>{{ quota.label }}</dt><dd>{{ quota.value }}</dd></div></template></dl>
            <ul class="plan-features"><li><Icon name="check" size="sm"/>{{ t('presale.concurrency', { count: plan.concurrency }) }}</li><li v-for="feature in plan.features" :key="feature"><Icon name="check" size="sm"/>{{ feature }}</li></ul>
            <div v-if="plan.custom_multiplier_enabled" class="plan-multiplier">
              <span>{{ t('payment.planCard.multiplier') }}</span>
              <Select
                class="multiplier-select"
                :model-value="planMultiplier(plan)"
                :options="multiplierOptions(plan)"
                :placeholder="t('payment.planCard.multiplier')"
                :searchable="false"
                :clearable="false"
                :disabled="!canReserve(plan)"
                @update:model-value="updateMultiplier(plan, $event)"
              />
            </div>
            <div v-if="availability(plan).kind !== 'available' && availability(plan).kind !== 'checking'" class="plan-availability" role="status" :data-state="availability(plan).kind">
              <strong>{{ t(`presale.availability.${availability(plan).kind}`) }}</strong>
              <p>{{ availabilityMessage(plan) }}</p>
            </div>
            <div v-if="canResume(plan)" class="plan-resume">
              <button class="btn btn-secondary plan-buy" @click="selectPlan(plan)">{{ t('presale.availability.resume') }} <Icon name="arrowRight" size="sm" /></button>
              <RouterLink to="/orders" class="quiet-link">{{ t('presale.viewOrders') }}</RouterLink>
            </div>
            <RouterLink v-else-if="['reserved', 'refund', 'pending', 'existing'].includes(availability(plan).kind)" :to="['pending', 'existing'].includes(availability(plan).kind) ? '/orders' : '/subscriptions'" class="btn btn-secondary plan-buy">
              {{ t(['pending', 'existing'].includes(availability(plan).kind) ? 'presale.viewOrders' : 'presale.viewSubscriptions') }} <Icon name="arrowRight" size="sm" />
            </RouterLink>
            <button v-else-if="availability(plan).kind === 'error'" class="btn btn-secondary plan-buy" @click="load">{{ t('presale.availability.retry') }}</button>
            <button
              v-else class="btn btn-primary plan-buy"
              :class="{ 'is-checking': availability(plan).kind === 'checking' }"
              :disabled="!canReserve(plan)"
              :aria-busy="availability(plan).kind === 'checking' ? true : undefined"
              :aria-label="availability(plan).kind === 'checking' ? t('common.loading') : undefined"
              @click="selectPlan(plan)"
            >
              <span class="plan-buy-label" :aria-hidden="availability(plan).kind === 'checking' ? true : undefined">
                {{ t(availability(plan).kind === 'blocked' ? 'presale.availability.blocked' : auth.isAuthenticated ? 'presale.buy' : 'presale.loginBuy') }} <Icon v-if="availability(plan).kind !== 'blocked'" name="arrowRight" size="sm" />
              </span>
              <Transition name="reserve-loading">
                <LoadingSpinner
                  v-if="availability(plan).kind === 'checking'"
                  class="plan-buy-loader"
                  variant="steam"
                  color="current"
                  overlay
                  decorative
                />
              </Transition>
            </button>
          </article>
        </div>
      </section>
      <section class="value-section"><div class="section-heading"><div><p class="eyebrow">{{ t('presale.philosophy') }}</p><h2>{{ t('presale.valueTitle') }}</h2></div></div><PresaleBenefits /></section>
      <section v-if="catalog" class="policy-section"><div><p class="eyebrow">{{ t('presale.policy') }}</p><h2>{{ t('presale.policyTitle') }}</h2></div><div class="policy-details"><div><span class="policy-figure">100<span>%</span></span><div><h3>{{ t('presale.refundFull') }}</h3><p>{{ t('presale.refundFullCopy', { date: date(catalog.period.full_refund_before) }) }}</p></div></div><div><span class="policy-figure">80<span>%</span></span><div><h3>{{ t('presale.refundFee') }}</h3><p>{{ t('presale.refundFeeCopy', { date: date(catalog.period.full_refund_before) }) }}</p></div></div><div><span class="policy-figure">DAY</span><div><h3>{{ t('presale.refundDaily') }}</h3><p>{{ t('presale.refundDailyCopy') }}</p></div></div><div class="period-note">{{ t('presale.termCopy', { start: date(catalog.period.starts_at), end: date(catalog.period.expires_at) }) }}</div></div></section>
      <section class="balance-callout"><div><p class="eyebrow">NO NEED TO WAIT</p><h2>{{ t('presale.balanceTitle') }}</h2><p>{{ t('presale.balanceCopy') }}</p></div><RouterLink :to="balancePath" class="btn btn-primary">{{ t('presale.balance') }} <Icon name="arrowRight" size="sm" /></RouterLink></section>
    </main>
    <footer><RouterLink to="/">{{ app.siteName }}</RouterLink></footer>
    <BaseDialog :show="!!selectedPlanId" :title="t('presale.checkout')" width="wide" @close="closeCheckout">
      <PaymentView v-if="selectedPlanId && catalog && auth.isAuthenticated" :key="`${selectedPlanId}:${selectedPlanMultiplier}`" :presale-plan-id="selectedPlanId" :presale-month="catalog.period.month" :presale-multiplier="selectedPlanMultiplier" @close="closeCheckout" />
    </BaseDialog>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useEventListener, useWindowScroll } from '@vueuse/core'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { presaleAPI, type PresaleCatalog } from '@/api/presale'
import type { SubscriptionPlan } from '@/types/payment'
import { formatPresaleDate } from '@/utils/presale'
import { isCustomSubscriptionForPlan, subscriptionCustomMultiplier } from '@/utils/subscriptionCustom'
import Icon from '@/components/icons/Icon.vue'
import HomeHeader from '@/components/home/HomeHeader.vue'
import { initializeTheme } from '@/utils/theme'
import { sanitizeUrl } from '@/utils/url'
import { extractApiErrorCode, extractApiErrorMetadata } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import PresaleBenefits from '@/components/presale/PresaleBenefits.vue'
import PaymentView from '@/views/user/PaymentView.vue'
import { hasWechatResumeQuery } from '@/views/user/paymentWechatResume'
import { PAYMENT_RECOVERY_STORAGE_KEY, readPaymentRecoverySnapshot, type PaymentRecoverySnapshot } from '@/components/payment/paymentFlow'

const { t, locale } = useI18n()
const app = useAppStore(), auth = useAuthStore(), route = useRoute(), router = useRouter()
const subscriptions = useSubscriptionStore()
const siteName = computed(() => app.cachedPublicSettings?.site_name || app.siteName || 'Sub2API')
const docUrl = computed(() => sanitizeUrl(app.cachedPublicSettings?.doc_url || app.docUrl || '', { allowRelative: true }))
const entryPath = computed(() => !auth.isAuthenticated ? '/login' : auth.isAdmin ? '/admin/dashboard' : '/dashboard')
const isDark = ref(initializeTheme())
const compactHeader = ref(false)
const { y: scrollY } = useWindowScroll()
// Match the homepage's hysteresis without changing header geometry on scroll.
watch(scrollY, y => { compactHeader.value = y > (compactHeader.value ? 8 : 64) }, { immediate: true })
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}
const catalog = ref<PresaleCatalog | null>(null), loading = ref(true), error = ref(''), selectedPlanId = ref<number | null>(null)
const planMultipliers = ref<Record<number, number>>({})
type AvailabilityKind = 'checking' | 'available' | 'reserved' | 'pending' | 'refund' | 'existing' | 'blocked' | 'error'
interface PlanAvailability { kind: AvailabilityKind; reason?: string; orderId?: number; orderPlanId?: number; renewalOpensAt?: string }
const planAvailability = ref<Record<number, PlanAvailability>>({})
const recovery = ref<PaymentRecoverySnapshot | null>(null)
let loadRequest = 0, availabilityRequest = 0
let availabilityLoading = false, routeCheckoutHandled = false
function availability(plan: SubscriptionPlan): PlanAvailability {
  if (!auth.isAuthenticated) return { kind: 'available' }
  return planAvailability.value[plan.id] ?? { kind: 'checking' }
}
function canReserve(plan: SubscriptionPlan) { return availability(plan).kind === 'available' }
function canResume(plan: SubscriptionPlan) {
  const state = availability(plan)
  return state.kind === 'pending' && state.orderPlanId === plan.id && recovery.value?.orderType === 'subscription'
    && recovery.value.orderId === state.orderId
}
function availabilityMessage(plan: SubscriptionPlan): string {
  const state = availability(plan)
  if (state.reason === 'PRESALE_RENEWAL_TOO_EARLY' && state.renewalOpensAt) {
    return t('presale.renewalOpensCopy', { date: date(state.renewalOpensAt) })
  }
  if (state.reason) return t(`presale.errors.${state.reason}`)
  return t(`presale.availability.${state.kind}Copy`, { month: date(catalog.value?.period.starts_at, true) })
}
async function refreshAvailability() {
  const request = ++availabilityRequest
  availabilityLoading = false
  planAvailability.value = {}
  try { recovery.value = readPaymentRecoverySnapshot(localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)) }
  catch { recovery.value = null }
  if (!auth.isAuthenticated || !catalog.value?.enabled) return
  availabilityLoading = true
  const month = catalog.value.period.month
  await Promise.all(catalog.value.plans.map(async plan => {
    let state: PlanAvailability
    try {
      const { data } = await presaleAPI.quote(plan.id)
      state = data.month === month ? { kind: 'available' } : { kind: 'error', reason: 'PRESALE_MONTH_CHANGED' }
    } catch (err: unknown) {
      const reason = extractApiErrorCode(err)
      if (reason === 'PRESALE_ALREADY_RESERVED') {
        const metadata = extractApiErrorMetadata(err)
        const status = metadata?.order_status
        const kind = status === 'PENDING' ? 'pending'
          : ['REFUND_REQUESTED', 'REFUNDING', 'REFUND_FAILED'].includes(String(status)) ? 'refund'
          : ['PAID', 'RECHARGING', 'COMPLETED', 'FAILED'].includes(String(status)) ? 'reserved' : 'existing'
        state = { kind, orderId: Number(metadata?.order_id), orderPlanId: Number(metadata?.order_plan_id) }
      } else if (['PRESALE_COVERAGE_OVERLAP', 'PRESALE_LEGACY_SUBSCRIPTION', 'PRESALE_NOT_AVAILABLE', 'PRESALE_RENEWAL_TOO_EARLY'].includes(reason ?? '')) {
        const opensAt = extractApiErrorMetadata(err)?.renewal_opens_at
        state = { kind: 'blocked', reason, renewalOpensAt: typeof opensAt === 'string' ? opensAt : undefined }
      } else {
        state = { kind: 'error' }
      }
    }
    if (request === availabilityRequest) planAvailability.value[plan.id] = state
  })).finally(() => { if (request === availabilityRequest) availabilityLoading = false })
}
const selectedPlanMultiplier = computed(() => {
  const plan = catalog.value?.plans.find(p => p.id === selectedPlanId.value)
  return plan ? planMultiplier(plan) : undefined
})
const balancePath = computed(() => auth.isAuthenticated ? '/purchase' : { path: '/login', query: { redirect: '/purchase' } })
const date = (value?: string, monthOnly = false) => formatPresaleDate(value, locale.value, monthOnly)
const monthAbbreviation = computed(() => {
  const month = Number(catalog.value?.period.month.split('-')[1])
  return ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'][month - 1] || '—'
})
const daysInPeriod = computed(() => catalog.value ? Math.round((Date.parse(catalog.value.period.expires_at) - Date.parse(catalog.value.period.starts_at)) / 86400000) : '—')
const timeline = computed(() => [
  { label: t('presale.reserve'), date: t('presale.reserveCopy'), copy: t('presale.notImmediate') },
  { label: t('presale.prepare'), date: date(catalog.value?.period.full_refund_before), copy: t('presale.prepareCopy') },
  { label: t('presale.activate'), date: date(catalog.value?.period.starts_at), copy: t('presale.activateCopy') },
])
function renewalMultiplier(plan: SubscriptionPlan): number | null {
  if (!auth.isAuthenticated) return null
  return subscriptionCustomMultiplier(subscriptions.activeSubscriptions.find(sub =>
    sub.status === 'active' && (!sub.expires_at || Date.parse(sub.expires_at) > Date.now())
      && isCustomSubscriptionForPlan(sub, plan.id),
  ))
}
function multiplierRange(plan: SubscriptionPlan) {
  const min = Math.max(1, plan.custom_multiplier_min ?? 1)
  return { min, max: Math.max(min, plan.custom_multiplier_max || min) }
}
function planMultiplier(plan: SubscriptionPlan): number {
  if (!plan.custom_multiplier_enabled) return 1
  const { min, max } = multiplierRange(plan)
  const value = planMultipliers.value[plan.id] ?? renewalMultiplier(plan) ?? min
  return Number.isFinite(value) ? Math.min(max, Math.max(min, Math.trunc(value))) : min
}
function multiplierOptions(plan: SubscriptionPlan): SelectOption[] {
  const { min, max } = multiplierRange(plan)
  return Array.from({ length: max - min + 1 }, (_, index) => ({ value: min + index, label: `${min + index}x` }))
}
function updateMultiplier(plan: SubscriptionPlan, value: string | number | boolean | null) {
  const multiplier = Number(value)
  if (multiplierOptions(plan).some(option => option.value === multiplier)) planMultipliers.value[plan.id] = multiplier
}
function scaledPlanValue(plan: SubscriptionPlan, value: number): number {
  return Math.round(value * planMultiplier(plan) * 100) / 100
}
function quotas(plan: SubscriptionPlan) {
  return ([['daily', plan.daily_limit_usd], ['weekly', plan.weekly_limit_usd], ['monthly', plan.monthly_limit_usd]] as const)
    .filter(([, value]) => value != null && value > 0)
    .map(([key, value]) => ({ label: t(`presale.${key}`), value: `$${scaledPlanValue(plan, value!).toLocaleString(locale.value)}` }))
}
function selectPlan(plan: SubscriptionPlan) {
  if (!canReserve(plan) && !canResume(plan)) return
  const query: Record<string, string> = { plan: String(plan.id) }
  if (plan.custom_multiplier_enabled) query.multiplier = String(planMultiplier(plan))
  const redirect = `/presale?${new URLSearchParams(query)}`
  if (!auth.isAuthenticated) { void router.push({ path: '/login', query: { redirect } }); return }
  routeCheckoutHandled = true
  selectedPlanId.value = plan.id
  void router.replace({ path: '/presale', query, hash: route.hash })
}
function restoreSelection() {
  // A return-link is a one-shot intent, not a command to reopen after every refresh.
  if (routeCheckoutHandled) return
  const plan = catalog.value?.enabled && catalog.value.plans.find(p => p.id === Number(route.query.plan))
  if (!plan) return
  if (route.query.multiplier != null) planMultipliers.value[plan.id] = Number(route.query.multiplier)
  // Login-return links wait for the same eligibility check as the cards.
  // Provider returns still resume inside PaymentView's server-side guards.
  if (auth.isAuthenticated && (canReserve(plan) || canResume(plan) || hasWechatResumeQuery(route.query))) {
    routeCheckoutHandled = true
    selectedPlanId.value = plan.id
  }
}
function closeCheckout() {
  // Take effect synchronously, even while URL cleanup or older checks are pending.
  routeCheckoutHandled = true
  selectedPlanId.value = null
  void router.replace({ path: '/presale', hash: route.hash })
  void refreshAvailability()
}
async function load() {
  const request = ++loadRequest
  ++availabilityRequest
  planAvailability.value = {}
  loading.value = true; error.value = ''
  try {
    const [result] = await Promise.all([
      presaleAPI.catalog(),
      auth.isAuthenticated ? subscriptions.fetchActiveSubscriptions(true) : Promise.resolve(),
    ])
    if (request !== loadRequest) return
    catalog.value = result.data
    loading.value = false
    await refreshAvailability()
    if (request !== loadRequest) return
    restoreSelection()
  }
  catch { if (request === loadRequest) error.value = t('presale.failed') }
  finally { if (request === loadRequest) loading.value = false }
}
watch([() => route.query.plan, () => route.query.multiplier], restoreSelection)
// Compare scalar identities, not a fresh array on every user-profile replacement.
watch([() => auth.isAuthenticated, () => auth.user?.id], () => { selectedPlanId.value = null; void load() })
function refreshOnReturn() {
  if (!document.hidden && !loading.value && !availabilityLoading && selectedPlanId.value == null) void refreshAvailability()
}
useEventListener(window, 'focus', refreshOnReturn)
useEventListener(document, 'visibilitychange', refreshOnReturn)
onBeforeUnmount(() => { ++loadRequest; ++availabilityRequest })
onMounted(() => {
  void load()
  if (!app.publicSettingsLoaded) void app.fetchPublicSettings()
})
</script>
<style scoped>
.presale-page { --cafe-page: #f6f3ed; --cafe-surface: #fffdf8; --cafe-ink: #302b26; --cafe-muted: #797268; --cafe-accent: #987647; --cafe-line: #ded8cf; min-height: 100vh; color: var(--cafe-ink); background: var(--cafe-page); }
.dark .presale-page { --cafe-page: #141513; --cafe-surface: #1b1c19; --cafe-ink: #eae5dc; --cafe-muted: #a29c90; --cafe-accent: #c6ac7c; --cafe-line: #33342f; }
main, footer { width: min(1200px, calc(100% - 80px)); margin-inline: auto; }
main section[id] { scroll-margin-top: 120px; }
.presale-hero { display: grid; grid-template-columns: 1.2fr 1fr; align-items: center; gap: 28px; min-height: 580px; padding: 65px 0 55px; }
.eyebrow { color: var(--cafe-accent); font: italic 14px Georgia, serif; letter-spacing: .035em; }
h1 { font: 400 clamp(32px, 3.5vw, 48px)/1.55 Georgia, 'Noto Serif CJK SC', 'Songti SC', serif; margin-top: 24px; letter-spacing: -.035em; }
h1 em { display: block; font-style: normal; color: var(--cafe-accent); }
.intro { max-width: 400px; color: var(--cafe-muted); font-size: 14px; line-height: 1.9; margin-top: 24px; }
.hero-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 24px; margin-top: 32px; }.btn { gap: 12px; padding: 13px 20px; border-radius: 8px; font-size: 13px; }.quiet-link { color: var(--cafe-muted); font-size: 12px; }.quiet-link:hover { color: var(--cafe-accent); }
.month-art {
  --ticket-paper: #fbf5ea;
  --ticket-edge: #d9c9b1;
  --ticket-ink: #785b38;
  --ticket-muted: #7e694f;
  position: relative;
  isolation: isolate;
  height: 420px;
  display: grid;
  place-items: center;
  color: var(--cafe-accent);
  background: radial-gradient(ellipse at 48% 55%, #b18d5820, transparent 66%);
}
.dark .month-art { --ticket-paper: #2b251e; --ticket-edge: #554535; --ticket-ink: #e2c69b; --ticket-muted: #b8a58a; }
.month-art::before {
  content: '';
  position: absolute;
  width: 244px;
  height: 278px;
  border: 1px solid var(--ticket-edge);
  border-radius: 6px;
  background: var(--ticket-paper);
  opacity: .55;
  transform: translate(10px, 6px) rotate(5deg);
}
.month-setting { position: absolute; width: 100%; height: 100%; pointer-events: none; }
.coffee-outline { opacity: .3; }
.coffee-steam { opacity: .3; animation: coffee-warmth 6s ease-in-out infinite; }
.month-ticket {
  position: relative;
  width: 244px;
  color: var(--ticket-ink);
  background: linear-gradient(135deg, #ffffff06, transparent 60%), var(--ticket-paper);
  border: 1px solid var(--ticket-edge);
  border-radius: 6px;
  text-align: center;
  box-shadow: 0 4px 8px #24180f0a, 0 22px 40px -14px #24180f35;
  transform: rotate(-4deg);
  padding-top: 28px;
  animation: ticket-arrive 900ms ease both;
}
.dark .month-ticket { box-shadow: 0 4px 10px #00000024, 0 24px 42px -12px #00000070; }
.ticket-kicker { font-size: 9px; letter-spacing: .28em; }
.ticket-date { display: flex; justify-content: center; align-items: baseline; gap: 12px; margin: 22px 0 18px; white-space: nowrap; }
.ticket-date strong { font: 400 100px/1 Georgia, serif; letter-spacing: -.05em; }
.ticket-month-abbr { font: italic 400 26px/1.15 Georgia, serif; letter-spacing: -.015em; }
.ticket-month { font-size: 11px; letter-spacing: .08em; color: var(--ticket-muted); }
.ticket-perforation { border-top: 1px dashed var(--ticket-edge); margin-top: 27px; }
.ticket-footer { display: flex; justify-content: space-between; padding: 17px 20px; font-size: 9px; color: var(--ticket-muted); }
.art-note { position: absolute; bottom: 2px; font-size: 8px; letter-spacing: .25em; opacity: .6; }
.timeline { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); column-gap: 24px; border-block: 1px solid var(--cafe-line); padding: 30px 0; }.timeline-step { display: flex; gap: 16px; }.step-index { font: italic 20px Georgia, serif; color: var(--cafe-accent); }.step-label { font-size: 12px; color: var(--cafe-muted); }.step-date { font-size: 16px; margin: 10px 0 8px; }.step-copy { font-size: 11px; color: var(--cafe-muted); }
.plans-section, .value-section { padding: 78px 0; }.section-heading { display: flex; align-items: end; justify-content: space-between; gap: 24px; margin-bottom: 32px; }h2 { font: 400 28px/1.5 Georgia, 'Noto Serif CJK SC', 'Songti SC', serif; margin-top: 12px; letter-spacing: -.02em; }.section-heading>p { max-width: 330px; color: var(--cafe-muted); font-size: 12px; line-height: 1.8; }
.plan-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(245px, 1fr)); gap: 18px; }.presale-plan { padding: 27px; border: 1px solid var(--cafe-line); border-radius: 12px; background: var(--cafe-surface); display: flex; flex-direction: column; min-width: 0; transition: border-color .3s, transform .3s; }.presale-plan:hover { border-color: var(--cafe-accent); transform: translateY(-4px); }.plan-top { display: flex; align-items: center; justify-content: space-between; min-height: 24px; gap: 8px; }.plan-index { color: var(--cafe-muted); font-size: 9px; letter-spacing: .08em; }.plan-badge { color: var(--cafe-accent); font-size: 10px; padding: 3px 8px; border: 1px solid var(--cafe-line); border-radius: 20px; }.presale-plan h3 { font: 400 26px Georgia, serif; margin-top: 22px; }.plan-description { font-size: 12px; color: var(--cafe-muted); line-height: 1.8; margin-top: 12px; min-height: 44px; }.plan-price { display: flex; align-items: baseline; flex-wrap: wrap; gap: 8px; padding: 25px 0; }.plan-price strong { font: 400 40px Georgia, serif; } .price-currency { font-size: .55em; margin-right: 2px; }.plan-price>span { font-size: 10px; color: var(--cafe-muted); }.old-price { text-decoration: line-through; }.plan-quotas { border-block: 1px solid var(--cafe-line); padding: 10px 0; }.plan-quotas div { display: flex; justify-content: space-between; gap: 12px; font-size: 12px; padding: 7px 0; }.plan-quotas dt { color: var(--cafe-muted); }.plan-features { flex: 1; margin-block: 22px 30px; }.plan-features li { display: flex; align-items: start; gap: 9px; font-size: 12px; line-height: 1.8; margin-bottom: 9px; }.plan-features svg { flex-shrink: 0; margin-top: 3px; color: var(--cafe-accent); }.plan-multiplier { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; font-size: 12px; color: var(--cafe-muted); }.multiplier-select { width: 112px; flex-shrink: 0; }.plan-buy { width: 100%; }.empty-state { border: 1px dashed var(--cafe-line); border-radius: 12px; padding: 55px 28px; text-align: center; }.empty-state h3 { font: 400 23px Georgia, serif; margin-top: 12px; }.empty-state>p:not(.eyebrow) { color: var(--cafe-muted); font-size: 13px; margin-top: 14px; }.empty-state .quiet-link { display: inline-block; margin-top: 22px; }
.value-section { border-top: 1px solid var(--cafe-line); }.policy-section { display: grid; grid-template-columns: 1fr 1.4fr; gap: 60px; padding: 60px 0 80px; border-top: 1px solid var(--cafe-line); }.policy-details>div:not(.period-note) { display: flex; align-items: start; gap: 25px; padding-block: 20px; border-bottom: 1px solid var(--cafe-line); }.policy-details>div:first-child { padding-top: 0; }.policy-figure { width: 85px; flex-shrink: 0; font: 400 30px Georgia, serif; color: var(--cafe-accent); }.policy-figure>span { font-size: 15px; }.policy-details h3 { font-size: 13px; margin-bottom: 8px; }.policy-details p,.period-note { color: var(--cafe-muted); font-size: 12px; line-height: 1.8; }.period-note { padding-top: 20px; }
.balance-callout { display: flex; justify-content: space-between; align-items: center; gap: 25px; padding: 35px; border-radius: 12px; border: 1px solid var(--cafe-line); background: var(--cafe-surface); margin-bottom: 60px; }.balance-callout h2 { font-size: 23px; }.balance-callout p:last-child { font-size: 12px; line-height: 1.8; max-width: 530px; color: var(--cafe-muted); margin-top: 12px; }.balance-callout .btn { flex-shrink: 0; }footer { padding: 30px 0; border-top: 1px solid var(--cafe-line); display: flex; justify-content: center; color: var(--cafe-muted); font-size: 11px; }
.plan-availability { margin-bottom: 14px; padding: 12px 14px; border: 1px solid var(--cafe-line); border-radius: 8px; background: var(--cafe-page); }
.plan-availability strong { display: block; color: var(--cafe-accent); font-size: 12px; font-weight: 500; }
.plan-availability p { margin-top: 5px; font-size: 11px; line-height: 1.7; color: var(--cafe-muted); }
.plan-buy:disabled { opacity: .55; cursor: not-allowed; }
.plan-buy-label { display: inline-flex; align-items: center; justify-content: center; gap: 12px; transition: opacity .2s ease, transform .2s ease; }
/* Keep the original label in flow so loading never resizes the button. */
.plan-buy.is-checking:disabled { opacity: 1; cursor: wait; box-shadow: none; }
.plan-buy.is-checking .plan-buy-label { opacity: 0; transform: translateY(3px); }
.plan-buy.is-checking::before { opacity: 0; }
.reserve-loading-enter-active, .reserve-loading-leave-active { transition: opacity .16s ease; }
.reserve-loading-enter-from, .reserve-loading-leave-to { opacity: 0; }
.plan-resume .quiet-link { display: block; text-align: center; margin-top: 12px; }
@keyframes coffee-warmth { 0%, 100% { opacity: .2; transform: translateY(2px); } 50% { opacity: .4; transform: translateY(-5px); } }
@keyframes ticket-arrive { from { opacity: 0; transform: translateY(15px) rotate(-1deg); } }
@media(max-width: 900px) { main, footer { width: calc(100% - 48px); }.presale-hero { gap: 0; min-height: 480px; }.month-art { height: 340px; }.month-ticket, .month-art::before { width: 210px; }.month-art::before { height: 257px; }.ticket-date strong { font-size: 84px; }.ticket-month-abbr { font-size: 24px; }.step-date { font-size: 13px; }.policy-section { gap: 30px; }.balance-callout { flex-direction: column; align-items: start; } }
@media(max-width: 640px) { main, footer { width: calc(100% - 36px); }.presale-hero { grid-template-columns: 1fr; padding: 45px 0 28px; }.month-art { height: 340px; margin-top: 24px; overflow: hidden; }.timeline { grid-template-columns: 1fr; gap: 24px; }.step-date { font-size: 15px; }.section-heading { flex-direction: column; align-items: start; gap: 14px; }h2 { font-size: 24px; }.plans-section,.value-section { padding: 45px 0; }.policy-section { grid-template-columns: 1fr; padding-block: 40px; }.balance-callout { padding: 24px; }.hero-actions { gap: 18px; }footer { flex-wrap: wrap; } }
@media(prefers-reduced-motion: reduce) { .coffee-steam,.month-ticket { animation: none; }.presale-plan { transition: none; } }
@media(prefers-reduced-motion: reduce) {
  .plan-buy-label, .reserve-loading-enter-active, .reserve-loading-leave-active { transition: none; }
}
</style>
