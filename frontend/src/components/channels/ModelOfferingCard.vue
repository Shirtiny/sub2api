<template>
  <article class="model-offering">
    <header class="model-topline">
      <span class="model-kind">{{ modeLabel }}</span>
      <span v-if="model.pricing" class="model-rate" :title="t('availableChannels.catalog.multiplierHint')">
        {{ multiplier }}×
      </span>
    </header>

    <button class="model-copy" type="button" :aria-label="t('availableChannels.catalog.copyModel', { model: model.name })" @click="copyToClipboard(model.name)">
      <code>{{ model.name }}</code>
      <Icon :name="copied ? 'check' : 'copy'" size="md" class="shrink-0" />
    </button>

    <template v-if="model.pricing">
      <div class="price-heading">
        <span>{{ t('availableChannels.catalog.modelPrice') }}</span>
        <span>{{ tokenMode ? 'USD / 1M tokens' : t('availableChannels.catalog.perRequestUnit') }}</span>
      </div>
      <dl class="main-prices">
        <div v-for="field in primaryFields" :key="field.key">
          <dt>{{ field.label }}</dt>
          <dd>{{ amount(model.pricing[field.key]) }}</dd>
          <span class="list-price">
            <template v-if="multiplier !== 1 && model.pricing[field.key] != null">
              {{ t('availableChannels.catalog.listPrice') }} <s>{{ amount(model.pricing[field.key], false) }}</s>
            </template>
          </span>
        </div>
      </dl>

      <button
        v-if="tokenMode || model.pricing.intervals.length"
        type="button" class="price-details" aria-haspopup="dialog"
        :aria-expanded="pricingOpen" @click="pricingOpen = true"
      >
        {{ t('availableChannels.catalog.priceDetails') }}
        <Icon name="chevronRight" size="sm" />
      </button>

      <BaseDialog
        :show="pricingOpen" :title="t('availableChannels.catalog.pricingTitle')"
        :width="model.pricing.intervals.length ? 'wide' : 'normal'"
        close-on-click-outside @close="pricingOpen = false"
      >
        <div class="pricing-sheet">
          <header class="pricing-sheet-heading">
            <code>{{ model.name }}</code>
            <span class="pricing-sheet-rate" :title="t('availableChannels.catalog.multiplierHint')">{{ multiplier }}×</span>
          </header>
          <p class="pricing-sheet-unit">{{ tokenMode ? 'USD / 1M tokens' : t('availableChannels.catalog.perRequestUnit') }}</p>
          <dl v-if="tokenMode" class="cache-prices">
            <div v-for="field in extraFields" :key="field.key">
              <dt>{{ field.label }}</dt>
              <dd>{{ amount(model.pricing[field.key]) }}</dd>
            </div>
          </dl>
          <section v-if="model.pricing.intervals.length" class="tier-section">
            <h4>{{ t('availableChannels.pricing.intervals') }}</h4>
            <div class="tier-table-scroll" tabindex="0" role="region" :aria-label="t('availableChannels.pricing.intervals')">
              <table class="tier-table" :class="{ 'tier-table-token': tokenMode }">
                <thead><tr>
                  <th scope="col">{{ t('availableChannels.catalog.contextTier') }}</th>
                  <th v-for="field in intervalFields" :key="field.key" scope="col">{{ field.label }}</th>
                </tr></thead>
                <tbody>
                  <tr>
                    <th scope="row">{{ t('availableChannels.catalog.defaultPrice') }}</th>
                    <td v-for="field in intervalFields" :key="field.key">{{ amount(model.pricing[field.key]) }}</td>
                  </tr>
                  <tr v-for="(interval, index) in model.pricing.intervals" :key="index">
                    <th scope="row">{{ interval.tier_label || contextRange(interval.min_tokens, interval.max_tokens) }}</th>
                    <td v-for="field in intervalFields" :key="field.key">{{ amount(interval[field.key]) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </div>
      </BaseDialog>
    </template>
    <p v-else class="unpriced">{{ t('availableChannels.noPricing') }}</p>
  </article>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserSupportedModel } from '@/api/channels'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useClipboard } from '@/composables/useClipboard'
import { formatScaled } from '@/utils/pricing'

const props = defineProps<{ model: UserSupportedModel }>()
const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()
const pricingOpen = ref(false)
const multiplier = computed(() => props.model.pricing?.price_multiplier ?? 1)
const tokenMode = computed(() => !props.model.pricing?.billing_mode || props.model.pricing.billing_mode === 'token')
const modeLabel = computed(() => t(`availableChannels.pricing.${tokenMode.value ? 'billingModeToken' : props.model.pricing?.billing_mode === 'image' ? 'billingModeImage' : 'billingModePerRequest'}`))
type PriceKey = 'input_price' | 'output_price' | 'cache_write_price' | 'cache_read_price' | 'per_request_price'
const field = (key: PriceKey, label: string) => ({ key, label: t(`availableChannels.pricing.${label}`) })
const primaryFields = computed(() => tokenMode.value
  ? [field('input_price', 'inputPrice'), field('output_price', 'outputPrice')]
  : [field('per_request_price', 'perRequestPrice')])
const extraFields = computed(() => [
  field('cache_read_price', 'cacheReadPrice'), field('cache_write_price', 'cacheWritePrice'),
  { key: 'image_output_price' as const, label: t('availableChannels.pricing.imageOutputPrice') }
])
const intervalFields = computed<{ key: PriceKey; label: string }[]>(() => tokenMode.value
  ? [...primaryFields.value, field('cache_read_price', 'cacheReadPrice'), field('cache_write_price', 'cacheWritePrice')]
  : primaryFields.value)
function contextRange(min: number, max: number | null): string {
  // Keep exact boundaries rather than rounding pricing thresholds.
  const compact = (tokens: number) => tokens >= 1000 ? `${tokens / 1000}k` : String(tokens)
  if (max == null) {
    return min === 0
      ? t('availableChannels.catalog.contextAny')
      : t('availableChannels.catalog.contextAbove', { min: compact(min) })
  }
  return min === 0
    ? t('availableChannels.catalog.contextUpTo', { max: compact(max) })
    : t('availableChannels.catalog.contextBetween', { min: compact(min), max: compact(max) })
}
function amount(value: number | null, effective = true): string {
  if (value == null) return '—'
  return formatScaled(value, (tokenMode.value ? 1_000_000 : 1) * (effective ? multiplier.value : 1))
}
</script>

<style scoped>
.model-offering { min-width: 0; padding: 26px; border: 1px solid var(--offering-line); border-radius: 20px; background: var(--offering-card); transition: border-color .25s, box-shadow .25s, transform .25s; }
.model-offering:hover { border-color: var(--offering-accent); box-shadow: 0 12px 34px #0000000b; transform: translateY(-2px); }
.model-topline, .price-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.model-topline { min-height: 26px; }
.model-kind, .price-heading, .list-price { font-size: 11px; color: var(--offering-muted); }
.model-kind { letter-spacing: .08em; }
.model-rate { padding: 4px 10px; border-radius: 999px; background: var(--offering-tint); border: 1px solid var(--offering-line); font-size: 12px; font-variant-numeric: tabular-nums; color: var(--offering-accent); }
.model-copy { display: flex; width: 100%; gap: 18px; align-items: center; justify-content: space-between; padding: 16px 0 8px; text-align: left; color: var(--offering-ink); }
.model-copy code { font-size: clamp(16px, 1.3vw, 20px); line-height: 1.5; overflow-wrap: anywhere; }
.model-copy svg { color: var(--offering-muted); transition: color .2s; }
.model-copy:hover svg, .model-copy:hover code { color: var(--offering-accent); }
.model-copy:focus-visible, .price-details:focus-visible { outline: 2px solid var(--offering-accent); outline-offset: 4px; border-radius: 4px; }
.price-heading { border-top: 1px solid var(--offering-line); margin-top: 24px; padding-top: 20px; flex-wrap: wrap; gap: 6px; }
.main-prices { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; margin: 16px 0 20px; }
.main-prices dt { font-size: 12px; color: var(--offering-muted); }
.main-prices dd { margin: 6px 0 3px; font-size: 28px; font-family: Georgia, serif; font-variant-numeric: lining-nums tabular-nums; overflow-wrap: anywhere; }
.list-price { display: block; min-height: 16px; }
.list-price s { margin-left: 4px; }
.price-details { display: flex; width: 100%; align-items: center; justify-content: space-between; gap: 16px; padding-top: 16px; border-top: 1px solid var(--offering-line); font-size: 12px; text-align: left; color: var(--offering-muted); }
.price-details:hover { color: var(--offering-accent); }
.pricing-sheet { --price-line: #e4d9ca; --price-panel: #f8f3eb; --price-ink: #382a20; --price-muted: #756456; --price-accent: #865630; color: var(--price-ink); }
.dark .pricing-sheet { --price-line: #43372c; --price-panel: #27211b; --price-ink: #f2e9dc; --price-muted: #b8a99a; --price-accent: #d8ad7d; }
.pricing-sheet-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.pricing-sheet-heading code { min-width: 0; overflow-wrap: anywhere; font-size: 16px; }
.pricing-sheet-rate { flex-shrink: 0; padding: 4px 10px; border: 1px solid var(--price-line); border-radius: 999px; font-size: 12px; color: var(--price-accent); }
.pricing-sheet-unit { margin: 8px 0 22px; font-size: 11px; color: var(--price-muted); }
.cache-prices { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border: 1px solid var(--price-line); border-radius: 12px; background: var(--price-panel); }
.cache-prices > div { min-width: 0; padding: 16px 12px; }
.cache-prices > div + div { border-left: 1px solid var(--price-line); }
.cache-prices dt { font-size: 11px; color: var(--price-muted); }
.cache-prices dd { margin-top: 8px; font-size: 17px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.tier-section { margin-top: 24px; }
.tier-section h4 { margin-bottom: 12px; font-size: 12px; font-weight: 500; }
.tier-table-scroll { max-height: min(44vh, 360px); overflow: auto; overscroll-behavior: contain; border: 1px solid var(--price-line); border-radius: 12px; }
.tier-table-scroll:focus-visible { outline: 2px solid var(--price-accent); outline-offset: 3px; }
.tier-table { width: 100%; border-collapse: separate; border-spacing: 0; font-size: 12px; font-variant-numeric: tabular-nums; }
.tier-table-token { min-width: 560px; }
.tier-table th, .tier-table td { padding: 13px 14px; text-align: right; white-space: nowrap; }
.tier-table thead th { position: sticky; top: 0; z-index: 1; background: var(--price-panel); border-bottom: 1px solid var(--price-line); font-size: 11px; font-weight: 400; color: var(--price-muted); }
.tier-table th:first-child { text-align: left; }
.tier-table tbody th { font-weight: 400; color: var(--price-muted); }
.tier-table tbody tr + tr > * { border-top: 1px solid var(--price-line); }
.tier-table tbody tr:hover { background: var(--price-panel); }
.unpriced { padding: 40px 0 20px; color: var(--offering-muted); font-size: 13px; }
@media (max-width: 480px) { .model-offering { padding: 22px; } }
@media (prefers-reduced-motion: reduce) { .model-offering { transition: none; transform: none; } }
</style>
