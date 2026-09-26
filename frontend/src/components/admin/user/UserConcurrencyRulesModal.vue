<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.concurrencyRules.title')"
    width="wide"
    :close-on-escape="!saving"
    :close-on-click-outside="!saving"
    @close="close"
  >
    <div class="space-y-6" :aria-busy="loading || saving">
      <div class="space-y-2 rounded-xl bg-surface-secondary p-4 text-sm leading-relaxed text-content-secondary">
        <p>{{ t('admin.users.concurrencyRules.description') }}</p>
        <p id="balance-concurrency-coverage">{{ t('admin.users.concurrencyRules.coverage') }}</p>
      </div>

      <div v-if="loading" role="status" class="py-10 text-center text-content-secondary">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="loadError" class="space-y-3">
        <p role="alert" class="text-sm text-red-600 dark:text-red-400">{{ loadError }}</p>
        <button type="button" class="btn btn-secondary" data-testid="rules-retry" @click="load">
          {{ t('admin.users.concurrencyRules.retry') }}
        </button>
      </div>
      <form
        v-else-if="loaded"
        id="balance-concurrency-form"
        class="space-y-5"
        novalidate
        aria-describedby="balance-concurrency-coverage"
        @submit.prevent="save"
      >
        <fieldset
          v-for="(row, index) in rows"
          :key="index"
          :disabled="saving"
          class="min-w-0 rounded-xl border border-stroke-default p-4 sm:p-5"
          data-testid="concurrency-tier"
        >
          <legend class="px-2 text-sm font-semibold text-content-primary">
            {{ t('admin.users.concurrencyRules.tier', { number: index + 1 }) }}
          </legend>
          <div class="grid items-start gap-5 sm:grid-cols-3">
            <div class="min-w-0 space-y-2">
              <label :for="`balance-tier-min-${index}`" class="block text-sm font-medium text-content-primary">
                {{ t('admin.users.concurrencyRules.minBalance') }}
              </label>
              <input
                :id="`balance-tier-min-${index}`"
                v-model.number="row.min_balance"
                type="number"
                min="0"
                max="1000000000000"
                step="any"
                inputmode="decimal"
                class="input w-full"
                :disabled="index === 0"
                :aria-invalid="!!rowErrors[index].balance"
                :aria-describedby="`balance-tier-min-hint-${index}`"
                data-testid="tier-min"
              />
              <p :id="`balance-tier-min-hint-${index}`" class="text-xs leading-relaxed" :class="rowErrors[index].balance ? 'text-red-600 dark:text-red-400' : 'text-content-tertiary'">
                {{ rowErrors[index].balance || (index === 0 ? t('admin.users.concurrencyRules.firstFixed') : '') }}
              </p>
            </div>
            <div class="min-w-0 space-y-2">
              <p class="text-sm font-medium text-content-primary">{{ t('admin.users.concurrencyRules.upperBalance') }}</p>
              <p class="break-all rounded-lg bg-surface-secondary px-3 py-2.5 text-sm tabular-nums text-content-secondary" data-testid="tier-upper">
                {{ upperBound(index) }}
              </p>
            </div>
            <div class="min-w-0 space-y-2">
              <label :for="`balance-tier-concurrency-${index}`" class="block text-sm font-medium text-content-primary">
                {{ t('admin.users.concurrencyRules.concurrency') }}
              </label>
              <input
                :id="`balance-tier-concurrency-${index}`"
                v-model.number="row.concurrency"
                type="number"
                min="1"
                max="1000"
                step="1"
                inputmode="numeric"
                class="input w-full"
                :aria-invalid="!!rowErrors[index].concurrency"
                :aria-describedby="rowErrors[index].concurrency ? `balance-tier-concurrency-error-${index}` : undefined"
                data-testid="tier-concurrency"
              />
              <p v-if="rowErrors[index].concurrency" :id="`balance-tier-concurrency-error-${index}`" class="text-xs leading-relaxed text-red-600 dark:text-red-400">
                {{ rowErrors[index].concurrency }}
              </p>
            </div>
          </div>
          <div v-if="index > 0" class="mt-4 flex justify-end">
            <button
              type="button"
              class="btn btn-secondary text-sm"
              :aria-label="t('admin.users.concurrencyRules.removeTierLabel', { number: index + 1 })"
              data-testid="tier-remove"
              @click="removeTier(index)"
            >
              {{ t('admin.users.concurrencyRules.removeTier') }}
            </button>
          </div>
        </fieldset>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm text-content-tertiary">{{ t('admin.users.concurrencyRules.tierCount', { count: rows.length }) }}</p>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="saving || rows.length >= 20"
            data-testid="tier-add"
            @click="addTier"
          >
            {{ t('admin.users.concurrencyRules.addTier') }}
          </button>
        </div>
        <p v-if="invalid" role="alert" class="text-sm text-red-600 dark:text-red-400">
          {{ t('admin.users.concurrencyRules.validation.summary') }}
        </p>
        <p v-if="saveError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ saveError }}</p>
      </form>
    </div>
    <template #footer>
      <div class="flex w-full flex-wrap items-center justify-end gap-3">
        <span v-if="saving" role="status" class="mr-auto text-sm text-content-secondary">{{ t('common.saving') }}</span>
        <button type="button" class="btn btn-secondary" :disabled="saving" data-testid="rules-cancel" @click="close">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="balance-concurrency-form"
          class="btn btn-primary"
          :disabled="!loaded || loading || saving || invalid"
          data-testid="rules-save"
        >
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'

interface DraftTier {
  min_balance: number | string
  concurrency: number | string
}

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ (event: 'close'): void; (event: 'success'): void }>()
const { t } = useI18n()
const appStore = useAppStore()
const rows = ref<DraftTier[]>([])
const loading = ref(false)
const loaded = ref(false)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')
let loadController: AbortController | undefined

function validateRow(row: DraftTier, index: number, tiers: DraftTier[]) {
  let balance = ''
  if (typeof row.min_balance !== 'number' || !Number.isFinite(row.min_balance) || row.min_balance < 0 || row.min_balance > 1e12) {
    balance = t('admin.users.concurrencyRules.validation.balance')
  } else if (index === 0 ? row.min_balance !== 0 : row.min_balance <= Number(tiers[index - 1].min_balance)) {
    balance = t('admin.users.concurrencyRules.validation.order')
  }
  const concurrency = typeof row.concurrency !== 'number' || !Number.isInteger(row.concurrency) || row.concurrency < 1 || row.concurrency > 1000
    ? t('admin.users.concurrencyRules.validation.concurrency')
    : ''
  return { balance, concurrency }
}

const rowErrors = computed(() => rows.value.map(validateRow))
const invalid = computed(() => rows.value.length < 1 || rows.value.length > 20 || rowErrors.value.some(error => error.balance || error.concurrency))

function upperBound(index: number): string {
  const next = rows.value[index + 1]
  if (!next) return t('admin.users.concurrencyRules.noUpperBound')
  if (rowErrors.value[index + 1].balance) return t('admin.users.concurrencyRules.pendingBound')
  return `$${next.min_balance}`
}

function errorMessage(error: unknown, fallback: string): string {
  // The shared client normalizes HTTP and network failures to { message }.
  return error && typeof error === 'object' && 'message' in error && typeof error.message === 'string' && error.message
    ? `${fallback} ${error.message}`
    : fallback
}

async function load() {
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  loading.value = true
  loaded.value = false
  loadError.value = ''
  saveError.value = ''
  rows.value = []
  try {
    const rules = await adminAPI.users.getConcurrencyRules(controller.signal)
    if (controller.signal.aborted) return
    const tiers = rules.balance_tiers
    if (!Array.isArray(tiers) || tiers.length < 1 || tiers.length > 20 || tiers.some((row, index) => !row || Object.values(validateRow(row, index, tiers)).some(Boolean))) {
      throw new Error(t('admin.users.concurrencyRules.validation.summary'))
    }
    rows.value = tiers.map(row => ({ min_balance: row.min_balance, concurrency: row.concurrency }))
    loaded.value = true
  } catch (error) {
    if (!controller.signal.aborted) loadError.value = errorMessage(error, t('admin.users.concurrencyRules.loadFailed'))
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}

watch(() => props.show, show => {
  if (show) void load()
  else loadController?.abort()
}, { immediate: true })
onBeforeUnmount(() => loadController?.abort())

function close() {
  if (!saving.value) emit('close')
}

function addTier() {
  if (!saving.value && loaded.value && rows.value.length < 20) rows.value.push({ min_balance: '', concurrency: 1 })
}

function removeTier(index: number) {
  if (!saving.value && index > 0) rows.value.splice(index, 1)
}

async function save() {
  if (!props.show || !loaded.value || loading.value || saving.value || invalid.value) return
  saving.value = true
  saveError.value = ''
  try {
    await adminAPI.users.updateConcurrencyRules({
      balance_tiers: rows.value.map(row => ({ min_balance: Number(row.min_balance), concurrency: Number(row.concurrency) }))
    })
    appStore.showSuccess(t('admin.users.concurrencyRules.saved'))
    emit('success')
    emit('close')
  } catch (error) {
    saveError.value = errorMessage(error, t('admin.users.concurrencyRules.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>
