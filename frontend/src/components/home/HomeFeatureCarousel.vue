<template>
  <div
    ref="carousel"
    class="feature-carousel"
    role="region"
    :aria-label="t('home.landing.featureShowcase.label')"
    :aria-roledescription="t('home.landing.featureShowcase.carousel')"
    :data-running="animated"
    :data-animated="animated"
    :data-autoplay="autoplay"
    :data-feature="slides[active].key"
    :data-reduced-motion="reducedMotion === 'reduce'"
  >
    <div class="feature-grid" role="group" :aria-label="t('home.landing.featureShowcase.choose')">
      <article
        v-for="(slide, index) in slides"
        :key="slide.key"
        class="feature"
        :data-selected="active === index"
        data-reveal
        :style="{ '--reveal-delay': `${300 + index * 130}ms` }"
      >
        <div class="feature-heading">
          <Icon :name="slide.icon" size="lg" aria-hidden="true" />
          <h3>
            <button
              type="button"
              class="feature-selector"
              :aria-pressed="active === index"
              :aria-controls="`everyday-scene-${slide.key}`"
              :aria-describedby="`everyday-description-${slide.key}`"
              @click="select(index)"
              @keydown="onSelectorKey($event, index)"
            >{{ t(`home.landing.features.${slide.copyKey}.title`) }}</button>
          </h3>
          <span class="feature-number" aria-hidden="true">0{{ index + 1 }}</span>
        </div>
        <span class="feature-track" aria-hidden="true" />
      </article>
    </div>
    <div ref="presentation" class="feature-presentation" data-reveal style="--reveal-delay: 660ms">
      <div class="feature-stage" aria-live="off" aria-atomic="false">
        <div
          v-for="(slide, index) in slides"
          :id="`everyday-scene-${slide.key}`"
          :key="slide.key"
          class="feature-frame"
          :class="{ 'is-current': active === index }"
          :data-feature="slide.key"
          :data-holding="active === index && phase === 'hold'"
          :data-entering="active === index && phase === 'enter'"
          role="group"
          :aria-roledescription="t('home.landing.featureShowcase.slide')"
          :aria-label="`${index + 1} / 3 · ${t(`home.landing.featureShowcase.${slide.key}.label`)}`"
          :aria-hidden="active !== index"
        >
          <div class="feature-art">
            <HomeFeatureScene :key="`${slide.key}-${sequences[index]}`" :kind="slide.key" />
            <span
              v-if="active === index"
              :key="`clock-${sequences[index]}-${phase}`"
              class="feature-clock"
              :data-phase="phase"
              :style="{ '--phase-duration': `${phaseDuration}ms` }"
              aria-hidden="true"
              @animationend.self="finishPhase(index)"
            />
            <p v-if="slide.key === 'trust'" class="sr-only">{{ t('home.landing.featureShowcase.billingStory.summary') }}</p>
            <div v-if="slide.key === 'intelligence'" :key="`intelligence-${sequences[index]}`" class="intelligence-story" aria-hidden="true">
              <div v-for="shot in ['understand', 'connect', 'resolve']" :key="shot" class="intelligence-caption" :data-phase="shot">
                <p class="intelligence-line">{{ t(`home.landing.featureShowcase.intelligenceStory.${shot}`) }}</p>
              </div>
              <div class="intelligence-finale">
                <!-- Outlined letters avoid a late-loading display font during the reveal. -->
                <svg class="astra-signature" viewBox="0 0 420 72" fill="none" aria-hidden="true">
                  <title>ASTRA</title>
                  <g stroke="currentColor" stroke-width="2.1" stroke-linejoin="bevel">
                    <path d="M6 66 31 6 56 66M15 45H47" />
                    <path d="M137 13C126 4 105 3 97 14S96 34 117 37 143 47 135 59 106 73 93 60" />
                    <path d="M173 7H228M200.5 7V66" />
                    <path d="M269 66V7H293C321 7 321 38 293 38H269M292 38 318 66" />
                    <path d="M360 66 385 6 410 66M369 45H401" />
                  </g>
                </svg>
              </div>
            </div>
          </div>
          <div class="feature-copy">
            <p :id="`everyday-description-${slide.key}`" class="feature-description">{{ t(`home.landing.features.${slide.copyKey}.description`) }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDocumentVisibility, useIntersectionObserver, usePreferredReducedMotion } from '@vueuse/core'
import Icon from '@/components/icons/Icon.vue'
import HomeFeatureScene from './HomeFeatureScene.vue'

const { t } = useI18n()
// Intelligence's last highlight clears the route at 96%; hold only after that,
// on the fully visible result plateau, before the native reset/fade.
const slides = [
  { key: 'intelligence', copyKey: 'connect', icon: 'cpu', duration: 14000, completeAt: 13580 },
  { key: 'speed', copyKey: 'create', icon: 'bolt', duration: 16000, completeAt: 14720 },
  { key: 'trust', copyKey: 'manage', icon: 'shield', duration: 16000, completeAt: 14880 }
] as const
const carousel = ref<HTMLElement | null>(null)
const presentation = ref<HTMLElement | null>(null)
const active = ref(0)
const sequences = ref([0, 0, 0])
const autoplay = ref(true)
const phase = ref<'enter' | 'play' | 'hold' | 'outro'>('play')
const phaseDuration = computed(() => {
  const slide = slides[active.value]
  if (phase.value === 'enter') return 900
  if (phase.value === 'hold') return 1500
  return phase.value === 'play' ? slide.completeAt : slide.duration - slide.completeAt
})
const inView = ref(false)
const visibility = useDocumentVisibility()
const reducedMotion = usePreferredReducedMotion()
const animated = computed(() => inView.value && visibility.value === 'visible' && reducedMotion.value !== 'reduce')

useIntersectionObserver(presentation, ([entry]) => {
  inView.value = (entry?.isIntersecting ?? false) && (entry?.intersectionRatio ?? 0) >= .3
}, { threshold: [0, .3] })

function show(index: number) {
  const changing = index !== active.value
  active.value = index
  sequences.value[index]!++
  // Let the fresh scene dissolve in before starting its own animation/clock.
  // The outgoing scene stays mounted and paused throughout the crossfade.
  phase.value = changing && reducedMotion.value !== 'reduce' ? 'enter' : 'play'
}

function select(index: number) {
  autoplay.value = false
  if (index !== active.value) show(index)
}

// A native CSS clock shares visibility pauses with the artwork, so hidden-tab
// time cannot skip scenes or consume the extra 1.5s end hold. No wall-clock timer.
function finishPhase(index: number) {
  if (!animated.value || index !== active.value) return
  if (phase.value === 'enter') phase.value = 'play'
  else if (phase.value === 'play') phase.value = 'hold'
  else if (phase.value === 'hold') {
    if (autoplay.value) show((index + 1) % slides.length)
    else phase.value = 'outro'
  } else show(index)
}

function onSelectorKey(event: KeyboardEvent, index: number) {
  let next: number
  if (event.key === 'ArrowRight') next = (index + 1) % slides.length
  else if (event.key === 'ArrowLeft') next = (index - 1 + slides.length) % slides.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = slides.length - 1
  else return
  event.preventDefault()
  select(next)
  carousel.value?.querySelectorAll<HTMLButtonElement>('.feature-selector')[next]?.focus({ preventScroll: true })
}
</script>

<style scoped>
.feature-carousel {
  --intelligence-duration: 14s;
  overflow: hidden;
  border: 1px solid var(--cafe-line);
  border-radius: 14px;
  background: linear-gradient(115deg, rgba(var(--cafe-page-rgb), .7), var(--cafe-surface));
}
.feature-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); }
.feature { position: relative; min-width: 0; padding: 20px 24px; background: transparent; transition: background-color .4s; }
.feature + .feature { border-left: 1px solid var(--cafe-line); }
.feature[data-selected='true'] { background: rgba(var(--cafe-page-rgb), .72); }
.feature-heading { display: flex; align-items: center; gap: 12px; color: var(--cafe-accent); }
.feature-heading > svg { flex-shrink: 0; }
.feature-number { font: italic 14px Georgia, serif; color: var(--cafe-muted); }
.feature h3 { flex: 1; min-width: 0; color: var(--cafe-ink); font-size: 16px; font-weight: 600; line-height: 1.6; }
.feature-selector { text-align: left; transition: color .3s; }
.feature-selector::after { content: ''; position: absolute; inset: 0; cursor: pointer; }
.feature-selector[aria-pressed='true'] { color: var(--cafe-accent); }
.feature-selector:focus-visible::after { outline: 2px solid var(--cafe-accent); outline-offset: -4px; border-radius: 8px; }
.feature-track { position: absolute; bottom: 0; left: 0; width: 100%; height: 2px; overflow: hidden; pointer-events: none; background: transparent; transition: background-color .6s ease; }
.feature[data-selected='true'] .feature-track { background: var(--cafe-accent); }
.feature-presentation { position: relative; border-top: 1px solid var(--cafe-line); }
.feature-stage { display: grid; isolation: isolate; overflow: hidden; background: var(--cafe-page); }
/* All three tabs use the original intelligence frame. Only artwork differs;
   canvas sizing, background, divider and description spacing are shared. */
.feature-frame {
  grid-area: 1 / 1;
  position: relative;
  display: grid;
  grid-template-rows: minmax(320px, 1fr) auto;
  min-width: 0;
  padding: 0;
  z-index: 0;
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  transition: opacity .9s cubic-bezier(.4, 0, .2, 1), visibility 0s .9s;
}
.feature-frame.is-current { z-index: 1; opacity: 1; visibility: visible; pointer-events: auto; transition-delay: 0s; }
.feature-art { grid-area: 1 / 1; position: relative; width: 100%; min-height: 0; overflow: hidden; }
.feature-art :deep(.feature-scene) { display: block; width: 100%; height: 100%; background: transparent; }
/* The background and divider stay still. Only the artwork and copy move;
   transparent canvases allow both scenes to remain visible during the dissolve. */
.feature-art :deep(.feature-scene), .intelligence-story { transform: translateY(12px) scale(.985); transform-origin: center; transition: transform .9s cubic-bezier(.22, 1, .36, 1); }
.feature-frame.is-current .feature-art :deep(.feature-scene), .feature-frame.is-current .intelligence-story { transform: none; }
/* Freshly mounted SVGs have no previous computed style to transition from. */
.feature-frame[data-entering='true'] .feature-art :deep(.feature-scene), .feature-frame[data-entering='true'] .intelligence-story { animation: feature-art-enter .9s cubic-bezier(.22, 1, .36, 1) both; }
@keyframes feature-art-enter { from { transform: translateY(12px) scale(.985); } to { transform: translateY(0) scale(1); } }
.feature-clock { position: absolute; width: 1px; height: 1px; opacity: 0; pointer-events: none; animation: feature-phase-clock var(--phase-duration) linear both; }
@keyframes feature-phase-clock { from { transform: translateX(0); } to { transform: translateX(1px); } }
.feature-copy { position: relative; z-index: 1; display: flex; justify-content: flex-end; margin-top: 0; padding: 16px 32px 24px; border-top: 1px solid var(--cafe-line); }
.feature-description { width: 100%; min-width: 0; max-width: none; margin: 0; padding-right: 0; color: var(--cafe-muted); font-size: 14px; line-height: 1.85; text-align: right; }
.feature-description { opacity: 0; transform: translateY(5px); transition: opacity .28s ease, transform .45s cubic-bezier(.22, 1, .36, 1); }
.feature-frame.is-current .feature-description { opacity: 1; transform: none; transition-delay: .16s; }
.intelligence-story { position: absolute; inset: 0; pointer-events: none; overflow: hidden; }
.intelligence-caption { position: absolute; right: 6%; bottom: 7%; width: max-content; max-width: 75%; text-align: right; opacity: 0; }
.intelligence-line { margin: 0; color: var(--cafe-muted); font-size: clamp(13px, 1.25vw, 16px); font-weight: 400; line-height: 1.6; letter-spacing: .24em; }
.intelligence-finale { position: absolute; right: 6%; bottom: 7%; width: clamp(96px, 13vw, 156px); opacity: 0; }
.intelligence-caption[data-phase='understand'] { animation: intelligence-understand var(--intelligence-duration) ease-in-out infinite; }
.intelligence-caption[data-phase='connect'] { animation: intelligence-connect var(--intelligence-duration) ease-in-out infinite; }
.intelligence-caption[data-phase='resolve'] { animation: intelligence-resolve var(--intelligence-duration) ease-in-out infinite; }
.intelligence-finale { animation: intelligence-finale var(--intelligence-duration) ease-in-out infinite; }
.astra-signature { display: block; width: 100%; height: auto; overflow: visible; color: var(--cafe-accent); }
.astra-signature g { stroke-dasharray: 220; animation: intelligence-inscription var(--intelligence-duration) ease-in-out infinite; }
/* Quiet margin notes follow the continuous system, not a sequence of title cards. Animate only transform/opacity, never text layout. */
@keyframes intelligence-understand {
  0%, 2%, 23%, 100% { opacity: 0; transform: translateX(-10px); }
  6% { opacity: .85; transform: translateX(0); }
  18% { opacity: .85; transform: translateX(8px); }
  22% { opacity: 0; transform: translateX(16px); }
}
@keyframes intelligence-connect {
  0%, 25%, 46%, 100% { opacity: 0; transform: translate(10px, 5px); }
  29% { opacity: .85; transform: translate(0, 0); }
  41% { opacity: .85; transform: translate(-8px, -2px); }
  45% { opacity: 0; transform: translate(-16px, -4px); }
}
@keyframes intelligence-resolve {
  0%, 49%, 71%, 100% { opacity: 0; transform: translateY(8px); }
  53% { opacity: .85; transform: translateY(0); }
  66% { opacity: .85; transform: translateY(-4px); }
  70% { opacity: 0; transform: translateY(-10px); }
}
@keyframes intelligence-finale {
  0%, 71%, 100% { opacity: 0; transform: scale(.97); }
  76%, 98% { opacity: 1; transform: scale(1); }
  99% { opacity: 0; transform: scale(1.035); }
}
@keyframes intelligence-inscription { 0%, 71% { stroke-dashoffset: 220; } 79%, 100% { stroke-dashoffset: 0; } }
.feature-carousel[data-animated='false'] :deep(.feature-scene),
.feature-carousel[data-animated='false'] .intelligence-story,
.feature-carousel[data-animated='false'] :deep(.feature-scene *),
.feature-frame:not(.is-current) :deep(.feature-scene *),
.feature-frame[data-holding='true'] :deep(.feature-scene *),
.feature-frame[data-entering='true'] :deep(.feature-scene *),
.feature-carousel[data-animated='false'] .intelligence-story :deep(*),
.feature-frame:not(.is-current) .intelligence-story :deep(*),
.feature-frame[data-holding='true'] .intelligence-story :deep(*),
.feature-frame[data-entering='true'] .intelligence-story :deep(*),
.feature-carousel[data-animated='false'] .feature-clock { animation-play-state: paused !important; }
/* Hold the completed task, not the surrounding network traffic. Ambient loops
   must still stop when the tab is inactive, offscreen or the document is hidden. */
.feature-carousel[data-animated='true'] .feature-frame.is-current[data-holding='true'] :deep(.scene-ambient) { animation-play-state: running !important; }
.feature-carousel button:focus-visible { outline: 2px solid var(--cafe-accent); outline-offset: -3px; border-radius: 4px; }
@media (max-width: 1023px) {
  .feature { padding-inline: 16px; }
}
@media (max-width: 767px) {
  .feature-grid { grid-template-columns: minmax(0, 1fr); }
  .feature { padding: 12px 18px; }
  .feature + .feature { border-left: 0; border-top: 1px solid var(--cafe-line); }
  .feature h3 { font-size: 14px; }
}
@media (max-width: 639px) {
  .feature-frame { grid-template-rows: minmax(400px, 1fr) auto; }
  .feature-copy { padding: 14px 18px 20px; }
  .feature-description { font-size: 13px; padding-right: 0; }
  .intelligence-caption[data-phase], .intelligence-finale { bottom: 4%; }
  .intelligence-line { font-size: 13px; letter-spacing: .18em; }
  .intelligence-finale { width: 105px; }
}
@media (min-width: 1024px) {
  /* Use the full row, not a narrow reading column. Keep natural wrapping for
     translations/zoom rather than clipping copy or forcing horizontal overflow. */
  .feature-copy { padding: 12px 28px; }
  .feature-description { font-size: clamp(13px, 1.3vw, 14px); line-height: 1.7; }
}
@media (min-width: 1024px) and (min-height: 700px) {
  .feature-carousel {
    display: grid;
    grid-template-rows: auto minmax(180px, 1fr);
    min-height: 0;
  }
  .feature { padding: clamp(14px, 1.8dvh, 20px) 24px; }
  .feature-presentation { min-height: 0; }
  .feature-stage { height: 100%; min-height: 0; grid-template-rows: minmax(0, 1fr); }
  .feature-frame { min-height: 0; grid-template-rows: minmax(200px, 1fr) auto; }
  .feature-art { height: 100%; min-height: 0; }
  .feature-art :deep(.feature-scene) { position: absolute; inset: 0; width: 100%; height: 100%; }
}
.feature-carousel[data-reduced-motion='true'] .intelligence-finale { opacity: 1; }
.feature-carousel[data-reduced-motion='true'] :deep(*) { animation: none !important; transition: none !important; }
@media (prefers-reduced-motion: reduce) {
  .intelligence-finale { opacity: 1; }
  .feature-carousel :deep(*) { animation: none !important; transition: none !important; }
}
</style>
