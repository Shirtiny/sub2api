<template>
  <!-- Preserve administrator-configured homepages. -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent"
      :title="siteName"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML content is an administrator-only setting, as in the original homepage. -->
    <div v-else v-html="homeContent"></div>
  </div>

  <HomeSectionSlider v-else class="cafe-home" :slides="slides">
    <template #header="{ compact }">
      <a href="#home-main" class="skip-link">{{ t('home.landing.skipToContent') }}</a>

      <header class="home-header" :class="{ 'header-compact': compact }">
        <div class="header-surface">
          <div class="home-container header-row">
            <HomeBrand :site-name="siteName" />

            <nav v-if="docUrl" class="hidden items-center gap-7 lg:flex" :aria-label="t('home.landing.navigation')">
              <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">
                {{ t('home.docs') }} <Icon name="externalLink" size="xs" aria-hidden="true" />
              </a>
            </nav>

            <div class="header-actions flex shrink-0 items-center gap-1 sm:gap-3">
              <LocaleSwitcher />
              <button
                type="button"
                class="theme-toggle"
                :aria-label="t(isDark ? 'home.switchToLight' : 'home.switchToDark')"
                :title="t(isDark ? 'home.switchToLight' : 'home.switchToDark')"
                @click="toggleTheme"
              >
                <Icon :name="isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
              </button>
              <RouterLink :to="entryPath" class="header-entry">
                {{ t(isAuthenticated ? 'home.dashboard' : 'home.login') }}
                <Icon name="arrowRight" size="sm" class="hidden sm:block" aria-hidden="true" />
              </RouterLink>
            </div>
          </div>
          <nav v-if="docUrl" class="mobile-nav home-container lg:hidden" :aria-label="t('home.landing.navigation')">
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
          </nav>
        </div>
      </header>
    </template>
    <div class="home-slide slide-welcome" data-home-slide="welcome">
      <div class="slide-content">
        <section id="welcome" class="hero" aria-labelledby="hero-title">
          <div class="hero-art" aria-hidden="true">
            <img :src="cafeBanner" alt="" width="2560" height="1024" fetchpriority="high" />
          </div>
          <div class="home-container hero-inner">
            <div class="hero-copy">
              <p class="eyebrow flex items-center gap-3" data-reveal>
                <span class="eyebrow-line" aria-hidden="true"></span>
                {{ t('home.landing.eyebrow') }}
              </p>
              <h1 id="hero-title" data-reveal style="--reveal-delay: 110ms">
                {{ t('home.landing.heroLineOne') }}
                <span>{{ t('home.landing.heroLineTwo') }}</span>
              </h1>
              <div class="hero-actions flex flex-wrap gap-3">
                <RouterLink :to="entryPath" class="btn btn-primary cafe-button cafe-button-primary" data-reveal style="--reveal-delay: 230ms">
                  {{ t(isAuthenticated ? 'home.goToDashboard' : 'home.getStarted') }}
                  <Icon name="arrowRight" size="md" aria-hidden="true" />
                </RouterLink>
                <button type="button" class="cafe-button cafe-button-secondary" data-reveal style="--reveal-delay: 330ms" @click="showWaitlist = true">
                  <Icon name="mail" size="md" aria-hidden="true" />
                  {{ t('home.landing.waitlist.button') }}
                </button>
              </div>
              <p class="hero-note" data-reveal style="--reveal-delay: 430ms">{{ t('home.landing.heroNote') }}</p>
            </div>
            <p class="hero-caption" aria-hidden="true">{{ t('home.landing.artCaption') }}</p>
          </div>
        </section>

        <div data-reveal style="--reveal-delay: 520ms" class="home-container tool-note" role="group" :aria-label="t('home.landing.creativeTools')">
          <span class="tool-note-label">{{ t('home.landing.creativeTools') }}</span>
          <ul class="tool-marks">
            <li class="tool-mark">
              <img :src="codexMark" class="tool-icon tool-icon-codex" alt="" width="22" height="22" />
              <span>Codex</span>
            </li>
            <li class="tool-mark">
              <img :src="piMark" class="tool-icon tool-icon-pi" alt="" width="22" height="22" />
              <span>pi</span>
            </li>
          </ul>
          <span class="tool-note-rule" aria-hidden="true"></span>
        </div>
      </div>
    </div>

    <div class="home-slide slide-possibilities" data-home-slide="possibilities">
      <div class="slide-content">
        <section id="possibilities" class="home-container possibilities" aria-labelledby="possibilities-title">
          <div class="section-heading flex flex-col justify-between gap-5 md:flex-row md:items-end">
            <div>
              <p class="eyebrow" data-reveal>{{ t('home.landing.possibilitiesEyebrow') }}</p>
              <h2 id="possibilities-title" data-reveal style="--reveal-delay: 110ms">{{ t('home.landing.possibilitiesTitle') }}</h2>
            </div>
            <p class="section-intro" data-reveal style="--reveal-delay: 210ms">{{ t('home.landing.possibilitiesDescription') }}</p>
          </div>
          <HomeFeatureCarousel />
        </section>
      </div>
    </div>

    <div class="home-slide slide-supported-models" data-home-slide="supported-models">
      <div class="slide-content">
        <HomeModelShowcase />
      </div>
    </div>

    <div class="home-slide slide-billing" data-home-slide="billing">
      <div class="slide-content">
        <HomeBillingSection :entry-path="entryPath" />
      </div>
    </div>

    <div class="home-slide slide-questions" data-home-slide="questions">
      <div class="slide-content">
        <section id="questions" class="home-container questions" aria-labelledby="questions-title">
          <div class="policy-heading">
            <div>
              <p class="eyebrow" data-reveal>{{ t('home.landing.faqEyebrow') }}</p>
              <h2 id="questions-title" data-reveal style="--reveal-delay: 110ms">{{ t('home.landing.faqTitle') }}</h2>
            </div>
            <p class="section-intro" data-reveal style="--reveal-delay: 210ms">{{ t('home.landing.faqDescription') }}</p>
          </div>
          <section id="getting-started" class="quick-guide" aria-labelledby="getting-started-title">
            <div class="quick-guide-header">
              <div class="quick-guide-title" data-reveal style="--reveal-delay: 240ms">
                <h3 id="getting-started-title">{{ t('home.landing.quickStart') }}</h3>
                <p class="eyebrow">{{ t('home.landing.guideEyebrow') }}</p>
              </div>
              <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="text-link" data-reveal style="--reveal-delay: 430ms">
                {{ t('home.viewDocs') }} <Icon name="externalLink" size="sm" aria-hidden="true" />
              </a>
            </div>
            <ol class="quick-steps">
              <li v-for="(step, index) in steps" :key="step" data-reveal :style="{ '--reveal-delay': `${280 + index * 60}ms` }">
                <span aria-hidden="true">0{{ index + 1 }}</span>
                <RouterLink v-if="index === 0" :to="entryPath">{{ t('home.landing.steps.account.title') }}</RouterLink>
                <p v-else>{{ t(`home.landing.steps.${step}.title`) }}</p>
              </li>
            </ol>
          </section>
          <article data-reveal style="--reveal-delay: 400ms" class="policy-card policy-notice" aria-labelledby="policy-usage-title">
            <Icon name="shield" size="xl" class="accent shrink-0" aria-hidden="true" />
            <div class="policy-notice-body">
              <div class="policy-notice-heading">
                <h3 id="policy-usage-title">{{ t('home.landing.questions.service.title') }}</h3>
                <strong class="policy-warning">{{ t('home.landing.policy.warning') }}</strong>
              </div>
              <p>{{ t('home.landing.questions.service.answer') }}</p>
            </div>
          </article>
          <div class="policy-grid">
            <article class="policy-card" aria-labelledby="policy-clients-title" data-reveal style="--reveal-delay: 480ms">
              <p class="policy-kicker" aria-hidden="true">01 / CLIENTS</p>
              <h3 id="policy-clients-title">{{ t('home.landing.questions.models.title') }}</h3>
              <div class="policy-clients">
                <a href="https://openai.com/codex/" target="_blank" rel="noopener noreferrer">
                  <img :src="codexMark" alt="" width="24" height="24" />Codex
                </a>
                <a href="https://pi.dev/" target="_blank" rel="noopener noreferrer">
                  <img :src="piMark" alt="" width="24" height="24" />Pi
                </a>
              </div>
              <p>{{ t('home.landing.questions.models.answer') }}</p>
            </article>
            <article class="policy-card policy-requests" aria-labelledby="policy-requests-title" data-reveal style="--reveal-delay: 560ms">
              <p class="policy-kicker" aria-hidden="true">02 / REQUEST CONTROL</p>
              <h3 id="policy-requests-title">{{ t('home.landing.requests.title') }}</h3>
              <p class="request-summary">{{ t('home.landing.requests.description') }}</p>
              <dl class="request-retention">
                <div v-for="rule in retentionRules" :key="rule">
                  <dt>{{ t(`home.landing.requests.${rule}.title`) }}</dt>
                  <dd>{{ t(`home.landing.requests.${rule}.retention`) }}</dd>
                </div>
              </dl>
            </article>
          </div>
          <div id="next-step" class="closing" role="group" aria-labelledby="closing-title">
            <div class="closing-inner flex flex-col justify-between gap-7 sm:flex-row sm:items-center">
              <div>
                <p class="eyebrow" data-reveal>{{ t('home.landing.closingEyebrow') }}</p>
                <h2 id="closing-title" data-reveal style="--reveal-delay: 110ms">{{ t('home.landing.closingTitle') }}</h2>
                <p class="mt-3" data-reveal style="--reveal-delay: 210ms">{{ t('home.landing.closingDescription') }}</p>
              </div>
              <RouterLink :to="entryPath" class="btn btn-primary cafe-button cafe-button-primary shrink-0 self-start sm:self-auto" data-reveal style="--reveal-delay: 350ms">
                {{ t(isAuthenticated ? 'home.goToDashboard' : 'home.getStarted') }}
                <Icon name="arrowRight" size="md" aria-hidden="true" />
              </RouterLink>
            </div>
          </div>
        </section>
        <footer data-reveal style="--reveal-delay: 470ms" class="home-container home-footer flex flex-col justify-between gap-5 sm:flex-row sm:items-center">
          <p>&copy; {{ currentYear }} {{ siteName }}</p>
          <div class="flex flex-wrap items-center gap-x-6 gap-y-3">
            <a href="#getting-started">{{ t('home.landing.quickStart') }}</a>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
            <RouterLink v-for="document in legalDocuments" :key="document.id" :to="`/legal/${encodeURIComponent(document.id)}`">
              {{ document.title }}
            </RouterLink>
          </div>
        </footer>
      </div>
    </div>

  </HomeSectionSlider>
  <HomeWaitlistDialog v-if="showWaitlist && !homeContent" :is-dark="isDark" @close="showWaitlist = false" />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import HomeBrand from '@/components/home/HomeBrand.vue'
import HomeWaitlistDialog from '@/components/home/HomeWaitlistDialog.vue'
import { initializeTheme } from '@/utils/theme'
import HomeBillingSection from '@/components/home/HomeBillingSection.vue'
import HomeFeatureCarousel from '@/components/home/HomeFeatureCarousel.vue'
import HomeModelShowcase from '@/components/home/HomeModelShowcase.vue'
import HomeSectionSlider from '@/components/home/HomeSectionSlider.vue'
import Icon from '@/components/icons/Icon.vue'
import cafeBanner from '@/assets/home/cafe-banner.webp'
import codexMark from '@/assets/home/codex-mark.svg'
import piMark from '@/assets/home/pi-mark.svg'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '', { allowRelative: true }))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content?.trim() || '')
const isHomeContentUrl = computed(() => /^https?:\/\//i.test(homeContent.value))
const legalDocuments = computed(() => appStore.cachedPublicSettings?.login_agreement_enabled
  ? appStore.cachedPublicSettings.login_agreement_documents || []
  : [])

const isAuthenticated = computed(() => authStore.isAuthenticated)
const entryPath = computed(() => !isAuthenticated.value ? '/login' : authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
// Share the same explicit preference / dark default as the console and bootstrap.
const isDark = ref(initializeTheme())
const showWaitlist = ref(false)
const currentYear = new Date().getFullYear()

const steps = ['account', 'key', 'configure'] as const
const retentionRules = ['audit', 'exceptions'] as const
const slides = computed(() => [
  { id: 'welcome', label: t('home.landing.slides.welcome') },
  { id: 'possibilities', label: t('home.landing.slides.everyday') },
  { id: 'supported-models', label: t('home.landing.models.title') },
  { id: 'billing', label: t('home.landing.billing.title') },
  { id: 'questions', label: t('home.landing.faqDescription') }
])

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    void appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
/* A page-local palette: the dashboard and other routes keep their existing theme. */
.cafe-home {
  --chapter-space: clamp(80px, 10vh, 128px);
  --chapter-gap: clamp(40px, 5vh, 64px);
  --cafe-page: #faf7f2;
  --cafe-page-rgb: 250, 247, 242;
  --cafe-surface: #f2ece3;
  --cafe-ink: #382a20;
  --cafe-muted: #756456;
  --cafe-accent: #865630;
  --cafe-line: #e4d9ca;
  min-height: 0;
  background: var(--cafe-page);
  color: var(--cafe-ink);
}

.dark .cafe-home {
  --cafe-page: #1d1915;
  --cafe-page-rgb: 29, 25, 21;
  --cafe-surface: #27211b;
  --cafe-ink: #f2e9dc;
  --cafe-muted: #b8a99a;
  --cafe-accent: #d8ad7d;
  --cafe-line: #43372c;
}

.home-container {
  width: min(1280px, calc(100% - 96px));
  margin-inline: auto;
}
.cafe-home :is(a, button, summary):focus-visible {
  outline: 2px solid var(--cafe-accent);
  outline-offset: 5px;
}
.cafe-home :is(section, main)[id] {
  scroll-margin-top: 0;
}
.cafe-home :is(h1, h2) {
  font-family: Georgia, 'Noto Serif CJK SC', 'Songti SC', SimSun, serif;
  font-weight: 500;
  text-wrap: balance;
}
.cafe-home h2 {
  margin-top: 14px;
  font-size: clamp(27px, 3vw, 36px);
  line-height: 1.5;
}
.cafe-home h3 {
  font-size: 18px;
  font-weight: 600;
  line-height: 1.6;
}
.cafe-home p {
  overflow-wrap: anywhere;
}
.cafe-home a {
  transition: color 160ms, background-color 160ms;
}
/* Two fixed backdrop planes dissolve; never resize/re-rasterize a moving blur edge. */
.home-header {
  position: sticky;
  display: flow-root;
  top: 0;
  z-index: 40;
  height: var(--slider-header-height);
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
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  gap: 24px;
  min-height: 88px;
}
.header-actions {
  grid-column: -2;
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
.home-header nav a:hover, .home-footer a:hover {
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
  gap: 26px;
  padding-block: 0 16px;
  flex-wrap: wrap;

}
/* Let Tailwind's lg:hidden control this row at desktop widths. */
@media (max-width: 1023px) { .mobile-nav { display: flex; } }

.hero {
  position: relative;
  isolation: isolate;
}
.hero-art {
  position: absolute;
  inset: 0;
  z-index: -1;
  overflow: hidden;
  background: var(--cafe-surface);
}
.hero-art img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: 72% center;
}
.hero-art::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(90deg, var(--cafe-page) 0%, rgba(var(--cafe-page-rgb), .98) 24%, rgba(var(--cafe-page-rgb), .88) 39%, rgba(var(--cafe-page-rgb), .34) 55%, transparent 72%), linear-gradient(0deg, var(--cafe-page), transparent 19%);
}
.hero-inner {
  position: relative;
  display: flex;
  align-items: center;
  min-height: 640px;
  padding-block: 72px 96px;
}
.hero-copy {
  width: 57%;
  max-width: 670px;
}
.eyebrow {
  color: var(--cafe-accent);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: .18em;
  line-height: 1.8;
  text-transform: uppercase;
}
.eyebrow-line {
  display: inline-block;
  width: 28px;
  height: 1px;
  background: currentColor;
}
.hero h1 {
  margin-block: 27px 22px;
  font-size: clamp(38px, 4.2vw, 60px);
  line-height: 1.4;
  letter-spacing: -.04em;
}
.hero h1 span {
  display: block;
  color: var(--cafe-accent);
}
.hero-actions {
  margin-top: 30px;
}
.cafe-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 20px;
  min-height: 50px;
  padding: 12px 24px;
  border: 1px solid transparent;
  border-radius: 6px;
  font-size: 14px;
  font-weight: 500;
}
.cafe-button-secondary {
  border-color: var(--cafe-line);
  background: rgba(var(--cafe-page-rgb), .7);
}
.cafe-button-secondary:hover {
  background: var(--cafe-surface);
}
.hero-note {
  margin-top: 20px;
  font-size: 12px;
  color: var(--cafe-muted);
}
.hero-caption {
  position: absolute;
  bottom: 51px;
  right: 0;
  color: #fff3dc;
  text-shadow: 0 1px 8px #281a0e;
  font: italic 17px Georgia, serif;
}

.tool-note { display: flex; align-items: center; gap: 28px; padding-block: 16px 12px; }
.tool-note-label { color: var(--cafe-muted); font-size: 10px; letter-spacing: .12em; text-transform: uppercase; white-space: nowrap; }
.tool-marks { display: flex; align-items: center; gap: 26px; flex-shrink: 0; }
.tool-mark { display: flex; align-items: center; gap: 8px; color: var(--cafe-muted); font-size: 13px; }
.tool-icon { display: block; width: 22px; height: 22px; }
.tool-icon-codex { opacity: .8; }
.dark .tool-icon-codex { filter: invert(1); }
.tool-icon-pi { transform: scale(1.55); }
.tool-note-rule { flex: 1; height: 1px; background: var(--cafe-line); }
@media (max-width: 767px) {
  .tool-note { gap: 18px; padding-block: 18px 4px; }
  .tool-note-label { font-size: 9px; letter-spacing: .08em; }
  .tool-marks { gap: 18px; }
  .tool-mark { gap: 6px; font-size: 12px; }
  .tool-icon { width: 20px; height: 20px; }
}

.possibilities {
  padding-block: var(--chapter-space);
}
.possibilities .section-heading { margin-bottom: var(--chapter-gap); }
.section-heading {
  margin-bottom: 40px;
}
.section-intro {
  max-width: 360px;
  color: var(--cafe-muted);
  font-size: 14px;
  line-height: 1.9;
}
.accent { color: var(--cafe-accent); }

.text-link {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  color: var(--cafe-accent);
  font-size: 13px;
  font-weight: 600;
}
.text-link:hover {
  text-decoration: underline;
  text-underline-offset: 5px;
}
.quick-guide {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 14px;
  margin-bottom: clamp(24px, 3dvh, 32px);
  padding-block: 16px;
  border-block: 1px solid var(--cafe-line);
}
.quick-guide-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px 24px; }
.quick-guide-title { display: flex; align-items: baseline; flex-wrap: wrap; gap: 6px 14px; }
.quick-guide h3 { margin: 0; font-size: 15px; }
.quick-guide .eyebrow { font-size: 10px; }
.quick-steps { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; }
.quick-steps li { display: flex; align-items: baseline; gap: 10px; min-width: 0; font-size: 13px; }
.quick-steps li + li { padding-left: 20px; border-left: 1px solid var(--cafe-line); }
.quick-steps span { color: var(--cafe-accent); font: italic 18px Georgia, serif; }
.quick-steps a:hover { text-decoration: underline; text-underline-offset: 4px; }

.questions {
  padding-block: var(--chapter-space);
}
.policy-heading {
  display: flex;
  align-items: end;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: var(--chapter-gap);
}
.policy-card {
  min-width: 0;
  padding: clamp(28px, 3.5vw, 44px);
  border: 1px solid var(--cafe-line);
  border-radius: 12px;
}
.policy-card p:not(.policy-kicker) {
  margin-top: 14px;
  color: var(--cafe-muted);
  font-size: 14px;
  line-height: 1.9;
}
.policy-notice {
  display: flex;
  align-items: flex-start;
  gap: 18px;
  margin-bottom: 28px;
  border-top: 3px solid var(--cafe-accent);
  background: var(--cafe-surface);
}
.policy-notice-body {
  min-width: 0;
  flex: 1;
}
.policy-notice-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.policy-warning {
  padding: 5px 12px;
  border: 1px solid var(--cafe-accent);
  border-radius: 4px;
  color: var(--cafe-accent);
  font-size: 13px;
  line-height: 1.7;
}
.policy-grid {
  display: grid;
  grid-template-columns: minmax(0, .8fr) minmax(0, 1.2fr);
  gap: 28px;
}
.policy-kicker {
  margin-bottom: 12px;
  color: var(--cafe-accent);
  font-size: 11px;
  letter-spacing: .14em;
}
.policy-clients {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  width: fit-content;
  max-width: 100%;
  gap: 12px;
  margin-top: 22px;
}
.policy-clients a {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 10px 16px;
  border: 1px solid var(--cafe-line);
  border-radius: 6px;
  background: var(--cafe-surface);
  font-size: 16px;
  font-weight: 500;
}
.policy-clients a:hover {
  border-color: var(--cafe-accent);
  color: var(--cafe-accent);
}
.dark .policy-clients img {
  filter: invert(1);
}
.request-retention {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid var(--cafe-line);
}
.request-retention > div + div { padding-left: 20px; border-left: 1px solid var(--cafe-line); }
.request-retention dt { font-size: 12px; line-height: 1.6; color: var(--cafe-muted); }
.request-retention dd { margin-top: 6px; font-size: 20px; font-weight: 500; line-height: 1.4; }
.request-retention > div:last-child dd { color: var(--cafe-accent); }
@media (max-width: 1023px) {
  .policy-grid { grid-template-columns: 1fr; }
}
@media (max-width: 639px) {
  .quick-steps { grid-template-columns: 1fr; gap: 12px; }
  .quick-steps li + li { padding-left: 0; border-left: 0; }
}
@media (max-width: 479px) {
  .request-retention { grid-template-columns: 1fr; gap: 14px; }
  .request-retention > div + div { padding: 14px 0 0; border-left: 0; border-top: 1px solid var(--cafe-line); }
}
.closing {
  margin-top: 64px;
}
.closing-inner {
  border-top: 1px solid var(--cafe-line);
  padding-top: 48px;
}
.closing h2 {
  font-size: 29px;
}
.closing-inner > div > p:last-child {
  font-size: 14px;
  color: var(--cafe-muted);
}
.home-footer {
  padding-block: 27px;
  border-top: 1px solid var(--cafe-line);
  color: var(--cafe-muted);
  font-size: 12px;
}
.skip-link {
  position: absolute;
  z-index: 100;
  top: 8px;
  left: 8px;
  padding: 12px;
  background: var(--cafe-page);
  transform: translateY(-160%);
}
.skip-link:focus {
  transform: translateY(0);
}

/* Let the brand breathe on the narrowest phones instead of compressing its name. */
@media (max-width: 359px) {
  .home-header .header-row { display: flex; flex-wrap: wrap; padding-block: 10px; gap: 6px; }
  .header-actions { margin-left: auto; }
}

@media (min-width: 1600px) {
  .hero-inner {
    min-height: 690px;
  }
  .hero-art img {
    object-position: center 40%;
  }
}

@media (max-width: 1023px) {
  .header-row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .home-container {
    width: calc(100% - 64px);
  }
  .hero-inner {
    min-height: 610px;
  }
  .hero-copy {
    width: 65%;
  }
  .hero h1 {
    font-size: 42px;
  }
  .hero-art img {
    object-position: 72% center;
  }
  .hero-art::after {
    background: linear-gradient(90deg, var(--cafe-page), rgba(var(--cafe-page-rgb), .94) 30%, rgba(var(--cafe-page-rgb), .6) 53%, transparent 88%), linear-gradient(0deg, var(--cafe-page), transparent 22%);
  }
}

@media (max-width: 767px) {
  .cafe-home { --chapter-space: 64px; --chapter-gap: 36px; }
  .home-container {
    width: calc(100% - 40px);
  }
  .header-row {
    min-height: 76px;
    gap: 12px;
  }
  .header-entry {
    padding-inline: 10px;
  }
  .theme-toggle {
    width: 34px;
  }
  .mobile-nav {
    gap: 24px;
  }
  .home-header .mobile-nav {
    font-size: 12px;
  }
  .hero-inner {
    display: block;
    min-height: 0;
    padding-block: 42px 38px;
  }
  .hero-copy {
    width: 100%;
    max-width: none;
  }
  .hero h1 {
    margin-top: 22px;
    font-size: clamp(32px, 7.5vw, 46px);
  }
  .hero {
    display: flex;
    flex-direction: column;
  }
  .hero-art {
    position: relative;
    order: 2;
    height: clamp(220px, 53vw, 340px);
  }
  .hero-art img {
    object-position: 78% 43%;
  }
  .hero-art::after {
    background: linear-gradient(180deg, var(--cafe-page), transparent 23%, transparent 80%, var(--cafe-page));
  }
  .hero-caption {
    display: none;
  }
  .hero-actions {
    margin-top: 25px;
  }
  .cafe-button {
    gap: 12px;
    padding-inline: 20px;
  }
  .closing h2 {
    font-size: 26px;
  }
}

/* Keep the original first-screen composition and chapter-rail clearance. */
@media (min-width: 1024px) {
  .home-container { width: min(1280px, calc(100% - 160px)); }
  .slide-welcome .slide-content { display: flex; flex: 1; flex-direction: column; }
  .slide-welcome .hero { display: flex; flex: 1; }
  .slide-welcome .hero-inner { min-height: 0; padding-block: 64px 80px; }
  .slide-welcome .tool-note { padding-block: 20px 24px; }
}
.slide-supported-models :deep(.model-showcase) { padding-block: var(--chapter-space); }
.slide-billing { background: var(--cafe-surface); }
.closing-inner { width: 100%; }

/* Desktop chapters share the space below the sticky header. Keep text intrinsic,
   give illustrations the remaining space, and let exceptional content grow rather
   than clipping it. Narrow/short screens keep the normal scrollable layout. */
@media (min-width: 1024px) and (min-height: 700px) {
  .cafe-home {
    --chapter-viewport: calc(100dvh - var(--slider-header-height));
    --chapter-space: clamp(12px, calc((100dvh - 640px) * .08), 36px);
    --chapter-gap: clamp(12px, 2dvh, 24px);
  }
  .possibilities {
    display: grid;
    grid-template-rows: auto minmax(min-content, 1fr);
    height: var(--chapter-viewport);
    min-height: min-content;
  }
  .possibilities .section-heading { flex-shrink: 0; }

  /* Keep policy cards intrinsic. Only the gap before the bottom-aligned closing
     callout absorbs spare height; short layouts retain their minimum spacing. */
  .slide-questions .slide-content {
    display: grid;
    grid-template-rows: minmax(min-content, 1fr) auto;
    min-height: var(--chapter-viewport);
  }
  .questions {
    display: grid;
    grid-template-rows: repeat(4, max-content) minmax(max-content, 1fr);
    padding-bottom: 12px;
  }
  .quick-guide { padding-block: 12px; gap: 10px; }
  .request-retention { margin-top: 12px; padding-top: 12px; }
  .policy-heading h2 { margin-top: 8px; font-size: clamp(27px, 2.3vw, 32px); line-height: 1.35; }
  .policy-card { padding: clamp(16px, 2dvh, 24px); }
  .policy-card h3 { font-size: 16px; line-height: 1.5; }
  .policy-card p:not(.policy-kicker) { margin-top: 8px; font-size: 13px; line-height: 1.7; }
  .policy-notice { gap: 14px; margin-bottom: 14px; }
  .policy-warning { padding: 3px 9px; font-size: 12px; }
  .policy-grid { gap: 16px; }
  .policy-kicker { margin-bottom: 6px; font-size: 10px; }
  .policy-clients { margin-top: 14px; }
  .policy-clients a { padding: 8px 12px; font-size: 14px; }
  .closing { align-self: end; margin-top: clamp(14px, 2dvh, 24px); }
  .closing-inner { padding-top: clamp(14px, 2dvh, 24px); }
  .closing h2 { margin-top: 6px; font-size: 26px; line-height: 1.35; }
  .closing-inner > div > p:last-child { margin-top: 6px; font-size: 13px; }
  .home-footer { padding-block: 12px; }
}
@media (max-width: 1023px) {
  .header-surface::after { inset: 6px 12px; border-radius: 14px; }
}

@media (prefers-reduced-motion: reduce) {
  .cafe-home *, .cafe-home :deep(*), .header-surface::before, .header-surface::after {
    transition: none !important;
  }
}
</style>
