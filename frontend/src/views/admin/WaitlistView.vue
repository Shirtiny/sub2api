<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex items-center justify-between gap-4">
          <p class="text-sm text-content-secondary">{{ t('admin.waitlist.description') }}</p>
          <button type="button" class="btn btn-secondary shrink-0" :disabled="loading" @click="load">
            {{ t('common.refresh') }}
          </button>
        </div>
        <p v-if="error" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert">{{ error }}</p>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="entries" :loading="loading">
          <template #cell-created_at="{ value }">{{ formatDateTime(value) }}</template>
        </DataTable>
      </template>
      <template #pagination>
        <Pagination v-if="total > 0" :page="page" :page-size="pageSize" :total="total" :show-page-size-selector="false" @update:page="changePage" />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listWaitlist, type WaitlistEntry } from '@/api/admin/waitlist'
import { formatDateTime } from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'

const { t } = useI18n()
const entries = ref<WaitlistEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const error = ref('')
const columns = computed(() => [
  { key: 'email', label: t('home.landing.waitlist.email') },
  { key: 'created_at', label: t('admin.waitlist.joinedAt') }
])
let request: AbortController | undefined
async function load() {
  request?.abort()
  const current = request = new AbortController()
  loading.value = true
  error.value = ''
  entries.value = []
  try {
    const result = await listWaitlist(page.value, pageSize, current.signal)
    if (current.signal.aborted) return
    entries.value = result.items
    total.value = result.total
  } catch {
    if (!current.signal.aborted) error.value = t('admin.waitlist.failed')
  } finally {
    if (!current.signal.aborted) loading.value = false
  }
}
function changePage(value: number) { page.value = value; void load() }
onMounted(load)
onBeforeUnmount(() => request?.abort())
</script>
