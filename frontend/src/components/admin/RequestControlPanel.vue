<template>
  <div class="space-y-6" data-test="request-control-panel">
    <div class="flex flex-col gap-3 rounded-lg border border-gray-100 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h2 class="text-lg font-semibold text-content-primary">{{ t('admin.riskControl.requestControl.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.description') }}</p>
      </div>
      <div class="flex items-center gap-3">
        <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.enabled') }}</span>
        <Toggle v-model="form.enabled" />
        <button data-test="request-control-save" type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" @click="save">
          <Icon name="check" size="sm" :class="saving ? 'animate-pulse' : ''" />
          {{ saving ? t('common.saving') : t('admin.riskControl.requestControl.save') }}
        </button>
      </div>
    </div>

    <div v-if="status && form.enabled && !status.risk_control_enabled" class="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-200">
      <Icon name="exclamationTriangle" size="sm" class="mt-0.5 flex-shrink-0" />
      <span>{{ t('admin.riskControl.requestControl.globalSwitchOff') }}</span>
    </div>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
      <div v-for="item in statusItems" :key="item.key" class="rounded-lg border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
        <p class="mt-2 text-2xl font-semibold text-content-primary">{{ item.value }}</p>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
      <section class="card">
        <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h3 class="text-base font-semibold text-content-primary">{{ t('admin.riskControl.requestControl.scopeTitle') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.scopeHint') }}</p>
        </div>
        <div class="space-y-5 p-5">
          <div>
            <div class="flex items-center justify-between gap-3">
              <label class="input-label mb-0">{{ t('admin.riskControl.requestControl.groups') }}</label>
              <Toggle v-model="form.all_groups" />
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ form.all_groups ? t('admin.riskControl.requestControl.allGroups') : t('admin.riskControl.requestControl.selectedGroups') }}</p>
            <div v-if="!form.all_groups" class="mt-3 max-h-48 space-y-2 overflow-y-auto rounded-lg border border-gray-100 p-3 dark:border-dark-700">
              <label v-for="group in groups" :key="group.id" class="flex items-center gap-2 text-sm text-content-secondary">
                <input v-model="form.group_ids" type="checkbox" :value="group.id" class="checkbox" />
                <span>{{ group.name }}</span>
                <span class="ml-auto text-xs text-gray-400">{{ group.platform }}</span>
              </label>
              <p v-if="groups.length === 0" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.noGroups') }}</p>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('admin.riskControl.requestControl.models') }}</label>
            <Select v-model="form.model_filter.type" :options="modelFilterOptions" />
            <textarea v-if="form.model_filter.type !== 'all'" v-model="modelText" data-test="request-control-models" class="input mt-3 min-h-24 resize-y font-mono text-sm" :placeholder="t('admin.riskControl.requestControl.modelsPlaceholder')" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.riskControl.requestControl.globalUA') }}</label>
            <textarea v-model="globalUAText" data-test="request-control-global-ua" class="input min-h-24 resize-y font-mono text-sm" :placeholder="t('admin.riskControl.requestControl.uaPlaceholder')" />
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.uaHint') }}</p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.riskControl.requestControl.protocolBlocking') }}</label>
            <div class="space-y-2 rounded-lg border border-gray-100 p-3 dark:border-dark-700">
              <label class="flex items-center justify-between gap-3 text-sm text-content-secondary">
                <span>{{ t('admin.riskControl.requestControl.blockOpenAIChat') }}</span>
                <Toggle v-model="form.block_openai_chat" />
              </label>
              <label class="flex items-center justify-between gap-3 text-sm text-content-secondary">
                <span>{{ t('admin.riskControl.requestControl.blockClaudeMessages') }}</span>
                <Toggle v-model="form.block_claude_messages" />
              </label>
              <label class="flex items-center justify-between gap-3 text-sm text-content-secondary">
                <span>{{ t('admin.riskControl.requestControl.blockOpenAIResponses') }}</span>
                <Toggle v-model="form.block_openai_responses" />
              </label>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.protocolBlockingHint') }}</p>
          </div>

          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.requestControl.blockStatus') }}</label>
              <input v-model.number="form.block_status" type="number" min="400" max="599" class="input" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.blockStatusHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.requestControl.blockMessage') }}</label>
              <input v-model.trim="form.block_message" type="text" class="input" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.blockMessageHint') }}</p>
            </div>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <div class="flex items-center justify-between gap-3">
            <div>
              <h3 class="text-base font-semibold text-content-primary">{{ t('admin.riskControl.requestControl.usersTitle') }}</h3>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.usersHint') }}</p>
            </div>
            <Toggle v-model="form.all_users" />
          </div>
        </div>
        <div class="space-y-4 p-5">
          <div class="relative">
            <div class="flex gap-2">
              <input v-model.trim="userSearch" data-test="request-control-user-search" type="search" class="input" :placeholder="t('admin.riskControl.requestControl.searchUser')" @keyup.enter="searchUsers" />
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="userSearchLoading" @click="searchUsers">
                <Icon name="search" size="sm" :class="userSearchLoading ? 'animate-pulse' : ''" />
                {{ userSearchLoading ? t('admin.riskControl.requestControl.searching') : t('common.search') }}
              </button>
            </div>
            <div v-if="userSearchResults.length > 0" class="mt-2 max-h-52 overflow-y-auto rounded-lg border border-gray-100 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <button v-for="user in userSearchResults" :key="user.id" type="button" class="flex w-full items-center gap-3 border-b border-gray-100 px-3 py-2 text-left last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700" @click="selectUser(user)">
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-medium text-content-primary">{{ user.username || user.email }}</span>
                  <span class="block truncate text-xs text-gray-500 dark:text-gray-400">{{ user.email }}</span>
                </span>
                <span class="font-mono text-xs text-gray-400">UID {{ user.id }}</span>
              </button>
            </div>
            <p v-else-if="userSearchDone" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.noUsersFound') }}</p>
            <p v-if="selectedUserLabel" class="mt-2 text-xs font-medium text-primary-700 dark:text-primary-300">{{ t('admin.riskControl.requestControl.selectedUser', { user: selectedUserLabel }) }}</p>
          </div>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-[120px_1fr_auto]">
            <input v-model.number="ruleUserID" data-test="request-control-user-id" type="number" min="1" class="input" :disabled="editingRuleUserID != null" :placeholder="t('admin.riskControl.requestControl.userID')" />
            <textarea v-model="ruleUAText" rows="2" class="input resize-y font-mono text-sm" :placeholder="t('admin.riskControl.requestControl.userUAPlaceholder')" />
            <div class="flex gap-2">
              <button type="button" class="btn btn-secondary inline-flex flex-1 items-center justify-center gap-2" @click="addRule">
                <Icon :name="editingRuleUserID != null ? 'check' : 'plus'" size="sm" />
                {{ editingRuleUserID != null ? t('admin.riskControl.requestControl.updateUser') : t('admin.riskControl.requestControl.addUser') }}
              </button>
              <button v-if="editingRuleUserID != null" type="button" class="btn btn-ghost inline-flex items-center justify-center gap-2" @click="cancelEditRule">
                <Icon name="x" size="sm" />
                {{ t('common.cancel') }}
              </button>
            </div>
          </div>
          <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:bg-dark-700/50 dark:text-gray-400">
            <span>{{ form.all_users ? t('admin.riskControl.requestControl.allUsersOn') : t('admin.riskControl.requestControl.onlyConfiguredUsers') }}</span>
            <span>{{ t('admin.riskControl.requestControl.ruleCount', { count: form.user_rules.length }) }}</span>
          </div>
          <div class="max-h-80 overflow-y-auto rounded-lg border border-gray-100 dark:border-dark-700">
            <table class="min-w-full divide-y divide-gray-100 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-700/50">
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">{{ t('admin.riskControl.requestControl.userID') }}</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">{{ t('admin.riskControl.requestControl.participate') }}</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">{{ t('admin.riskControl.requestControl.userUA') }}</th>
                  <th class="px-3 py-2"></th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="rule in form.user_rules" :key="rule.user_id">
                  <td class="px-3 py-2 font-mono text-sm text-content-secondary">{{ rule.user_id }}</td>
                  <td class="px-3 py-2"><Toggle v-model="rule.participate" /></td>
                  <td class="max-w-[260px] px-3 py-2 text-xs text-content-secondary">{{ rule.user_agent_whitelist.join(', ') || '-' }}</td>
                  <td class="px-3 py-2 text-right">
                    <button type="button" class="btn btn-ghost btn-sm" :title="t('admin.riskControl.requestControl.editUserUA')" :aria-label="t('admin.riskControl.requestControl.editUserUA')" @click="editRule(rule)"><Icon name="edit" size="sm" /></button>
                    <button type="button" class="btn btn-ghost btn-sm" :title="t('admin.riskControl.requestControl.removeUser')" @click="removeRule(rule.user_id)"><Icon name="trash" size="sm" /></button>
                  </td>
                </tr>
                <tr v-if="form.user_rules.length === 0"><td colspan="4" class="px-3 py-8 text-center text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.noUserRules') }}</td></tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>
    </div>

    <section class="card">
      <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
        <h3 class="text-base font-semibold text-content-primary">{{ t('admin.riskControl.requestControl.notificationsTitle') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.notificationsHint') }}</p>
      </div>
      <div class="grid grid-cols-1 gap-4 p-5 lg:grid-cols-2">
        <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
          <div>
            <p class="text-sm font-medium text-content-primary">{{ t('admin.riskControl.requestControl.emailOnHit') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.emailOnHitHint') }}</p>
          </div>
          <Toggle v-model="form.email_on_hit" />
        </div>
        <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
          <div>
            <p class="text-sm font-medium text-content-primary">{{ t('admin.riskControl.requestControl.autoBan') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.autoBanHint') }}</p>
          </div>
          <Toggle v-model="form.auto_ban_enabled" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.riskControl.requestControl.banThreshold') }}</label>
          <input v-model.number="form.ban_threshold" type="number" min="1" max="1000" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.riskControl.requestControl.violationWindowHours') }}</label>
          <input v-model.number="form.violation_window_hours" type="number" min="1" max="720" class="input" />
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400 lg:col-span-2">{{ t('admin.riskControl.requestControl.violationWindowHint') }}</p>
      </div>
    </section>

    <section class="card">
      <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h3 class="text-base font-semibold text-content-primary">{{ t('admin.riskControl.requestControl.logsTitle') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.logsHint') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="logsLoading" @click="loadLogs"><Icon name="refresh" size="sm" :class="logsLoading ? 'animate-spin' : ''" />{{ t('admin.riskControl.requestControl.refresh') }}</button>
      </div>
      <div class="grid grid-cols-1 gap-3 border-b border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30 md:grid-cols-2 xl:grid-cols-5">
        <Select v-model="filters.action" :options="actionOptions" @change="reloadLogs" />
        <Select v-model="filters.protocol" :options="protocolOptions" @change="reloadLogs" />
        <Select v-model="filters.group_id" :options="groupFilterOptions" @change="reloadLogs" />
        <input v-model.trim="filters.search" type="search" class="input" :placeholder="t('admin.riskControl.requestControl.search')" @keyup.enter="reloadLogs" />
        <button type="button" class="btn btn-secondary" @click="reloadLogs">{{ t('admin.riskControl.requestControl.applyFilters') }}</button>
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-100 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-700/50">
            <tr>
              <th v-for="heading in logHeadings" :key="heading" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">{{ heading }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-if="logsLoading"><td colspan="8" class="px-4 py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</td></tr>
            <tr v-else-if="logs.length === 0"><td colspan="8" class="px-4 py-10 text-center text-sm text-gray-500">{{ t('admin.riskControl.requestControl.emptyLogs') }}</td></tr>
            <template v-else>
              <tr v-for="row in logs" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
              <td class="whitespace-nowrap px-4 py-3 text-sm text-content-secondary">{{ formatDate(row.created_at) }}</td>
              <td class="px-4 py-3 text-sm text-content-secondary"><div>{{ row.group_name || '-' }}</div><div class="text-xs text-gray-400">{{ row.user_email || `UID ${row.user_id ?? '-'}` }}</div></td>
              <td class="px-4 py-3 text-sm text-content-secondary"><div>{{ row.endpoint || '-' }}</div><div class="text-xs text-gray-400">{{ row.model || '-' }}</div></td>
              <td class="px-4 py-3 text-sm"><div class="text-[11px] text-gray-400">{{ t('admin.riskControl.requestControl.actual') }}</div><span class="rounded-md px-2 py-1 text-xs font-medium" :class="actionClass(row)">{{ row.action }}</span><div class="mt-1 text-xs text-gray-400">{{ row.reason }}</div><div v-if="row.expected_action" class="mt-1 text-xs text-blue-600 dark:text-blue-300">{{ t('admin.riskControl.requestControl.expected') }}: {{ row.expected_action }} / {{ row.expected_reason }}<span v-if="row.expected_status_code"> ({{ row.expected_status_code }})</span></div></td>
              <td class="px-4 py-3 text-sm text-content-secondary">{{ row.client_kind || '-' }}</td>
              <td class="max-w-[300px] px-4 py-3 text-xs text-content-secondary"><div class="truncate" :title="row.user_agent">{{ row.user_agent || '-' }}</div><div v-if="row.violation_count > 0 || row.auto_banned" class="mt-1 text-gray-400"><span v-if="row.counted_violation">{{ t('admin.riskControl.requestControl.violationCount', { count: row.violation_count }) }}</span><span v-else>{{ t('admin.riskControl.requestControl.notCounted') }}</span><span v-if="row.hit_email_sent"> / {{ t('admin.riskControl.requestControl.hitEmailSent') }}</span><span v-if="row.ban_email_sent"> / {{ t('admin.riskControl.requestControl.banEmailSent') }}</span><span v-else-if="row.email_sent && !row.hit_email_sent"> / {{ t('admin.riskControl.requestControl.emailSent') }}</span><span v-if="row.auto_banned"> / {{ t('admin.riskControl.requestControl.autoBanned') }}</span></div><div v-if="Object.keys(row.details || {}).length" class="mt-1 truncate text-gray-400">{{ detailText(row) }}</div></td>
              <td class="max-w-[220px] px-4 py-3 text-xs text-gray-500">
                <span class="block truncate" :title="row.tls_fingerprint">{{ row.tls_fingerprint || t('admin.riskControl.requestControl.tlsUnavailable') }}</span>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-right">
                <button
                  type="button"
                  class="btn btn-ghost btn-sm"
                  :title="t('admin.riskControl.requestControl.viewDetail')"
                  :aria-label="t('admin.riskControl.requestControl.viewDetail')"
                  @click="openLogDetail(row)"
                >
                  <Icon name="eye" size="sm" />
                </button>
              </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
      <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="onPageChange" @update:pageSize="onPageSizeChange" />
    </section>
  </div>

  <BaseDialog
    :show="detailOpen"
    :title="t('admin.riskControl.requestControl.detailTitle')"
    width="extra-wide"
    @close="closeLogDetail"
  >
    <div v-if="detailLoading" class="py-12 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="detailError" class="py-12 text-center text-sm text-red-600 dark:text-red-300">
      {{ t('admin.riskControl.requestControl.detailFailed') }}
    </div>
    <div v-else-if="detailLog" class="space-y-5">
      <div class="grid grid-cols-1 gap-3 rounded-lg border border-gray-100 bg-gray-50 p-4 text-sm dark:border-dark-700 dark:bg-dark-900/30 sm:grid-cols-2 lg:grid-cols-4">
        <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.requestID') }}</p><p class="mt-1 break-all font-mono text-content-primary">{{ detailLog.request_id || '-' }}</p></div>
        <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.endpoint') }}</p><p class="mt-1 break-all text-content-primary">{{ detailLog.endpoint || '-' }}</p></div>
        <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.client') }}</p><p class="mt-1 break-all text-content-primary">{{ detailLog.client_kind || '-' }}</p></div>
        <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.result') }}</p><p class="mt-1 text-xs text-gray-400">{{ t('admin.riskControl.requestControl.actual') }}</p><p class="mt-1 text-content-primary">{{ detailLog.action || '-' }} / {{ detailLog.reason || '-' }}</p><p v-if="detailLog.expected_action" class="mt-1 text-xs text-blue-600 dark:text-blue-300">{{ t('admin.riskControl.requestControl.expected') }}: {{ detailLog.expected_action }} / {{ detailLog.expected_reason }}<span v-if="detailLog.expected_status_code"> ({{ detailLog.expected_status_code }})</span></p></div>
      </div>
      <div class="grid grid-cols-1 gap-5 xl:grid-cols-2">
        <section class="min-w-0">
          <h4 class="mb-2 text-sm font-semibold text-content-primary">{{ t('admin.riskControl.requestControl.requestHeaders') }}</h4>
          <pre class="max-h-[32rem] min-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg border border-gray-100 bg-gray-950 p-4 text-xs leading-5 text-gray-100 dark:border-dark-700">{{ formatMetadata(detailLog.request_headers) }}</pre>
        </section>
        <section class="min-w-0">
          <h4 class="mb-2 text-sm font-semibold text-content-primary">{{ t('admin.riskControl.requestControl.requestBodyMetadata') }}</h4>
          <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.requestControl.detailBodyHint') }}</p>
          <pre class="max-h-[32rem] min-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg border border-gray-100 bg-gray-950 p-4 text-xs leading-5 text-gray-100 dark:border-dark-700">{{ formatMetadata(detailLog.request_body_metadata) }}</pre>
        </section>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api/admin'
import type { RequestControlConfig, RequestControlLog, RequestControlLogDetail, RequestControlStatus, RequestControlUserRule } from '@/api/admin/riskControl'
import type { AdminGroup, AdminUser } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const saving = ref(false)
const logsLoading = ref(false)
const groups = ref<AdminGroup[]>([])
const logs = ref<RequestControlLog[]>([])
const detailOpen = ref(false)
const detailLoading = ref(false)
const detailError = ref(false)
const detailLog = ref<RequestControlLogDetail | null>(null)
const status = ref<RequestControlStatus | null>(null)
const ruleUserID = ref<number | null>(null)
const ruleUAText = ref('')
const editingRuleUserID = ref<number | null>(null)
const userSearch = ref('')
const userSearchLoading = ref(false)
const userSearchDone = ref(false)
const userSearchResults = ref<AdminUser[]>([])
const selectedUserLabel = ref('')
const modelText = ref('')
const globalUAText = ref('')

const form = reactive<RequestControlConfig>({
  enabled: false,
  block_openai_chat: true,
  block_claude_messages: true,
  block_openai_responses: true,
  all_groups: true,
  group_ids: [],
  model_filter: { type: 'all', models: [] },
  all_users: true,
  user_rules: [],
  global_user_agent_whitelist: [],
  block_status: 403,
  block_message: '内容违规，多次尝试将被封禁',
  email_on_hit: true,
  auto_ban_enabled: true,
  ban_threshold: 4,
  violation_window_hours: 720,
})
const filters = reactive({ action: '', protocol: '', group_id: 0, search: '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0, pages: 1 })

const modelFilterOptions = computed(() => [
  { value: 'all', label: t('admin.riskControl.modelFilterAll') },
  { value: 'include', label: t('admin.riskControl.modelFilterInclude') },
  { value: 'exclude', label: t('admin.riskControl.modelFilterExclude') },
])
const actionOptions = computed(() => [
  { value: '', label: t('admin.riskControl.requestControl.allActions') },
  { value: 'block', label: t('admin.riskControl.requestControl.blocked') },
  { value: 'observe', label: t('admin.riskControl.requestControl.observed') },
])
const protocolOptions = computed(() => [
  { value: '', label: t('admin.riskControl.requestControl.allProtocols') },
  { value: 'openai_chat_completions', label: 'OpenAI Chat' },
  { value: 'anthropic_messages', label: 'Claude Messages' },
  { value: 'openai_responses', label: 'OpenAI Responses' },
])
const groupFilterOptions = computed(() => [
  { value: 0, label: t('admin.riskControl.requestControl.allGroupsFilter') },
  ...groups.value.map((group) => ({ value: group.id, label: group.name })),
])
const logHeadings = computed(() => [
  t('admin.riskControl.requestControl.time'), t('admin.riskControl.requestControl.user'), t('admin.riskControl.requestControl.endpoint'), t('admin.riskControl.requestControl.result'), t('admin.riskControl.requestControl.client'), t('admin.riskControl.requestControl.observation'), t('admin.riskControl.requestControl.loggedAt'), t('admin.riskControl.requestControl.detail'),
])
const statusItems = computed(() => [
  { key: 'queue', label: t('admin.riskControl.requestControl.queue'), value: `${status.value?.queue_length ?? 0}/${status.value?.queue_size ?? 0}` },
  { key: 'processed', label: t('admin.riskControl.requestControl.processed'), value: String(status.value?.processed ?? 0) },
  { key: 'dropped', label: t('admin.riskControl.requestControl.dropped'), value: String(status.value?.dropped ?? 0) },
  { key: 'errors', label: t('admin.riskControl.requestControl.errors'), value: String(status.value?.errors ?? 0) },
])

function lines(value: string): string[] { return value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean) }
function applyConfig(config: RequestControlConfig) {
  Object.assign(form, config)
  form.block_openai_chat = config.block_openai_chat ?? true
  form.block_claude_messages = config.block_claude_messages ?? true
  form.block_openai_responses = config.block_openai_responses ?? true
  form.group_ids = [...(config.group_ids || [])]
  form.model_filter = { type: config.model_filter?.type || 'all', models: [...(config.model_filter?.models || [])] }
  form.user_rules = (config.user_rules || []).map((rule) => ({ ...rule, user_agent_whitelist: [...(rule.user_agent_whitelist || [])] }))
  form.global_user_agent_whitelist = [...(config.global_user_agent_whitelist || [])]
  modelText.value = form.model_filter.models.join('\n')
  globalUAText.value = form.global_user_agent_whitelist.join('\n')
}
function addRule() {
  const userID = Number(ruleUserID.value)
  if (!Number.isInteger(userID) || userID <= 0) { appStore.showError(t('admin.riskControl.requestControl.invalidUserID')); return }
  const index = form.user_rules.findIndex((rule) => rule.user_id === userID)
  const next: RequestControlUserRule = { user_id: userID, participate: index >= 0 ? form.user_rules[index].participate : true, user_agent_whitelist: lines(ruleUAText.value) }
  if (index >= 0) form.user_rules[index] = next
  else form.user_rules.push(next)
  ruleUserID.value = null
  ruleUAText.value = ''
  editingRuleUserID.value = null
  selectedUserLabel.value = ''
}
function editRule(rule: RequestControlUserRule) {
  editingRuleUserID.value = rule.user_id
  ruleUserID.value = rule.user_id
  ruleUAText.value = rule.user_agent_whitelist.join('\n')
  selectedUserLabel.value = `UID ${rule.user_id}`
}
function cancelEditRule() {
  editingRuleUserID.value = null
  ruleUserID.value = null
  ruleUAText.value = ''
  selectedUserLabel.value = ''
}
function removeRule(userID: number) {
  form.user_rules = form.user_rules.filter((rule) => rule.user_id !== userID)
  if (editingRuleUserID.value === userID) cancelEditRule()
}
async function searchUsers() {
  const search = userSearch.value.trim()
  if (!search) return
  userSearchLoading.value = true
  userSearchDone.value = false
  try {
    if (/^\d+$/.test(search)) {
      const user = await adminAPI.users.getById(Number(search))
      userSearchResults.value = user ? [user] : []
    } else {
      const result = await adminAPI.users.list(1, 10, { search, include_subscriptions: false })
      userSearchResults.value = result.items || []
    }
    userSearchDone.value = true
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.requestControl.userSearchFailed')))
  } finally {
    userSearchLoading.value = false
  }
}
function selectUser(user: AdminUser) {
  if (editingRuleUserID.value != null) cancelEditRule()
  ruleUserID.value = user.id
  selectedUserLabel.value = `${user.username || user.email} (UID ${user.id})`
  userSearch.value = ''
  userSearchResults.value = []
  userSearchDone.value = false
}
async function load() {
  try {
    const [config, allGroups] = await Promise.all([adminAPI.riskControl.getRequestControlConfig(), adminAPI.groups.getAll()])
    applyConfig(config)
    groups.value = allGroups
    await Promise.all([loadStatus(), loadLogs()])
  } catch (error) { appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.requestControl.loadFailed'))) }
}
async function loadStatus() { try { status.value = await adminAPI.riskControl.getRequestControlStatus() } catch (error) { appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.requestControl.statusFailed'))) } }
async function save() {
  saving.value = true
  try {
    form.model_filter.models = form.model_filter.type === 'all' ? [] : lines(modelText.value)
    form.global_user_agent_whitelist = lines(globalUAText.value)
    const updated = await adminAPI.riskControl.updateRequestControlConfig({ ...form, group_ids: [...form.group_ids], user_rules: form.user_rules.map((rule) => ({ ...rule, user_agent_whitelist: [...rule.user_agent_whitelist] })) })
    applyConfig(updated)
    await loadStatus()
    appStore.showSuccess(t('admin.riskControl.requestControl.saved'))
  } catch (error) { appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.requestControl.saveFailed'))) }
  finally { saving.value = false }
}
async function loadLogs() {
  logsLoading.value = true
  try {
    const result = await adminAPI.riskControl.listRequestControlLogs({ page: pagination.page, page_size: pagination.page_size, action: filters.action || undefined, protocol: filters.protocol || undefined, group_id: filters.group_id || undefined, search: filters.search || undefined })
    logs.value = result.items || []
    pagination.total = result.total || 0
    pagination.pages = result.pages || 1
  } catch (error) { appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.requestControl.logsFailed'))) }
  finally { logsLoading.value = false }
}
function reloadLogs() { pagination.page = 1; void loadLogs() }
function onPageChange(page: number) { pagination.page = page; void loadLogs() }
function onPageSizeChange(pageSize: number) { pagination.page_size = pageSize; pagination.page = 1; void loadLogs() }
function formatDate(value: string) { return formatDateTime(value) || '-' }
function actionClass(row: RequestControlLog) { return row.blocked ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300' : row.observed ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300' : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300' }
function detailText(row: RequestControlLog) { return Object.entries(row.details || {}).map(([key, value]) => `${key}: ${value}`).join(' · ') }
function formatMetadata(value: unknown) {
  try { return JSON.stringify(value ?? {}, null, 2) }
  catch { return '{}' }
}
function closeLogDetail() {
  detailOpen.value = false
  detailLog.value = null
  detailError.value = false
}
async function openLogDetail(row: RequestControlLog) {
  detailOpen.value = true
  detailLoading.value = true
  detailError.value = false
  detailLog.value = null
  try {
    detailLog.value = await adminAPI.riskControl.getRequestControlLog(row.id)
  } catch (error) {
    detailError.value = true
    appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.requestControl.detailFailed')))
  } finally {
    detailLoading.value = false
  }
}

onMounted(() => { void load() })
</script>
