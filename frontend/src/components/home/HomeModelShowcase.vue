<template>
  <section
    id="supported-models"
    ref="section"
    class="home-container model-showcase"
    :class="{
      'model-entered': entered,
      'model-sequence-active': introRunning,
      'model-manual-brand': manualBrand
    }"
    :data-stage="stage"
    :data-paused="animationPaused"
    :data-reduced-motion="reducedMotion === 'reduce'"
    aria-labelledby="models-title"
  >
    <h2 id="models-title" class="models-title" data-reveal>{{ t('home.landing.models.title') }}</h2>
    <div class="model-presentation" data-model="chatgpt">
      <div class="model-brand">
        <div
          class="model-art"
          data-reveal
          style="--reveal-delay: 100ms"
          @click="toggleStage"
        >
          <svg ref="brandLogo" class="model-logo" viewBox="-7 -7 38 38" fill-rule="evenodd" shape-rendering="geometricPrecision" aria-hidden="true">
            <g class="model-logo-turn">
              <animateTransform attributeName="transform" type="rotate" from="0 12 12" to="360 12 12" dur="36s" repeatCount="indefinite" />
              <g class="model-echo model-echo-first">
                <path v-for="(path, i) in chatgpt.paths" :key="i" :d="path" />
              </g>
              <g class="model-echo model-echo-second">
                <path v-for="(path, i) in chatgpt.paths" :key="i" :d="path" />
              </g>
              <g class="model-solid">
                <path v-for="(path, i) in chatgpt.paths" :key="i" :d="path" />
              </g>
              <g class="model-outline">
                <path v-for="(path, i) in chatgpt.paths" :key="i" :d="path" pathLength="1" />
              </g>
            </g>
          </svg>
        </div>
        <p class="model-name" data-reveal style="--reveal-delay: 180ms">ChatGPT</p>
      </div>
      <ul class="model-groups" :aria-label="t('home.landing.models.groups')">
        <li v-for="(group, index) in groups" :key="group.name" class="model-group-stage" data-reveal :style="{ '--reveal-delay': `${300 + index * 120}ms` }">
          <div class="model-group" :style="{ '--group-delay': `${(2.45 + index * .14).toFixed(2)}s` }" @animationend="finishIntro(index, $event)">
            <span class="model-group-index" aria-hidden="true">0{{ index + 1 }}</span>
            <HomeModelScene :kind="group.kind" :label="group.name" :active="entered && !manualBrand && inView && visibility === 'visible'" :reduced-motion="reducedMotion === 'reduce'" />
            <div class="model-group-caption">
              <p>{{ group.version }}</p>
              <h3>{{ group.name }}</h3>
            </div>
          </div>
        </li>
      </ul>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDocumentVisibility, useIntersectionObserver, usePreferredReducedMotion } from '@vueuse/core'
import modelMarks from '@/assets/home/model-marks.json'
import HomeModelScene from './HomeModelScene.vue'

const { t } = useI18n()
const chatgpt = modelMarks.find(model => model.id === 'chatgpt')!
const groups = [
  { name: 'Luna', version: 'GPT-5.6', kind: 'luna' },
  { name: 'Terra', version: 'GPT-5.6', kind: 'terra' },
  { name: 'Sol', version: 'GPT-5.6', kind: 'sol' },
  { name: 'Astra', version: 'GPT-6', kind: 'astra' }
] as const
const section = ref<HTMLElement | null>(null)
const brandLogo = ref<SVGSVGElement | null>(null)
const entered = ref(false)
const introRunning = ref(false)
const manualBrand = ref(false)
const inView = ref(true)
const visibility = useDocumentVisibility()
const reducedMotion = usePreferredReducedMotion()
useIntersectionObserver(section, ([entry]) => {
  inView.value = entry?.isIntersecting ?? false
  if (inView.value && !entered.value) {
    entered.value = true
    introRunning.value = reducedMotion.value !== 'reduce'
  }
}, { threshold: .25 })
const animationPaused = computed(() => !inView.value || visibility.value !== 'visible' || reducedMotion.value === 'reduce')
const stage = computed(() => introRunning.value ? 'intro' : manualBrand.value ? 'brand' : 'models')

// Rotate the vector paths inside SVG, not a composited HTML image of the logo.
// The SVG stays mounted so toggling stages does not restart its rotation.
watchEffect(() => {
  const svg = brandLogo.value
  if (animationPaused.value) svg?.pauseAnimations?.()
  else svg?.unpauseAnimations?.()
})

function finishIntro(index: number, event: AnimationEvent) {
  if (index === groups.length - 1 && event.target === event.currentTarget) introRunning.value = false
}

function toggleStage() {
  if (!introRunning.value) manualBrand.value = !manualBrand.value
}
</script>

<style scoped>
.model-showcase { padding-top: 44px; }
.models-title { margin-bottom: var(--chapter-gap, 44px); color: var(--cafe-ink); font-family: Georgia, 'Noto Serif CJK SC', 'Songti SC', SimSun, serif; font-size: clamp(27px, 3vw, 36px); font-weight: 500; line-height: 1.5; }
.model-presentation { --model-brand-top: 44px; position: relative; isolation: isolate; padding: 220px 24px 48px; border-block: 1px solid var(--cafe-line); }
/* Anchor the scaled brand at its reserved top inset, not at the center of its
   unscaled box: the latter pushes the compact mark into the model cards. */
.model-brand { position: absolute; z-index: 2; top: var(--model-brand-top); left: 50%; display: flex; width: 300px; flex-direction: column; align-items: center; transform: translateX(-50%) scale(var(--model-brand-scale, .44)); transform-origin: top center; transition: top .8s cubic-bezier(.22, 1, .36, 1), transform .8s cubic-bezier(.22, 1, .36, 1); }
.model-art { width: 240px; color: var(--cafe-ink); cursor: pointer; user-select: none; }
.model-logo { display: block; width: 100%; height: auto; overflow: visible; }
.model-solid { fill: currentColor; }
.model-outline { fill: none; stroke: currentColor; stroke-width: .14; opacity: 0; }
.model-echo { fill: none; stroke: var(--cafe-accent); stroke-width: .08; transform-origin: 12px 12px; opacity: 0; }
.model-name { margin-top: -16px; color: var(--cafe-ink); font-family: Arial, Helvetica, sans-serif; font-size: 44px; font-weight: 500; line-height: 1.15; letter-spacing: -.055em; }
.model-groups { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; list-style: none; }
.model-group-stage { min-width: 0; }
.model-group { position: relative; min-width: 0; overflow: hidden; border: 1px solid #293039; border-radius: 10px; background: #080c12; transition: opacity .55s ease, transform .7s cubic-bezier(.22, 1, .36, 1); }
.model-group-index { position: absolute; z-index: 1; top: 14px; left: 16px; color: #b6c3cd; font: italic 11px Georgia, serif; }
.model-group-caption { position: relative; margin-top: -26px; padding: 0 24px 26px; background: linear-gradient(transparent, #080c12 26px); }
.model-group-caption p { color: #a5b2c0; font-size: 10px; line-height: 1.8; letter-spacing: .16em; }
.model-group h3 { margin-top: 5px; color: #f1f4f7; font: 400 clamp(26px, 2.8vw, 36px)/1.2 Georgia, 'Times New Roman', serif; letter-spacing: -.02em; }
/* The first visit runs one shortened automatic sequence. Manual switching is
   persistent: the brand remains open until the visitor clicks it again. */
.model-sequence-active .model-brand { animation: model-brand-settle .7s 1.9s cubic-bezier(.22, 1, .36, 1) both; }
.model-sequence-active .model-outline { animation: model-outline-fade 1.25s ease both; }
.model-sequence-active .model-outline path { stroke-dasharray: 1; animation: model-draw .95s cubic-bezier(.3, 0, .2, 1) both; }
.model-sequence-active .model-solid { animation: model-fill .55s .65s ease both; }
.model-sequence-active .model-echo-first { animation: model-echo .85s .9s ease-out both; }
.model-sequence-active .model-echo-second { animation: model-echo .85s 1.2s ease-out both; }
.model-sequence-active .model-group { animation: model-group-arrive .7s var(--group-delay) cubic-bezier(.22, 1, .36, 1) both; }
.model-sequence-active .model-art { pointer-events: none; }
.model-manual-brand .model-brand { top: 50%; transform: translate(-50%, -50%) scale(1); }
.model-manual-brand .model-groups { pointer-events: none; }
.model-manual-brand .model-group { opacity: 0; transform: translateY(18px); pointer-events: none; }
.model-showcase[data-paused='true'] :is(.model-brand, .model-logo-turn, .model-outline, .model-outline path, .model-solid, .model-echo, .model-group) { animation-play-state: paused; }
@keyframes model-draw { from { stroke-dashoffset: 1; } to { stroke-dashoffset: 0; } }
@keyframes model-outline-fade { 0%, 70% { opacity: .8; } 100% { opacity: 0; } }
@keyframes model-fill { from { opacity: 0; } to { opacity: 1; } }
@keyframes model-echo { 0% { opacity: 0; transform: scale(1); } 15% { opacity: .26; } 100% { opacity: 0; transform: scale(1.55); } }
@keyframes model-brand-settle {
  from { top: 50%; transform: translate(-50%, -50%) scale(1); }
  to { top: var(--model-brand-top); transform: translate(-50%, 0) scale(var(--model-brand-scale, .44)); }
}
@keyframes model-group-arrive { from { opacity: 0; transform: translateY(18px); } to { opacity: 1; transform: translateY(0); } }
@media (max-width: 767px) {
  .model-showcase { padding-top: 32px; }
  .model-presentation { padding-inline: 0; }
  .model-brand { width: min(280px, 100%); }
  .model-art { width: min(240px, 100%); }
  .model-name { font-size: 40px; }
  .model-groups { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
  .model-group-caption { padding: 0 16px 22px; }
}
@media (min-width: 1024px) and (min-height: 700px) {
  .model-showcase {
    display: grid;
    grid-template-rows: auto minmax(min-content, 1fr);
    height: var(--chapter-viewport, calc(100dvh - 104px));
    min-height: min-content;
  }
  .model-presentation {
    --model-brand-scale: .38;
    --model-brand-top: 12px;
    display: grid;
    grid-template-rows: minmax(180px, 1fr);
    min-height: 0;
    padding: clamp(124px, 15dvh, 164px) 24px 16px;
  }
  .model-groups { min-height: 0; grid-template-rows: minmax(0, 1fr); }
  .model-group-stage { display: grid; min-height: 0; }
  .model-group {
    display: grid;
    grid-template-rows: minmax(100px, 1fr) auto;
    align-self: center;
    height: 100%;
    max-height: 380px;
    min-height: 0;
  }
  .model-group :deep(.celestial-scene) { min-height: 0; height: 100%; aspect-ratio: auto; }
  .model-group-caption { margin-top: 0; padding: 8px 20px 16px; }
  .model-group h3 { font-size: clamp(24px, 2.4vw, 32px); }
}
.model-showcase[data-reduced-motion='true'] :is(.model-brand, .model-logo-turn, .model-outline, .model-outline path, .model-solid, .model-echo, .model-group) { animation: none; transition: none; }
@media (prefers-reduced-motion: reduce) {
  .model-showcase :is(.model-brand, .model-logo-turn, .model-outline, .model-outline path, .model-solid, .model-echo, .model-group) { animation: none; transition: none; }
}
</style>
