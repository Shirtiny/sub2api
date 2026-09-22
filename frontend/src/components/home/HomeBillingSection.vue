<template>
  <section id="billing" class="billing-section home-container" aria-labelledby="billing-title">
    <div class="billing-heading">
      <div>
        <p class="eyebrow" data-reveal>{{ t('home.landing.billing.eyebrow') }}</p>
        <h2 id="billing-title" data-reveal style="--reveal-delay: 110ms">{{ t('home.landing.billing.title') }}</h2>
      </div>
      <p class="billing-intro" data-reveal style="--reveal-delay: 210ms">{{ t('home.landing.billing.description') }}</p>
    </div>

    <div class="billing-options">
      <article v-for="(plan, index) in plans" :key="plan" class="billing-option" :aria-labelledby="`billing-${plan}-title`" data-reveal :style="{ '--reveal-delay': `${280 + index * 140}ms` }">
        <div class="billing-option-heading">
          <p class="billing-kicker">0{{ index + 1 }} / {{ plan.toUpperCase() }}</p>
          <span class="billing-badge">{{ t(`home.landing.billing.${plan}.badge`) }}</span>
        </div>
        <h3 :id="`billing-${plan}-title`">{{ t(`home.landing.billing.${plan}.title`) }}</h3>
        <div class="billing-metric">
          <strong>{{ t(`home.landing.billing.${plan}.highlight`) }}</strong>
          <span>{{ t(`home.landing.billing.${plan}.unit`) }}</span>
        </div>
        <p class="billing-caption">{{ t(`home.landing.billing.${plan}.caption`) }}</p>
        <dl class="billing-facts">
          <div v-for="fact in facts" :key="fact">
            <dt>{{ t(`home.landing.billing.${plan}.${fact}Label`) }}</dt>
            <dd>{{ t(`home.landing.billing.${plan}.${fact}`) }}</dd>
          </div>
        </dl>
      </article>
    </div>

    <div class="billing-footnote" data-reveal style="--reveal-delay: 550ms">
      <p>{{ t('home.landing.billing.note') }}</p>
      <RouterLink :to="entryPath">{{ t('home.dashboard') }} <Icon name="arrowRight" size="sm" aria-hidden="true" /></RouterLink>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ entryPath: string }>()
const { t } = useI18n()
const plans = ['subscription', 'balance'] as const
const facts = ['access', 'period'] as const
</script>

<style scoped>
.billing-section { padding-block: var(--chapter-space); }
.billing-section h2 { margin-top: 14px; font: 500 clamp(27px, 3vw, 36px)/1.5 Georgia, 'Noto Serif CJK SC', 'Songti SC', SimSun, serif; text-wrap: balance; }
.billing-section h3 { font-size: 18px; font-weight: 600; }
.billing-section a:focus-visible { outline: 2px solid var(--cafe-accent); outline-offset: 5px; }
.billing-heading { display: flex; align-items: end; justify-content: space-between; gap: 24px; margin-bottom: var(--chapter-gap); }
.eyebrow, .billing-kicker { color: var(--cafe-accent); font-size: 11px; letter-spacing: .12em; }
.billing-intro { max-width: 360px; color: var(--cafe-muted); font-size: 14px; line-height: 1.8; }
.billing-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 24px; }
.billing-option { display: flex; flex-direction: column; min-width: 0; padding: clamp(24px, 3vw, 40px); border: 1px solid var(--cafe-line); border-radius: 12px; background: var(--cafe-page); }
.billing-option-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; margin-bottom: 18px; }
.billing-badge { border: 1px solid var(--cafe-line); border-radius: 999px; padding: 3px 10px; font-size: 11px; color: var(--cafe-muted); }
.billing-metric { display: flex; flex-wrap: wrap; align-items: baseline; gap: 10px; margin-top: 20px; color: var(--cafe-accent); }
.billing-metric strong { font: 500 clamp(32px, 3.5vw, 44px)/1.2 Georgia, 'Noto Serif CJK SC', 'Songti SC', SimSun, serif; letter-spacing: -.025em; }
.billing-metric span { font-size: 14px; }
.billing-caption { margin-top: 8px; font-size: 12px; color: var(--cafe-muted); }
.billing-facts { display: grid; grid-template-rows: repeat(2, 1fr); flex: 1; margin-top: 24px; }
.billing-facts > div { display: grid; grid-template-columns: 80px minmax(0, 1fr); gap: 18px; padding-block: 12px; border-top: 1px solid var(--cafe-line); font-size: 13px; line-height: 1.75; }
.billing-facts dt { color: var(--cafe-ink); font-weight: 500; }
.billing-facts dd { color: var(--cafe-muted); }
.billing-facts > div:last-child { padding-bottom: 0; }
.billing-footnote { display: flex; align-items: center; justify-content: space-between; gap: 24px; margin-top: var(--chapter-gap); font-size: 12px; line-height: 1.8; color: var(--cafe-muted); }
.billing-footnote a { display: inline-flex; align-items: center; gap: 10px; flex-shrink: 0; color: var(--cafe-accent); }
.billing-footnote a:hover { text-decoration: underline; text-underline-offset: 4px; }
@media (min-width: 1024px) and (min-height: 700px) {
  .billing-option { padding: clamp(20px, 3dvh, 32px); }
  .billing-option-heading { margin-bottom: 12px; }
  .billing-metric { margin-top: 14px; }
  .billing-metric strong { font-size: clamp(32px, 4.5dvh, 42px); }
  .billing-facts { margin-top: 18px; }
  .billing-facts > div { padding-block: 10px; }
}
@media (max-width: 767px) {
  .billing-heading { flex-direction: column; align-items: start; gap: 16px; }
  .billing-options { grid-template-columns: 1fr; gap: 18px; }
  .billing-footnote { flex-direction: column; align-items: start; gap: 12px; }
}
@media (max-width: 359px) {
  .billing-facts > div { grid-template-columns: 1fr; gap: 4px; }
}
</style>
