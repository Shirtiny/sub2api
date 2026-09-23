<template>
  <section role="status" class="rounded-2xl border border-stroke-subtle bg-surface-secondary p-6">
    <div class="mb-4 inline-flex rounded-xl bg-primary-50 p-3 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400">
      <Icon name="key" size="lg" />
    </div>
    <h2 class="text-lg font-semibold text-content-primary">
      {{ t(mode === 'login' ? 'auth.registrationSignInTitle' : 'auth.registrationDisabled') }}
    </h2>
    <p class="mt-3 text-sm leading-7 text-content-secondary">
      {{ t(mode === 'login' ? 'auth.registrationSignInHint' : 'auth.registrationClosedHint') }}
    </p>
    <button type="button" class="btn btn-primary mt-5 w-full" @click="$emit('login')">
      {{ t('auth.signIn') }}
    </button>
    <slot />
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { RegistrationRecoveryMode } from '@/utils/authError'

defineProps<{ mode: RegistrationRecoveryMode }>()
defineEmits<{ login: [] }>()
const { t } = useI18n()
</script>
