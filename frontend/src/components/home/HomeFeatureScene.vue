<template>
  <HomeIntelligenceScene v-if="kind === 'intelligence'" :compact="compact" />
  <HomeSpeedScene v-else-if="kind === 'speed'" :compact="compact" />
  <HomeTrustScene v-else :compact="compact" />
</template>

<script setup lang="ts">
import { useMediaQuery } from '@vueuse/core'
import HomeIntelligenceScene from './HomeIntelligenceScene.vue'
import HomeSpeedScene from './HomeSpeedScene.vue'
import HomeTrustScene from './HomeTrustScene.vue'

defineProps<{ kind: 'intelligence' | 'speed' | 'trust' }>()
const compact = useMediaQuery('(max-width: 639px)')
</script>
