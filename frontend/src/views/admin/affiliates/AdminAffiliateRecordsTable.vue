<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative w-full md:w-80">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input v-model="filters.search" type="text" class="input pl-10" :placeholder="t('admin.affiliates.records.searchPlaceholder')" @input="debounceLoad" />
          </div>
          <input v-model="filters.start_at" type="date" class="input w-full sm:w-44" :title="t('admin.affiliates.records.startAt')" @change="reloadFromFirstPage" />
          <input v-model="filters.end_at" type="date" class="input w-full sm:w-44" :title="t('admin.affiliates.records.endAt')" @change="reloadFromFirstPage" />
          <button class="btn btn-secondary px-2 md:px-3" :disabled="loading" :title="t('common.refresh')" @click="loadRecords">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="records"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          :sort-storage-key="sortStorageKey"
          @sort="handleSort"
        >
          <template #cell-inviter="{ row }">
            <UserCell
              :id="row.inviter_id"
              :email="row.inviter_email"
              :username="row.inviter_username"
              :clickable="props.type !== 'transfers'"
              @open="openUserOverview"
            />
          </template>
          <template #cell-invitee="{ row }">
            <UserCell
              :id="row.invitee_id"
              :email="row.invitee_email"
              :username="row.invitee_username"
              :clickable="props.type !== 'transfers'"
              @open="openUserOverview"
            />
          </template>
          <template #cell-user="{ row }">
            <UserCell
              :id="row.user_id"
              :email="row.user_email"
              :username="row.username"
              :clickable="true"
              @open="openUserOverview"
            />
          </template>
          <template #cell-aff_code="{ row }">
            <span class="font-mono text-sm text-content-secondary">{{ row.aff_code || '-' }}</span>
          </template>
          <template #cell-order="{ row }">
            <div class="space-y-0.5">
              <div class="font-mono text-sm text-content-primary">#{{ row.order_id }}</div>
              <div class="max-w-56 truncate text-sm text-content-tertiary">{{ row.out_trade_no }}</div>
            </div>
          </template>
          <template #cell-rebate_group="{ row }">
            <span class="text-sm text-content-secondary">{{ formatGroupName(row.subscription_group_name, row.subscription_group_id) }}</span>
          </template>

          <template #cell-payment_type="{ row }">
            {{ t('payment.methods.' + row.payment_type, row.payment_type || '-') }}
          </template>
          <template #cell-transfer_action="{ row }">
            <span class="text-sm text-content-secondary">{{ formatTransferAction(row.action) }}</span>
          </template>
          <template #cell-transfer_group="{ row }">
            <span class="text-sm text-content-secondary">{{ formatGroupName(row.subscription_group_name, row.subscription_group_id) }}</span>
          </template>
          <template #cell-order_status="{ row }">
            <OrderStatusBadge :status="row.order_status" />
          </template>
          <template #cell-total_rebate_points="{ row }">
            <PointsText :value="row.total_rebate_points ?? row.total_rebate" />
          </template>
          <template #cell-order_amount="{ row }">
            <AmountText :value="row.order_amount" />
          </template>
          <template #cell-pay_amount="{ row }">
            <span class="text-sm text-content-primary">¥{{ formatAmount(row.pay_amount) }}</span>
          </template>
          <template #cell-rebate_points="{ row }">
            <div class="text-right">
              <PointsText :value="row.rebate_points ?? row.rebate_amount" strong />
            </div>
          </template>
          <template #cell-amount="{ row }">
            <span class="text-sm font-semibold text-emerald-600 dark:text-emerald-400">
              {{ t('admin.affiliates.records.pointsValue', { points: formatPoints(row.redeemed_points ?? row.amount) }) }}
            </span>
          </template>
          <template #cell-balance_after="{ row }">
            <NullableAmountText :value="row.balance_after" />
          </template>
          <template #cell-available_points_after="{ row }">
            <NullablePointsText :value="row.available_points_after ?? row.available_quota_after" />
          </template>
          <template #cell-frozen_points_after="{ row }">
            <NullablePointsText :value="row.frozen_points_after ?? row.frozen_quota_after" />
          </template>
          <template #cell-history_points_after="{ row }">
            <NullablePointsText :value="row.history_points_after ?? row.history_quota_after" />
          </template>
          <template #cell-created_at="{ row }">
            <span class="text-sm text-content-secondary">{{ formatDateTime(row.created_at) }}</span>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="overviewDialog"
      :title="t('admin.affiliates.overview.title')"
      width="normal"
      @close="overviewDialog = false"
    >
      <div v-if="overviewLoading" class="flex justify-center py-8">
        <div class="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
      </div>
      <div v-else-if="selectedOverview" class="space-y-4">
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="font-mono text-sm text-content-primary">#{{ selectedOverview.user_id }}</div>
          <div class="mt-1 text-sm font-medium text-content-primary">{{ selectedOverview.email || '-' }}</div>
          <div class="mt-0.5 text-sm text-content-tertiary">{{ selectedOverview.username || '-' }}</div>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <OverviewStat :label="t('admin.affiliates.overview.affCode')" :value="selectedOverview.aff_code || '-'" mono />
          <OverviewStat :label="t('admin.affiliates.overview.rebateRate')" :value="formatPercent(selectedOverview.rebate_rate_percent)" />
          <OverviewStat :label="t('admin.affiliates.overview.invitedCount')" :value="String(selectedOverview.invited_count)" />
          <OverviewStat :label="t('admin.affiliates.overview.rebatedInviteeCount')" :value="String(selectedOverview.rebated_invitee_count)" />
          <OverviewStat :label="t('admin.affiliates.overview.availablePoints')" :value="formatPoints(selectedOverview.available_rebate_points ?? selectedOverview.available_quota)" />
          <OverviewStat :label="t('admin.affiliates.overview.totalPoints')" :value="formatPoints(selectedOverview.total_rebate_points ?? selectedOverview.history_quota)" />
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { affiliatesAPI, type AffiliateInviteRecord, type AffiliateRebateRecord, type AffiliateTransferRecord, type AffiliateUserOverview, type ListAffiliateRecordsParams } from '@/api/admin/affiliates'
import type { PaginatedResponse } from '@/types'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDisplayDateTime } from '@/utils/format'

type RecordType = 'invites' | 'rebates' | 'transfers'
type AffiliateRecord = AffiliateInviteRecord | AffiliateRebateRecord | AffiliateTransferRecord

const props = defineProps<{
  type: RecordType
}>()

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const records = ref<AffiliateRecord[]>([])
const filters = reactive({ search: '', start_at: '', end_at: '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const overviewDialog = ref(false)
const overviewLoading = ref(false)
const selectedOverview = ref<AffiliateUserOverview | null>(null)
let debounceTimer: ReturnType<typeof setTimeout> | null = null

const columns = computed<Column[]>(() => {
  if (props.type === 'invites') {
    return [
      { key: 'inviter', label: t('admin.affiliates.records.inviter'), sortable: true },
      { key: 'invitee', label: t('admin.affiliates.records.invitee'), sortable: true },
      { key: 'aff_code', label: t('admin.affiliates.records.affCode'), sortable: true },
      { key: 'total_rebate_points', label: t('admin.affiliates.records.totalRebatePoints'), sortable: true },
      { key: 'created_at', label: t('admin.affiliates.records.invitedAt'), sortable: true },
    ]
  }
  if (props.type === 'rebates') {
    return [
      { key: 'order', label: t('admin.affiliates.records.order'), sortable: true },
      { key: 'inviter', label: t('admin.affiliates.records.inviter'), sortable: true },
      { key: 'invitee', label: t('admin.affiliates.records.invitee'), sortable: true },
      { key: 'order_amount', label: t('admin.affiliates.records.orderAmount'), sortable: true },
      { key: 'pay_amount', label: t('admin.affiliates.records.payAmount'), sortable: true },
      { key: 'rebate_points', label: t('admin.affiliates.records.rebatePoints') },
      { key: 'rebate_group', label: t('admin.affiliates.records.subscriptionGroup') },
      { key: 'payment_type', label: t('admin.affiliates.records.paymentType'), sortable: true },
      { key: 'order_status', label: t('admin.affiliates.records.orderStatus'), sortable: true },
      { key: 'created_at', label: t('admin.affiliates.records.rebatedAt'), sortable: true },
    ]
  }
  return [
    { key: 'user', label: t('admin.affiliates.records.user'), sortable: true },
    { key: 'transfer_action', label: t('admin.affiliates.records.transferType'), sortable: true },
    { key: 'amount', label: t('admin.affiliates.records.transferAmount'), sortable: true },
    { key: 'transfer_group', label: t('admin.affiliates.records.subscriptionGroup') },
    { key: 'balance_after', label: t('admin.affiliates.records.balanceAfter'), sortable: true },
    { key: 'available_points_after', label: t('admin.affiliates.records.availablePointsAfter'), sortable: true },
    { key: 'frozen_points_after', label: t('admin.affiliates.records.frozenPointsAfter'), sortable: true },
    { key: 'history_points_after', label: t('admin.affiliates.records.historyPointsAfter'), sortable: true },
    { key: 'created_at', label: t('admin.affiliates.records.transferredAt'), sortable: true },
  ]
})

const sortStorageKey = computed(() => `admin-affiliate-${props.type}-table-sort`)

function loadInitialSortState(): { sort_by: string; sort_order: 'asc' | 'desc' } {
  const fallback = { sort_by: 'created_at', sort_order: 'desc' as 'asc' | 'desc' }
  try {
    const raw = localStorage.getItem(sortStorageKey.value)
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as { key?: string; order?: string }
    const key = typeof parsed.key === 'string' ? parsed.key : ''
    if (!columns.value.some((column) => column.key === key && column.sortable)) return fallback
    return {
      sort_by: key,
      sort_order: parsed.order === 'asc' ? 'asc' : 'desc',
    }
  } catch {
    return fallback
  }
}

const sortState = reactive(loadInitialSortState())

function userTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return 'UTC'
  }
}

function buildParams(): ListAffiliateRecordsParams {
  return {
    page: pagination.page,
    page_size: pagination.page_size,
    search: filters.search.trim() || undefined,
    start_at: filters.start_at || undefined,
    end_at: filters.end_at || undefined,
    sort_by: sortState.sort_by,
    sort_order: sortState.sort_order,
    timezone: userTimezone(),
  }
}

async function fetchRecords(params: ListAffiliateRecordsParams): Promise<PaginatedResponse<AffiliateRecord>> {
  if (props.type === 'invites') {
    return affiliatesAPI.listInviteRecords(params)
  }
  if (props.type === 'rebates') {
    return affiliatesAPI.listRebateRecords(params)
  }
  return affiliatesAPI.listTransferRecords(params)
}

async function loadRecords() {
  loading.value = true
  try {
    const res = await fetchRecords(buildParams())
    records.value = res.items || []
    pagination.total = res.total || 0
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.affiliates.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

function debounceLoad() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => reloadFromFirstPage(), 300)
}

function reloadFromFirstPage() {
  pagination.page = 1
  void loadRecords()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadRecords()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  void loadRecords()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  void loadRecords()
}

function formatAmount(value: number | null | undefined): string {
  return Number(value || 0).toFixed(2)
}

function formatPoints(value: number | null | undefined): string {
  const amount = Number(value || 0)
  const formatted = new Intl.NumberFormat(undefined, {
    minimumFractionDigits: amount > 0 && amount < 0.01 ? 6 : 2,
    maximumFractionDigits: amount > 0 && amount < 0.01 ? 6 : 2,
  }).format(amount)
  return `¥${formatted}`
}

function formatPercent(value: number | null | undefined): string {
  const rounded = Math.round(Number(value || 0) * 100) / 100
  return `${Number.isInteger(rounded) ? rounded.toString() : rounded.toString()}%`
}


function formatDateTime(value: string | null | undefined): string {
  return value ? formatDisplayDateTime(value) : '-'
}

function formatGroupName(name: string | null | undefined, id: number | null | undefined): string {
  if (name) return name
  if (id) return `#${id}`
  return '-'
}

function formatTransferAction(action: string | null | undefined): string {
  if (action === 'transfer_subscription' || action === 'redeem_subscription') return t('admin.affiliates.records.transferSubscription')
  return t('admin.affiliates.records.transferBalance')
}

async function openUserOverview(userId: number) {
  if (!userId) return
  overviewDialog.value = true
  overviewLoading.value = true
  selectedOverview.value = null
  try {
    selectedOverview.value = await affiliatesAPI.getUserOverview(userId)
  } catch (error) {
    overviewDialog.value = false
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.affiliates.errors', t('common.error')))
  } finally {
    overviewLoading.value = false
  }
}

const UserCell = defineComponent({
  props: {
    id: { type: Number, required: true },
    email: { type: String, default: '' },
    username: { type: String, default: '' },
    clickable: { type: Boolean, default: false },
  },
  emits: ['open'],
  setup(cellProps, { emit }) {
    return () => h('div', { class: 'space-y-0.5' }, [
      h('div', { class: 'font-mono text-sm text-content-primary' }, `#${cellProps.id}`),
      h(cellProps.clickable ? 'button' : 'div', {
        class: cellProps.clickable
          ? 'max-w-56 truncate text-left text-sm font-medium text-primary-600 hover:text-primary-700 hover:underline dark:text-primary-400 dark:hover:text-primary-300'
          : 'max-w-56 truncate text-sm text-content-secondary',
        type: cellProps.clickable ? 'button' : undefined,
        onClick: cellProps.clickable ? () => emit('open', cellProps.id) : undefined,
      }, cellProps.email || '-'),
      h('div', { class: 'max-w-56 truncate text-sm text-content-tertiary' }, cellProps.username || '-'),
    ])
  },
})

const AmountText = defineComponent({
  props: {
    value: { type: Number, default: 0 },
    strong: { type: Boolean, default: false },
  },
  setup(amountProps) {
    return () => h('span', {
      class: amountProps.strong
        ? 'text-sm font-semibold text-emerald-600 dark:text-emerald-400'
        : 'text-sm text-content-primary',
    }, `$${formatAmount(amountProps.value)}`)
  },
})

const NullableAmountText = defineComponent({
  props: {
    value: { type: Number as PropType<number | null | undefined>, default: null },
  },
  setup(amountProps) {
    return () => {
      const value = amountProps.value
      if (value === null || value === undefined) {
        return h('span', { class: 'text-sm text-content-tertiary' }, '-')
      }
      return h(AmountText, { value })
    }
  },
})

const PointsText = defineComponent({
  props: {
    value: { type: Number as PropType<number | null | undefined>, default: 0 },
    strong: { type: Boolean, default: false },
  },
  setup(pointsProps) {
    return () => h('span', {
      class: pointsProps.strong
        ? 'text-sm font-semibold text-emerald-600 dark:text-emerald-400'
        : 'text-sm text-content-primary',
    }, t('admin.affiliates.records.pointsValue', { points: formatPoints(pointsProps.value) }))
  },
})

const NullablePointsText = defineComponent({
  props: {
    value: { type: Number as PropType<number | null | undefined>, default: null },
  },
  setup(pointsProps) {
    return () => {
      const value = pointsProps.value
      if (value === null || value === undefined) {
        return h('span', { class: 'text-sm text-content-tertiary' }, '-')
      }
      return h(PointsText, { value })
    }
  },
})

const OverviewStat = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    mono: { type: Boolean, default: false },
  },
  setup(statProps) {
    return () => h('div', { class: 'rounded-lg border border-gray-100 bg-white p-3 dark:border-dark-700 dark:bg-dark-900' }, [
      h('div', { class: 'text-sm text-content-tertiary' }, statProps.label),
      h('div', {
        class: statProps.mono
          ? 'mt-1 font-mono text-base font-semibold text-content-primary'
          : 'mt-1 text-base font-semibold text-content-primary',
      }, statProps.value),
    ])
  },
})

onMounted(() => {
  void loadRecords()
})
</script>
