import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import RequestControlPanel from '../RequestControlPanel.vue'
import type { RequestControlConfig } from '@/api/admin/riskControl'

const {
  getConfig,
  updateConfig,
  getStatus,
  listLogs,
  getGroups,
  listUsers,
  getUser,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  getStatus: vi.fn(),
  listLogs: vi.fn(),
  getGroups: vi.fn(),
  listUsers: vi.fn(),
  getUser: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    riskControl: {
      getRequestControlConfig: getConfig,
      updateRequestControlConfig: updateConfig,
      getRequestControlStatus: getStatus,
      listRequestControlLogs: listLogs,
    },
    groups: { getAll: getGroups },
    users: { list: listUsers, getById: getUser },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_error: unknown, fallback: string) => fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) =>
        key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`)),
    }),
  }
})

const baseConfig = (): RequestControlConfig => ({
  enabled: true,
  all_groups: true,
  group_ids: [],
  model_filter: { type: 'all', models: [] },
  all_users: true,
  user_rules: [],
  global_user_agent_whitelist: [],
  block_status: 403,
  block_message: 'request blocked',
})

describe('RequestControlPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getConfig.mockResolvedValue(baseConfig())
    updateConfig.mockImplementation(async (payload: RequestControlConfig) => ({ ...baseConfig(), ...payload }))
    getStatus.mockResolvedValue({ queue_size: 8192, queue_length: 0, enqueued: 0, processed: 0, dropped: 0, errors: 0 })
    listLogs.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    getGroups.mockResolvedValue([])
    listUsers.mockResolvedValue({
      items: [{ id: 42, username: 'tester', email: 'tester@example.com', role: 'user', balance: 0, concurrency: 1, status: 'active', allowed_groups: null, balance_notify_enabled: false, balance_notify_threshold: null, balance_notify_extra_emails: [], notes: '', created_at: '', updated_at: '' }],
      total: 1,
      page: 1,
      page_size: 10,
      pages: 1,
    })
    getUser.mockResolvedValue(null)
  })

  function mountPanel() {
    return mount(RequestControlPanel, {
      global: {
        stubs: { Icon: true, Select: true, Toggle: true, Pagination: true },
      },
    })
  }

  it('loads the policy and saves normalized global UA prefixes', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(getConfig).toHaveBeenCalledOnce()
    expect(getStatus).toHaveBeenCalledOnce()
    expect(listLogs).toHaveBeenCalledOnce()

    await wrapper.get('[data-test="request-control-global-ua"]').setValue('codex_cli_rs/\ntrusted-client/')
    await wrapper.get('[data-test="request-control-save"]').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      global_user_agent_whitelist: ['codex_cli_rs/', 'trusted-client/'],
    }))
    expect(showSuccess).toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  it('searches and selects a concrete user for a rule', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.get('[data-test="request-control-user-search"]').setValue('tester@example.com')
    await wrapper.get('[data-test="request-control-user-search"]').trigger('keyup.enter')
    await flushPromises()

    expect(listUsers).toHaveBeenCalledWith(1, 10, {
      search: 'tester@example.com',
      include_subscriptions: false,
    })
    const userButton = wrapper.findAll('button').find((button) => button.text().includes('UID 42'))
    expect(userButton).toBeTruthy()
    await userButton!.trigger('click')
    expect((wrapper.get('[data-test="request-control-user-id"]').element as HTMLInputElement).value).toBe('42')
  })
})
