if (import.meta.env.DEV) {
  import("react-grab");
}

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n } from './i18n'
import { useAppStore } from '@/stores/app'
import './style.css'

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const shouldUseDark = savedTheme !== 'light'
  document.documentElement.classList.toggle('dark', shouldUseDark)
}

function initPrimaryButtonPointer() {
  document.addEventListener('pointermove', (event) => {
    const target = event.target as Element | null
    const button = target?.closest('.btn-primary') as HTMLElement | null
    if (!button) return

    const rect = button.getBoundingClientRect()
    button.style.setProperty('--btn-x', `${((event.clientX - rect.left) / rect.width) * 100}%`)
    button.style.setProperty('--btn-y', `${((event.clientY - rect.top) / rect.height) * 100}%`)
  })
}

async function bootstrap() {
  // Apply theme class globally before app mount to keep all routes consistent.
  initThemeClass()
  initPrimaryButtonPointer()

  const app = createApp(App)
  const pinia = createPinia()
  app.use(pinia)

  // Initialize settings from injected config BEFORE mounting (prevents flash)
  // This must happen after pinia is installed but before router and i18n
  const appStore = useAppStore()
  appStore.initFromInjectedConfig()

  // Set document title immediately after config is loaded
  if (appStore.siteName && appStore.siteName !== 'Sub2API') {
    document.title = `${appStore.siteName} - AI API Gateway`
  }

  await initI18n()

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
}

bootstrap()
