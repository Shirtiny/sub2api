<template>
  <BaseDialog :show="show" :title="t('payment.admin.activityConfig')" width="extra-wide" @close="emit('close')">
    <div class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(360px,0.9fr)]">
      <section class="min-w-0">
        <div class="mb-3 flex items-center justify-between gap-3">
          <p class="text-sm text-content-tertiary">{{ t('payment.admin.activityConfigHint') }}</p>
          <button type="button" class="btn btn-primary shrink-0" @click="startCreate">
            {{ t('payment.admin.createActivity') }}
          </button>
        </div>
        <div v-if="loading" class="py-12 text-center text-sm text-content-tertiary">{{ t('common.loading') }}</div>
        <div v-else-if="activities.length === 0" class="rounded-xl border border-dashed border-gray-300 py-12 text-center text-sm text-content-tertiary dark:border-dark-600">
          {{ t('payment.admin.noActivities') }}
        </div>
        <div v-else class="space-y-2">
          <article
            v-for="activity in activities"
            :key="activity.id"
            class="rounded-xl border p-3 transition-colors"
            :class="editingId === activity.id ? 'border-primary-400 bg-primary-50/40 dark:bg-primary-950/10' : 'border-gray-200 dark:border-dark-600'"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="truncate text-sm font-semibold text-content-primary">{{ activity.name }}</span>
                  <span :class="['badge text-xs', statusClass(activity.status)]">{{ statusText(activity.status) }}</span>
                </div>
                <p class="mt-1 text-xs text-content-tertiary">
                  {{ formatDate(activity.starts_at) }} - {{ formatDate(activity.ends_at) }}
                </p>
                <p class="mt-1 text-xs text-content-tertiary">
                  {{ t('payment.admin.activityPlanCount', { count: activity.plan_bonuses.length }) }}
                  {{ t('payment.admin.activityUserLimit', { count: activity.max_uses_per_user }) }}
                </p>
              </div>
              <div class="flex shrink-0 gap-1">
                <button type="button" class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" @click="startEdit(activity)">
                  <Icon name="edit" size="sm" />
                </button>
                <button type="button" class="rounded-lg p-2 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/20" @click="askDelete(activity)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
          </article>
        </div>
      </section>

      <form id="activity-form" class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-600" @submit.prevent="saveActivity">
        <h4 class="font-semibold text-content-primary">
          {{ editingId ? t('payment.admin.editActivity') : t('payment.admin.createActivity') }}
        </h4>
        <div>
          <label class="input-label">{{ t('payment.admin.activityName') }} <span class="text-red-500">*</span></label>
          <input v-model.trim="form.name" class="input" required maxlength="100" />
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.activityType') }}</label>
          <input class="input" :value="t('payment.admin.subscriptionBonusDays')" disabled />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="input-label">{{ t('payment.admin.activityStartsAt') }} <span class="text-red-500">*</span></label>
            <input v-model="form.starts_at" type="datetime-local" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.activityEndsAt') }} <span class="text-red-500">*</span></label>
            <input v-model="form.ends_at" type="datetime-local" class="input" required />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.maxUsesPerUser') }}</label>
          <input v-model.number="form.max_uses_per_user" type="number" min="1" max="1000" step="1" class="input" required />
          <p class="mt-1 text-xs text-content-tertiary">{{ t('payment.admin.maxUsesPerUserHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.activityPlans') }} <span class="text-red-500">*</span></label>
          <div class="mt-2 max-h-64 space-y-2 overflow-y-auto rounded-lg border border-gray-200 p-2 dark:border-dark-600">
            <label v-for="plan in plans" :key="plan.id" class="flex items-center gap-3 rounded-lg px-2 py-2 hover:bg-gray-50 dark:hover:bg-dark-700/50">
              <input v-model="selectedPlanIds" type="checkbox" :value="plan.id" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span class="min-w-0 flex-1 truncate text-sm text-content-secondary">{{ plan.name }}</span>
              <div v-if="selectedPlanIds.includes(plan.id)" class="flex items-center gap-1" @click.stop>
                <input v-model.number="bonusDays[plan.id]" type="number" min="1" max="36500" step="1" class="input w-24" required />
                <span class="text-xs text-content-tertiary">{{ t('payment.days') }}</span>
              </div>
            </label>
          </div>
        </div>
        <label class="flex items-center gap-3">
          <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-content-secondary">{{ t('payment.admin.activityEnabled') }}</span>
        </label>
        <div class="flex justify-end gap-2 pt-1">
          <button type="button" class="btn btn-secondary" @click="resetForm">{{ t('common.reset') }}</button>
          <button type="submit" class="btn btn-primary" :disabled="saving">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </form>
    </div>
  </BaseDialog>

  <ConfirmDialog
    :show="showDeleteConfirm"
    :title="t('payment.admin.deleteActivity')"
    :message="t('payment.admin.deleteActivityConfirm')"
    :confirm-text="t('common.delete')"
    danger
    @confirm="deleteActivity"
    @cancel="showDeleteConfirm = false"
  />
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminPaymentAPI } from '@/api/admin/payment'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { PromotionActivity, PromotionActivityStatus, SubscriptionPlan, UpsertPromotionActivityRequest } from '@/types/payment'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean; plans: SubscriptionPlan[] }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const { t, locale } = useI18n()
const appStore = useAppStore()

const activities = ref<PromotionActivity[]>([])
const loading = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const selectedPlanIds = ref<number[]>([])
const bonusDays = reactive<Record<number, number>>({})
const showDeleteConfirm = ref(false)
const deletingActivity = ref<PromotionActivity | null>(null)
const form = reactive({ name: '', starts_at: '', ends_at: '', max_uses_per_user: 1, enabled: true })

function toLocalInput(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

function defaultWindow(): { starts_at: string; ends_at: string } {
  const starts = new Date()
  starts.setSeconds(0, 0)
  const ends = new Date(starts)
  ends.setDate(ends.getDate() + 7)
  return { starts_at: toLocalInput(starts.toISOString()), ends_at: toLocalInput(ends.toISOString()) }
}

function resetForm(): void {
  editingId.value = null
  selectedPlanIds.value = []
  Object.keys(bonusDays).forEach(key => delete bonusDays[Number(key)])
  Object.assign(form, { name: '', ...defaultWindow(), max_uses_per_user: 1, enabled: true })
}

function startCreate(): void {
  resetForm()
}

function startEdit(activity: PromotionActivity): void {
  editingId.value = activity.id
  Object.assign(form, {
    name: activity.name,
    starts_at: toLocalInput(activity.starts_at),
    ends_at: toLocalInput(activity.ends_at),
    max_uses_per_user: activity.max_uses_per_user,
    enabled: activity.enabled,
  })
  selectedPlanIds.value = activity.plan_bonuses.map(item => item.plan_id)
  Object.keys(bonusDays).forEach(key => delete bonusDays[Number(key)])
  activity.plan_bonuses.forEach(item => { bonusDays[item.plan_id] = item.bonus_days })
}

async function loadActivities(): Promise<void> {
  loading.value = true
  try {
    const response = await adminPaymentAPI.getActivities()
    activities.value = response.data || []
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

async function saveActivity(): Promise<void> {
  const editingDetachedActivity = editingId.value != null
    && activities.value.some(activity => activity.id === editingId.value && activity.plan_bonuses.length === 0)
  if (selectedPlanIds.value.length === 0 && !editingDetachedActivity) {
    appStore.showError(t('payment.admin.activityPlansRequired'))
    return
  }
  const payload: UpsertPromotionActivityRequest = {
    name: form.name,
    type: 'subscription_bonus_days',
    enabled: form.enabled,
    starts_at: new Date(form.starts_at).toISOString(),
    ends_at: new Date(form.ends_at).toISOString(),
    max_uses_per_user: form.max_uses_per_user,
    plan_bonuses: selectedPlanIds.value.map(planId => ({ plan_id: planId, bonus_days: Number(bonusDays[planId] || 0) })),
  }
  if (payload.plan_bonuses.some(item => item.bonus_days < 1)) {
    appStore.showError(t('payment.admin.activityBonusDaysRequired'))
    return
  }
  saving.value = true
  try {
    if (editingId.value) await adminPaymentAPI.updateActivity(editingId.value, payload)
    else await adminPaymentAPI.createActivity(payload)
    appStore.showSuccess(t('common.saved'))
    await loadActivities()
    emit('changed')
    resetForm()
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  } finally {
    saving.value = false
  }
}

function askDelete(activity: PromotionActivity): void {
  deletingActivity.value = activity
  showDeleteConfirm.value = true
}

async function deleteActivity(): Promise<void> {
  if (!deletingActivity.value) return
  try {
    await adminPaymentAPI.deleteActivity(deletingActivity.value.id)
    appStore.showSuccess(t('common.deleted'))
    showDeleteConfirm.value = false
    deletingActivity.value = null
    await loadActivities()
    emit('changed')
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  }
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))
}

function statusText(status: PromotionActivityStatus): string {
  return t(`payment.admin.activityStatus.${status}`)
}

function statusClass(status: PromotionActivityStatus): string {
  if (status === 'active') return 'badge-success'
  if (status === 'scheduled') return 'badge-primary'
  return status === 'ended' ? 'badge-warning' : 'badge-secondary'
}

watch(() => props.show, visible => {
  if (!visible) return
  resetForm()
  void loadActivities()
})
</script>
