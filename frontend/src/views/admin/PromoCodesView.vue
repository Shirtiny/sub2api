<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="space-y-4">
          <div class="border-b border-stroke-default">
            <nav class="-mb-px flex gap-4" aria-label="Promo tabs" role="tablist">
              <button
                v-for="tab in tabs"
                :key="tab.value"
                type="button"
                role="tab"
                :aria-selected="activeTab === tab.value"
                :data-test="`promo-tab-${tab.value}`"
                @click="switchTab(tab.value)"
                :class="[
                  'border-b-2 px-1 py-2 text-sm font-medium transition-colors',
                  activeTab === tab.value
                    ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                    : 'border-transparent text-content-tertiary hover:border-stroke-default hover:text-content-primary'
                ]"
              >
                {{ tab.label }}
              </button>
            </nav>
          </div>

          <div v-if="activeTab === 'registration'" class="flex flex-wrap items-center gap-3">
            <div class="flex-1 sm:max-w-64">
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.promo.searchCodes')"
                class="input"
                @input="handleSearch"
              />
            </div>
            <Select
              v-model="filters.status"
              :options="filterStatusOptions"
              class="w-36"
              @change="loadCodes"
            />

            <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
              <button
                @click="loadCodes"
                :disabled="loading"
                class="btn btn-secondary"
                :title="t('common.refresh')"
              >
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <button @click="showCreateDialog = true" class="btn btn-primary">
                <Icon name="plus" size="md" class="mr-1" />
                {{ t('admin.promo.createCode') }}
              </button>
            </div>
          </div>

          <div v-else class="flex flex-wrap items-center gap-3">
            <div class="flex-1 sm:max-w-64">
              <input
                v-model="cafeSearchQuery"
                type="text"
                :placeholder="t('admin.promo.cafeCoupon.searchPlaceholder')"
                class="input"
                @input="handleCafeSearch"
              />
            </div>
            <Select
              v-model="cafeFilters.status"
              :options="cafeStatusFilterOptions"
              class="w-36"
              @change="loadCafeCoupons"
            />
            <Select
              v-model="cafeFilters.type"
              :options="cafeTypeOptions"
              class="w-36"
              @change="loadCafeCoupons"
            />
            <Select
              v-model="cafeFilters.membership_level"
              :options="cafeMembershipLevelOptions"
              class="w-36"
              @change="loadCafeCoupons"
            />

            <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
              <button
                @click="loadCafeCoupons"
                :disabled="cafeLoading"
                class="btn btn-secondary"
                :title="t('common.refresh')"
              >
                <Icon name="refresh" size="md" :class="cafeLoading ? 'animate-spin' : ''" />
              </button>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          v-if="activeTab === 'registration'"
          :columns="columns"
          :data="codes"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-code="{ value }">
            <div class="flex items-center space-x-2">
              <code class="font-mono text-sm text-content-primary">{{ value }}</code>
              <button
                @click="copyToClipboard(value)"
                :class="[
                  'flex items-center transition-colors',
                  copiedCode === value
                    ? 'text-green-500'
                    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                ]"
                :title="copiedCode === value ? t('admin.promo.copied') : t('keys.copyToClipboard')"
              >
                <Icon v-if="copiedCode !== value" name="copy" size="sm" :stroke-width="2" />
                <svg v-else class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              </button>
            </div>
          </template>

          <template #cell-bonus_amount="{ value }">
            <span class="text-sm font-medium text-content-primary">
              ${{ value.toFixed(2) }}
            </span>
          </template>

          <template #cell-usage="{ row }">
            <span class="text-sm text-gray-600 dark:text-gray-300">
              {{ row.used_count }} / {{ row.max_uses === 0 ? '∞' : row.max_uses }}
            </span>
          </template>

          <template #cell-status="{ value, row }">
            <span :class="['badge', getStatusClass(value, row)]">
              {{ getStatusLabel(value, row) }}
            </span>
          </template>

          <template #cell-expires_at="{ value }">
            <span class="text-sm text-content-tertiary">
              {{ value ? formatDateTime(value) : t('admin.promo.neverExpires') }}
            </span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-content-tertiary">
              {{ formatDateTime(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center space-x-1">
              <button
                @click="copyRegisterLink(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
                :title="t('admin.promo.copyRegisterLink')"
              >
                <Icon name="link" size="sm" />
              </button>
              <button
                @click="handleViewUsages(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                :title="t('admin.promo.viewUsages')"
              >
                <Icon name="eye" size="sm" />
              </button>
              <button
                @click="handleEdit(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-300"
                :title="t('common.edit')"
              >
                <Icon name="edit" size="sm" />
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('common.delete')"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
        </DataTable>

        <DataTable
          v-else
          :columns="cafeCouponColumns"
          :data="cafeCoupons"
          :loading="cafeLoading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleCafeSort"
        >
          <template #cell-code="{ value }">
            <div class="flex items-center space-x-2">
              <code class="font-mono text-sm text-content-primary">{{ value }}</code>
              <button
                @click="copyToClipboard(value)"
                :class="[
                  'flex items-center transition-colors',
                  copiedCode === value
                    ? 'text-green-500'
                    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                ]"
                :title="copiedCode === value ? t('admin.promo.copied') : t('keys.copyToClipboard')"
              >
                <Icon v-if="copiedCode !== value" name="copy" size="sm" :stroke-width="2" />
                <Icon v-else name="check" size="sm" :stroke-width="2" />
              </button>
            </div>
          </template>

          <template #cell-user="{ row }">
            <div class="text-sm">
              <div class="font-medium text-content-primary">{{ getCafeCouponUserLabel(row) }}</div>
              <div class="text-xs text-content-tertiary">ID: {{ row.user_id }}</div>
            </div>
          </template>

          <template #cell-membership_level="{ value }">
            <span class="text-sm text-content-primary">
              {{ t('admin.promo.cafeCoupon.levelLabel', { level: value }) }}
            </span>
          </template>

          <template #cell-type_value="{ row }">
            <div class="text-sm">
              <div class="font-medium text-content-primary">{{ getCafeCouponTypeLabel(row.type) }}</div>
              <div class="text-xs text-content-tertiary">{{ formatCafeCouponValue(row) }}</div>
            </div>
          </template>

          <template #cell-status="{ value }">
            <span :class="['badge', getCafeCouponStatusClass(value)]">
              {{ getCafeCouponStatusLabel(value) }}
            </span>
          </template>

          <template #cell-expires_at="{ value, row }">
            <div class="text-sm">
              <div class="text-content-primary">{{ formatDateTime(value) }}</div>
              <div class="text-xs text-content-tertiary">
                {{ t('admin.promo.cafeCoupon.periodRange', { start: formatDateTime(row.period_start), end: formatDateTime(row.period_end) }) }}
              </div>
            </div>
          </template>

          <template #cell-applied_order="{ row }">
            <div class="text-sm">
              <div class="text-content-primary">
                {{ row.applied_at ? formatDateTime(row.applied_at) : t('admin.promo.cafeCoupon.notApplied') }}
              </div>
              <div v-if="row.order_id" class="text-xs text-content-tertiary">
                {{ t('admin.promo.cafeCoupon.orderPrefix', { id: row.order_id }) }}
              </div>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-content-tertiary">
              {{ formatDateTime(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center space-x-1">
              <button
                type="button"
                data-test="status-cafe-coupon"
                @click="handleEditCafeCouponStatus(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                :title="t('admin.promo.cafeCoupon.changeStatus')"
              >
                <Icon name="edit" size="sm" />
              </button>
              <button
                type="button"
                data-test="reset-cafe-coupon"
                @click="handleResetCafeCoupon(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-amber-50 hover:text-amber-600 dark:hover:bg-amber-900/20 dark:hover:text-amber-400"
                :title="t('admin.promo.cafeCoupon.resetClaimPeriod')"
              >
                <Icon name="refresh" size="sm" />
              </button>
              <button
                v-if="canVoidCafeCoupon(row)"
                type="button"
                data-test="void-cafe-coupon"
                @click="handleVoidCafeCoupon(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('admin.promo.cafeCoupon.void')"
              >
                <Icon name="ban" size="sm" />
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="activeTab === 'registration' && pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
        <Pagination
          v-else-if="activeTab === 'cafe' && cafePagination.total > 0"
          :page="cafePagination.page"
          :total="cafePagination.total"
          :page-size="cafePagination.page_size"
          @update:page="handleCafePageChange"
          @update:pageSize="handleCafePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="showCreateDialog"
      :title="t('admin.promo.createCode')"
      width="normal"
      @close="showCreateDialog = false"
    >
      <form id="create-promo-form" @submit.prevent="handleCreate" class="space-y-4">
        <div>
          <label class="input-label">
            {{ t('admin.promo.code') }}
            <span class="ml-1 text-xs font-normal text-gray-400">({{ t('admin.promo.autoGenerate') }})</span>
          </label>
          <input
            v-model="createForm.code"
            type="text"
            class="input font-mono uppercase"
            :placeholder="t('admin.promo.codePlaceholder')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.promo.bonusAmount') }}</label>
          <input
            v-model.number="createForm.bonus_amount"
            type="number"
            step="0.01"
            min="0"
            required
            class="input"
          />
        </div>
        <div>
          <label class="input-label">
            {{ t('admin.promo.maxUses') }}
            <span class="ml-1 text-xs font-normal text-gray-400">({{ t('admin.promo.zeroUnlimited') }})</span>
          </label>
          <input
            v-model.number="createForm.max_uses"
            type="number"
            min="0"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">
            {{ t('admin.promo.expiresAt') }}
            <span class="ml-1 text-xs font-normal text-gray-400">({{ t('common.optional') }})</span>
          </label>
          <input
            v-model="createForm.expires_at_str"
            type="datetime-local"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">
            {{ t('admin.promo.notes') }}
            <span class="ml-1 text-xs font-normal text-gray-400">({{ t('common.optional') }})</span>
          </label>
          <textarea
            v-model="createForm.notes"
            rows="2"
            class="input"
            :placeholder="t('admin.promo.notesPlaceholder')"
          ></textarea>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showCreateDialog = false" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" form="create-promo-form" :disabled="creating" class="btn btn-primary">
            {{ creating ? t('common.creating') : t('common.create') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showEditDialog"
      :title="t('admin.promo.editCode')"
      width="normal"
      @close="closeEditDialog"
    >
      <form id="edit-promo-form" @submit.prevent="handleUpdate" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.promo.code') }}</label>
          <input
            v-model="editForm.code"
            type="text"
            class="input font-mono uppercase"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.promo.bonusAmount') }}</label>
          <input
            v-model.number="editForm.bonus_amount"
            type="number"
            step="0.01"
            min="0"
            required
            class="input"
          />
        </div>
        <div>
          <label class="input-label">
            {{ t('admin.promo.maxUses') }}
            <span class="ml-1 text-xs font-normal text-gray-400">({{ t('admin.promo.zeroUnlimited') }})</span>
          </label>
          <input
            v-model.number="editForm.max_uses"
            type="number"
            min="0"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.promo.status') }}</label>
          <Select v-model="editForm.status" :options="statusOptions" />
        </div>
        <div>
          <label class="input-label">
            {{ t('admin.promo.expiresAt') }}
            <span class="ml-1 text-xs font-normal text-gray-400">({{ t('common.optional') }})</span>
          </label>
          <input
            v-model="editForm.expires_at_str"
            type="datetime-local"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">
            {{ t('admin.promo.notes') }}
            <span class="ml-1 text-xs font-normal text-gray-400">({{ t('common.optional') }})</span>
          </label>
          <textarea
            v-model="editForm.notes"
            rows="2"
            class="input"
          ></textarea>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="closeEditDialog" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" form="edit-promo-form" :disabled="updating" class="btn btn-primary">
            {{ updating ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showUsagesDialog"
      :title="t('admin.promo.usageRecords')"
      width="wide"
      @close="showUsagesDialog = false"
    >
      <div v-if="usagesLoading" class="flex items-center justify-center py-8">
        <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
      </div>
      <div v-else-if="usages.length === 0" class="py-8 text-center text-gray-500 dark:text-gray-400">
        {{ t('admin.promo.noUsages') }}
      </div>
      <div v-else class="space-y-3">
        <div
          v-for="usage in usages"
          :key="usage.id"
          class="flex items-center justify-between rounded-lg border border-gray-200 p-3 dark:border-dark-600"
        >
          <div class="flex items-center gap-3">
            <div class="flex h-8 w-8 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
              <Icon name="user" size="sm" class="text-green-600 dark:text-green-400" />
            </div>
            <div>
              <p class="text-sm font-medium text-content-primary">
                {{ usage.user?.email || t('admin.promo.userPrefix', { id: usage.user_id }) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ formatDateTime(usage.used_at) }}
              </p>
            </div>
          </div>
          <div class="text-right">
            <span class="text-sm font-medium text-green-600 dark:text-green-400">
              +${{ usage.bonus_amount.toFixed(2) }}
            </span>
          </div>
        </div>
        <div v-if="usagesTotal > usagesPageSize" class="mt-4">
          <Pagination
            :page="usagesPage"
            :total="usagesTotal"
            :page-size="usagesPageSize"
            @update:page="handleUsagesPageChange"
            @update:page-size="(size: number) => { usagesPageSize = size; usagesPage = 1; loadUsages() }"
          />
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button type="button" @click="showUsagesDialog = false" class="btn btn-secondary">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.promo.deleteCode')"
      :message="t('admin.promo.deleteCodeConfirm')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />

    <BaseDialog
      :show="showCafeStatusDialog"
      :title="t('admin.promo.cafeCoupon.changeStatus')"
      width="normal"
      @close="closeCafeStatusDialog"
    >
      <div class="space-y-4">
        <p v-if="editingCafeCoupon" class="text-sm text-content-secondary">
          {{ editingCafeCoupon.code }} · {{ getCafeCouponStatusLabel(editingCafeCoupon.status) }}
        </p>
        <div>
          <label class="input-label">{{ t('admin.promo.cafeCoupon.targetStatus') }}</label>
          <Select v-model="cafeStatusForm.status" :options="cafeStatusOptions" />
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="closeCafeStatusDialog" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button type="button" :disabled="updatingCafeCouponStatus" @click="confirmCafeCouponStatus" class="btn btn-primary">
            {{ updatingCafeCouponStatus ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showResetCafeDialog"
      :title="t('admin.promo.cafeCoupon.resetClaimPeriod')"
      :message="t('admin.promo.cafeCoupon.resetClaimPeriodConfirm')"
      :confirm-text="t('admin.promo.cafeCoupon.resetClaimPeriod')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmResetCafeCoupon"
      @cancel="closeResetCafeDialog"
    />

    <ConfirmDialog
      :show="showVoidCafeDialog"
      :title="t('admin.promo.cafeCoupon.voidCoupon')"
      :message="t('admin.promo.cafeCoupon.voidCouponConfirm')"
      :confirm-text="t('admin.promo.cafeCoupon.void')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmVoidCafeCoupon"
      @cancel="closeVoidCafeDialog"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type {
  AdminCafeCoupon,
  CafeCouponMembershipLevel,
  CafeCouponStatus,
  CafeCouponType,
  PromoCode,
  PromoCodeUsage
} from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

type PromoTab = 'registration' | 'cafe'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard: clipboardCopy } = useClipboard()

const activeTab = ref<PromoTab>('registration')

const tabs = computed(() => [
  { value: 'registration' as PromoTab, label: t('admin.promo.tabs.registration') },
  { value: 'cafe' as PromoTab, label: t('admin.promo.tabs.cafeCoupons') }
])

const codes = ref<PromoCode[]>([])
const loading = ref(false)
const creating = ref(false)
const updating = ref(false)
const searchQuery = ref('')
const copiedCode = ref<string | null>(null)

const cafeCoupons = ref<AdminCafeCoupon[]>([])
const cafeLoading = ref(false)
const cafeSearchQuery = ref('')
const voidingCafeCoupon = ref(false)
const updatingCafeCouponStatus = ref(false)
const resettingCafeCoupon = ref(false)

const filters = reactive({
  status: ''
})

const cafeFilters = reactive<{
  status: string
  type: string
  membership_level: '' | CafeCouponMembershipLevel
}>({
  status: '',
  type: '',
  membership_level: ''
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const cafePagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})
const cafeSortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const showCreateDialog = ref(false)
const showEditDialog = ref(false)
const showDeleteDialog = ref(false)
const showUsagesDialog = ref(false)
const showVoidCafeDialog = ref(false)
const showCafeStatusDialog = ref(false)
const showResetCafeDialog = ref(false)

const editingCode = ref<PromoCode | null>(null)
const deletingCode = ref<PromoCode | null>(null)
const voidingCoupon = ref<AdminCafeCoupon | null>(null)
const editingCafeCoupon = ref<AdminCafeCoupon | null>(null)
const resettingCoupon = ref<AdminCafeCoupon | null>(null)

const usages = ref<PromoCodeUsage[]>([])
const usagesLoading = ref(false)
const currentViewingCode = ref<PromoCode | null>(null)
const usagesPage = ref(1)
const usagesPageSize = ref(20)
const usagesTotal = ref(0)

const createForm = reactive({
  code: '',
  bonus_amount: 1,
  max_uses: 0,
  expires_at_str: '',
  notes: ''
})

const editForm = reactive({
  code: '',
  bonus_amount: 0,
  max_uses: 0,
  status: 'active' as 'active' | 'disabled',
  expires_at_str: '',
  notes: ''
})

const cafeStatusForm = reactive({
  status: 'issued' as CafeCouponStatus
})

const filterStatusOptions = computed(() => [
  { value: '', label: t('admin.promo.allStatus') },
  { value: 'active', label: t('admin.promo.statusActive') },
  { value: 'disabled', label: t('admin.promo.statusDisabled') }
])

const statusOptions = computed(() => [
  { value: 'active', label: t('admin.promo.statusActive') },
  { value: 'disabled', label: t('admin.promo.statusDisabled') }
])

const cafeStatusFilterOptions = computed(() => [
  { value: '', label: t('admin.promo.allStatus') },
  { value: 'issued', label: t('admin.promo.cafeCoupon.statusIssued') },
  { value: 'applied', label: t('admin.promo.cafeCoupon.statusApplied') },
  { value: 'void', label: t('admin.promo.cafeCoupon.statusVoid') }
])

const cafeStatusOptions = computed(() => [
  { value: 'issued', label: t('admin.promo.cafeCoupon.statusIssued') },
  { value: 'void', label: t('admin.promo.cafeCoupon.statusVoid') }
])

const cafeTypeOptions = computed(() => [
  { value: '', label: t('admin.promo.cafeCoupon.allTypes') },
  { value: 'cash', label: t('admin.promo.cafeCoupon.typeCash') },
  { value: 'discount', label: t('admin.promo.cafeCoupon.typeDiscount') }
])

const cafeMembershipLevelOptions = computed(() => [
  { value: '', label: t('admin.promo.cafeCoupon.allLevels') },
  ...[0, 1, 2, 3].map((level) => ({
    value: level,
    label: t('admin.promo.cafeCoupon.levelLabel', { level })
  }))
])

const columns = computed<Column[]>(() => [
  { key: 'code', label: t('admin.promo.columns.code') },
  { key: 'bonus_amount', label: t('admin.promo.columns.bonusAmount'), sortable: true },
  { key: 'usage', label: t('admin.promo.columns.usage') },
  { key: 'status', label: t('admin.promo.columns.status'), sortable: true },
  { key: 'expires_at', label: t('admin.promo.columns.expiresAt'), sortable: true },
  { key: 'created_at', label: t('admin.promo.columns.createdAt'), sortable: true },
  { key: 'actions', label: t('admin.promo.columns.actions') }
])

const cafeCouponColumns = computed<Column[]>(() => [
  { key: 'code', label: t('admin.promo.cafeCoupon.columns.code'), sortable: true },
  { key: 'user', label: t('admin.promo.cafeCoupon.columns.user') },
  { key: 'membership_level', label: t('admin.promo.cafeCoupon.columns.membershipLevel'), sortable: true },
  { key: 'type_value', label: t('admin.promo.cafeCoupon.columns.typeValue') },
  { key: 'status', label: t('admin.promo.cafeCoupon.columns.status'), sortable: true },
  { key: 'expires_at', label: t('admin.promo.cafeCoupon.columns.expiresAt'), sortable: true },
  { key: 'applied_order', label: t('admin.promo.cafeCoupon.columns.appliedOrder') },
  { key: 'created_at', label: t('admin.promo.cafeCoupon.columns.createdAt'), sortable: true },
  { key: 'actions', label: t('admin.promo.cafeCoupon.columns.actions') }
])

const getStatusClass = (status: string, row: PromoCode) => {
  if (row.expires_at && new Date(row.expires_at) < new Date()) {
    return 'badge-danger'
  }
  if (row.max_uses > 0 && row.used_count >= row.max_uses) {
    return 'badge-gray'
  }
  return status === 'active' ? 'badge-success' : 'badge-gray'
}

const getStatusLabel = (status: string, row: PromoCode) => {
  if (row.expires_at && new Date(row.expires_at) < new Date()) {
    return t('admin.promo.statusExpired')
  }
  if (row.max_uses > 0 && row.used_count >= row.max_uses) {
    return t('admin.promo.statusMaxUsed')
  }
  return status === 'active' ? t('admin.promo.statusActive') : t('admin.promo.statusDisabled')
}

const getCafeCouponTypeLabel = (type: CafeCouponType | string) => {
  return type === 'discount'
    ? t('admin.promo.cafeCoupon.typeDiscount')
    : t('admin.promo.cafeCoupon.typeCash')
}

const formatCafeCouponValue = (coupon: AdminCafeCoupon) => {
  return coupon.type === 'discount' ? `${coupon.value}%` : `$${coupon.value.toFixed(2)}`
}

const getCafeCouponStatusClass = (status: CafeCouponStatus | string) => {
  switch (status) {
    case 'issued':
      return 'badge-success'
    case 'applied':
      return 'badge-primary'
    default:
      return 'badge-gray'
  }
}

const getCafeCouponStatusLabel = (status: CafeCouponStatus | string) => {
  switch (status) {
    case 'issued':
      return t('admin.promo.cafeCoupon.statusIssued')
    case 'applied':
      return t('admin.promo.cafeCoupon.statusApplied')
    case 'void':
      return t('admin.promo.cafeCoupon.statusVoid')
    default:
      return status
  }
}

const getCafeCouponUserLabel = (coupon: AdminCafeCoupon) => {
  return coupon.user?.email || coupon.user?.username || t('admin.promo.userPrefix', { id: coupon.user_id })
}

const canVoidCafeCoupon = (coupon: AdminCafeCoupon) => {
  return coupon.status === 'issued' && !coupon.order_id && !coupon.applied_at
}

let abortController: AbortController | null = null
let cafeAbortController: AbortController | null = null

const loadCodes = async () => {
  if (abortController) {
    abortController.abort()
  }
  const currentController = new AbortController()
  abortController = currentController
  loading.value = true

  try {
    const response = await adminAPI.promo.list(
      pagination.page,
      pagination.page_size,
      {
        status: filters.status || undefined,
        search: searchQuery.value || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      { signal: currentController.signal }
    )
    if (currentController.signal.aborted || abortController !== currentController) return

    codes.value = response.items
    pagination.total = response.total
  } catch (error: any) {
    if (
      currentController.signal.aborted ||
      abortController !== currentController ||
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED'
    ) {
      return
    }
    appStore.showError(t('admin.promo.failedToLoad'))
    console.error('Error loading promo codes:', error)
  } finally {
    if (abortController === currentController) {
      loading.value = false
      abortController = null
    }
  }
}

const loadCafeCoupons = async () => {
  if (cafeAbortController) {
    cafeAbortController.abort()
  }
  const currentController = new AbortController()
  cafeAbortController = currentController
  cafeLoading.value = true

  try {
    const response = await adminAPI.promo.listCafeCoupons(
      cafePagination.page,
      cafePagination.page_size,
      {
        search: cafeSearchQuery.value || undefined,
        status: cafeFilters.status || undefined,
        type: cafeFilters.type || undefined,
        membership_level: cafeFilters.membership_level === '' ? undefined : cafeFilters.membership_level,
        sort_by: cafeSortState.sort_by,
        sort_order: cafeSortState.sort_order
      },
      { signal: currentController.signal }
    )
    if (currentController.signal.aborted || cafeAbortController !== currentController) return

    cafeCoupons.value = response.items
    cafePagination.total = response.total
  } catch (error: any) {
    if (
      currentController.signal.aborted ||
      cafeAbortController !== currentController ||
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED'
    ) {
      return
    }
    appStore.showError(t('admin.promo.cafeCoupon.failedToLoad'))
    console.error('Error loading cafe coupons:', error)
  } finally {
    if (cafeAbortController === currentController) {
      cafeLoading.value = false
      cafeAbortController = null
    }
  }
}

let searchTimeout: ReturnType<typeof setTimeout>
let cafeSearchTimeout: ReturnType<typeof setTimeout>

const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadCodes()
  }, 300)
}

const handleCafeSearch = () => {
  clearTimeout(cafeSearchTimeout)
  cafeSearchTimeout = setTimeout(() => {
    cafePagination.page = 1
    loadCafeCoupons()
  }, 300)
}

const switchTab = (tab: PromoTab) => {
  if (activeTab.value === tab) return
  activeTab.value = tab
  if (tab === 'cafe' && cafeCoupons.value.length === 0) {
    loadCafeCoupons()
  }
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadCodes()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadCodes()
}

const handleCafePageChange = (page: number) => {
  cafePagination.page = page
  loadCafeCoupons()
}

const handleCafePageSizeChange = (pageSize: number) => {
  cafePagination.page_size = pageSize
  cafePagination.page = 1
  loadCafeCoupons()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadCodes()
}

const handleCafeSort = (key: string, order: 'asc' | 'desc') => {
  cafeSortState.sort_by = key
  cafeSortState.sort_order = order
  cafePagination.page = 1
  loadCafeCoupons()
}

const copyToClipboard = async (text: string) => {
  const success = await clipboardCopy(text, t('admin.promo.copied'))
  if (success) {
    copiedCode.value = text
    setTimeout(() => {
      copiedCode.value = null
    }, 2000)
  }
}

const handleCreate = async () => {
  creating.value = true
  try {
    await adminAPI.promo.create({
      code: createForm.code || undefined,
      bonus_amount: createForm.bonus_amount,
      max_uses: createForm.max_uses,
      expires_at: createForm.expires_at_str ? Math.floor(new Date(createForm.expires_at_str).getTime() / 1000) : undefined,
      notes: createForm.notes || undefined
    })
    appStore.showSuccess(t('admin.promo.codeCreated'))
    showCreateDialog.value = false
    resetCreateForm()
    loadCodes()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promo.failedToCreate'))
  } finally {
    creating.value = false
  }
}

const resetCreateForm = () => {
  createForm.code = ''
  createForm.bonus_amount = 1
  createForm.max_uses = 0
  createForm.expires_at_str = ''
  createForm.notes = ''
}

const handleEdit = (code: PromoCode) => {
  editingCode.value = code
  editForm.code = code.code
  editForm.bonus_amount = code.bonus_amount
  editForm.max_uses = code.max_uses
  editForm.status = code.status
  editForm.expires_at_str = code.expires_at ? new Date(code.expires_at).toISOString().slice(0, 16) : ''
  editForm.notes = code.notes || ''
  showEditDialog.value = true
}

const closeEditDialog = () => {
  showEditDialog.value = false
  editingCode.value = null
}

const handleUpdate = async () => {
  if (!editingCode.value) return

  updating.value = true
  try {
    await adminAPI.promo.update(editingCode.value.id, {
      code: editForm.code,
      bonus_amount: editForm.bonus_amount,
      max_uses: editForm.max_uses,
      status: editForm.status,
      expires_at: editForm.expires_at_str ? Math.floor(new Date(editForm.expires_at_str).getTime() / 1000) : 0,
      notes: editForm.notes
    })
    appStore.showSuccess(t('admin.promo.codeUpdated'))
    closeEditDialog()
    loadCodes()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promo.failedToUpdate'))
  } finally {
    updating.value = false
  }
}

const copyRegisterLink = async (code: PromoCode) => {
  const baseUrl = window.location.origin
  const registerLink = `${baseUrl}/register?promo=${encodeURIComponent(code.code)}`

  try {
    await navigator.clipboard.writeText(registerLink)
    appStore.showSuccess(t('admin.promo.registerLinkCopied'))
  } catch (error) {
    const textArea = document.createElement('textarea')
    textArea.value = registerLink
    document.body.appendChild(textArea)
    textArea.select()
    document.execCommand('copy')
    document.body.removeChild(textArea)
    appStore.showSuccess(t('admin.promo.registerLinkCopied'))
  }
}

const handleDelete = (code: PromoCode) => {
  deletingCode.value = code
  showDeleteDialog.value = true
}

const confirmDelete = async () => {
  if (!deletingCode.value) return

  try {
    await adminAPI.promo.delete(deletingCode.value.id)
    appStore.showSuccess(t('admin.promo.codeDeleted'))
    showDeleteDialog.value = false
    deletingCode.value = null
    loadCodes()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promo.failedToDelete'))
  }
}

const handleEditCafeCouponStatus = (coupon: AdminCafeCoupon) => {
  editingCafeCoupon.value = coupon
  cafeStatusForm.status = coupon.status
  showCafeStatusDialog.value = true
}

const closeCafeStatusDialog = () => {
  showCafeStatusDialog.value = false
  editingCafeCoupon.value = null
}

const confirmCafeCouponStatus = async () => {
  if (!editingCafeCoupon.value || updatingCafeCouponStatus.value) return
  updatingCafeCouponStatus.value = true
  try {
    await adminAPI.promo.updateCafeCouponStatus(editingCafeCoupon.value.id, { status: cafeStatusForm.status })
    appStore.showSuccess(t('admin.promo.cafeCoupon.statusUpdated'))
    closeCafeStatusDialog()
    loadCafeCoupons()
  } catch (error: any) {
    const reason = error.response?.data?.reason || error.reason
    appStore.showError(
      reason === 'CAFE_COUPON_STATUS_NOT_ALLOWED'
        ? t('admin.promo.cafeCoupon.statusNotAllowed')
        : error.response?.data?.detail || t('admin.promo.cafeCoupon.failedToUpdateStatus')
    )
  } finally {
    updatingCafeCouponStatus.value = false
  }
}

const handleResetCafeCoupon = (coupon: AdminCafeCoupon) => {
  resettingCoupon.value = coupon
  showResetCafeDialog.value = true
}

const closeResetCafeDialog = () => {
  showResetCafeDialog.value = false
  resettingCoupon.value = null
}

const confirmResetCafeCoupon = async () => {
  if (!resettingCoupon.value || resettingCafeCoupon.value) return
  resettingCafeCoupon.value = true
  try {
    await adminAPI.promo.resetCafeCouponClaimPeriod(resettingCoupon.value.id)
    appStore.showSuccess(t('admin.promo.cafeCoupon.claimPeriodReset'))
    closeResetCafeDialog()
    loadCafeCoupons()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promo.cafeCoupon.failedToResetClaimPeriod'))
  } finally {
    resettingCafeCoupon.value = false
  }
}

const handleVoidCafeCoupon = (coupon: AdminCafeCoupon) => {
  voidingCoupon.value = coupon
  showVoidCafeDialog.value = true
}

const closeVoidCafeDialog = () => {
  showVoidCafeDialog.value = false
  voidingCoupon.value = null
}

const confirmVoidCafeCoupon = async () => {
  if (!voidingCoupon.value || voidingCafeCoupon.value) return

  voidingCafeCoupon.value = true
  try {
    await adminAPI.promo.voidCafeCoupon(voidingCoupon.value.id)
    appStore.showSuccess(t('admin.promo.cafeCoupon.voided'))
    closeVoidCafeDialog()
    loadCafeCoupons()
  } catch (error: any) {
    const reason = error.response?.data?.reason || error.reason
    appStore.showError(
      reason === 'CAFE_COUPON_VOID_NOT_ALLOWED'
        ? t('admin.promo.cafeCoupon.voidNotAllowed')
        : error.response?.data?.detail || t('admin.promo.cafeCoupon.failedToVoid')
    )
  } finally {
    voidingCafeCoupon.value = false
  }
}

const handleViewUsages = async (code: PromoCode) => {
  currentViewingCode.value = code
  showUsagesDialog.value = true
  usagesPage.value = 1
  await loadUsages()
}

const loadUsages = async () => {
  if (!currentViewingCode.value) return
  usagesLoading.value = true
  usages.value = []

  try {
    const response = await adminAPI.promo.getUsages(
      currentViewingCode.value.id,
      usagesPage.value,
      usagesPageSize.value
    )
    usages.value = response.items
    usagesTotal.value = response.total
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promo.failedToLoadUsages'))
  } finally {
    usagesLoading.value = false
  }
}

const handleUsagesPageChange = (page: number) => {
  usagesPage.value = page
  loadUsages()
}

onMounted(() => {
  loadCodes()
})

onUnmounted(() => {
  clearTimeout(searchTimeout)
  clearTimeout(cafeSearchTimeout)
  abortController?.abort()
  cafeAbortController?.abort()
})
</script>
