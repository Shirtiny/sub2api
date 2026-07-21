<template>
  <BaseDialog
    :show="show"
    :title="dialogTitle"
    width="full"
    @close="emit('close')"
  >
    <!-- ==================== Overview ==================== -->
    <div v-if="view === 'overview'" class="space-y-5">
      <div v-if="statsError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-500/30 dark:bg-rose-500/10 dark:text-rose-200">
        {{ statsError }}
      </div>

      <!-- (a) Summary cards -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.stats.remainingToday') }}
          </p>
          <p class="mt-1 text-2xl font-semibold tabular-nums text-content-primary" data-test="card-today">
            {{ statsLoading ? '—' : formatUsd(stats?.totals.remaining_today_usd) }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.stats.dailyLimitedCount', { count: stats?.totals.daily_limited_subscriptions ?? 0 }) }}
          </p>
        </div>

        <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.stats.remainingWeek') }}
          </p>
          <p class="mt-1 text-2xl font-semibold tabular-nums text-content-primary" data-test="card-week">
            {{ statsLoading ? '—' : formatUsd(stats?.totals.remaining_week_usd) }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.stats.weeklyLimitedCount', { count: stats?.totals.weekly_limited_subscriptions ?? 0 }) }}
          </p>
        </div>

        <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center justify-between gap-2">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.subscriptions.stats.horizonCapacity') }}
            </p>
            <div class="w-24 flex-shrink-0">
              <Select
                v-model="horizonDays"
                :options="horizonOptions"
                @change="handleHorizonChange"
              />
            </div>
          </div>
          <p class="mt-1 text-2xl font-semibold tabular-nums text-content-primary" data-test="card-horizon">
            {{ statsLoading ? '—' : formatUsd(stats?.totals.horizon_capacity_usd) }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.stats.totalSubscriptions', { count: stats?.totals.active_subscriptions ?? 0 }) }}
          </p>
        </div>
      </div>

      <!-- (b) Per-plan breakdown -->
      <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="border-b border-gray-200 bg-gray-50 px-4 py-2 dark:border-dark-700 dark:bg-dark-700/50">
          <h4 class="text-sm font-semibold text-gray-700 dark:text-gray-200">
            {{ t('admin.subscriptions.stats.byPlan') }}
          </h4>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2 text-left font-medium">{{ t('admin.subscriptions.stats.colPlan') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.subscriptions.stats.colSubscriptions') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.subscriptions.stats.colRemainingToday') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.subscriptions.stats.colRemainingWeek') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.subscriptions.stats.colRemainingMonth') }}</th>
                <th class="px-3 py-2 text-right font-medium">
                  {{ t('admin.subscriptions.stats.colHorizon', { days: stats?.horizon_days ?? horizonDays }) }}
                </th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.subscriptions.stats.colUsed') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.subscriptions.stats.colUsageRatio') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="statsLoading">
                <td colspan="8" class="px-3 py-6 text-center text-gray-500 dark:text-gray-400">
                  {{ t('common.loading') }}
                </td>
              </tr>
              <tr v-else-if="plans.length === 0">
                <td colspan="8" class="px-3 py-6 text-center text-gray-500 dark:text-gray-400">
                  {{ t('admin.subscriptions.stats.noData') }}
                </td>
              </tr>
              <tr
                v-for="plan in plans"
                v-else
                :key="plan.group_id"
                class="border-b border-stroke-subtle text-content-secondary last:border-0"
              >
                <td class="px-3 py-2">
                  <span class="font-medium text-content-primary">{{ plan.group_name }}</span>
                </td>
                <td class="px-3 py-2 text-right tabular-nums">{{ plan.subscriptions }}</td>
                <td class="px-3 py-2 text-right tabular-nums">
                  {{ plan.daily_limit_usd > 0 ? formatUsd(plan.remaining_today_usd) : '—' }}
                </td>
                <td class="px-3 py-2 text-right tabular-nums">
                  {{ plan.weekly_limit_usd > 0 ? formatUsd(plan.remaining_week_usd) : '—' }}
                </td>
                <td class="px-3 py-2 text-right tabular-nums">
                  {{ plan.monthly_limit_usd > 0 ? formatUsd(plan.remaining_month_usd) : '—' }}
                </td>
                <td class="px-3 py-2 text-right tabular-nums">{{ formatUsd(plan.horizon_capacity_usd) }}</td>
                <td class="px-3 py-2 text-right tabular-nums">{{ formatUsd(plan.used_usd) }}</td>
                <td class="px-3 py-2 text-right tabular-nums">{{ formatRatio(plan.usage_ratio) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- (c) Usage ranking -->
      <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="flex items-center gap-2 border-b border-gray-200 bg-gray-50 px-4 py-2 dark:border-dark-700 dark:bg-dark-700/50">
          <h4 class="mr-2 text-sm font-semibold text-gray-700 dark:text-gray-200">
            {{ t('admin.subscriptions.stats.ranking') }}
          </h4>
          <button
            v-for="tab in rankingTabs"
            :key="tab"
            type="button"
            :data-test="`ranking-tab-${tab}`"
            :class="[
              'rounded-lg px-3 py-1 text-xs font-medium transition-colors',
              rankingTab === tab
                ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300'
                : 'text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700'
            ]"
            @click="rankingTab = tab"
          >
            {{ t(`admin.subscriptions.stats.rankingTab.${tab}`) }}
          </button>
        </div>

        <div v-if="statsLoading" class="px-3 py-6 text-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('common.loading') }}
        </div>
        <div
          v-else-if="rankingRows.length === 0"
          class="px-3 py-6 text-center text-sm text-gray-500 dark:text-gray-400"
        >
          {{ t('admin.subscriptions.stats.noData') }}
        </div>
        <ul v-else class="divide-y divide-stroke-subtle">
          <li v-for="item in rankingRows" :key="item.subscription_id">
            <button
              type="button"
              data-test="ranking-row"
              class="w-full px-3 py-2.5 text-left transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/50"
              @click="openDetail(item)"
            >
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div class="min-w-0">
                  <span class="text-sm font-medium text-content-primary">
                    {{ item.username || item.email || `#${item.user_id}` }}
                  </span>
                  <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">{{ item.group_name }}</span>
                </div>
                <div class="flex items-center gap-3 text-xs tabular-nums text-content-secondary">
                  <span>{{ formatUsd(item.used_usd) }} / {{ formatUsd(item.limit_usd) }}</span>
                  <span class="text-gray-500 dark:text-gray-400">
                    {{ t('admin.subscriptions.stats.remainingShort') }} {{ formatUsd(item.remaining_usd) }}
                  </span>
                  <span class="w-12 text-right font-semibold">{{ formatRatio(item.usage_ratio) }}</span>
                </div>
              </div>
              <div class="mt-1.5 flex items-center gap-2">
                <div class="h-1.5 min-w-0 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    class="h-1.5 rounded-full transition-all"
                    :class="getRatioBarClass(item.usage_ratio)"
                    :style="{ width: getRatioWidth(item.usage_ratio) }"
                  ></div>
                </div>
                <span class="w-32 flex-shrink-0 text-right text-[10px] text-blue-600 dark:text-blue-400">
                  {{ formatResetCountdown(item.window_resets_at) }}
                </span>
              </div>
            </button>
          </li>
        </ul>
      </div>
    </div>

    <!-- ==================== Detail (usage series) ==================== -->
    <div v-else class="space-y-5">
      <button
        type="button"
        class="btn btn-secondary btn-sm"
        data-test="detail-back"
        @click="backToOverview"
      >
        <Icon name="arrowLeft" size="sm" class="mr-1" />
        {{ t('admin.subscriptions.stats.back') }}
      </button>

      <div v-if="seriesError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-500/30 dark:bg-rose-500/10 dark:text-rose-200">
        {{ seriesError }}
      </div>

      <div v-if="seriesLoading" class="px-3 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>

      <template v-else-if="series">
        <div>
          <h3 class="text-base font-semibold text-content-primary">
            {{ series.username || `#${series.user_id}` }}
            <span class="ml-2 text-sm font-normal text-gray-500 dark:text-gray-400">{{ series.group_name }}</span>
          </h3>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ formatDateOnly(series.starts_at) }} → {{ formatDateOnly(series.expires_at) }}
          </p>
        </div>

        <!-- History truncation warning -->
        <div
          v-if="!series.data_complete"
          data-test="incomplete-warning"
          class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200"
        >
          {{ series.data_from
            ? t('admin.subscriptions.stats.historyTruncated', { date: series.data_from })
            : t('admin.subscriptions.stats.historyEmpty') }}
        </div>

        <!-- Whole cycle. 只有「分组完全没配置任何窗口限额」时后端才回 null；
             有限额但零用量的订阅 cost_usd=0、使用率 0%，必须照常渲染。 -->
        <div
          v-if="series.cycle !== null"
          class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"
        >
          <h4 class="mb-3 text-sm font-semibold text-gray-700 dark:text-gray-200">
            {{ t('admin.subscriptions.stats.cycleUsage') }}
          </h4>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.stats.cycleCost') }}</p>
              <p class="mt-0.5 text-lg font-semibold tabular-nums text-content-primary">
                {{ formatUsd(series.cycle.cost_usd) }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.stats.cycleQuota') }}</p>
              <p class="mt-0.5 text-lg font-semibold tabular-nums text-content-primary">
                {{ formatUsd(series.cycle.quota_usd) }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.stats.colUsageRatio') }}</p>
              <p class="mt-0.5 text-lg font-semibold tabular-nums text-content-primary">
                {{ formatRatio(series.cycle.usage_ratio) }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.stats.windowsElapsed') }}</p>
              <p class="mt-0.5 text-lg font-semibold tabular-nums text-content-primary">
                {{ series.cycle.windows_elapsed }}
                <span class="text-xs font-normal text-gray-500 dark:text-gray-400">
                  {{ t(`admin.subscriptions.stats.windowKind.${series.cycle.window_kind}`) }}
                </span>
              </p>
            </div>
          </div>
          <div class="mt-3 h-2 rounded-full bg-gray-200 dark:bg-dark-600">
            <div
              class="h-2 rounded-full transition-all"
              :class="getRatioBarClass(series.cycle.usage_ratio)"
              :style="{ width: getRatioWidth(series.cycle.usage_ratio) }"
            ></div>
          </div>
        </div>

        <!-- Daily usage bars -->
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-700 dark:text-gray-200">
            {{ t('admin.subscriptions.stats.dailyUsage') }}
          </h4>
          <p v-if="series.daily.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.stats.noData') }}
          </p>
          <div v-else class="space-y-1.5">
            <div
              v-for="point in series.daily"
              :key="point.date"
              data-test="daily-point"
              class="flex items-center gap-2"
            >
              <span class="w-20 flex-shrink-0 text-xs tabular-nums text-gray-500 dark:text-gray-400">
                {{ point.date }}
              </span>
              <div class="h-2.5 min-w-0 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="h-2.5 rounded-full transition-all"
                  :class="getRatioBarClass(point.usage_ratio)"
                  :style="{ width: getRatioWidth(point.usage_ratio) }"
                ></div>
              </div>
              <span class="w-40 flex-shrink-0 text-right text-xs tabular-nums text-content-secondary">
                {{ formatUsd(point.cost_usd) }} / {{ formatUsd(point.limit_usd) }}
                <span
                  v-if="point.limit_is_derived"
                  data-test="derived-badge"
                  class="ml-1 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400"
                  :title="t('admin.subscriptions.stats.derivedDailyHint')"
                >
                  {{ t('admin.subscriptions.stats.derived') }}
                </span>
              </span>
              <span class="w-12 flex-shrink-0 text-right text-xs font-semibold tabular-nums text-content-primary">
                {{ formatRatio(point.usage_ratio) }}
              </span>
            </div>
          </div>
        </div>

        <!-- Weekly usage -->
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-700 dark:text-gray-200">
            {{ t('admin.subscriptions.stats.weeklyUsage') }}
          </h4>
          <p v-if="series.weekly.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.subscriptions.stats.noData') }}
          </p>
          <div v-else class="space-y-1.5">
            <div
              v-for="point in series.weekly"
              :key="point.week_start"
              data-test="weekly-point"
              class="flex items-center gap-2"
            >
              <span class="w-40 flex-shrink-0 text-xs tabular-nums text-gray-500 dark:text-gray-400">
                {{ point.week_start }} → {{ point.week_end }}
              </span>
              <div class="h-2.5 min-w-0 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="h-2.5 rounded-full transition-all"
                  :class="getRatioBarClass(point.usage_ratio)"
                  :style="{ width: getRatioWidth(point.usage_ratio) }"
                ></div>
              </div>
              <span class="w-40 flex-shrink-0 text-right text-xs tabular-nums text-content-secondary">
                {{ formatUsd(point.cost_usd) }} / {{ formatUsd(point.limit_usd) }}
                <span
                  v-if="point.limit_is_derived"
                  data-test="derived-badge"
                  class="ml-1 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400"
                  :title="t('admin.subscriptions.stats.derivedWeeklyHint')"
                >
                  {{ t('admin.subscriptions.stats.derived') }}
                </span>
              </span>
              <span class="w-12 flex-shrink-0 text-right text-xs font-semibold tabular-nums text-content-primary">
                {{ formatRatio(point.usage_ratio) }}
              </span>
            </div>
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex items-center justify-between gap-3">
        <span v-if="stats" class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.subscriptions.stats.generatedAt', { time: formatDateTime(stats.generated_at) }) }}
        </span>
        <span v-else></span>
        <div class="flex gap-3">
          <button
            v-if="view === 'overview'"
            type="button"
            class="btn btn-secondary"
            :disabled="statsLoading"
            @click="loadStats"
          >
            {{ t('common.refresh') }}
          </button>
          <button type="button" class="btn btn-primary" @click="emit('close')">
            {{ t('common.close') }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  SubscriptionStats,
  SubscriptionStatsPlan,
  SubscriptionStatsRankingItem,
  SubscriptionUsageSeries
} from '@/types'
import { formatDateOnly, formatDateTime } from '@/utils/format'
import { getRemainingDurationParts, ratioToneClass } from '@/utils/subscriptionQuota'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()

type RankingTab = 'daily' | 'weekly'
type View = 'overview' | 'detail'

const rankingTabs: readonly RankingTab[] = ['daily', 'weekly'] as const

const view = ref<View>('overview')
const horizonDays = ref<number>(7)
const rankingTab = ref<RankingTab>('weekly')

const stats = ref<SubscriptionStats | null>(null)
const statsLoading = ref(false)
const statsError = ref('')

const series = ref<SubscriptionUsageSeries | null>(null)
const seriesLoading = ref(false)
const seriesError = ref('')

let statsController: AbortController | null = null
let seriesController: AbortController | null = null

const horizonOptions = computed(() =>
  [1, 3, 7, 14, 30].map((days) => ({
    value: days,
    label: t('admin.subscriptions.stats.horizonOption', { days })
  }))
)

const dialogTitle = computed(() =>
  view.value === 'overview'
    ? t('admin.subscriptions.stats.title')
    : t('admin.subscriptions.stats.detailTitle')
)

const plans = computed<SubscriptionStatsPlan[]>(() => stats.value?.plans ?? [])

// 后端已按 usage_ratio 降序排好，这里只做取用，不重复排序。
const rankingRows = computed<SubscriptionStatsRankingItem[]>(() =>
  rankingTab.value === 'daily' ? (stats.value?.ranking.daily ?? []) : (stats.value?.ranking.weekly ?? [])
)

const formatUsd = (value: number | null | undefined): string => `$${(value ?? 0).toFixed(2)}`

// 使用率可以超过 100%（管理员中途重置配额后，汇总表实际花费会高于窗口计费额），
// 这里刻意不 clamp，数字必须显示真实值。
const formatRatio = (ratio: number | null | undefined): string =>
  `${Math.round((ratio ?? 0) * 100)}%`

const getRatioBarClass = ratioToneClass

// 只有条宽 clamp 到 [0,100]，避免超额时把容器撑破。
const getRatioWidth = (ratio: number | null | undefined): string => {
  const percentage = Math.min(Math.max((ratio ?? 0) * 100, 0), 100)
  return `${percentage}%`
}

// window_resets_at 为 null 表示窗口已过期、但还没被下一次请求惰性重置，
// 后端刻意不回报陈旧起点，这里显示占位而不是 NaN 或负数倒计时。
const formatResetCountdown = (resetsAt: string | null): string => {
  if (!resetsAt) return t('admin.subscriptions.stats.pendingReset')
  const parts = getRemainingDurationParts(resetsAt)
  if (!parts) return t('admin.subscriptions.stats.pendingReset')
  if (parts.days > 0) {
    return t('admin.subscriptions.resetInDaysHours', { days: parts.days, hours: parts.hours })
  }
  if (parts.hours > 0) {
    return t('admin.subscriptions.resetInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }
  return t('admin.subscriptions.resetInMinutes', { minutes: parts.minutes })
}

const isCanceled = (error: unknown): boolean => {
  const err = error as { name?: string; code?: string } | null
  return err?.name === 'AbortError' || err?.code === 'ERR_CANCELED'
}

const loadStats = async () => {
  statsController?.abort()
  const controller = new AbortController()
  statsController = controller

  statsLoading.value = true
  statsError.value = ''
  try {
    const result = await adminAPI.subscriptions.getStats(
      { horizon_days: horizonDays.value },
      { signal: controller.signal }
    )
    if (statsController !== controller) return
    stats.value = result
  } catch (error: unknown) {
    if (isCanceled(error) || statsController !== controller) return
    statsError.value = t('admin.subscriptions.stats.loadFailed')
    console.error('Error loading subscription stats:', error)
  } finally {
    if (statsController === controller) {
      statsLoading.value = false
      statsController = null
    }
  }
}

const loadSeries = async (subscriptionId: number) => {
  seriesController?.abort()
  const controller = new AbortController()
  seriesController = controller

  seriesLoading.value = true
  seriesError.value = ''
  try {
    const result = await adminAPI.subscriptions.getUsageSeries(subscriptionId, {
      signal: controller.signal
    })
    if (seriesController !== controller) return
    series.value = result
  } catch (error: unknown) {
    if (isCanceled(error) || seriesController !== controller) return
    seriesError.value = t('admin.subscriptions.stats.seriesLoadFailed')
    console.error('Error loading subscription usage series:', error)
  } finally {
    if (seriesController === controller) {
      seriesLoading.value = false
      seriesController = null
    }
  }
}

const handleHorizonChange = () => {
  loadStats()
}

const openDetail = (item: SubscriptionStatsRankingItem) => {
  view.value = 'detail'
  series.value = null
  loadSeries(item.subscription_id)
}

const backToOverview = () => {
  seriesController?.abort()
  seriesController = null
  seriesLoading.value = false
  view.value = 'overview'
  series.value = null
  seriesError.value = ''
}

watch(
  () => props.show,
  (isOpen) => {
    if (isOpen) {
      view.value = 'overview'
      series.value = null
      seriesError.value = ''
      loadStats()
    } else {
      statsController?.abort()
      statsController = null
      seriesController?.abort()
      seriesController = null
      statsLoading.value = false
      seriesLoading.value = false
    }
  },
  { immediate: true }
)

onUnmounted(() => {
  statsController?.abort()
  seriesController?.abort()
})

defineExpose({
  view,
  horizonDays,
  rankingTab,
  stats,
  series,
  loadStats,
  openDetail,
  backToOverview
})
</script>
