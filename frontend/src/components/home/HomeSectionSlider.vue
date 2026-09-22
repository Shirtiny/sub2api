<template>
  <div
    ref="scroller"
    class="home-slider"
    :data-ready="ready"
    :data-reduced-motion="reducedMotion === 'reduce'"
    @scroll.passive="scheduleUpdate"
    @click="handleAnchor"
    @wheel.passive="cancelNavigation"
    @touchstart.passive="cancelNavigation"
    @keydown="handleScrollKey"
  >
    <slot name="header" :compact="compact" />
    <main id="home-main" tabindex="-1">
      <slot />
    </main>
    <nav class="chapter-nav" :aria-label="t('home.landing.slides.navigation')">
      <span class="chapter-count" aria-hidden="true">{{ number(activeIndex + 1) }} <span>/ {{ number(slides.length) }}</span></span>
      <div class="chapter-stops">
        <button
          v-for="(slide, index) in slides"
          :key="slide.id"
          type="button"
          class="chapter-stop"
          :aria-label="`${number(index + 1)} · ${slide.label}`"
          :aria-controls="slide.id"
          :aria-current="index === activeIndex ? 'step' : undefined"
          @click="goTo(index)"
          @keydown="onChapterKey($event, index)"
        >
          <span class="chapter-label">{{ slide.label }}</span>
          <span class="chapter-mark" aria-hidden="true"></span>
        </button>
      </div>
      <button
        type="button"
        class="chapter-next"
        :aria-label="t(activeIndex === slides.length - 1 ? 'home.landing.slides.backToTop' : 'home.landing.slides.next')"
        @click="goTo((activeIndex + 1) % slides.length)"
      >
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" :class="{ 'is-last': activeIndex === slides.length - 1 }">
          <path d="M12 5v14m-5-5 5 5 5-5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useEventListener, usePreferredReducedMotion, useResizeObserver } from '@vueuse/core'

const props = defineProps<{ slides: { id: string; label: string }[] }>()
const { t } = useI18n()
const scroller = ref<HTMLElement | null>(null)
const activeIndex = ref(0)
const compact = ref(false)
const ready = ref(false)
const reducedMotion = usePreferredReducedMotion()
const number = (value: number) => String(value).padStart(2, '0')
let frame = 0
let navigationTarget: number | null = null
let openedAt = 0

function panels() {
  return Array.from(scroller.value?.querySelectorAll<HTMLElement>('[data-home-slide]') ?? [])
}

function headerHeight() {
  return scroller.value?.querySelector<HTMLElement>('.home-header')?.offsetHeight ?? 0
}

function update() {
  frame = 0
  const root = scroller.value
  if (!root) return
  // Separate enter/exit thresholds keep tiny trackpad movements from toggling the glass.
  compact.value = root.scrollTop > (compact.value ? 8 : 64)
  const items = panels()
  const viewport = root.getBoundingClientRect()
  const contentTop = viewport.top + headerHeight()
  // A reading line, rather than intersection ratios, also handles panels taller than a phone.
  const line = contentTop + (root.clientHeight - headerHeight()) * .35
  let current = 0
  items.forEach((panel, index) => {
    if (panel.getBoundingClientRect().top <= line) current = index
  })
  activeIndex.value = current
  const viewportBottom = viewport.top + root.clientHeight
  const availableHeight = Math.max(1, viewportBottom - contentTop)
  const opening = reducedMotion.value === 'reduce' ? 1 : Math.min(1, (performance.now() - openedAt) / 750)
  items.forEach((panel, index) => {
    panel.dataset.active = String(index === current)
    const bounds = panel.getBoundingClientRect()
    const height = bounds.bottom - bounds.top
    const visible = Math.max(0, Math.min(bounds.bottom, viewportBottom) - Math.max(bounds.top, contentTop))
    const coverage = Math.min(1, visible / Math.max(1, Math.min(height, availableHeight)))
    const longPanel = height > availableHeight + 4
    panel.dataset.long = String(longPanel)
    panel.querySelectorAll<HTMLElement>('[data-reveal]').forEach(element => {
      const delay = Math.max(0, parseFloat(element.style.getPropertyValue('--reveal-delay')) || 0)
      let progress: number
      if (longPanel) {
        // Never dim already-read content inside long mobile/pricing chapters.
        if (visible === 0) delete element.dataset.revealed
        else if (!element.dataset.revealed) {
          const rect = element.getBoundingClientRect()
          if (rect.bottom > contentTop && rect.top < viewportBottom - 24) element.dataset.revealed = 'true'
        }
        progress = element.dataset.revealed ? 1 : 0
      } else {
        // Stagger by spatial entry progress, not by blanking the page or waiting for a timer.
        const start = Math.min(.5, .03 + delay / 1400)
        const position = Math.max(0, Math.min(1, (Math.min(coverage, opening) - start) / .48))
        progress = position * position * (3 - 2 * position)
        if (progress > 0) element.dataset.revealed = 'true'
        else delete element.dataset.revealed
      }
      const value = progress.toFixed(3)
      if (element.style.getPropertyValue('--reveal-progress') !== value) element.style.setProperty('--reveal-progress', value)
    })
  })
  if (navigationTarget !== null && Math.abs(root.scrollTop - navigationTarget) < 1) navigationTarget = null
  if (opening < 1) scheduleUpdate()
}

function scheduleUpdate() {
  if (!frame) frame = requestAnimationFrame(update)
}

function goTo(index: number, immediate = false) {
  const root = scroller.value
  const panel = panels()[index]
  if (!root || !panel) return
  const top = panel.getBoundingClientRect().top - root.getBoundingClientRect().top + root.scrollTop - headerHeight()
  const instant = immediate || reducedMotion.value === 'reduce'
  navigationTarget = instant ? null : Math.max(0, top)
  root.scrollTo({ top: Math.max(0, top), behavior: instant ? 'instant' : 'smooth' })
  if (instant) scheduleUpdate()
}

function cancelNavigation() {
  if (navigationTarget === null) return
  navigationTarget = null
  const root = scroller.value
  if (root) root.scrollTo({ top: root.scrollTop, behavior: 'instant' })
}

function handleScrollKey(event: KeyboardEvent) {
  if (!event.defaultPrevented && ['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', 'Tab', ' '].includes(event.key)) cancelNavigation()
}

function chapterForAnchor(id: string) {
  const index = props.slides.findIndex(slide => slide.id === id)
  if (index >= 0) return index
  const target = document.getElementById(id)
  const panel = target && scroller.value?.contains(target) ? target.closest('[data-home-slide]') : null
  return panels().findIndex(item => item === panel)
}

function handleAnchor(event: MouseEvent) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
  const anchor = event.target instanceof Element ? event.target.closest<HTMLAnchorElement>('a[href^="#"]') : null
  const id = anchor?.getAttribute('href')?.slice(1)
  if (!id || anchor?.target === '_blank') return
  const index = chapterForAnchor(id)
  if (index < 0 && id !== 'home-main') return
  event.preventDefault()
  goTo(Math.max(0, index), id === 'home-main')
  // Preserve skip/guide link keyboard semantics, without stealing focus from the chapter controls.
  const target = document.getElementById(id)
  target?.setAttribute('tabindex', '-1')
  target?.focus({ preventScroll: true })
}

function onChapterKey(event: KeyboardEvent, index: number) {
  let next: number
  if (event.key === 'ArrowDown' || event.key === 'ArrowRight') next = (index + 1) % props.slides.length
  else if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') next = (index - 1 + props.slides.length) % props.slides.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = props.slides.length - 1
  else return
  event.preventDefault()
  scroller.value?.querySelectorAll<HTMLButtonElement>('.chapter-stop')[next]?.focus({ preventScroll: true })
  goTo(next)
}

function restoreHash() {
  const index = chapterForAnchor(window.location.hash.slice(1))
  if (index >= 0) goTo(index, true)
}

useEventListener(window, 'hashchange', restoreHash)
useEventListener(scroller, 'scrollend', () => { navigationTarget = null })
watch(reducedMotion, () => {
  cancelNavigation()
  scheduleUpdate()
})
useResizeObserver(scroller, scheduleUpdate)
onMounted(async () => {
  await nextTick()
  openedAt = performance.now() - (window.location.hash ? 750 : 0)
  restoreHash()
  ready.value = true
  scheduleUpdate()
})
onBeforeUnmount(() => {
  cancelAnimationFrame(frame)
  cancelNavigation()
})
</script>

<style scoped>
.home-slider {
  --slider-header-height: 104px;
  position: relative;
  height: 100vh;
  height: 100dvh;
  overflow-y: auto;
  overflow-x: hidden;
  overscroll-behavior-y: contain;
  scroll-snap-type: y mandatory;
  scroll-behavior: smooth;
  scroll-padding-top: var(--slider-header-height);
  scrollbar-width: thin;
  scrollbar-color: var(--cafe-line) var(--cafe-page);
}
.home-slider :deep(.home-slide) {
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-height: calc(100dvh - var(--slider-header-height));
  position: relative;
  scroll-snap-align: start;
  overflow: clip;
}
.home-slider :deep(.slide-content) {
  width: 100%;
}
.home-slider[data-ready='true'] :deep([data-reveal]) {
  opacity: var(--reveal-progress, 0);
  transform: translate3d(
    calc(var(--reveal-x, 0px) * (1 - var(--reveal-progress, 0))),
    calc(var(--reveal-y, 24px) * (1 - var(--reveal-progress, 0))), 0);
  transition: opacity .42s ease-out, transform .62s cubic-bezier(.22, 1, .36, 1);
}
.home-slider :deep([data-long='true'] [data-reveal]) { transition-delay: var(--reveal-delay, 0ms); }
.home-slider :deep(.guide-intro [data-reveal]) { --reveal-x: -22px; --reveal-y: 8px; }
.home-slider :deep(.step[data-reveal]) { --reveal-x: 22px; --reveal-y: 8px; }
.home-slider :deep([data-reveal]:focus-within) { opacity: 1 !important; transform: none !important; }

.chapter-nav {
  position: fixed;
  z-index: 30;
  right: 16px;
  top: 50%;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 14px 4px;
  border: 1px solid var(--cafe-line);
  border-radius: 32px;
  background: rgba(var(--cafe-page-rgb), .88);
  backdrop-filter: blur(14px);
  transform: translateY(-50%);
  color: var(--cafe-muted);
}
.chapter-count { margin-bottom: 18px; font-size: 11px; font-variant-numeric: tabular-nums; }
.chapter-count > span { display: none; }
.chapter-stop { position: relative; display: grid; width: 36px; height: 38px; place-items: center; }
.chapter-mark { width: 4px; height: 4px; border-radius: 8px; background: currentColor; opacity: .35; transition: height .5s cubic-bezier(.22, 1, .36, 1), opacity .4s; }
.chapter-stop[aria-current] .chapter-mark { height: 22px; opacity: 1; color: var(--cafe-accent); }
.chapter-label { position: absolute; right: 38px; width: max-content; padding: 7px 12px; border: 1px solid var(--cafe-line); border-radius: 6px; background: rgba(var(--cafe-page-rgb), .96); font-size: 11px; opacity: 0; transform: translateX(6px); pointer-events: none; transition: opacity .25s, transform .3s; }
.chapter-stop:focus-visible .chapter-label { opacity: 1; transform: none; }
@media (hover: hover) { .chapter-stop:hover .chapter-label { opacity: 1; transform: none; } }
.chapter-next { display: grid; width: 36px; height: 36px; margin-top: 18px; place-items: center; border: 1px solid var(--cafe-line); border-radius: 50%; background: rgba(var(--cafe-page-rgb), .9); }
.chapter-next svg { width: 17px; height: 17px; transition: transform .5s; }
.chapter-next svg.is-last { transform: rotate(180deg); }
.chapter-nav button:focus-visible { outline: 2px solid var(--cafe-accent); outline-offset: 3px; border-radius: 6px; }
@media (max-width: 1023px) {
  .home-slider { scroll-snap-type: y proximity; }
  .home-slider :deep(.home-slide) { padding-bottom: 72px; }
  .chapter-nav { top: auto; bottom: max(12px, env(safe-area-inset-bottom)); right: auto; left: 50%; flex-direction: row; gap: 10px; padding: 4px 12px; border: 1px solid var(--cafe-line); border-radius: 40px; background: rgba(var(--cafe-page-rgb), .94); backdrop-filter: blur(16px); box-shadow: 0 4px 20px #23180b0a; transform: translateX(-50%); }
  .chapter-count { margin: 0 4px 0 0; }
  .chapter-stops { display: flex; }
  .chapter-stop { width: 30px; height: 36px; }
  .chapter-mark { transition: width .5s, opacity .4s; }
  .chapter-stop[aria-current] .chapter-mark { width: 18px; height: 4px; }
  .chapter-label { right: 50%; bottom: 46px; transform: translate(50%, 4px); }
  .chapter-stop:focus-visible .chapter-label { transform: translate(50%, 0); }
  .chapter-next { margin: 0; width: 30px; height: 30px; }
}
@media (max-width: 1023px) and (hover: hover) { .chapter-stop:hover .chapter-label { transform: translate(50%, 0); } }
@media (max-width: 359px) { .home-slider { --slider-header-height: 136px; } }
@media (prefers-reduced-motion: reduce) {
  .home-slider { scroll-snap-type: none; scroll-behavior: auto; }
  .chapter-nav * { transition: none !important; }
  .home-slider[data-ready='true'] :deep([data-reveal]) { transition: none !important; opacity: 1; transform: none; }
}
</style>
