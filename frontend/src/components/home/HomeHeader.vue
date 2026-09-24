<template>
  <header class="home-header" :class="{ 'header-compact': compact }">
    <div class="header-surface">
      <div class="home-container header-row">
        <HomeBrand :site-name="siteName" />

        <div class="header-navigation hidden lg:grid">
          <nav :class="{ 'navigation-hidden': compact }" :inert="compact || undefined" class="header-menu" :aria-label="t('home.landing.navigation')">
            <RouterLink to="/presale">{{ t('presale.nav') }}</RouterLink>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }} <Icon name="externalLink" size="xs" aria-hidden="true" /></a>
          </nav>
          <RouterLink v-if="presaleOpen" to="/presale" class="presale-header-banner" :class="{ 'navigation-hidden': !compact }" :inert="!compact || undefined"><span class="presale-live-dot" aria-hidden="true" />{{ t('presale.banner') }} <Icon name="arrowRight" size="sm" /></RouterLink>
        </div>

        <div class="header-actions flex shrink-0 items-center gap-1 sm:gap-3">
          <LocaleSwitcher />
          <button
            type="button"
            class="theme-toggle"
            :aria-label="t(isDark ? 'home.switchToLight' : 'home.switchToDark')"
            :title="t(isDark ? 'home.switchToLight' : 'home.switchToDark')"
            @click="emit('toggleTheme')"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
          </button>
          <RouterLink :to="entryPath" class="header-entry">
            {{ t(isAuthenticated ? 'home.dashboard' : 'home.login') }}
            <Icon name="arrowRight" size="sm" class="hidden sm:block" aria-hidden="true" />
          </RouterLink>
        </div>
      </div>
      <nav class="mobile-nav home-container lg:hidden" :aria-label="t('home.landing.navigation')">
        <template v-if="!compact"><RouterLink to="/presale">{{ t('presale.nav') }}</RouterLink><a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a></template>
        <RouterLink v-else-if="presaleOpen" to="/presale" class="presale-mobile-banner"><span class="presale-live-dot" aria-hidden="true" />{{ t('presale.banner') }} →</RouterLink>
      </nav>
    </div>
  </header>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import HomeBrand from './HomeBrand.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  siteName: string
  docUrl: string
  entryPath: string
  isAuthenticated: boolean
  isDark: boolean
  compact: boolean
  presaleOpen: boolean
}>()
const emit = defineEmits<{ toggleTheme: [] }>()
const { t } = useI18n()
</script>

<style scoped>
/* Public pages share one header palette, independent of their content surfaces. */
.home-header {
  --cafe-page: #faf7f2;
  --cafe-page-rgb: 250, 247, 242;
  --cafe-surface: #f2ece3;
  --cafe-ink: #382a20;
  --cafe-muted: #756456;
  --cafe-accent: #865630;
  --cafe-line: #e4d9ca;
  color: var(--cafe-ink);
}
.dark .home-header {
  --cafe-page: #1d1915;
  --cafe-page-rgb: 29, 25, 21;
  --cafe-surface: #27211b;
  --cafe-ink: #f2e9dc;
  --cafe-muted: #b8a99a;
  --cafe-accent: #d8ad7d;
  --cafe-line: #43372c;
}
.home-container { width: min(1280px, calc(100% - 96px)); margin-inline: auto; }
.home-header :is(a, button):focus-visible { outline: 2px solid var(--cafe-accent); outline-offset: 5px; }
.home-header a { transition: color 160ms, background-color 160ms; }
/* Two fixed backdrop planes dissolve; never resize/re-rasterize a moving blur edge. */
.home-header {
  position: sticky;
  display: flow-root;
  top: 0;
  z-index: 40;
  height: var(--slider-header-height, 104px);
  pointer-events: none;
}
.header-surface {
  position: relative;
  isolation: isolate;
  width: 100%;
  pointer-events: auto;
}
.header-surface::before,
.header-surface::after {
  content: '';
  position: absolute;
  z-index: -1;
  pointer-events: none;
  transition: opacity .5s cubic-bezier(.4, 0, .2, 1);
  will-change: opacity;
}
.header-surface::before {
  inset: 0;
  border-bottom: 1px solid var(--cafe-line);
  background: rgba(var(--cafe-page-rgb), .96);
}
.header-surface::after {
  inset: 8px max(24px, calc((100% - 1320px) / 2));
  border: 1px solid transparent;
  border-color: rgba(var(--cafe-page-rgb), .55);
  border-radius: 16px;
  background: rgba(var(--cafe-page-rgb), .64);
  -webkit-backdrop-filter: blur(24px) saturate(1.35);
  backdrop-filter: blur(24px) saturate(1.35);
  box-shadow: 0 3px 10px #23180b17, 0 18px 42px -8px #23180b38, inset 0 1px 0 rgba(var(--cafe-page-rgb), .55);
  opacity: 0;
}
.dark .header-surface::after {
  box-shadow: 0 3px 12px #00000040, 0 18px 44px -8px #00000099, inset 0 1px 0 rgba(var(--cafe-page-rgb), .55);
}
.header-compact .header-surface::before { opacity: 0; }
.header-compact .header-surface::after { opacity: 1; }

.header-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 24px;
  min-height: 88px;
}
.header-navigation { min-width: 240px; align-items: center; }
.header-menu, .presale-header-banner { grid-area: 1 / 1; transition: opacity .32s ease, transform .32s ease; }
.header-menu { display: flex; justify-content: flex-end; align-items: center; gap: 28px; }
.navigation-hidden { opacity: 0; pointer-events: none; transform: translateY(4px); }
.presale-header-banner { justify-self: end; display: inline-flex; align-items: center; justify-content: center; gap: 10px; border: 1px solid var(--cafe-line); background: color-mix(in srgb, var(--cafe-accent) 7%, transparent); border-radius: 999px; padding: 9px 15px; color: var(--cafe-accent); font-size: 12px; }
.presale-live-dot { display: inline-block; width: 5px; height: 5px; background: currentColor; border-radius: 50%; box-shadow: 0 0 0 4px color-mix(in srgb, currentColor 10%, transparent); }
.presale-mobile-banner { display: flex; align-items: center; gap: 10px; }
.header-actions {
  justify-self: end;
}
.home-header nav {
  font-size: 14px;
  color: var(--cafe-muted);
}
.home-header nav a {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.home-header nav a:hover {
  color: var(--cafe-accent);
}
.header-entry {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 40px;
  padding: 8px 16px;
  border: 1px solid var(--cafe-line);
  border-radius: 12px;
  font-size: 13px;
  white-space: nowrap;
}
.header-entry:hover, .theme-toggle:hover {
  background: var(--cafe-surface);
}
.theme-toggle {
  display: grid;
  width: 40px;
  height: 40px;
  place-items: center;
  border-radius: 50%;
  color: var(--cafe-muted);
}
.mobile-nav {
  justify-content: flex-end;
  align-items: center;
  min-height: 28px;
  gap: 24px;
  padding-bottom: 10px;
  line-height: 18px;

}
/* Let Tailwind's lg:hidden control this row at desktop widths. */
@media (max-width: 1023px) { .mobile-nav { display: flex; } }

@media (max-width: 1023px) {
  .home-container { width: calc(100% - 64px); }
  .header-row { grid-template-columns: minmax(0, 1fr) auto; min-height: 76px; }
  .home-header .mobile-nav { font-size: 12px; }
  .header-surface::after { inset: 6px 12px; border-radius: 14px; }
}
@media (max-width: 767px) {
  .home-container { width: calc(100% - 40px); }
  .header-row { gap: 12px; }
  .header-entry { padding-inline: 10px; }
  .theme-toggle { width: 34px; }
}
@media (max-width: 359px) {
  .home-header { height: var(--slider-header-height, 136px); }
  .header-row { display: flex; flex-wrap: wrap; padding-block: 10px; gap: 6px; }
  .header-actions { margin-left: auto; }
}
@media (prefers-reduced-motion: reduce) {
  .home-header *, .header-surface::before, .header-surface::after { transition: none !important; }
}
</style>
