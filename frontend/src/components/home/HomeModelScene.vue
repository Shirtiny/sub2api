<template>
  <div class="celestial-scene" :data-scene="kind" :data-paused="!active || reducedMotion" :data-reduced-motion="reducedMotion" role="img" :aria-label="label">
    <svg class="celestial-svg" viewBox="0 0 320 280" :data-body-radius="bodyRadius" aria-hidden="true">
      <defs>
        <clipPath :id="clipId"><circle :cx="centerX" cy="132" :r="bodyRadius" /></clipPath>
        <radialGradient :id="id('moon')" cx="27%" cy="23%" r="80%">
          <stop offset="0" stop-color="#ece9df" /><stop offset=".53" stop-color="#bdbdb7" /><stop offset="1" stop-color="#687074" />
        </radialGradient>
        <filter v-if="kind === 'luna'" :id="id('lunar-finish')" x="0" y="0" width="100%" height="100%" color-interpolation-filters="sRGB">
          <feTurbulence type="fractalNoise" baseFrequency=".065" numOctaves="3" seed="29" />
          <feColorMatrix type="saturate" values="0" />
          <feComponentTransfer>
            <feFuncR type="table" tableValues=".15 .3 .62 .82 1" />
            <feFuncG type="table" tableValues=".15 .3 .62 .82 1" />
            <feFuncB type="table" tableValues=".15 .3 .62 .82 1" />
          </feComponentTransfer>
          <feComposite in2="SourceGraphic" operator="in" />
        </filter>
        <linearGradient v-if="kind === 'luna'" :id="id('crater-bowl')" x1=".12" y1=".08" x2=".82" y2=".95">
          <stop offset="0" stop-color="#404b48" stop-opacity=".7" />
          <stop offset=".3" stop-color="#72796f" stop-opacity=".52" />
          <stop offset=".64" stop-color="#a1a798" stop-opacity=".3" />
          <stop offset=".9" stop-color="#c6cbbb" stop-opacity=".26" />
          <stop offset="1" stop-color="#dbdfce" stop-opacity=".2" />
        </linearGradient>
        <linearGradient v-if="kind === 'luna'" :id="id('crater-wall')" x1="0" y1="0" x2=".7" y2=".9">
          <stop offset="0" stop-color="#263632" stop-opacity=".55" />
          <stop offset=".6" stop-color="#526259" stop-opacity=".2" />
          <stop offset="1" stop-color="#526259" stop-opacity="0" />
        </linearGradient>
        <filter v-if="kind === 'luna'" :id="id('lunar-rim-soften')" x="-5%" y="-5%" width="110%" height="110%">
          <feGaussianBlur stdDeviation=".65" />
        </filter>
        <radialGradient :id="id('earth')" cx="32%" cy="28%" r="78%">
          <stop offset="0" stop-color="#5a9cc1" /><stop offset=".5" stop-color="#28658f" /><stop offset="1" stop-color="#071929" />
        </radialGradient>
        <linearGradient :id="id('land')" x1="0" y1="0" x2=".8" y2="1">
          <stop offset="0" stop-color="#bcc9b1" /><stop offset=".5" stop-color="#8da98f" /><stop offset="1" stop-color="#638c7c" />
        </linearGradient>
        <linearGradient :id="id('night')" x1="0" x2="1">
          <stop offset=".22" stop-color="#03080d" stop-opacity="0" /><stop offset=".7" stop-color="#03080d" stop-opacity=".3" /><stop offset="1" stop-color="#03080d" stop-opacity=".97" />
        </linearGradient>
        <radialGradient :id="id('sun')" cx="43%" cy="45%" r="58%">
          <stop offset="0" stop-color="#d95d10" /><stop offset=".55" stop-color="#f89420" /><stop offset=".82" stop-color="#ffbf48" /><stop offset="1" stop-color="#ffe29a" />
        </radialGradient>
        <radialGradient :id="id('solar-limb')">
          <stop offset=".64" stop-color="#ff6b16" stop-opacity="0" /><stop offset=".91" stop-color="#ff9a32" stop-opacity=".1" />
          <stop offset=".975" stop-color="#ffe0a0" stop-opacity=".8" /><stop offset="1" stop-color="#ffad48" stop-opacity=".9" />
        </radialGradient>
        <radialGradient :id="id('corona')">
          <stop offset=".53" stop-color="#ff9c32" stop-opacity=".6" /><stop offset=".68" stop-color="#ed5413" stop-opacity=".28" /><stop offset="1" stop-color="#b12d0b" stop-opacity="0" />
        </radialGradient>
        <radialGradient :id="id('solar-flame')">
          <stop offset=".76" stop-color="#ffe1a3" /><stop offset=".86" stop-color="#ff9a32" stop-opacity=".8" /><stop offset="1" stop-color="#e3450c" stop-opacity="0" />
        </radialGradient>
        <!-- Static fine-grain SVG texture; projected plasma patches above it move around the sphere. -->
        <filter :id="id('plasma')" x="0" y="0" width="100%" height="100%" color-interpolation-filters="sRGB">
          <feTurbulence type="fractalNoise" baseFrequency=".09" numOctaves="4" seed="17" />
          <feColorMatrix type="saturate" values="0" />
          <feComponentTransfer>
            <feFuncR type="table" tableValues=".35 .75 1 1 1" />
            <feFuncG type="table" tableValues=".025 .16 .52 .82 .98" />
            <feFuncB type="table" tableValues="0 .01 .055 .28 .65" />
          </feComponentTransfer>
          <feComposite in2="SourceGraphic" operator="in" />
        </filter>
        <filter :id="id('solar-haze')" x="-20%" y="-20%" width="140%" height="140%">
          <feGaussianBlur stdDeviation="2" />
        </filter>
        <radialGradient :id="id('core')">
          <stop offset="0" stop-color="#fff9e9" stop-opacity=".9" /><stop offset=".17" stop-color="#c9e6f8" stop-opacity=".48" /><stop offset="1" stop-color="#6893b9" stop-opacity="0" />
        </radialGradient>
      </defs>
      <g class="star-field" fill="#a9c8db">
        <circle v-for="(star, i) in stars" :key="i" :cx="star.x" :cy="star.y" :r="star.r" :opacity="star.opacity" />
      </g>

      <template v-if="kind === 'luna'">
        <circle class="planet-body" :cx="centerX" cy="132" :r="bodyRadius" :fill="`url(#${id('moon')})`" />
        <g :clip-path="`url(#${clipId})`">
          <g :transform="`translate(${centerX} 132) scale(${bodyRadius / 100})`">
            <g class="lunar-surface sphere-surface">
              <g v-for="(crater, index) in lunarCraters" :key="index" class="lunar-crater">
                <path class="lunar-crater-floors" :d="crater.bowl" :fill="`url(#${id('crater-bowl')})`" />
                <path class="lunar-crater-basin" :d="crater.floor" fill="#adb3a5" opacity=".32" :filter="`url(#${id('lunar-rim-soften')})`" />
                <path class="lunar-crater-shadows" :d="crater.wall" :fill="`url(#${id('crater-wall')})`" :filter="`url(#${id('lunar-rim-soften')})`" />
                <path class="lunar-crater-terrace" :d="crater.terrace" fill="#4d584f" opacity=".26" />
                <path v-if="crater.peakShadow" class="lunar-crater-peak-shadow" :d="crater.peakShadow" fill="#546054" opacity=".5" />
                <path v-if="crater.peakLight" class="lunar-crater-peak-light" :d="crater.peakLight" fill="#e8e7d7" opacity=".65" />
                <path class="lunar-crater-rims" :d="crater.rim" fill="#eeeede" opacity=".48" :filter="`url(#${id('lunar-rim-soften')})`" />
              </g>
              <circle class="lunar-finish" r="100" fill="#aaa9a3" opacity=".12" :filter="`url(#${id('lunar-finish')})`" />
            </g>
          </g>
          <circle class="lunar-shadow" :cx="centerX" cy="132" :r="bodyRadius + 1" :fill="`url(#${id('night')})`" />
        </g>
        <path class="lunar-rim" :d="`M${centerX - 18} 113a26 26 0 0 1 26-6`" fill="none" stroke="#dbe0db" stroke-width=".6" opacity=".7" />
      </template>

      <template v-else-if="kind === 'terra'">
        <circle :cx="centerX" cy="132" :r="bodyRadius + 2" fill="none" stroke="#7cc4e7" stroke-width="2" opacity=".23" />
        <circle class="planet-body" :cx="centerX" cy="132" :r="bodyRadius" :fill="`url(#${id('earth')})`" />
        <g :clip-path="`url(#${clipId})`">
          <g :transform="`translate(${centerX} 132) scale(${bodyRadius / 100})`">
            <g class="terra-drift sphere-surface" :fill="`url(#${id('land')})`">
              <path :d="spherePaths[0]" />
            </g>
            <g class="terra-clouds sphere-surface" fill="#f0f4ee">
              <path class="cloud-veil" :d="spherePaths[1]" opacity=".24" />
              <path class="cloud-clusters" :d="spherePaths[2]" opacity=".66" />
            </g>
          </g>
          <circle :cx="centerX" cy="132" :r="bodyRadius + 1" :fill="`url(#${id('night')})`" />
        </g>
        <path :d="`M${centerX - 35} 99a48 48 0 0 1 55-10`" fill="none" stroke="#bde9fa" stroke-width="1" opacity=".65" />
      </template>

      <template v-else-if="kind === 'sol'">
        <circle class="motion solar-corona" :cx="centerX" cy="132" r="126" :fill="`url(#${id('corona')})`" />
        <g :transform="`translate(${centerX} 132)`">
          <g class="motion solar-prominences" :fill="`url(#${id('solar-flame')})`">
            <path :d="solarCrown" :filter="`url(#${id('solar-haze')})`" />
            <path :d="solarCrown" opacity=".6" />
            <path transform="scale(.85)" opacity=".7" d="M53-58C66-76 91-80 95-62 97-50 83-39 73-31 86-49 94-57 87-64 78-72 65-64 58-53Z" />
          </g>
        </g>
        <circle class="planet-body" :cx="centerX" cy="132" :r="bodyRadius" :fill="`url(#${id('sun')})`" />
        <g :clip-path="`url(#${clipId})`">
          <g :transform="`translate(${centerX} 132)`">
            <circle class="solar-plasma" r="88" fill="#e76116" :filter="`url(#${id('plasma')})`" />
            <g class="solar-surface sphere-surface" transform="scale(.8)">
              <path :d="spherePaths[0]" fill="#a93404" opacity=".2" :filter="`url(#${id('solar-haze')})`" />
              <path :d="spherePaths[1]" fill="#fff0b3" opacity=".25" :filter="`url(#${id('solar-haze')})`" />
            </g>
          </g>
          <circle :cx="centerX" cy="132" :r="bodyRadius" :fill="`url(#${id('solar-limb')})`" />
        </g>
      </template>

      <template v-else>
        <circle class="motion galaxy-glow" cx="160" cy="132" r="55" :fill="`url(#${id('core')})`" />
        <g class="motion galaxy-arms">
          <circle v-for="(star, i) in galaxyStars" :key="i" :cx="star.x" :cy="star.y" :r="star.r" :fill="i % 7 ? '#bddbea' : '#fff5d7'" :opacity="star.opacity" />
        </g>
        <g class="motion galaxy-sparkles" stroke="#d9effb" stroke-width=".7" stroke-linecap="round" opacity=".7">
          <path d="M216 103h7m-3.5-3.5v7M119 166h6m-3-3v6M170 79h5m-2.5-2.5v5" />
        </g>
        <circle cx="160" cy="132" r="2.5" fill="#fff6e4" />
      </template>
    </svg>
  </div>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance, onMounted, ref, watch } from 'vue'
import { useRafFn } from '@vueuse/core'
import { createLunarCraterProjector, createSphereProjector } from './sphereSurface'

type CelestialKind = 'luna' | 'terra' | 'sol' | 'astra'
const props = defineProps<{ kind: CelestialKind; label: string; active: boolean; reducedMotion: boolean }>()
// Shared scale preserves the requested size hierarchy; these are illustrative sizes.
const bodyRadius = computed(() => ({ luna: 26, terra: 48, sol: 80, astra: 112 })[props.kind])
const centerX = 160
const instanceId = getCurrentInstance()!.uid
const id = (name: string) => `celestial-${name}-${instanceId}`
const clipId = id('clip')
const angle = ref(0)
const projectLunarCraters = createLunarCraterProjector()
const lunarCraters = computed(() => props.kind === 'luna' ? projectLunarCraters(angle.value) : [])
const projector = computed(() => props.kind === 'astra' ? null : createSphereProjector(props.kind))
const spherePaths = computed(() => projector.value?.(angle.value) ?? [])
let pendingTime = 0
const { pause, resume } = useRafFn(({ delta }) => {
  pendingTime += Math.min(delta, 100)
  if (pendingTime < 1000 / 24) return
  const period = props.kind === 'luna' ? 96 : props.kind === 'terra' ? 64 : 80
  angle.value = (angle.value + pendingTime * 360 / (period * 1000)) % 18000
  pendingTime = 0
}, { immediate: false })
const running = computed(() => props.active && !props.reducedMotion && props.kind !== 'astra')
function syncMotion() {
  pendingTime = 0
  if (running.value) resume()
  else pause()
}
onMounted(syncMotion)
watch(running, syncMotion)

let seed = 41
const random = () => { seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0; return seed / 4294967296 }
const stars = Array.from({ length: 46 }, () => ({ x: 10 + random() * 300, y: 16 + random() * 232, r: .3 + random() * .6, opacity: .12 + random() * .35 }))
const galaxyStars = Array.from({ length: 720 }, (_, i) => {
  const radius = 8 + Math.pow(random(), .72) * 104
  const angle = (i % 4) * Math.PI / 2 + radius * .045 + (random() - .5) * .48
  const x = Math.cos(angle) * radius, y = Math.sin(angle) * radius + (random() - .5) * 3
  return { x: 160 + x * .97 - y * .24, y: 132 + x * .24 + y * .97, r: .35 + random() * .72, opacity: .3 + random() * .65 }
})
// An uneven filled corona silhouette, not repeated triangular icon rays.
const solarCrown = Array.from({ length: 240 }, (_, i) => {
  const angle = i / 240 * Math.PI * 2
  const radius = 83 + Math.pow(random(), 2) * 14 + Math.sin(angle * 7) * 2
  return `${i ? 'L' : 'M'}${(Math.cos(angle) * radius).toFixed(2)} ${(Math.sin(angle) * radius).toFixed(2)}`
}).join('') + 'Z'

</script>

<style scoped>
.celestial-scene { width: 100%; aspect-ratio: 8 / 7; overflow: hidden; background: radial-gradient(ellipse at 30% 42%, #111b22, #05090e 85%); }
.celestial-svg { display: block; width: 100%; height: 100%; }
.motion { transform-origin: 160px 132px; }
.solar-corona { transform-origin: 160px 132px; animation: solar-breathe 7s ease-in-out infinite; }
.solar-prominences { transform-origin: 0 0; animation: prominence-flow 9s ease-in-out infinite; }
.galaxy-arms { animation: galaxy-turn 240s linear infinite; }
.galaxy-glow, .galaxy-sparkles { animation: stellar-glow 28s ease-in-out infinite; }
.celestial-scene[data-paused='true'] .motion { animation-play-state: paused; }
.celestial-scene[data-reduced-motion='true'] .motion { animation: none; }
@keyframes solar-breathe { 0%, 100% { transform: scale(.94); opacity: .75; } 50% { transform: scale(1.015); opacity: 1; } }
@keyframes prominence-flow { 0%, 100% { opacity: .6; transform: scale(.96); } 50% { opacity: 1; transform: scale(1.015); } }
@keyframes galaxy-turn { to { transform: rotate(360deg); } }
@keyframes stellar-glow { 0%, 100% { opacity: .86; } 50% { opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .motion { animation: none; } }
</style>
