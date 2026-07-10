<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import type { CustomEndpoint } from '@/types'

const props = defineProps<{
  apiBaseUrl: string
  customEndpoints: CustomEndpoint[]
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const copiedEndpoint = ref<string | null>(null)
const endpointLatencies = ref<Record<string, number | null>>({})

let copiedResetTimer: number | undefined
let latencyScheduleGeneration = 0
let isMounted = false
const latencyTimers: number[] = []
const activePingControllers = new Set<AbortController>()

const PING_TIMEOUT_MS = 10_000
// Immediate, then 30 seconds later, then 60 seconds after the second check.
const LATENCY_REFRESH_DELAYS_MS = [30_000, 90_000] as const

const allEndpoints = computed(() => {
  const items: Array<{ name: string; endpoint: string; description: string; isDefault: boolean }> = []
  if (props.apiBaseUrl) {
    items.push({
      name: t('keys.endpoints.title'),
      endpoint: props.apiBaseUrl,
      description: '',
      isDefault: true,
    })
  }
  for (const ep of props.customEndpoints) {
    items.push({ ...ep, isDefault: false })
  }
  return items
})

async function copy(url: string) {
  const success = await copyToClipboard(url, t('keys.endpoints.copied'))
  if (!success) return

  copiedEndpoint.value = url
  if (copiedResetTimer !== undefined) {
    window.clearTimeout(copiedResetTimer)
  }
  copiedResetTimer = window.setTimeout(() => {
    if (copiedEndpoint.value === url) {
      copiedEndpoint.value = null
    }
  }, 1800)
}

function tooltipHint(endpoint: string): string {
  return copiedEndpoint.value === endpoint
    ? t('keys.endpoints.copiedHint')
    : t('keys.endpoints.clickToCopy')
}

function buildPingUrl(endpoint: string): string | null {
  try {
    const url = new URL(endpoint, window.location.origin)
    url.pathname = '/v1/ping'
    url.search = ''
    url.searchParams.set('_ts', Date.now().toString())
    url.hash = ''
    return url.toString()
  } catch {
    return null
  }
}

function latencyLabel(endpoint: string): string {
  const latency = endpointLatencies.value[endpoint.trim()]
  if (latency === undefined) return '...ms'
  if (latency === null) return '--ms'
  return `${latency}ms`
}

function latencyTitle(endpoint: string): string {
  const latency = endpointLatencies.value[endpoint.trim()]
  if (latency === undefined) return t('keys.endpoints.checkingLatency')
  if (latency === null) return t('keys.endpoints.latencyUnavailable')
  return `${t('keys.endpoints.latency')}: ${latency}ms`
}

function latencyTextClass(endpoint: string): string {
  const latency = endpointLatencies.value[endpoint.trim()]
  if (typeof latency !== 'number') return 'text-content-tertiary'
  if (latency < 200) return 'text-emerald-600 dark:text-emerald-400'
  if (latency < 500) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

function requestTtfbMs(pingUrl: string, fallbackTtfb: number): number {
  let ttfb = fallbackTtfb
  if (typeof performance.getEntriesByName === 'function') {
    const entries = performance.getEntriesByName(pingUrl, 'resource')
    const entry = entries[entries.length - 1] as PerformanceResourceTiming | undefined
    if (entry && entry.requestStart > 0 && entry.responseStart >= entry.requestStart) {
      ttfb = entry.responseStart - entry.requestStart
    }
  }
  return Math.max(0, Math.round(ttfb))
}

async function measureEndpointLatency(endpoint: string): Promise<number | null> {
  const pingUrl = buildPingUrl(endpoint)
  if (!pingUrl) return null

  const controller = new AbortController()
  activePingControllers.add(controller)
  const timeout = window.setTimeout(() => controller.abort(), PING_TIMEOUT_MS)
  const startedAt = performance.now()
  try {
    const response = await fetch(pingUrl, {
      method: 'GET',
      credentials: 'omit',
      signal: controller.signal,
    })
    const fallbackTtfb = performance.now() - startedAt
    if (!response.ok) return null

    await response.json()
    return requestTtfbMs(pingUrl, fallbackTtfb)
  } catch {
    return null
  } finally {
    window.clearTimeout(timeout)
    activePingControllers.delete(controller)
  }
}

async function refreshEndpointLatencies(generation: number) {
  const endpoints = [...new Set(
    allEndpoints.value
      .map((item) => item.endpoint.trim())
      .filter(Boolean),
  )]
  const results = await Promise.all(endpoints.map(async (endpoint) => (
    [endpoint, await measureEndpointLatency(endpoint)] as const
  )))

  if (!isMounted || generation !== latencyScheduleGeneration) return
  endpointLatencies.value = Object.fromEntries(results)
}

function clearLatencySchedule() {
  for (const timer of latencyTimers.splice(0)) {
    window.clearTimeout(timer)
  }
  for (const controller of activePingControllers) {
    controller.abort()
  }
  activePingControllers.clear()
}

function startLatencySchedule() {
  clearLatencySchedule()
  endpointLatencies.value = {}
  const generation = ++latencyScheduleGeneration

  void refreshEndpointLatencies(generation)
  for (const delay of LATENCY_REFRESH_DELAYS_MS) {
    latencyTimers.push(window.setTimeout(() => {
      void refreshEndpointLatencies(generation)
    }, delay))
  }
}

const endpointSignature = computed(() => allEndpoints.value
  .map((item) => item.endpoint.trim())
  .join('\u0000'))

onMounted(() => {
  isMounted = true
  startLatencySchedule()
})

watch(endpointSignature, () => {
  if (isMounted) startLatencySchedule()
})

onBeforeUnmount(() => {
  isMounted = false
  latencyScheduleGeneration += 1
  clearLatencySchedule()
  if (copiedResetTimer !== undefined) {
    window.clearTimeout(copiedResetTimer)
  }
})
</script>

<template>
  <div v-if="allEndpoints.length > 0" class="flex flex-wrap gap-2">
    <div
      v-for="(item, index) in allEndpoints"
      :key="index"
      class="flex items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs transition-colors hover:border-primary-200 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-700"
    >
      <span class="font-medium text-gray-600 dark:text-gray-300">{{ item.name }}</span>
      <span
        v-if="item.isDefault"
        class="rounded bg-primary-50 px-1 py-px text-[10px] font-medium leading-tight text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
      >{{ t('keys.endpoints.default') }}</span>

      <span class="text-gray-300 dark:text-dark-500">|</span>

      <div class="group/endpoint relative flex items-center gap-1.5">
        <div
          role="tooltip"
          class="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 w-max max-w-[24rem] -translate-x-1/2 translate-y-1 rounded-lg border border-stroke-default bg-surface-card px-3 py-2.5 text-left opacity-0 shadow-xl ring-1 ring-stroke-subtle/70 transition-all duration-150 group-hover/endpoint:translate-y-0 group-hover/endpoint:opacity-100 group-focus-within/endpoint:translate-y-0 group-focus-within/endpoint:opacity-100"
        >
          <p
            v-if="item.description"
            class="max-w-[24rem] break-words text-xs leading-5 text-content-secondary"
          >
            {{ item.description }}
          </p>
          <p
            class="flex items-center gap-1.5 text-[11px] font-medium leading-4 text-content-primary"
            :class="item.description ? 'mt-1.5' : ''"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-primary-500 dark:bg-primary-600"></span>
            {{ tooltipHint(item.endpoint) }}
          </p>
          <div class="absolute left-1/2 top-full h-3 w-3 -translate-x-1/2 -translate-y-1/2 rotate-45 border-b border-r border-stroke-default bg-surface-card"></div>
        </div>

        <code
          class="cursor-pointer font-mono text-gray-500 decoration-gray-400 decoration-dashed underline-offset-2 hover:text-primary-600 hover:underline focus:text-primary-600 focus:underline focus:outline-none dark:text-gray-400 dark:decoration-gray-500 dark:hover:text-primary-400 dark:focus:text-primary-400"
          role="button"
          tabindex="0"
          @click="copy(item.endpoint)"
          @keydown.enter.prevent="copy(item.endpoint)"
          @keydown.space.prevent="copy(item.endpoint)"
        >{{ item.endpoint }}</code>

        <button
          type="button"
          class="rounded p-0.5 transition-colors"
          :class="copiedEndpoint === item.endpoint
            ? 'text-primary-500'
            : 'text-gray-400 hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400'"
          :aria-label="tooltipHint(item.endpoint)"
          @click="copy(item.endpoint)"
        >
          <svg v-if="copiedEndpoint === item.endpoint" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
          </svg>
          <svg v-else class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
        </button>

        <span
          class="shrink-0 whitespace-nowrap font-mono text-[11px] tabular-nums"
          :class="latencyTextClass(item.endpoint)"
          :title="latencyTitle(item.endpoint)"
          aria-live="polite"
        >
          {{ latencyLabel(item.endpoint) }}
        </span>
      </div>
    </div>
  </div>
</template>
