<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col" data-test="campaign-layout">
    <div class="flex shrink-0 flex-wrap items-center justify-between gap-4 border-b border-stroke-default px-4 py-5 sm:px-6" data-test="campaign-toolbar">
      <p class="max-w-2xl text-sm leading-relaxed text-content-secondary">{{ t('cafeCampaign.intro') }}</p>
      <div class="flex shrink-0 gap-2"><button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button><button class="btn btn-primary" @click="openCreate">{{ t('cafeCampaign.create') }}</button></div>
    </div>
    <DataTable :columns="columns" :data="rows" :loading="loading">
      <template #cell-code="{ row }"><div class="space-y-1"><p class="font-medium">{{ row.name }}</p><button class="font-mono text-xs text-content-secondary hover:text-content-primary" :title="t('keys.copyToClipboard')" @click="copy(row.code)">{{ row.code }}</button></div></template>
      <template #cell-discount_percent="{ row }"><span class="font-medium tabular-nums">{{ t('cafeCampaign.discountValue', { percent: row.discount_percent }) }}</span></template>
      <template #cell-starts_at="{ row }"><div class="space-y-1 text-xs tabular-nums"><p>{{ date(row.starts_at) }}</p><p class="text-content-tertiary">→ {{ date(row.expires_at) }} · {{ t('cafeCampaign.exclusive') }}</p></div></template>
      <template #cell-enabled="{ row }"><span class="badge" :class="row.enabled ? 'badge-success' : 'badge-secondary'">{{ campaignStatus(row) }}</span></template>
      <template #cell-actions="{ row }"><div class="flex gap-2"><button class="btn btn-secondary btn-sm" :disabled="changing" @click="toggleTarget = row">{{ t(row.enabled ? 'cafeCampaign.disable' : 'cafeCampaign.enable') }}</button><button class="btn btn-secondary btn-sm" @click="openUses(row)">{{ t('cafeCampaign.uses') }}</button></div></template>
    </DataTable>
    <Pagination v-if="total" class="shrink-0" :page="page" :total="total" :page-size="20" :show-page-size-selector="false" @update:page="changePage" />
    <p class="shrink-0 border-t border-stroke-default px-4 py-4 text-xs leading-relaxed text-content-tertiary sm:px-6" data-test="campaign-policy">{{ t('cafeCampaign.policy') }}</p>

    <BaseDialog :show="showCreate" :title="t('cafeCampaign.create')" @close="closeCreate">
      <form id="cafe-campaign-form" class="space-y-4" @submit.prevent="create">
        <fieldset :disabled="creating" class="space-y-4">
          <label class="block text-sm"><span class="flex items-center justify-between gap-2">{{ t('cafeCampaign.name') }}<span class="text-xs tabular-nums" :class="nameLength > 100 ? 'text-red-500' : 'text-content-tertiary'">{{ nameLength }}/100</span></span><input v-model="form.name" class="input mt-2" required :aria-invalid="nameLength > 100 || undefined" data-test="campaign-name" /></label>
          <div class="space-y-2">
            <label for="campaign-code" class="block text-sm">{{ t('cafeCampaign.code') }}</label>
            <div class="input flex items-center p-0 focus-within:border-stroke-brand focus-within:ring-2 focus-within:ring-primary-500/30">
              <span class="shrink-0 whitespace-nowrap pl-4 font-mono text-xs text-content-tertiary" data-test="campaign-code-prefix">CAFE-PUBLIC-</span>
              <input
                id="campaign-code"
                v-model="form.code"
                class="min-w-0 w-0 flex-1 rounded-r-xl bg-transparent py-2.5 pl-1 pr-4 font-mono text-sm uppercase text-content-primary outline-none placeholder:text-content-tertiary disabled:cursor-not-allowed"
                required
                pattern="[A-Za-z0-9][A-Za-z0-9-]{0,35}"
                maxlength="36"
                placeholder="SEP40"
                data-test="campaign-code"
              />
            </div>
          </div>
          <label class="block text-sm">{{ t('cafeCampaign.discount') }}<input v-model.number="form.discount_percent" class="input mt-2" type="number" min="1" max="99" step="1" required data-test="campaign-discount" /></label>
          <p class="text-sm text-content-secondary">{{ t('cafeCampaign.payable', { percent: 100 - (Number(form.discount_percent) || 0) }) }}</p>
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2"><label class="text-sm">{{ t('cafeCampaign.starts') }}<input v-model="form.start_date" class="input mt-2" type="date" required data-test="campaign-start" /></label><label class="text-sm">{{ t('cafeCampaign.ends') }}<input v-model="form.end_date" class="input mt-2" type="date" :min="form.start_date" required data-test="campaign-end" /></label></div>
          <p class="text-xs text-content-tertiary">{{ t('cafeCampaign.dateHint') }}</p>
        </fieldset>
        <p class="rounded-xl bg-surface-secondary p-3 text-xs leading-relaxed text-content-secondary">{{ t('cafeCampaign.immutable') }}</p>
      </form>
      <template #footer><button class="btn btn-secondary" :disabled="creating" @click="closeCreate">{{ t('common.cancel') }}</button><button class="btn btn-primary" form="cafe-campaign-form" type="submit" :disabled="creating || !valid">{{ t(creating ? 'common.processing' : 'common.create') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="!!toggleTarget" :title="toggleTarget?.code || ''" @close="closeToggle">
      <p class="text-sm leading-relaxed text-content-secondary">{{ t(toggleTarget?.enabled ? 'cafeCampaign.disableConfirm' : 'cafeCampaign.enableConfirm', { percent: toggleTarget?.discount_percent }) }}</p>
      <template #footer><button class="btn btn-secondary" :disabled="changing" @click="closeToggle">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="changing" data-test="campaign-toggle-confirm" @click="toggle">{{ t('common.confirm') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="!!usageTarget" :title="`${t('cafeCampaign.uses')} · ${usageTarget?.code || ''}`" width="wide" @close="closeUses">
      <p class="mb-4 text-xs text-content-tertiary">{{ t('cafeCampaign.usageHint') }}</p>
      <p v-if="usageError" role="alert" class="mb-4 text-sm text-red-500">{{ usageError }}</p>
      <DataTable :columns="usageColumns" :data="usages" :loading="usageLoading">
        <template #cell-user_email="{ row }"><p class="break-all text-sm">{{ row.user_email }}</p><p class="text-xs text-content-tertiary">#{{ row.user_id }}</p></template>
        <template #cell-order_id="{ row }"><button class="font-mono underline underline-offset-4" @click="openOrder(row.order_id)">#{{ row.order_id }}</button></template>
        <template #cell-order_status="{ row }"><OrderStatusBadge :status="row.order_status" /></template>
        <template #cell-used_at="{ row }"><span class="text-xs">{{ useStatus(row) }}</span></template>
      </DataTable>
      <Pagination v-if="usageTotal" :page="usagePage" :total="usageTotal" :page-size="20" :show-page-size-selector="false" @update:page="changeUsagePage" />
    </BaseDialog>
    <AdminOrderDetail :show="showOrder" :order="order" :summary="orderSummary" :audit-logs="orderAudits" :loading="orderLoading" :error="orderError" @close="closeOrder" @reload="reloadOrder" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { cafeCampaignAPI, type CafeCampaign, type CafeCampaignUse } from '@/api/admin/cafeCampaigns'
import { adminPaymentAPI, type AdminPaymentOrder, type AdminOrderSummary, type PaymentAuditLog } from '@/api/admin/payment'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatPresaleDate } from '@/utils/presale'
import type { Column } from '@/components/common/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import AdminOrderDetail from './AdminOrderDetail.vue'
const { t, locale } = useI18n()
const app = useAppStore()
const { copyToClipboard } = useClipboard()
const rows = ref<CafeCampaign[]>([]), page = ref(1), total = ref(0), loading = ref(false)
const creating = ref(false), changing = ref(false), showCreate = ref(false), toggleTarget = ref<CafeCampaign | null>(null)
const form = reactive({ code: '', name: '', discount_percent: 40, start_date: '', end_date: '' })
const usageTarget = ref<CafeCampaign | null>(null), usages = ref<CafeCampaignUse[]>([]), usagePage = ref(1), usageTotal = ref(0), usageLoading = ref(false), usageError = ref('')
const order = ref<AdminPaymentOrder | null>(null), orderSummary = ref<AdminOrderSummary | null>(null), orderAudits = ref<PaymentAuditLog[]>([])
const showOrder = ref(false), orderLoading = ref(false), orderError = ref(false)
let listSeq = 0, usageSeq = 0, orderSeq = 0, orderId = 0
const date = (value: string) => formatPresaleDate(value, locale.value)
const columns = computed<Column[]>(() => [{ key: 'code', label: t('cafeCampaign.code') }, { key: 'discount_percent', label: t('cafeCampaign.discount') }, { key: 'starts_at', label: t('cafeCampaign.window') }, { key: 'enabled', label: t('common.status') }, { key: 'actions', label: t('common.actions') }])
const usageColumns = computed<Column[]>(() => [{ key: 'user_email', label: t('cafeCampaign.user') }, { key: 'order_id', label: t('cafeCampaign.order') }, { key: 'order_status', label: t('common.status') }, { key: 'used_at', label: t('cafeCampaign.useStatus') }])
const nameLength = computed(() => Array.from(form.name.trim()).length)
const valid = computed(() => nameLength.value > 0 && nameLength.value <= 100 && /^[A-Za-z0-9][A-Za-z0-9-]{0,35}$/.test(form.code.trim()) && Number.isInteger(form.discount_percent) && form.discount_percent >= 1 && form.discount_percent <= 99 && /^\d{4}-\d{2}-\d{2}$/.test(form.start_date) && /^\d{4}-\d{2}-\d{2}$/.test(form.end_date) && form.end_date >= form.start_date)
function failure(err: unknown) { return extractI18nErrorMessage(err, t, 'cafeCampaign.errors', t('cafeCampaign.failed')) }
function campaignStatus(row: CafeCampaign) { return t(`cafeCampaign.${!row.enabled ? 'disabled' : Date.now() < Date.parse(row.starts_at) ? 'upcoming' : Date.now() >= Date.parse(row.expires_at) ? 'expired' : 'active'}`) }
function useStatus(row: CafeCampaignUse) { return t(`cafeCampaign.${row.used_at || row.paid_at ? 'used' : ['CANCELLED', 'EXPIRED', 'FAILED'].includes(row.order_status) || Date.parse(row.expires_at) <= Date.now() ? 'released' : 'reserved'}`) }
function copy(code: string) { return copyToClipboard(code, t('cafeCampaign.copied')) }
async function load() {
  const seq = ++listSeq; loading.value = true
  try { const { data } = await cafeCampaignAPI.list(page.value); if (seq === listSeq) { rows.value = data.items || []; total.value = data.total } }
  catch (err) { if (seq === listSeq) app.showError(failure(err)) }
  finally { if (seq === listSeq) loading.value = false }
}
function changePage(value: number) { page.value = value; void load() }
function openCreate() {
  const parts = new Intl.DateTimeFormat('en', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date())
  const part = (type: string) => parts.find(p => p.type === type)?.value || ''
  const last = new Date(Date.UTC(Number(part('year')), Number(part('month')), 0)).getUTCDate()
  Object.assign(form, { code: '', name: '', discount_percent: 40, start_date: `${part('year')}-${part('month')}-${part('day')}`, end_date: `${part('year')}-${part('month')}-${last}` }); showCreate.value = true
}
function closeCreate() { if (!creating.value) showCreate.value = false }
async function create() {
  if (!valid.value || creating.value) return
  creating.value = true
  try { await cafeCampaignAPI.create({ ...form, name: form.name.trim(), code: form.code.trim().toUpperCase(), enabled: false }); showCreate.value = false; page.value = 1; app.showSuccess(t('cafeCampaign.created')); await load() }
  catch (err) { app.showError(failure(err)) } finally { creating.value = false }
}
function closeToggle() { if (!changing.value) toggleTarget.value = null }
async function toggle() {
  if (!toggleTarget.value || changing.value) return
  changing.value = true
  try { await cafeCampaignAPI.setEnabled(toggleTarget.value, !toggleTarget.value.enabled); app.showSuccess(t('cafeCampaign.changed')) }
  catch (err) { app.showError(failure(err)) } finally { changing.value = false; toggleTarget.value = null; void load() }
}
function openUses(row: CafeCampaign) { usageTarget.value = row; usagePage.value = 1; usageTotal.value = 0; usages.value = []; void loadUses() }
function closeUses() { usageTarget.value = null; usageSeq++ }
function changeUsagePage(value: number) { usagePage.value = value; void loadUses() }
async function loadUses() {
  if (!usageTarget.value) return
  const seq = ++usageSeq; usageLoading.value = true; usageError.value = ''; usages.value = []
  try { const { data } = await cafeCampaignAPI.uses(usageTarget.value.id, usagePage.value); if (seq === usageSeq) { usages.value = data.items || []; usageTotal.value = data.total } }
  catch (err) { if (seq === usageSeq) usageError.value = failure(err) } finally { if (seq === usageSeq) usageLoading.value = false }
}
function openOrder(id: number) { orderId = id; showOrder.value = true; void reloadOrder() }
function closeOrder() { showOrder.value = false; orderSeq++ }
async function reloadOrder() {
  const seq = ++orderSeq, id = orderId; order.value = null; orderSummary.value = null; orderAudits.value = []; orderLoading.value = true; orderError.value = false
  try { const { data } = await adminPaymentAPI.getOrder(id); if (seq !== orderSeq) return; if (data.order.id !== id) throw new Error('Unexpected order'); order.value = data.order; orderSummary.value = data.summary; orderAudits.value = data.auditLogs }
  catch { if (seq === orderSeq) orderError.value = true } finally { if (seq === orderSeq) orderLoading.value = false }
}
onMounted(load)
onBeforeUnmount(() => { listSeq++; usageSeq++; orderSeq++ })
</script>
