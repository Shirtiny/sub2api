import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import LoadingSpinner from '../LoadingSpinner.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key === 'common.loading' ? 'Loading' : key })
}))

enableAutoUnmount(afterEach)

describe('LoadingSpinner', () => {
  it('preserves the default spinner and accessible loading label', () => {
    const wrapper = mount(LoadingSpinner)
    expect(wrapper.get('.spinner').classes()).toEqual(expect.arrayContaining(['w-8', 'h-8', 'border-2']))
    expect(wrapper.classes()).toContain('text-primary-500')
    expect(wrapper.attributes('role')).toBe('status')
    expect(wrapper.attributes('aria-label')).toBe('Loading')
    expect(wrapper.attributes('aria-hidden')).toBeUndefined()
    expect(wrapper.get('.sr-only').text()).toBe('Loading')
    expect(wrapper.find('svg').exists()).toBe(false)
    expect(wrapper.classes()).not.toContain('loading-overlay')
  })

  it.each([
    ['sm', ['w-4', 'h-4', 'border-2']],
    ['md', ['w-8', 'h-8', 'border-2']],
    ['lg', ['w-12', 'h-12', 'border-[3px]']],
    ['xl', ['w-16', 'h-16', 'border-4']]
  ] as const)('preserves the %s spinner size', (size, classes) => {
    const wrapper = mount(LoadingSpinner, { props: { size } })
    expect(wrapper.get('.spinner').classes()).toEqual(expect.arrayContaining([...classes]))
  })

  it.each([
    ['primary', 'text-primary-500'],
    ['secondary', 'text-content-tertiary'],
    ['white', 'text-white'],
    ['gray', 'text-content-tertiary'],
    ['current', 'text-current']
  ] as const)('supports the %s color', (color, className) => {
    const wrapper = mount(LoadingSpinner, { props: { color } })
    expect(wrapper.classes()).toContain(className)
  })

  it('renders the three steam strokes without visible loading text', () => {
    const wrapper = mount(LoadingSpinner, { props: { variant: 'steam' } })
    expect(wrapper.classes()).toContain('loading-steam')
    expect(wrapper.find('.spinner').exists()).toBe(false)
    const svg = wrapper.get('svg')
    expect(svg.attributes('viewBox')).toBe('0 0 64 24')
    expect(svg.attributes('stroke')).toBe('currentColor')
    expect(svg.attributes('aria-hidden')).toBe('true')
    expect(svg.attributes('focusable')).toBe('false')
    const strokes = svg.findAll('.steam-wisp')
    expect(strokes).toHaveLength(3)
    for (const stroke of strokes) expect(stroke.attributes('pathLength')).toBe('1')
    expect(wrapper.attributes('role')).toBe('status')
    expect(wrapper.get('.sr-only').text()).toBe('Loading')
    expect(wrapper.findAll('span')).toHaveLength(2) // Root plus screen-reader-only label.
  })

  it.each([
    ['sm', ['w-12', 'h-[18px]']],
    ['md', ['w-16', 'h-6']],
    ['lg', ['w-24', 'h-9']],
    ['xl', ['w-32', 'h-12']]
  ] as const)('keeps the steam aspect ratio at %s size', (size, classes) => {
    const wrapper = mount(LoadingSpinner, { props: { variant: 'steam', size } })
    expect(wrapper.get('svg').classes()).toEqual([...classes])
  })

  it('supports a decorative button overlay without duplicate announcements', () => {
    const wrapper = mount(LoadingSpinner, {
      props: { variant: 'steam', color: 'current', overlay: true, decorative: true },
      attrs: { class: 'button-loader' }
    })
    expect(wrapper.classes()).toEqual(expect.arrayContaining(['loading-overlay', 'loading-steam', 'text-current', 'button-loader']))
    expect(wrapper.attributes('aria-hidden')).toBe('true')
    expect(wrapper.attributes('role')).toBeUndefined()
    expect(wrapper.attributes('aria-label')).toBeUndefined()
    expect(wrapper.find('.sr-only').exists()).toBe(false)
    expect(wrapper.text()).toBe('')
  })

  it('can switch variants and return to standalone mode reactively', async () => {
    const wrapper = mount(LoadingSpinner, { props: { overlay: true, decorative: true } })
    await wrapper.setProps({ variant: 'steam', size: 'lg', color: 'current', overlay: false, decorative: false })
    expect(wrapper.find('.spinner').exists()).toBe(false)
    expect(wrapper.get('svg').classes()).toEqual(['w-24', 'h-9'])
    expect(wrapper.classes()).not.toContain('loading-overlay')
    expect(wrapper.attributes('aria-hidden')).toBeUndefined()
    expect(wrapper.attributes('role')).toBe('status')
    expect(wrapper.attributes('aria-label')).toBe('Loading')
    await wrapper.setProps({ variant: 'spinner' })
    expect(wrapper.find('svg').exists()).toBe(false)
    expect(wrapper.get('.spinner').classes()).toContain('w-12')
  })
})
