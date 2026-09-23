<template>
  <div class="presale-benefits" :class="{ compact }">
    <div v-for="(key, index) in keys" :key="key" class="benefit-item">
      <span class="benefit-number">0{{ index + 1 }}</span>
      <div><h3>{{ t(`presale.benefits.${key}.title`) }}</h3><p>{{ t(`presale.benefits.${key}.body`) }}</p></div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ compact?: boolean }>()
const { t } = useI18n()
const keys = ['value', 'pricing', 'service', 'preparation'] as const
</script>
<style scoped>
.presale-benefits { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 36px 48px; }
.benefit-item { display: flex; align-items: baseline; gap: 18px; }
.benefit-number { color: var(--cafe-accent, #ab8754); font: italic 22px Georgia, serif; }
h3 { margin: 0 0 10px; font-size: 16px; font-weight: 500; }
p { color: var(--cafe-muted, #9f9990); font-size: 13px; line-height: 1.85; }
.compact { gap: 16px 28px; }
.compact h3 { font-size: 13px; margin-bottom: 5px; }
.compact p { font-size: 12px; line-height: 1.65; }
.compact .benefit-item { gap: 12px; }
@media(max-width: 600px) { .presale-benefits { grid-template-columns: 1fr; gap: 22px; } }
</style>
