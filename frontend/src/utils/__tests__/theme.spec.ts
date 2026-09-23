import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { initializeTheme } from '../theme'
import bootstrapSource from '../../main.ts?raw'
import homeSource from '../../views/HomeView.vue?raw'
import sidebarSource from '../../components/layout/AppSidebar.vue?raw'
import keyUsageSource from '../../views/KeyUsageView.vue?raw'

beforeEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('dark')
})

afterEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
  document.documentElement.classList.remove('dark')
})

describe('shared dark-default theme', () => {
  it.each([false, true])('does not read or follow the OS theme (system dark=%s)', systemDark => {
    const systemTheme = vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: systemDark } as MediaQueryList)
    expect(initializeTheme()).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBeNull()
    expect(systemTheme).not.toHaveBeenCalled()
  })

  it.each([
    ['light', false], ['dark', true], ['system', true], ['auto', true], ['', true], ['unknown', true]
  ] as const)('honors only explicit light, otherwise defaults to dark (saved=%s)', (saved, dark) => {
    localStorage.setItem('theme', saved)
    document.documentElement.classList.toggle('dark', !dark)
    expect(initializeTheme()).toBe(dark)
    expect(document.documentElement.classList.contains('dark')).toBe(dark)
    expect(localStorage.getItem('theme')).toBe(saved)
  })

  it('falls back to dark when browser storage is unavailable', () => {
    vi.spyOn(localStorage, 'getItem').mockImplementationOnce(() => {
      throw new DOMException('Storage blocked', 'SecurityError')
    })
    expect(initializeTheme()).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('preserves a manual preference when another route initializes', () => {
    initializeTheme()
    localStorage.setItem('theme', 'light')
    expect(initializeTheme()).toBe(false)
    localStorage.setItem('theme', 'dark')
    expect(initializeTheme()).toBe(true)
  })

  it.each([
    ['bootstrap', bootstrapSource], ['homepage', homeSource],
    ['console sidebar', sidebarSource], ['key usage', keyUsageSource]
  ])('uses the same initialization policy in %s', (_route, source) => {
    expect(source).toContain("import { initializeTheme } from '@/utils/theme'")
    expect(source).toContain('initializeTheme()')
    expect(source).not.toMatch(/prefers-color-scheme|localStorage\.getItem\('theme'\)/)
  })
})
