<template>
  <BaseDialog :show="show" :title="t('payment.admin.activityRecords')" width="full" @close="emit('close')">
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <button v-if="level !== 'activities'" type="button" class="btn btn-secondary btn-sm mb-2" @click="goBack">
            <Icon name="chevronLeft" size="sm" />
            {{ level === 'participations' ? t('payment.admin.backToParticipants') : t('payment.admin.backToActivities') }}
          </button>
          <p class="truncate text-sm font-semibold text-content-primary">
            {{ selectedActivity?.name || t('payment.admin.activityRecords') }}
            <span v-if="selectedParticipant" class="font-normal text-content-tertiary"> · {{ participantLabel(selectedParticipant) }}</span>
          </p>
          <p class="text-xs text-content-tertiary">{{ levelHint }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <input v-model.trim="keyword" class="input w-64" :placeholder="t('payment.admin.activityRecordSearch')" @keyup.enter="search" />
          <select v-if="level !== 'participants'" v-model="status" class="input w-36" @change="search">
            <option value="">{{ t('payment.admin.allStatuses') }}</option>
            <template v-if="level === 'activities'">
              <option value="active">{{ t('payment.admin.activityStatus.active') }}</option>
              <option value="scheduled">{{ t('payment.admin.activityStatus.scheduled') }}</option>
              <option value="ended">{{ t('payment.admin.activityStatus.ended') }}</option>
              <option value="disabled">{{ t('payment.admin.activityStatus.disabled') }}</option>
            </template>
            <template v-else>
              <option value="reserved">{{ t('payment.admin.participationStatus.reserved') }}</option>
              <option value="granted">{{ t('payment.admin.participationStatus.granted') }}</option>
              <option value="released">{{ t('payment.admin.participationStatus.released') }}</option>
            </template>
          </select>
          <button type="button" class="btn btn-primary" @click="search">{{ t('common.search') }}</button>
        </div>
      </div>

      <div v-if="loading" class="py-16 text-center text-sm text-content-tertiary">{{ t('common.loading') }}</div>

      <template v-else-if="level === 'activities'">
        <div v-if="activities.length === 0" class="empty-panel">{{ t('payment.admin.noActivityRecords') }}</div>
        <div v-else class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-600">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="bg-gray-50 text-left text-xs text-content-tertiary dark:bg-dark-700/60">
              <tr>
                <th class="table-head">{{ t('payment.admin.activityName') }}</th>
                <th class="table-head">{{ t('common.status') }}</th>
                <th class="table-head">{{ t('payment.admin.activityPeriod') }}</th>
                <th class="table-head text-right">{{ t('payment.admin.participantCount') }}</th>
                <th class="table-head text-right">{{ t('payment.admin.participationCount') }}</th>
                <th class="table-head">{{ t('payment.admin.participationBreakdown') }}</th>
                <th class="table-head text-right">{{ t('payment.admin.grantedBonusDays') }}</th>
                <th class="table-head text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="activity in activities" :key="activity.id" class="hover:bg-gray-50/70 dark:hover:bg-dark-700/30">
                <td class="table-cell"><div class="font-medium text-content-primary">{{ activity.name }}</div><div class="text-xs text-content-tertiary">#{{ activity.id }}</div></td>
                <td class="table-cell"><span :class="['badge text-xs', activityStatusClass(activity.status)]">{{ t(`payment.admin.activityStatus.${activity.status}`) }}</span></td>
                <td class="table-cell whitespace-nowrap text-xs">{{ formatDate(activity.starts_at) }}<br>{{ formatDate(activity.ends_at) }}</td>
                <td class="table-cell text-right font-medium">{{ activity.participant_count }}</td>
                <td class="table-cell text-right font-medium">{{ activity.participation_count }}</td>
                <td class="table-cell"><div class="flex flex-wrap gap-1"><span class="status-pill status-reserved">{{ activity.reserved_count }}</span><span class="status-pill status-granted">{{ activity.granted_count }}</span><span class="status-pill status-released">{{ activity.released_count }}</span></div></td>
                <td class="table-cell text-right font-medium text-emerald-600 dark:text-emerald-400">+{{ activity.granted_bonus_days }}</td>
                <td class="table-cell text-right"><button type="button" class="btn btn-secondary btn-sm" @click="openParticipants(activity)">{{ t('payment.admin.viewParticipants') }}</button></td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <template v-else-if="level === 'participants'">
        <div v-if="participants.length === 0" class="empty-panel">{{ t('payment.admin.noParticipants') }}</div>
        <div v-else class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-600">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="bg-gray-50 text-left text-xs text-content-tertiary dark:bg-dark-700/60">
              <tr><th class="table-head">{{ t('payment.admin.participant') }}</th><th class="table-head text-right">{{ t('payment.admin.participationCount') }}</th><th class="table-head">{{ t('payment.admin.participationBreakdown') }}</th><th class="table-head text-right">{{ t('payment.admin.grantedBonusDays') }}</th><th class="table-head">{{ t('payment.admin.firstParticipatedAt') }}</th><th class="table-head">{{ t('payment.admin.lastParticipatedAt') }}</th><th class="table-head text-right">{{ t('common.actions') }}</th></tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="participant in participants" :key="participant.user_id" class="hover:bg-gray-50/70 dark:hover:bg-dark-700/30">
                <td class="table-cell"><div class="font-medium text-content-primary">{{ participant.user_name || '-' }}</div><div class="text-xs text-content-tertiary">{{ participant.user_email || `#${participant.user_id}` }}</div></td>
                <td class="table-cell text-right font-medium">{{ participant.participation_count }}</td>
                <td class="table-cell"><div class="flex flex-wrap gap-1"><span class="status-pill status-reserved">{{ participant.reserved_count }}</span><span class="status-pill status-granted">{{ participant.granted_count }}</span><span class="status-pill status-released">{{ participant.released_count }}</span></div></td>
                <td class="table-cell text-right font-medium text-emerald-600 dark:text-emerald-400">+{{ participant.granted_bonus_days }}</td>
                <td class="table-cell whitespace-nowrap text-xs">{{ formatDate(participant.first_participated_at) }}</td>
                <td class="table-cell whitespace-nowrap text-xs">{{ formatDate(participant.last_participated_at) }}</td>
                <td class="table-cell text-right"><button type="button" class="btn btn-secondary btn-sm" @click="openParticipations(participant)">{{ t('common.view') }}</button></td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <template v-else>
        <div v-if="participations.length === 0" class="empty-panel">{{ t('payment.admin.noParticipationRecords') }}</div>
        <div v-else class="grid gap-3 lg:grid-cols-2">
          <article v-for="record in participations" :key="record.id" class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div><p class="font-semibold text-content-primary">{{ t('payment.admin.orderNumber') }} #{{ record.order_id }}</p><p class="text-xs text-content-tertiary">{{ record.out_trade_no || '-' }}</p></div>
              <OrderStatusBadge :status="record.order_status" />
            </div>
            <dl class="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
              <div><dt class="detail-label">{{ t('payment.admin.planName') }}</dt><dd>{{ record.plan_name || `#${record.plan_id}` }}</dd></div>
              <div><dt class="detail-label">{{ t('payment.admin.participationStatusLabel') }}</dt><dd><span :class="['status-pill', `status-${record.status}`]">{{ t(`payment.admin.participationStatus.${record.status}`) }}</span></dd></div>
              <div><dt class="detail-label">{{ t('payment.admin.bonusDays') }}</dt><dd class="font-semibold text-emerald-600 dark:text-emerald-400">+{{ record.bonus_days }} {{ t('payment.days') }}</dd></div>
              <div><dt class="detail-label">{{ t('payment.admin.payAmount') }}</dt><dd>¥{{ record.pay_amount.toFixed(2) }}</dd></div>
              <div><dt class="detail-label">{{ t('payment.admin.reservedAt') }}</dt><dd>{{ formatDate(record.reserved_at) }}</dd></div>
              <div v-if="record.granted_at"><dt class="detail-label">{{ t('payment.admin.grantedAt') }}</dt><dd>{{ formatDate(record.granted_at) }}</dd></div>
              <div v-if="record.released_at"><dt class="detail-label">{{ t('payment.admin.releasedAt') }}</dt><dd>{{ formatDate(record.released_at) }}</dd></div>
              <div v-if="record.release_reason" class="col-span-2"><dt class="detail-label">{{ t('payment.admin.releaseReason') }}</dt><dd class="break-words">{{ record.release_reason }}</dd></div>
              <div v-if="record.failed_reason" class="col-span-2"><dt class="detail-label">{{ t('payment.admin.failedReason') }}</dt><dd class="break-words text-red-600 dark:text-red-400">{{ record.failed_reason }}</dd></div>
            </dl>
          </article>
        </div>
      </template>

      <Pagination v-if="pagination.total > pagination.pageSize" :total="pagination.total" :page="pagination.page" :page-size="pagination.pageSize" :show-page-size-selector="false" @update:page="changePage" />
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminPaymentAPI } from '@/api/admin/payment'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { PromotionActivityParticipant, PromotionActivityParticipationRecord, PromotionActivityRecord, PromotionActivityStatus } from '@/types/payment'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t, locale } = useI18n()
const appStore = useAppStore()

type Level = 'activities' | 'participants' | 'participations'
const level = ref<Level>('activities')
const loading = ref(false)
const keyword = ref('')
const status = ref('')
const activities = ref<PromotionActivityRecord[]>([])
const participants = ref<PromotionActivityParticipant[]>([])
const participations = ref<PromotionActivityParticipationRecord[]>([])
const selectedActivity = ref<PromotionActivityRecord | null>(null)
const selectedParticipant = ref<PromotionActivityParticipant | null>(null)
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

const levelHint = computed(() => {
  if (level.value === 'activities') return t('payment.admin.activityRecordsHint')
  if (level.value === 'participants') return t('payment.admin.participantsHint')
  return t('payment.admin.participationRecordsHint')
})

function resetFilters(): void { keyword.value = ''; status.value = ''; pagination.page = 1; pagination.total = 0 }
function participantLabel(item: PromotionActivityParticipant): string { return item.user_name || item.user_email || `#${item.user_id}` }

async function load(): Promise<void> {
  loading.value = true
  try {
    if (level.value === 'activities') {
      const response = await adminPaymentAPI.getActivityRecords({ page: pagination.page, page_size: pagination.pageSize, keyword: keyword.value || undefined, status: status.value || undefined })
      activities.value = response.data.items || []; pagination.total = response.data.total || 0
    } else if (level.value === 'participants' && selectedActivity.value) {
      const response = await adminPaymentAPI.getActivityParticipants(selectedActivity.value.id, { page: pagination.page, page_size: pagination.pageSize, keyword: keyword.value || undefined })
      participants.value = response.data.items || []; pagination.total = response.data.total || 0
    } else if (selectedActivity.value && selectedParticipant.value) {
      const response = await adminPaymentAPI.getActivityParticipations(selectedActivity.value.id, { page: pagination.page, page_size: pagination.pageSize, user_id: selectedParticipant.value.user_id, keyword: keyword.value || undefined, status: status.value || undefined })
      participations.value = response.data.items || []; pagination.total = response.data.total || 0
    }
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  } finally { loading.value = false }
}

function search(): void { pagination.page = 1; void load() }
function changePage(page: number): void { pagination.page = page; void load() }
function openParticipants(activity: PromotionActivityRecord): void { selectedActivity.value = activity; selectedParticipant.value = null; level.value = 'participants'; resetFilters(); void load() }
function openParticipations(participant: PromotionActivityParticipant): void { selectedParticipant.value = participant; level.value = 'participations'; resetFilters(); void load() }
function goBack(): void {
  if (level.value === 'participations') { level.value = 'participants'; selectedParticipant.value = null }
  else { level.value = 'activities'; selectedActivity.value = null }
  resetFilters(); void load()
}
function formatDate(value?: string): string { if (!value) return '-'; const date = new Date(value); return Number.isNaN(date.getTime()) ? '-' : new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short' }).format(date) }
function activityStatusClass(value: PromotionActivityStatus): string { if (value === 'active') return 'badge-success'; if (value === 'scheduled') return 'badge-primary'; return value === 'ended' ? 'badge-warning' : 'badge-secondary' }

watch(() => props.show, visible => {
  if (!visible) return
  level.value = 'activities'; selectedActivity.value = null; selectedParticipant.value = null; resetFilters(); void load()
})
</script>

<style scoped>
.table-head { @apply whitespace-nowrap px-4 py-3 font-medium; }
.table-cell { @apply px-4 py-3 text-content-secondary; }
.empty-panel { @apply rounded-xl border border-dashed border-gray-300 py-16 text-center text-sm text-content-tertiary dark:border-dark-600; }
.detail-label { @apply mb-1 text-xs text-content-tertiary; }
.status-pill { @apply inline-flex rounded-full px-2 py-0.5 text-xs font-medium; }
.status-reserved { @apply bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300; }
.status-granted { @apply bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300; }
.status-released { @apply bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300; }
</style>
