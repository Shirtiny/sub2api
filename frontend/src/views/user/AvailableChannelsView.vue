<template>
  <AppLayout>
    <div class="offerings-page">
      <div class="collection-toolbar">
        <label class="collection-search">
          <Icon name="search" size="md" />
          <input v-model="searchQuery" type="search" :placeholder="t('availableChannels.catalog.search')" :aria-label="t('availableChannels.catalog.search')" />
        </label>
        <button type="button" class="collection-refresh" :disabled="loading" :aria-label="t('common.refresh')" @click="loadChannels">
          <LoadingSpinner v-if="loading" size="sm" color="current" decorative />
          <Icon v-else name="refresh" size="md" />
        </button>
      </div>

      <div v-if="loading && channels.length === 0" class="collection-state"><LoadingSpinner variant="steam" size="lg" color="current" /></div>
      <div v-else-if="loadError" class="collection-state" role="alert">
        <p>{{ loadError }}</p>
        <button class="btn btn-secondary mt-4" @click="loadChannels">{{ t('common.retry') }}</button>
      </div>
      <div v-else-if="filteredChannels.length === 0" class="collection-state">
        <Icon name="search" size="lg" /><p>{{ searchQuery ? t('availableChannels.catalog.noResults') : t('availableChannels.empty') }}</p>
      </div>

      <section v-for="(channel, channelIndex) in filteredChannels" :key="channel.name + channelIndex" class="channel-collection">
        <header class="channel-heading">
          <div><p class="collection-eyebrow">{{ t('availableChannels.catalog.offering') }}</p><h2>{{ channel.name }}</h2></div>
          <p v-if="channel.description">{{ channel.description }}</p>
        </header>
        <section v-for="section in channel.platforms" :key="section.platform" class="platform-collection">
          <div class="platform-heading">
            <span class="platform-name"><PlatformIcon :platform="section.platform as GroupPlatform" size="sm" />{{ platformName(section.platform) }}</span>
            <div class="accessible-groups">
              <span class="groups-label">{{ t('availableChannels.catalog.groups') }}</span>
              <span v-for="group in section.groups" :key="group.id" class="group-pill" :title="t('availableChannels.catalog.groupMultiplier')">
                {{ group.name }} <span>{{ userGroupRates[group.id] ?? group.rate_multiplier }}×</span>
              </span>
            </div>
          </div>
          <div class="model-grid">
            <ModelOfferingCard v-for="model in section.supported_models" :key="model.name" :model="model" />
          </div>
          <p v-if="!section.supported_models.length" class="empty-models">{{ t('availableChannels.noModels') }}</p>
        </section>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ModelOfferingCard from '@/components/channels/ModelOfferingCard.vue'
import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import type { GroupPlatform } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const loadError = ref('')
const searchQuery = ref('')
let controller: AbortController | undefined

function platformName(platform: string) {
  const names: Record<string, string> = { openai: 'OpenAI', anthropic: 'Anthropic', gemini: 'Gemini', antigravity: 'Antigravity', grok: 'Grok' }
  return names[platform] || platform
}

const filteredChannels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return channels.value
  return channels.value.flatMap(channel => {
    if (`${channel.name} ${channel.description}`.toLowerCase().includes(query)) return [channel]
    const platforms = channel.platforms.flatMap(section => {
      if (section.platform.toLowerCase().includes(query) || section.groups.some(group => group.name.toLowerCase().includes(query))) return [section]
      const models = section.supported_models.filter(model => model.name.toLowerCase().includes(query))
      return models.length ? [{ ...section, supported_models: models }] : []
    })
    return platforms.length ? [{ ...channel, platforms }] : []
  })
})

async function loadChannels() {
  controller?.abort()
  const request = new AbortController()
  controller = request
  loading.value = true
  loadError.value = ''
  try {
    const [list, rates] = await Promise.all([
      userChannelsAPI.getAvailable({ signal: request.signal }),
      userGroupsAPI.getUserGroupRates().catch(() => ({} as Record<number, number>))
    ])
    if (request.signal.aborted) return
    channels.value = list
    userGroupRates.value = rates
  } catch (error: unknown) {
    if (!request.signal.aborted) loadError.value = extractApiErrorMessage(error, t('common.error'))
  } finally {
    if (!request.signal.aborted) loading.value = false
  }
}
onMounted(loadChannels)
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped>
.offerings-page { --offering-page: #faf7f2; --offering-card: #fffcf7; --offering-ink: #382a20; --offering-muted: #756456; --offering-accent: #865630; --offering-line: #e4d9ca; --offering-tint: #f2ece3; max-width: 1500px; margin: 0 auto; padding: clamp(22px, 3.5vw, 54px); border: 1px solid var(--offering-line); border-radius: 28px; background: var(--offering-page); color: var(--offering-ink); }
.dark .offerings-page { --offering-page: #1d1915; --offering-card: #27211b; --offering-ink: #f2e9dc; --offering-muted: #b8a99a; --offering-accent: #d8ad7d; --offering-line: #43372c; --offering-tint: #34291f; }
.collection-eyebrow { color: var(--offering-accent); font-size: 10px; letter-spacing: .2em; text-transform: uppercase; }
.collection-toolbar { display: flex; align-items: center; gap: 12px; margin-bottom: 42px; }
.collection-search { display: flex; width: min(100%, 440px); align-items: center; gap: 12px; padding: 13px 16px; border: 1px solid var(--offering-line); border-radius: 12px; background: var(--offering-card); color: var(--offering-muted); }
.collection-search:focus-within { outline: 2px solid var(--offering-accent); outline-offset: 2px; }
.collection-search input { width: 100%; min-width: 0; padding: 0; border: none; outline: none; box-shadow: none; background: none; font-size: 13px; color: var(--offering-ink); }
.collection-search input::placeholder { color: var(--offering-muted); }
.collection-refresh { display: grid; place-items: center; width: 46px; height: 46px; border: 1px solid var(--offering-line); border-radius: 12px; color: var(--offering-muted); transition: color .2s, background .2s; }
.collection-refresh:hover { color: var(--offering-accent); background: var(--offering-tint); }
.collection-refresh:focus-visible { outline: 2px solid var(--offering-accent); outline-offset: 3px; }
.channel-collection + .channel-collection { margin-top: 52px; }
.channel-heading { display: flex; align-items: flex-end; justify-content: space-between; flex-wrap: wrap; gap: 18px 28px; border-bottom: 1px solid var(--offering-line); padding-bottom: 24px; }
.channel-heading h2 { margin-top: 9px; font: 400 clamp(24px, 2.7vw, 36px)/1.25 Georgia, 'Songti SC', serif; }
.channel-heading > p { max-width: 480px; font-size: 12px; line-height: 1.8; color: var(--offering-muted); }
.platform-collection { margin-top: 26px; }
.platform-heading { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 16px; margin-bottom: 22px; }
.platform-name { display: flex; align-items: center; gap: 10px; font-size: 13px; }
.accessible-groups { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; font-size: 11px; color: var(--offering-muted); }
.groups-label { margin-right: 2px; }
.group-pill { padding: 6px 10px; border: 1px solid var(--offering-line); border-radius: 8px; }
.group-pill > span { margin-left: 6px; color: var(--offering-accent); font-variant-numeric: tabular-nums; }
.model-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; align-items: start; }
.collection-state { display: flex; flex-direction: column; justify-content: center; align-items: center; min-height: 260px; gap: 16px; text-align: center; font-size: 14px; color: var(--offering-muted); }
.empty-models { padding: 36px 0; color: var(--offering-muted); font-size: 13px; }
@media (min-width: 1700px) { .model-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 900px) { .model-grid { grid-template-columns: 1fr; } }
@media (max-width: 480px) { .offerings-page { padding: 22px 16px; border-radius: 20px; } .collection-toolbar { margin-bottom: 30px; } }
</style>
