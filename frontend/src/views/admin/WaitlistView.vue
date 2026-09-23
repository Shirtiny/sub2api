<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex items-center justify-between gap-4">
          <p class="text-sm text-content-secondary">{{ t('admin.waitlist.description') }}</p>
          <button type="button" class="btn btn-secondary shrink-0" :disabled="loading || approvingId !== null" @click="load">
            {{ t('common.refresh') }}
          </button>
        </div>
        <p v-if="error" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert">{{ error }}</p>
        <p v-if="actionError" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert">{{ actionError }}</p>
        <p v-if="notice" class="mt-3 text-sm text-emerald-700 dark:text-emerald-400" role="status">{{ notice }}</p>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="entries" :loading="loading">
          <template #cell-email="{ value }"><span class="inline-block max-w-[14rem] break-all sm:max-w-xs">{{ value }}</span></template>
          <template #cell-created_at="{ value }">{{ formatDateTime(value) }}</template>
          <template #cell-status="{ row }">
            <div class="space-y-1">
              <span :class="row.approved_at ? 'text-emerald-700 dark:text-emerald-400' : 'text-content-secondary'">
                {{ t(row.approved_at ? 'admin.waitlist.approved' : 'admin.waitlist.pending') }}
              </span>
              <p v-if="row.approved_at" class="text-xs text-content-tertiary">{{ formatDateTime(row.approved_at) }}</p>
            </div>
          </template>
          <template #cell-notification="{ row }">
            <span v-if="!row.approved_at" class="text-content-tertiary">—</span>
            <span v-else :class="row.approval_notice_sent_at ? 'text-content-secondary' : 'text-amber-700 dark:text-amber-400'">
              {{ t(row.approval_notice_sent_at ? 'admin.waitlist.notified' : 'admin.waitlist.notificationPending') }}
            </span>
          </template>
          <template #cell-actions="{ row }">
            <button v-if="!row.approved_at || !row.approval_notice_sent_at" type="button"
              :class="['btn whitespace-nowrap', row.approved_at ? 'btn-secondary' : 'btn-primary']"
              :disabled="approvingId !== null || loading"
              @click="row.approved_at ? approve(row) : selected = row">
              {{ t(approvingId === row.id ? 'admin.waitlist.processing' : row.approved_at ? 'admin.waitlist.retryNotice' : 'admin.waitlist.approve') }}
            </button>
            <span v-else class="text-content-tertiary">{{ t('admin.waitlist.completed') }}</span>
          </template>
        </DataTable>
      </template>
      <template #pagination>
        <Pagination v-if="total > 0" :page="page" :page-size="pageSize" :total="total" :show-page-size-selector="false" @update:page="changePage" />
      </template>
    </TablePageLayout>
    <ConfirmDialog :show="selected !== null" :title="t('admin.waitlist.confirmTitle')"
      :message="t('admin.waitlist.confirmMessage', { email: selected?.email || '' })"
      :confirm-text="t('admin.waitlist.approve')" :confirm-disabled="approvingId !== null"
      @confirm="selected && approve(selected)" @cancel="selected = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { approveWaitlist, listWaitlist, type WaitlistEntry } from '@/api/admin/waitlist'
import { buildAuthErrorMessage } from '@/utils/authError'
import { formatDateTime } from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const { t } = useI18n()
const entries = ref<WaitlistEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const error = ref('')
const actionError = ref('')
const notice = ref('')
const selected = ref<WaitlistEntry | null>(null)
const approvingId = ref<number | null>(null)
const columns = computed(() => [
  { key: 'email', label: t('home.landing.waitlist.email') },
  { key: 'created_at', label: t('admin.waitlist.joinedAt') },
  { key: 'status', label: t('admin.waitlist.status') },
  { key: 'notification', label: t('admin.waitlist.notification') },
  { key: 'actions', label: t('common.actions') }
])
let request: AbortController | undefined
let disposed = false
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
async function approve(entry: WaitlistEntry) {
  if (approvingId.value !== null) return
  approvingId.value = entry.id
  selected.value = null
  actionError.value = ''
  notice.value = ''
  try {
    await approveWaitlist(entry.id)
    if (!disposed) notice.value = t('admin.waitlist.success')
  } catch (error: unknown) {
    if (!disposed) actionError.value = buildAuthErrorMessage(error, {
      fallback: t('admin.waitlist.approveFailed'), t, namespace: 'admin.waitlist.errors'
    })
  } finally {
    // SMTP can fail after a durable approval; always reload the authoritative state.
    if (!disposed) await load()
    approvingId.value = null
  }
}
function changePage(value: number) { page.value = value; void load() }
onMounted(load)
onBeforeUnmount(() => { disposed = true; request?.abort() })
</script>
