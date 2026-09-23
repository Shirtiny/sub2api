<template>
  <BaseDialog :show="true" :title="t('home.landing.waitlist.title')" width="narrow" :z-index="100" @close="close">
    <div v-if="submitted" class="space-y-5 py-2">
      <div class="flex items-center gap-3" role="status" aria-atomic="true">
        <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary-100 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400" aria-hidden="true">
          <Icon name="check" size="md" />
        </div>
        <p class="min-w-0 flex-1 text-sm leading-6 text-content-primary">{{ t('home.landing.waitlist.success') }}</p>
      </div>
      <button type="button" class="btn btn-primary w-full" @click="close">{{ t('common.close') }}</button>
    </div>
    <form v-else class="space-y-5" novalidate :aria-busy="submitting" @submit.prevent="submit">
      <p class="text-sm leading-relaxed text-content-secondary">{{ t('home.landing.waitlist.description') }}</p>
      <div>
        <label for="waitlist-email" class="mb-2 block text-sm font-medium text-content-primary">{{ t('home.landing.waitlist.email') }}</label>
        <input
          id="waitlist-email" ref="emailInput" v-model="email" type="email" name="email" required maxlength="254"
          autocomplete="email" inputmode="email" autocapitalize="none" :spellcheck="false" class="input w-full"
          placeholder="you@example.com" :disabled="submitting" :aria-invalid="!!emailError"
          :aria-describedby="emailError ? 'waitlist-email-error waitlist-privacy' : 'waitlist-privacy'" @input="emailError = ''"
        />
        <p v-if="emailError" id="waitlist-email-error" class="mt-2 text-sm text-red-600 dark:text-red-400" role="alert">{{ emailError }}</p>
      </div>
      <TurnstileWidget
        v-if="turnstileEnabled && turnstileSiteKey" ref="turnstileRef" :site-key="turnstileSiteKey"
        :theme="isDark ? 'dark' : 'light'" @verify="token = $event" @expire="token = ''" @error="token = ''; error = t('auth.turnstileFailed')"
      />
      <p v-if="error" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ error }}</p>
      <button type="submit" class="btn btn-primary w-full" :disabled="submitting || (turnstileEnabled && !token)">
        {{ t(submitting ? 'home.landing.waitlist.submitting' : 'home.landing.waitlist.button') }}
        <Icon v-if="!submitting" name="arrowRight" size="sm" class="ml-2" aria-hidden="true" />
      </button>
      <p id="waitlist-privacy" class="text-xs leading-relaxed text-content-tertiary">{{ t('home.landing.waitlist.privacy') }}</p>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { joinWaitlist } from '@/api/waitlist'
import { normalizeWaitlistEmail } from '@/utils/waitlist'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ isDark: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const email = ref('')
const emailInput = ref<HTMLInputElement | null>(null)
const emailError = ref('')
const error = ref('')
const submitting = ref(false)
const submitted = ref(false)
const token = ref('')
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileEnabled = computed(() => !!appStore.cachedPublicSettings?.turnstile_enabled)
const turnstileSiteKey = computed(() => appStore.cachedPublicSettings?.turnstile_site_key || '')

onMounted(async () => { await nextTick(); emailInput.value?.focus() })
function close() { if (!submitting.value) emit('close') }

async function submit() {
  if (submitting.value || submitted.value) return
  error.value = ''
  const normalized = normalizeWaitlistEmail(email.value)
  if (!normalized) {
    emailError.value = t('home.landing.waitlist.invalidEmail')
    emailInput.value?.focus()
    return
  }
  emailError.value = ''
  if (turnstileEnabled.value && !token.value) { error.value = t('auth.completeVerification'); return }
  submitting.value = true
  try {
    await joinWaitlist(normalized, token.value)
    submitted.value = true
    email.value = ''
  } catch (cause: unknown) {
    const failure = cause as { status?: number; reason?: string }
    if (failure?.reason === 'WAITLIST_EMAIL_INVALID') emailError.value = t('home.landing.waitlist.invalidEmail')
    else if (failure?.reason === 'WAITLIST_CONFIRMATION_FAILED') error.value = t('home.landing.waitlist.mailFailed')
    else error.value = t(failure?.status === 429 ? 'home.landing.waitlist.rateLimited' : 'home.landing.waitlist.failed')
    token.value = ''
    turnstileRef.value?.reset()
  } finally { submitting.value = false }
}
</script>
