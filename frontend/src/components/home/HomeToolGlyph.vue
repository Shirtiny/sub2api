<template>
  <svg ref="stage" class="tool-stage" viewBox="0 0 64 64" aria-hidden="true" focusable="false">
    <!-- Reduced motion is a finished mark, not a frozen half-drawn frame. -->
    <g v-if="reducedMotion" transform="translate(12 12) scale(1.66667)">
      <path :d="codexPath" fill="currentColor" fill-rule="evenodd" />
    </g>
    <template v-else>
      <g class="codex-drawing" transform="translate(12 12) scale(1.66667)">
        <path class="codex-ink" :d="codexPath" fill="currentColor" fill-rule="evenodd" fill-opacity="0">
          <animate attributeName="fill-opacity" values="0;0;1;1;0;0" keyTimes="0;.075;.13;.40;.465;1" dur="16s" repeatCount="indefinite" />
        </path>
        <path class="codex-contour" :d="codexContour" fill="none" stroke="currentColor" stroke-width=".7" stroke-linecap="round" pathLength="1" stroke-dasharray="1" stroke-dashoffset="1">
          <animate attributeName="stroke-dashoffset" values="1;0;0;-1;-1" keyTimes="0;.07;.43;.54;1" dur="16s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="1;1;0;0;1;1;0;0" keyTimes="0;.08;.13;.39;.44;.51;.55;1" dur="16s" repeatCount="indefinite" />
        </path>
        <!-- Draw the terminal face before the negative-space ink version settles. -->
        <path class="codex-face" d="M6.55 8.73 8.25 11.69 6.55 14.54" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" pathLength="1" stroke-dasharray="1" stroke-dashoffset="1">
          <animate attributeName="stroke-dashoffset" values="1;1;0;0;-1;-1" keyTimes="0;.035;.08;.435;.51;1" dur="16s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="1;1;0;0;1;1;0;0" keyTimes="0;.08;.12;.41;.465;.50;.54;1" dur="16s" repeatCount="indefinite" />
        </path>
        <path class="codex-cursor" d="M12.73 15.395h4.848" fill="none" stroke="currentColor" stroke-width="1.695" stroke-linecap="round" pathLength="1" stroke-dasharray="1" stroke-dashoffset="1">
          <animate attributeName="stroke-dashoffset" values="1;1;0;0;-1;-1" keyTimes="0;.06;.10;.435;.51;1" dur="16s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="1;1;0;0;1;1;0;0" keyTimes="0;.09;.13;.41;.465;.50;.54;1" dur="16s" repeatCount="indefinite" />
        </path>
      </g>
      <g class="pi-drawing" transform="translate(12 12) scale(.0852) translate(-165.29 -165.29)">
        <path
          v-for="(tile, index) in piTiles" :key="tile.color"
          class="pi-tile" :d="tile.d" fill="currentColor" stroke="currentColor"
          fill-opacity="0" stroke-width="9" stroke-linejoin="round"
          pathLength="1" stroke-dasharray="1" stroke-dashoffset="1" opacity="0"
        >
          <animate attributeName="stroke-dashoffset" values="1;0;0;-1;-1" keyTimes="0;.07;.43;.54;1" :begin="`${-8 + index * .14}s`" dur="16s" repeatCount="indefinite" />
          <animate attributeName="fill-opacity" values="0;0;1;1;0;0" keyTimes="0;.04;.11;.40;.465;1" :begin="`${-8 + index * .14}s`" dur="16s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="1;1;0;0" keyTimes="0;.48;.55;1" :begin="`${-8 + index * .14}s`" dur="16s" repeatCount="indefinite" />
          <animateTransform attributeName="transform" type="translate" values="0 20;0 0;0 0;0 -12;0 -12" keyTimes="0;.09;.43;.55;1" :begin="`${-8 + index * .14}s`" dur="16s" repeatCount="indefinite" />
        </path>
      </g>
    </template>
  </svg>
</template>

<script setup lang="ts">
import { ref, watchPostEffect } from 'vue'
import { codexContour, codexPath, piTiles } from './toolArtwork'

const props = defineProps<{ paused: boolean; reducedMotion: boolean }>()
const stage = ref<SVGSVGElement>()

// One native SVG clock keeps all strokes/tiles synchronized. No JS frame timers.
watchPostEffect(() => {
  if (props.paused || props.reducedMotion) stage.value?.pauseAnimations?.()
  else stage.value?.unpauseAnimations?.()
})
</script>

<style scoped>
.tool-stage { display: block; width: 100%; height: 100%; overflow: visible; }
</style>
