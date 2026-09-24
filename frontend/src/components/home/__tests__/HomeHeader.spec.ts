import { describe, expect, it, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import HomeHeader from '../HomeHeader.vue'

vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))

function render(presaleOpen = true) {
  return shallowMount(HomeHeader, {
    props: { siteName: 'Café Shop', docUrl: '', entryPath: '/login', isAuthenticated: false, isDark: true, compact: false, presaleOpen },
    global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } },
  })
}

describe('shared public header', () => {
  it('crossfades the same navigation slot to the open-sale banner without exposing hidden links', async () => {
    const w = render()
    expect(w.get('.header-menu').classes()).not.toContain('navigation-hidden')
    expect(w.get('.presale-header-banner').attributes('inert')).toBeDefined()
    await w.setProps({ compact: true })
    expect(w.classes()).toContain('header-compact')
    expect(w.get('.header-menu').classes()).toContain('navigation-hidden')
    expect(w.get('.header-menu').attributes('inert')).toBeDefined()
    expect(w.get('.presale-header-banner').attributes('inert')).toBeUndefined()
    expect(w.get('.presale-mobile-banner').attributes('href')).toBe('/presale')
    await w.setProps({ compact: false })
    expect(w.get('.header-menu').attributes('inert')).toBeUndefined()
    w.unmount()
  })

  it('never advertises an open sale when the catalog is closed', async () => {
    const w = render(false)
    await w.setProps({ compact: true })
    expect(w.find('.presale-header-banner, .presale-mobile-banner').exists()).toBe(false)
    expect(w.find('.mobile-nav').exists()).toBe(true)
    w.unmount()
  })

  it('keeps theme control and entry routes independent of the page content', async () => {
    const w = render()
    await w.get('.theme-toggle').trigger('click')
    expect(w.emitted('toggleTheme')).toHaveLength(1)
    await w.setProps({ isAuthenticated: true, entryPath: '/admin/dashboard', siteName: 'New site' })
    expect(w.get('.header-entry').attributes('href')).toBe('/admin/dashboard')
    expect(w.getComponent({ name: 'HomeBrand' }).props('siteName')).toBe('New site')
    w.unmount()
  })
})
