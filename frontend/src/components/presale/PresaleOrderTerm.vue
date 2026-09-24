<template>
  <div v-if="order.presale_starts_at" class="rounded-lg border border-primary-200 bg-primary-50/40 p-4 text-left dark:border-primary-800 dark:bg-primary-950/20">
    <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="text-sm font-medium text-content-primary">{{ order.presale_plan_name || t('presale.nav') }}</h3><span class="text-xs text-primary-600 dark:text-primary-300">{{ t(`presale.${presaleStatus(order)}`) }}</span></div>
    <p v-if="order.presale_renewal" class="mt-2 text-xs text-content-secondary">{{ t('presale.renewal') }}</p>
    <p class="mt-3 text-xs leading-6 text-content-secondary">{{ t('presale.termCopy', { start: date(order.presale_starts_at), end: date(order.presale_expires_at) }) }}</p>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import { formatPresaleDate, presaleStatus } from '@/utils/presale'
defineProps<{ order: PaymentOrder }>()
const { t, locale } = useI18n()
const date = (value?: string) => formatPresaleDate(value, locale.value)
</script>
