<template>
  <span
    class="loading-indicator"
    :class="[colorClass, { 'loading-overlay': overlay, 'loading-steam': variant === 'steam' }]"
    :role="decorative ? undefined : 'status'"
    :aria-label="decorative ? undefined : t('common.loading')"
    :aria-hidden="decorative ? true : undefined"
  >
    <svg
      v-if="variant === 'steam'"
      :class="sizeClasses"
      viewBox="0 0 64 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.6"
      stroke-linecap="round"
      aria-hidden="true"
      focusable="false"
    >
      <path class="steam-wisp" pathLength="1" d="M23 20C15 15 29 11 21 4" />
      <path class="steam-wisp" pathLength="1" d="M33 20C25 15 39 11 31 4" />
      <path class="steam-wisp" pathLength="1" d="M43 20C35 15 49 11 41 4" />
    </svg>
    <span v-else :class="['spinner', sizeClasses]" aria-hidden="true" />
    <span v-if="!decorative" class="sr-only">{{ t('common.loading') }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'
type SpinnerColor = 'primary' | 'secondary' | 'white' | 'gray' | 'current'

interface Props {
  size?: SpinnerSize
  color?: SpinnerColor
  variant?: 'spinner' | 'steam'
  /** Center over a positioned parent without changing its layout. */
  overlay?: boolean
  /** The parent already owns the accessible loading announcement. */
  decorative?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
  color: 'primary',
  variant: 'spinner',
  overlay: false,
  decorative: false
})

const sizeClasses = computed(() => {
  if (props.variant === 'steam') {
    const sizes: Record<SpinnerSize, string> = {
      sm: 'w-12 h-[18px]',
      md: 'w-16 h-6',
      lg: 'w-24 h-9',
      xl: 'w-32 h-12'
    }
    return sizes[props.size]
  }
  const sizes: Record<SpinnerSize, string> = {
    sm: 'w-4 h-4 border-2',
    md: 'w-8 h-8 border-2',
    lg: 'w-12 h-12 border-[3px]',
    xl: 'w-16 h-16 border-4'
  }
  return sizes[props.size]
})

const colorClass = computed(() => {
  const colors: Record<SpinnerColor, string> = {
    primary: 'text-primary-500',
    secondary: 'text-content-tertiary',
    white: 'text-white',
    gray: 'text-content-tertiary',
    current: 'text-current'
  }
  return colors[props.color]
})
</script>

<style scoped>
.loading-indicator {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  vertical-align: middle;
  pointer-events: none;
}

.loading-overlay {
  position: absolute;
  inset: 0;
  overflow: hidden;
  border-radius: inherit;
}

.loading-steam.loading-overlay::before {
  content: '';
  position: absolute;
  inset: 0 auto 0 0;
  width: 45%;
  background: linear-gradient(105deg, transparent, rgb(255 255 255 / .12), transparent);
  animation: loading-sheen 2.8s ease-in-out infinite;
}

.steam-wisp {
  stroke-dasharray: .62 .38;
  animation: loading-steam 2.4s linear infinite;
}
.steam-wisp:nth-child(2) { animation-delay: -.8s; }
.steam-wisp:nth-child(3) { animation-delay: -1.6s; }

.spinner {
  @apply inline-block rounded-full border-solid border-current border-r-transparent;
  animation: spin 0.75s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

@keyframes loading-steam {
  0% { stroke-dashoffset: 1; opacity: .38; transform: translateY(1px); }
  50% { opacity: 1; transform: translateY(-1px); }
  100% { stroke-dashoffset: 0; opacity: .38; transform: translateY(1px); }
}

@keyframes loading-sheen {
  from { transform: translateX(-110%); }
  to { transform: translateX(330%); }
}

@media (prefers-reduced-motion: reduce) {
  .loading-steam.loading-overlay::before { animation: none; opacity: 0; }
  .steam-wisp { animation: none; stroke-dasharray: none; opacity: .85; }
  .spinner { animation: none; }
}
</style>
