import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import RequestControlPanel from '../RequestControlPanel.vue'
import type { RequestControlConfig } from '@/api/admin/riskControl'

const {
  getConfig,
  updateConfig,
  getStatus,
  listLogs,
  getLog,
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
  getLog: vi.fn(),
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
      getRequestControlLog: getLog,
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

describe('RequestControlPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getConfig.mockResolvedValue(baseConfig())
    updateConfig.mockImplementation(async (payload: RequestControlConfig) => ({ ...baseConfig(), ...payload }))
    getStatus.mockResolvedValue({ enabled: true, risk_control_enabled: true, queue_size: 8192, queue_length: 0, enqueued: 0, processed: 0, dropped: 0, errors: 0 })
    listLogs.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    getLog.mockResolvedValue(null)
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
      email_on_hit: true,
      auto_ban_enabled: true,
      ban_threshold: 4,
      violation_window_hours: 720,
    }))
    expect(getStatus).toHaveBeenCalledTimes(2)
    expect(showSuccess).toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  it('warns when the module is enabled but the global risk-control switch is off', async () => {
    getStatus.mockResolvedValue({ enabled: true, risk_control_enabled: false, queue_size: 8192, queue_length: 0, enqueued: 0, processed: 0, dropped: 0, errors: 0 })
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('admin.riskControl.requestControl.globalSwitchOff')
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

  it('edits an existing user UA whitelist instead of creating a duplicate rule', async () => {
    getConfig.mockResolvedValue({ ...baseConfig(), user_rules: [{ user_id: 374, participate: false, user_agent_whitelist: ['old/'] }] })
    const wrapper = mountPanel()
    await flushPromises()

    const edit = wrapper.find('button[title="admin.riskControl.requestControl.editUserUA"]')
    expect(edit.exists()).toBe(true)
    await edit.trigger('click')
    await wrapper.get('[data-test="request-control-user-id"]').setValue('374')
    await wrapper.find('textarea[placeholder="admin.riskControl.requestControl.userUAPlaceholder"]').setValue('new/\nsecond/')
    const updateButton = wrapper.findAll('button').find((button) => button.text().includes('admin.riskControl.requestControl.updateUser'))
    expect(updateButton).toBeTruthy()
    await updateButton!.trigger('click')
    await wrapper.get('[data-test="request-control-save"]').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      user_rules: [{ user_id: 374, participate: false, user_agent_whitelist: ['new/', 'second/'] }],
    }))
  })

  it('loads request metadata only when a record detail is opened', async () => {
    const row = {
      id: 7,
      request_id: 'req-7',
      user_id: 42,
      user_email: 'tester@example.com',
      api_key_id: null,
      api_key_name: '',
      group_id: null,
      group_name: '',
      endpoint: '/v1/responses',
      provider: 'openai',
      protocol: 'openai_responses',
      model: 'gpt-5',
      action: 'block',
      reason: 'test',
      allowed: false,
      blocked: true,
      observed: true,
      client_kind: 'codex',
      user_agent: 'codex_exec/1.0.0',
      originator: 'codex_exec',
      tls_fingerprint: '',
      tls_match: null,
      header_match: false,
      body_match: false,
      expected_action: 'block',
      expected_reason: 'test',
      expected_blocked: true,
      expected_status_code: 403,
      details: {},
      violation_count: 1,
      counted_violation: true,
      email_sent: false,
      hit_email_sent: false,
      ban_email_sent: false,
      auto_banned: false,
      event_count: 4,
      first_seen_at: '2026-01-01T00:00:00Z',
      last_seen_at: '2026-01-01T00:10:00Z',
      created_at: '2026-01-01T00:00:00Z',
    }
    listLogs.mockResolvedValue({ items: [row], total: 1, page: 1, page_size: 20, pages: 1 })
    getLog.mockResolvedValue({
      ...row,
      request_headers: { 'content-type': 'application/json' },
      request_body_metadata: { model: 'gpt-5', messages: { kind: 'array', count: 1 } },
      request_snapshot: {
        available: true,
        method: 'POST',
        host: 'www.cafecode.work',
        path: '/v1/responses',
        raw_query: '',
        client_ip: '127.0.0.1',
        remote_addr: '127.0.0.1:1234',
        content_length: 31,
        headers: { 'content-type': ['application/json'], authorization: ['[redacted]'] },
        body: '{"model":"gpt-5","input":[]}',
        body_bytes: 31,
        body_captured_bytes: 31,
        body_truncated: false,
        body_sha256: 'abc123',
        body_capture_mode: 'full',
        captured_at: '2026-01-01T00:10:00Z',
      },
    })
    const wrapper = mountPanel()
    await flushPromises()

    expect(getLog).not.toHaveBeenCalled()
    await wrapper.get('[aria-label="admin.riskControl.requestControl.viewDetail"]').trigger('click')
    await flushPromises()

    expect(getLog).toHaveBeenCalledWith(7)
    expect(document.body.textContent).toContain('admin.riskControl.requestControl.fullRequestHeaders')
    expect(document.body.textContent).toContain('content-type')
    expect(document.body.textContent).toContain('{"model":"gpt-5","input":[]}')
    expect(document.body.textContent).toContain('admin.riskControl.requestControl.eventCountLabel')
    expect(document.body.textContent).toContain('4')
  })
})
