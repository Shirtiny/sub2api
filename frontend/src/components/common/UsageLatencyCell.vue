<template>
  <div class="flex items-stretch gap-2">
    <span
      class="w-1 shrink-0 rounded-full"
      :class="latencyBarClass"
      aria-hidden="true"
    ></span>
    <div class="grid grid-cols-[max-content_max-content] items-baseline gap-x-2 gap-y-0.5 text-xs">
      <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyFirstByte') }}</span>
      <span
        v-if="hasValidFirstByte"
        class="font-medium tabular-nums"
        :class="LATENCY_TEXT_CLASSES[firstByteLevel]"
      >
        {{ formatLatencyDuration(firstByteMs) }}
      </span>
      <span v-else class="text-gray-400 dark:text-gray-500">-</span>

      <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyDuration') }}</span>
      <span
        v-if="hasValidDuration"
        class="font-medium tabular-nums"
        :class="LATENCY_TEXT_CLASSES[durationLevel]"
      >
        {{ formatLatencyDuration(durationMs) }}
      </span>
      <span v-else class="text-gray-400 dark:text-gray-500">-</span>

      <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyTps') }}</span>
      <span class="font-medium tabular-nums text-content-secondary">{{ tpsDisplay }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  LATENCY_BAR_CLASSES,
  LATENCY_BAR_FROM_CLASSES,
  LATENCY_BAR_TO_CLASSES,
  LATENCY_TEXT_CLASSES,
  calculateOutputTokensPerSecond,
  durationSeverity,
  firstByteSeverity,
  formatLatencyDuration,
} from '@/utils/latencyHealth'

interface Props {
  firstByteMs?: number | null
  durationMs?: number | null
  outputTokens?: number | null
  stream?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  firstByteMs: null,
  durationMs: null,
  outputTokens: null,
  stream: false,
})

const { t } = useI18n()

const isValidLatencyValue = (value: number | null | undefined): value is number =>
  typeof value === 'number' && Number.isFinite(value) && value >= 0

const hasValidFirstByte = computed(() => isValidLatencyValue(props.firstByteMs))
const hasValidDuration = computed(() => isValidLatencyValue(props.durationMs))
const firstByteLevel = computed(() => firstByteSeverity(hasValidFirstByte.value ? props.firstByteMs! : 0))
const durationLevel = computed(() => durationSeverity(hasValidDuration.value ? props.durationMs! : 0))

const latencyBarClass = computed(() => {
  if (!hasValidDuration.value) return 'bg-gray-300 dark:bg-gray-600'
  if (props.firstByteMs != null && (
    !hasValidFirstByte.value || props.firstByteMs > props.durationMs!
  )) {
    return 'bg-gray-300 dark:bg-gray-600'
  }
  if (!hasValidFirstByte.value) return LATENCY_BAR_CLASSES[durationLevel.value]
  return [
    'bg-gradient-to-b from-40% to-60%',
    LATENCY_BAR_FROM_CLASSES[firstByteLevel.value],
    LATENCY_BAR_TO_CLASSES[durationLevel.value],
  ]
})

const tpsDisplay = computed(() => {
  const tps = calculateOutputTokensPerSecond(
    props.outputTokens,
    props.durationMs,
    props.firstByteMs,
    props.stream,
  )
  return tps == null ? '-' : String(tps)
})
</script>
