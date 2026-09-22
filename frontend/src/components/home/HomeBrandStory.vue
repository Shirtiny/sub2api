<template>
  <svg ref="stage" class="brand-stage story-stage" :viewBox="story.viewBox" aria-hidden="true" focusable="false">
    <title class="brand-name">{{ siteName }}</title>
    <path v-if="reducedMotion" class="brand-wordmark" :d="story.staticD" fill="currentColor" fill-rule="evenodd" />
    <template v-else>
      <g class="brand-wordmark" fill="currentColor" fill-rule="evenodd">
        <template v-for="letter in wordmark.letters" :key="letter.index">
          <path v-if="letter.index === story.e.index" class="coffee-cup-letter" :d="story.body" />
          <path v-else :d="letter.d" />
        </template>
      </g>

      <!-- The steam simply breathes and sways at the accent; nothing else moves. -->
      <path class="accent-steam" :d="story.steam" fill="currentColor" opacity=".5">
        <animate attributeName="d" :values="story.steamValues" :keyTimes="STEAM.keyTimes" calcMode="spline" :keySplines="STEAM.keySplines" :dur="STORY_DURATION" repeatCount="indefinite" />
        <animate attributeName="opacity" :values="STEAM.opacity.values" :keyTimes="STEAM.opacity.keyTimes" :dur="STORY_DURATION" repeatCount="indefinite" />
      </path>
    </template>
  </svg>
</template>

<script setup lang="ts">
import { ref, watch, watchPostEffect } from 'vue'
import type { Wordmark } from './brandGeometry'
import { STEAM, STORY_DURATION, type BrandStory } from './brandStory'

const props = defineProps<{ wordmark: Wordmark; story: BrandStory; siteName: string; paused: boolean; reducedMotion: boolean }>()
const stage = ref<SVGSVGElement>()
watchPostEffect(() => {
  if (props.paused || props.reducedMotion) stage.value?.pauseAnimations?.()
  else stage.value?.unpauseAnimations?.()
})
// A changed name means a changed accent anchor: begin the new breath from its start.
watch(() => props.story, () => stage.value?.setCurrentTime?.(0), { flush: 'post' })
</script>

<style scoped>
.story-stage { display: block; width: 100%; height: auto; overflow: visible; }
</style>
