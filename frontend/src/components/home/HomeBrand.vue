<template>
  <div class="home-brand" :class="{ 'has-story': story }" :style="{ '--brand-name-width': `${story?.width ?? wordmark?.width ?? 224}px` }">
    <RouterLink to="/home" class="brand" :title="siteName" :aria-label="siteName">
      <HomeBrandStory v-if="story && wordmark" :wordmark="wordmark" :story="story" :site-name="siteName" :paused="animationPaused" :reduced-motion="reducedMotion === 'reduce'" />
      <svg v-else class="brand-stage" :viewBox="wordmark?.viewBox || '0 0 224 48'" preserveAspectRatio="xMinYMid" aria-hidden="true" focusable="false">
        <title class="brand-name">{{ siteName }}</title>
        <path v-if="wordmark" class="brand-wordmark" :d="wordmark.d" fill="currentColor" fill-rule="evenodd" />
        <!-- Preserve other scripts using local fonts, rather than substituting a fixed cafe name. -->
        <text v-else class="brand-fallback" x="0" y="36" :textLength="fallbackLength" lengthAdjust="spacingAndGlyphs">{{ siteName }}</text>
      </svg>
    </RouterLink>
    <!-- A separate control: tool marks never replace the site's identity or link. -->
    <button
      type="button"
      class="brand-tools"
      :data-paused="animationPaused"
      :disabled="reducedMotion === 'reduce'"
      :aria-pressed="paused"
      :aria-label="toolLabel"
      :title="toolLabel"
      @click="paused = !paused"
    >
      <svg v-if="story" class="story-pause" viewBox="0 0 24 24" aria-hidden="true">
        <path v-if="paused" d="m9 6 9 6-9 6Z" fill="currentColor" />
        <path v-else d="M9 6v12M15 6v12" fill="none" stroke="currentColor" stroke-width="1.5" />
      </svg>
      <HomeToolGlyph v-else :paused="animationPaused" :reduced-motion="reducedMotion === 'reduce'" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDocumentVisibility, usePreferredReducedMotion } from '@vueuse/core'
import { createWordmark } from './brandGeometry'
import HomeToolGlyph from './HomeToolGlyph.vue'
import HomeBrandStory from './HomeBrandStory.vue'
import { createBrandStory } from './brandStory'

const props = defineProps<{ siteName: string }>()
const { t } = useI18n()
const wordmark = computed(() => createWordmark(props.siteName))
const story = computed(() => createBrandStory(wordmark.value))
const fallbackLength = computed(() => Math.min(222, Array.from(props.siteName).length * 34))
const paused = ref(false)
const visibility = useDocumentVisibility()
const reducedMotion = usePreferredReducedMotion()
const animationPaused = computed(() => paused.value || visibility.value !== 'visible' || reducedMotion.value === 'reduce')
const toolLabel = computed(() => t(reducedMotion.value === 'reduce'
  ? 'home.landing.toolMarks'
  : paused.value ? 'home.landing.resumeToolAnimation' : 'home.landing.pauseToolAnimation'))
</script>

<style scoped>
.home-brand {
  --brand-name-scale: .9;
  --brand-tool-size: 32px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) var(--brand-tool-size);
  align-items: center;
  width: min(calc(var(--brand-name-width) * var(--brand-name-scale) + var(--brand-tool-size)), 100%);
  min-width: 0;
}
.brand {
  display: block;
  min-width: 0;
  color: var(--cafe-ink);
  animation: sign-arrive 650ms cubic-bezier(.22, 1, .36, 1) both;
}
.brand-stage { display: block; width: 100%; height: auto; }
.brand-fallback { font: 36px Georgia, 'Songti SC', SimSun, serif; }
.brand:focus-visible, .brand-tools:focus-visible { outline: 2px solid var(--cafe-accent); outline-offset: 5px; }
.brand-tools {
  /* Superscript-sized artwork, with a separate, comfortably clickable target. */
  align-self: start;
  transform: translateY(-10px);
  width: var(--brand-tool-size);
  height: var(--brand-tool-size);
  padding: 0;
  border: 0;
  background: none;
  border-radius: 6px;
  color: var(--cafe-ink);
  cursor: pointer;
}
.brand-tools:disabled { cursor: default; }
.home-brand.has-story { display: block; position: relative; width: min(calc(var(--brand-name-width) * var(--brand-name-scale)), 100%); }
.has-story .brand { animation: none; }
/* Just outside the wordmark's top-right corner, clear of the p while it plays pi. */
.has-story .brand-tools { position: absolute; right: -30px; top: -4px; width: 24px; height: 24px; transform: none; opacity: 0; }
.has-story:hover .brand-tools, .has-story:focus-within .brand-tools, .has-story .brand-tools[aria-pressed="true"] { opacity: .65; }
.has-story .brand-tools:disabled { visibility: hidden; }
.story-pause { width: 24px; height: 24px; }
@keyframes sign-arrive {
  from { opacity: .6; transform: translateY(3px); }
  to { opacity: 1; transform: translateY(0); }
}
@media (max-width: 767px) {
  .home-brand { --brand-name-scale: .79; --brand-tool-size: 28px; }
  .brand-tools { transform: translateY(-7px); }
}
@media (max-width: 359px) {
  .home-brand { flex: 0 1 auto; }
}
@media (prefers-reduced-motion: reduce) { .brand { animation: none; } }
</style>
