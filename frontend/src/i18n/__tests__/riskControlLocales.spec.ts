import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('risk control locale copy', () => {
  it('describes worker runtime as audit and pre-block record processing', () => {
    expect(zh.admin.riskControl.workerStatusHint).toContain('前置拦截记录任务')
    expect(zh.admin.riskControl.workerStatusHint).not.toContain('异步观察任务')
    expect(en.admin.riskControl.workerStatusHint).toContain('pre-block record tasks')
    expect(en.admin.riskControl.workerStatusHint).not.toContain('observation tasks')
  })

  it('keeps pre-block audit key summary aware of async worker load', () => {
    expect(zh.admin.riskControl.preBlockAPIKeyLoadSummary).toContain('worker：{workerActive} / {workerTotal}')
    expect(en.admin.riskControl.preBlockAPIKeyLoadSummary).toContain('worker: {workerActive} / {workerTotal}')
  })

  it('does not describe pre-block audit key polling as bypassing the worker pool', () => {
    expect(zh.admin.riskControl.preBlockAPIKeyLoadHint).toBe('同步前置拦截直接轮询可用审核 Key。')
    expect(zh.admin.riskControl.preBlockAPIKeyLoadHint).not.toContain('Worker 池')
    expect(en.admin.riskControl.preBlockAPIKeyLoadHint).not.toContain('worker pool')
  })

  it('presents Cyber policy as an independent settings tab', () => {
    expect(zh.admin.riskControl.tabs.cyberPolicy).toBe('Cyber 风控')
    expect(zh.admin.riskControl.cyberPolicySectionHint).toContain('独立')
    expect(en.admin.riskControl.tabs.cyberPolicy).toBe('Cyber Policy')
    expect(en.admin.riskControl.cyberPolicySectionHint).toContain('independent')
  })

  it('documents fingerprint aggregation and bounded snapshot retention', () => {
    expect(zh.admin.riskControl.requestControl.logsHint).toContain('请求格式指纹聚合')
    expect(zh.admin.riskControl.requestControl.logsHint).toContain('每 15 分钟刷新一次')
    expect(zh.admin.riskControl.requestControl.logsHint).toContain('最多 256KB、保留 3 天')
    expect(en.admin.riskControl.requestControl.logsHint).toContain('aggregated by user and request-format fingerprint')
    expect(en.admin.riskControl.requestControl.logsHint).toContain('at most once per 15 minutes')
    expect(en.admin.riskControl.requestControl.logsHint).toContain('capped at 256KB and kept for 3 days')
  })

  it('documents that request snapshots are opt-in', () => {
    expect(zh.admin.riskControl.requestControl.requestSnapshotHint).toContain('默认关闭')
    expect(zh.admin.riskControl.requestControl.requestSnapshotHint).toContain('关闭后不采集新快照')
    expect(en.admin.riskControl.requestControl.requestSnapshotHint).toContain('Off by default')
    expect(en.admin.riskControl.requestControl.requestSnapshotHint).toContain('stops new snapshot capture')
  })
})
