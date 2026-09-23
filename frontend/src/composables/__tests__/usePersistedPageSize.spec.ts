import { afterEach, describe, expect, it } from 'vitest'

import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

describe('usePersistedPageSize', () => {
  afterEach(() => {
    localStorage.clear()
    delete window.__APP_CONFIG__
  })

  it('preserves the user-selected page size over the system default', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 1000,
      table_page_size_options: [20, 50, 1000]
    } as any
    localStorage.setItem('table-page-size', '50')
    localStorage.setItem('table-page-size-source', 'user')

    expect(getPersistedPageSize()).toBe(50)
  })

  it('uses the configured default when no user preference is stored', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 1000,
      table_page_size_options: [20, 50, 1000]
    } as NonNullable<Window['__APP_CONFIG__']>

    expect(getPersistedPageSize()).toBe(1000)
  })
})
