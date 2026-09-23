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
    <!-- Names without the letter story retain decorative, non-interactive marks. -->
    <span v-if="!story" class="brand-tools" aria-hidden="true">
      <HomeToolGlyph :paused="animationPaused" :reduced-motion="reducedMotion === 'reduce'" />
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useDocumentVisibility, usePreferredReducedMotion } from '@vueuse/core'
import { createWordmark } from './brandGeometry'
import HomeToolGlyph from './HomeToolGlyph.vue'
import HomeBrandStory from './HomeBrandStory.vue'
import { createBrandStory } from './brandStory'

const props = defineProps<{ siteName: string }>()
const wordmark = computed(() => createWordmark(props.siteName))
const story = computed(() => createBrandStory(wordmark.value))
const fallbackLength = computed(() => Math.min(222, Array.from(props.siteName).length * 34))
const visibility = useDocumentVisibility()
const reducedMotion = usePreferredReducedMotion()
const animationPaused = computed(() => visibility.value !== 'visible' || reducedMotion.value === 'reduce')
</script>

<style scoped>
.home-brand {
  --brand-name-scale: .7;
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
.brand:focus-visible { outline: 2px solid var(--cafe-accent); outline-offset: 5px; }
.brand-tools {
  /* Superscript-sized decoration; never an extra hover target or tab stop. */
  align-self: start;
  transform: translateY(-10px);
  width: var(--brand-tool-size);
  height: var(--brand-tool-size);
  color: var(--cafe-ink);
  pointer-events: none;
}
.home-brand.has-story { display: block; position: relative; width: min(calc(var(--brand-name-width) * var(--brand-name-scale)), 100%); }
.has-story .brand { animation: none; }
@keyframes sign-arrive {
  from { opacity: .6; transform: translateY(3px); }
  to { opacity: 1; transform: translateY(0); }
}
@media (max-width: 767px) {
  .home-brand { --brand-name-scale: .62; --brand-tool-size: 28px; }
  .brand-tools { transform: translateY(-7px); }
}
@media (max-width: 359px) {
  .home-brand { flex: 0 1 auto; }
}
@media (prefers-reduced-motion: reduce) { .brand { animation: none; } }
</style>
