<template>
  <BaseDialog :show="show" :title="t('presaleNotice.title')" width="extra-wide" @close="close">
    <p class="mb-5 text-sm text-content-tertiary">{{ t('presaleNotice.intro') }}</p>
    <div class="grid min-w-0 gap-6 lg:grid-cols-[18rem_minmax(0,1fr)]">
      <section class="min-w-0 space-y-5">
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <p class="mb-3 text-sm font-medium text-content-primary">{{ t('presaleNotice.audience') }}</p>
          <p class="mb-3 text-sm text-content-secondary">{{ t('presaleNotice.access') }}</p>
          <label class="flex cursor-pointer items-start gap-2.5 text-sm text-content-primary">
            <input v-model="includeRestricted" data-test="include-restricted" type="checkbox" :disabled="busy" class="mt-0.5 h-4 w-4 rounded border-gray-300" />
            <span>{{ t('presaleNotice.includeRestricted') }}</span>
          </label>
          <p class="mt-3 text-xs leading-relaxed text-content-tertiary">{{ t('presaleNotice.audienceHint') }}</p>
        </div>
        <label class="block text-sm text-content-secondary">
          {{ t('presaleNotice.language') }}
          <select v-model="mailLocale" :disabled="busy" class="input mt-2"><option value="zh">中文</option><option value="en">English</option></select>
        </label>
        <div class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <label for="presale-notice-coupon" class="block text-sm font-medium text-content-primary">{{ t('presaleNotice.couponLabel') }}</label>
          <input id="presale-notice-coupon" v-model="couponCode" type="text" maxlength="48" autocomplete="off" :spellcheck="false" class="input font-mono text-xs" placeholder="CAFE-PUBLIC-…" :disabled="busy || loading" aria-describedby="presale-notice-coupon-hint" @keydown.enter.prevent="saveCoupon" />
          <p id="presale-notice-coupon-hint" class="text-xs leading-relaxed text-content-tertiary">{{ t('presaleNotice.couponHint') }}</p>
          <p v-if="couponDirty" class="text-xs leading-relaxed text-amber-700 dark:text-amber-200">{{ t('presaleNotice.couponUnsaved') }}</p>
          <p v-else-if="preview?.coupon_code && !preview.coupon_included" class="text-xs leading-relaxed text-amber-700 dark:text-amber-200">{{ t('presaleNotice.couponUnavailable') }}</p>
          <button type="button" class="btn btn-secondary w-full" data-test="save-coupon" :disabled="busy || loading || !preview || !couponDirty" @click="saveCoupon"><Icon v-if="savingCoupon" name="refresh" size="sm" class="animate-spin" />{{ t('presaleNotice.couponSave') }}</button>
        </div>
        <div v-if="preview" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
          <div class="mb-4 flex justify-between gap-3 text-sm"><span class="text-content-tertiary">{{ t('presaleNotice.month') }}</span><strong class="text-content-primary">{{ preview.month }}</strong></div>
          <dl class="grid grid-cols-2 gap-4">
            <div v-for="key in countKeys" :key="key"><dt class="text-xs text-content-tertiary">{{ t(`presaleNotice.${key === 'sending' ? 'sendingCount' : key}`) }}</dt><dd class="mt-1 text-xl tabular-nums text-content-primary">{{ preview.counts[key] }}</dd></div>
          </dl>
        </div>
        <form class="space-y-3 border-t border-gray-200 pt-5 dark:border-dark-600" @submit.prevent="sendTest">
          <label class="block text-sm text-content-secondary" for="presale-notice-test-email">{{ t('presaleNotice.testEmail') }}</label>
          <input id="presale-notice-test-email" v-model="email" type="email" required maxlength="254" class="input" placeholder="name@example.com" :disabled="busy" />
          <button type="submit" class="btn btn-secondary w-full" :disabled="busy || !preview || loading || couponDirty"><Icon v-if="testing" name="refresh" size="sm" class="animate-spin" />{{ t('presaleNotice.test') }}</button>
        </form>
        <p class="text-xs leading-relaxed text-content-tertiary">{{ t('presaleNotice.safety') }}</p>
      </section>
      <section class="min-w-0 space-y-3">
        <div class="flex items-center justify-between gap-3"><h4 class="text-sm font-medium text-content-primary">{{ t('presaleNotice.preview') }}</h4><button class="btn btn-secondary" :disabled="busy || loading" :title="t('presaleNotice.refresh')" @click="load()"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" /></button></div>
        <p v-if="preview && !preview.ready" class="rounded-xl bg-amber-50 p-3 text-xs leading-relaxed text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ t('presaleNotice.draft') }}</p>
        <p v-if="error" role="alert" class="rounded-xl bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
        <p v-if="message" role="status" class="text-sm text-content-secondary">{{ message }}</p>
        <div v-if="loading" class="flex h-96 items-center justify-center text-content-tertiary"><Icon name="refresh" size="md" class="animate-spin" /></div>
        <template v-else-if="preview"><p class="text-sm leading-relaxed text-content-secondary">{{ preview.subject }}</p><iframe :srcdoc="preview.html" sandbox="" referrerpolicy="no-referrer" :title="t('presaleNotice.preview')" class="h-[38rem] w-full rounded-xl border border-gray-200 bg-[#f5f0e8] dark:border-dark-600" /></template>
      </section>
    </div>
    <template #footer>
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p class="max-w-xl text-xs leading-relaxed text-content-tertiary">{{ t(sending ? 'presaleNotice.keepOpen' : preview && (preview.counts.uncertain || preview.counts.sending) ? 'presaleNotice.receiptWarning' : 'presaleNotice.keepOpen') }}</p>
        <button v-if="sending" class="btn btn-secondary shrink-0" :disabled="stopping" @click="stopping = true">{{ t(stopping ? 'presaleNotice.stopping' : 'presaleNotice.stop') }}</button>
        <button v-else class="btn btn-primary shrink-0" :disabled="!canSend" data-test="send-notice" @click="confirming = true">{{ t('presaleNotice.send') }}</button>
      </div>
    </template>
  </BaseDialog>
  <ConfirmDialog :show="confirming && show" :title="t('presaleNotice.confirmTitle')" :message="confirmation" :confirm-text="t('presaleNotice.send')" @confirm="sendAll" @cancel="confirming = false" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { PresaleNoticePreview, PresaleNoticeRequest } from '@/types/payment'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t, locale } = useI18n()
const mailLocale = ref(locale?.value?.startsWith('zh') ? 'zh' : 'en')
const includeRestricted = ref(false)
const email = ref('')
const couponCode = ref('')
const savingCoupon = ref(false)
const preview = ref<PresaleNoticePreview | null>(null)
const loading = ref(false), testing = ref(false), sending = ref(false), stopping = ref(false), confirming = ref(false)
const error = ref(''), message = ref('')
const busy = computed(() => testing.value || sending.value || savingCoupon.value)
const couponDirty = computed(() => couponCode.value.trim().toUpperCase() !== (preview.value?.coupon_code ?? ''))
const canSend = computed(() => !busy.value && !loading.value && !couponDirty.value && !!preview.value?.ready && preview.value.counts.pending > 0)
const countKeys = ['eligible', 'pending', 'sent', 'skipped', 'uncertain', 'sending'] as const
const confirmation = computed(() => t('presaleNotice.confirm', { count: preview.value?.counts.pending ?? 0, month: preview.value?.month ?? '', audience: t(includeRestricted.value ? 'presaleNotice.includeSummary' : 'presaleNotice.accessSummary') }))
let generation = 0
function close() { stopping.value = true; confirming.value = false; emit('close') }
async function load(resetCoupon = false) {
  if (!props.show) return
  const preserveCoupon = !resetCoupon && couponDirty.value
  const id = ++generation
  loading.value = true; confirming.value = false; preview.value = null; error.value = ''
  try {
    const { data } = await adminPaymentAPI.getPresaleNotice({ include_restricted: includeRestricted.value, locale: mailLocale.value })
    if (generation === id && props.show) {
      preview.value = data
      if (!preserveCoupon) couponCode.value = data.coupon_code
    }
  } catch (err) { if (generation === id) error.value = extractI18nErrorMessage(err, t, 'presaleNotice.errors', t('presaleNotice.loadFailed')) }
  finally { if (generation === id) loading.value = false }
}
async function saveCoupon() {
  if (busy.value || loading.value || !preview.value || !couponDirty.value) return
  const id = generation
  const request = { month: preview.value.month, coupon_code: couponCode.value.trim() }
  savingCoupon.value = true; confirming.value = false; error.value = ''; message.value = ''
  try {
    await adminPaymentAPI.updatePresaleNoticeConfig(request)
    if (generation === id && props.show) {
      await load(true)
      if (!error.value && props.show) message.value = t('presaleNotice.couponSaved')
    }
  } catch (err) {
    if (generation === id && props.show) error.value = extractI18nErrorMessage(err, t, 'presaleNotice.errors', t('presaleNotice.failed'))
  } finally { savingCoupon.value = false }
}
function payload(): PresaleNoticeRequest {
  if (!preview.value) throw new Error('No reviewed preview')
  return { include_restricted: includeRestricted.value, locale: mailLocale.value, month: preview.value.month, version: preview.value.version, confirmed: true }
}
async function sendTest() {
  if (busy.value || loading.value || !preview.value || couponDirty.value) return
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value.trim())) { error.value = t('presaleNotice.invalidEmail'); return }
  const request = { ...payload(), email: email.value.trim() }
  testing.value = true; error.value = ''; message.value = ''
  try { await adminPaymentAPI.testPresaleNotice(request); message.value = t('presaleNotice.testSuccess') }
  catch (err) { error.value = extractI18nErrorMessage(err, t, 'presaleNotice.errors', t('presaleNotice.failed')) }
  finally { testing.value = false }
}
async function sendAll() {
  if (!confirming.value || !canSend.value) return
  const request = payload()
  confirming.value = false; sending.value = true; stopping.value = false; error.value = ''; message.value = ''
  try {
    while (props.show && !stopping.value) {
      const { data } = await adminPaymentAPI.sendNextPresaleNotice(request)
      if (data.done) { message.value = t('presaleNotice.complete'); break }
      if (preview.value) {
        preview.value.counts.pending = Math.max(0, preview.value.counts.pending - 1)
        if (data.status === 'sent') preview.value.counts.sent++
        if (data.status === 'skipped') preview.value.counts.skipped++
      }
      await new Promise(resolve => setTimeout(resolve, 150))
    }
    if (stopping.value) message.value = t('presaleNotice.paused')
  } catch (err) { error.value = extractI18nErrorMessage(err, t, 'presaleNotice.errors', t('presaleNotice.failed')) }
  finally {
    sending.value = false
    // Keep errors visible. A fresh preview is required before another send.
    if (error.value) preview.value = null
    else if (props.show) await load()
  }
}
watch(() => props.show, show => { if (show) void load(true); else { generation++; stopping.value = true; confirming.value = false } }, { immediate: true })
watch(couponCode, () => { confirming.value = false; message.value = '' })
watch([includeRestricted, mailLocale], () => { message.value = ''; if (!busy.value) void load() })
onBeforeUnmount(() => { generation++; stopping.value = true })
</script>
