<template>
  <BaseDialog :show="show" :title="t('admin.users.editMembershipPoints')" width="narrow" @close="$emit('close')">
    <form v-if="user" id="membership-points-form" @submit.prevent="handleSubmit" class="space-y-5">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100">
          <span class="text-lg font-medium text-primary-700">{{ user.email.charAt(0).toUpperCase() }}</span>
        </div>
        <div class="flex-1">
          <p class="font-medium text-content-primary">{{ user.email }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.users.currentMembershipPoints') }}: {{ formatPoints(currentPoints) }}
          </p>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.users.currentMembershipLevel') }}: {{ membershipLabel(currentLevel) }}
          </p>
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.users.membershipOperation') }}</label>
        <Select
          v-model="form.operation"
          :options="operationOptions"
          @change="handleOperationChange"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.users.membershipPoints') }}</label>
        <div class="relative">
          <input v-model.number="form.points" type="number" step="any" min="0" class="input pr-10" />
          <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-gray-400 dark:text-dark-400">
            {{ t('admin.users.membershipPointsUnit') }}
          </span>
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.users.notes') }}</label>
        <textarea v-model="form.notes" rows="3" class="input"></textarea>
      </div>

      <div v-if="user" class="rounded-xl border border-blue-200 bg-blue-50 p-4 dark:border-blue-800 dark:bg-blue-950">
        <div class="flex items-center justify-between text-sm">
          <span class="text-content-secondary">{{ t('admin.users.newMembershipPoints') }}:</span>
          <span class="font-bold text-content-primary">{{ formatPoints(nextPoints) }}</span>
        </div>
        <div class="mt-2 flex items-center justify-between text-sm">
          <span class="text-content-secondary">{{ t('admin.users.newMembershipLevel') }}:</span>
          <span class="font-bold text-content-primary">{{ membershipLabel(nextLevel) }}</span>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="membership-points-form" :disabled="submitting" class="btn btn-primary">
          {{ submitting ? t('common.saving') : t('common.confirm') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'

const props = defineProps<{ show: boolean, user: AdminUser | null }>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'success', user: AdminUser): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const submitting = ref(false)
const form = reactive({
  operation: 'set' as 'set' | 'add' | 'subtract',
  points: 0,
  notes: ''
})

const currentPoints = computed(() => props.user?.total_recharged ?? 0)
const currentLevel = computed(() => props.user?.membership_level ?? resolveMembershipLevel(currentPoints.value))
const nextPoints = computed(() => {
  const amount = Number(form.points) || 0
  if (form.operation === 'add') return currentPoints.value + amount
  if (form.operation === 'subtract') return Math.max(currentPoints.value - amount, 0)
  return amount
})
const nextLevel = computed(() => resolveMembershipLevel(nextPoints.value))

const operationOptions = computed(() => [
  { value: 'set', label: t('admin.users.setPoints') },
  { value: 'add', label: t('admin.users.addPoints') },
  { value: 'subtract', label: t('admin.users.subtractPoints') }
])

watch(() => props.show, (visible) => {
  if (!visible) return
  form.operation = 'set'
  form.points = props.user?.total_recharged ?? 0
  form.notes = ''
})

const formatPoints = (value: number) => {
  if (value === 0) return '0.00'
  const formatted = value.toFixed(8).replace(/\.?0+$/, '')
  const parts = formatted.split('.')
  if (parts.length === 1) return formatted + '.00'
  if (parts[1].length === 1) return formatted + '0'
  return formatted
}

const resolveMembershipLevel = (points: number) => {
  if (points > 1000) return 3
  if (points > 300) return 2
  if (points > 20) return 1
  return 0
}

const membershipLabel = (level: number) => `LV.${level}`

const handleOperationChange = () => {
  if (form.operation === 'set') {
    form.points = props.user?.total_recharged ?? 0
    return
  }
  form.points = 0
}

const handleSubmit = async () => {
  if (!props.user) return
  if (Number.isNaN(form.points) || form.points < 0) {
    appStore.showError(t('admin.users.pointsRequired'))
    return
  }
  if (form.operation !== 'set' && form.points <= 0) {
    appStore.showError(t('admin.users.pointsRequired'))
    return
  }
  if (form.operation === 'subtract' && form.points > currentPoints.value) {
    appStore.showError(t('admin.users.insufficientMembershipPoints'))
    return
  }

  submitting.value = true
  try {
    const updated = await adminAPI.users.updateMembershipPoints(props.user.id, form.points, form.operation, form.notes)
    appStore.showSuccess(t('common.success'))
    emit('success', updated)
    emit('close')
  } catch (e: any) {
    console.error('Failed to update membership points:', e)
    appStore.showError(e.response?.data?.detail || t('common.error'))
  } finally {
    submitting.value = false
  }
}
</script>
