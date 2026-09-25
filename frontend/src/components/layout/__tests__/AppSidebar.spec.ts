import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar navigation', () => {
  it('keeps account billing links without a dedicated presale entry', () => {
    expect(componentSource).not.toMatch(/path:\s*['"]\/presale['"]/)
    for (const path of ['/subscriptions', '/purchase', '/orders']) {
      expect(componentSource).toContain(`path: '${path}'`)
    }
  })
})

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the shared SVG brand or its steam', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })

  it('reuses the homepage brand and closes the mobile drawer when it is clicked', () => {
    expect(componentSource).toContain("import HomeBrand from '@/components/home/HomeBrand.vue'")
    expect(componentSource).toMatch(/<HomeBrand\s+v-if="!sidebarCollapsed"[\s\S]*?:site-name="siteName"[\s\S]*?@click="closeMobile"/)
    expect(componentSource).toContain('const siteName = computed(() => appStore.siteName)')
    const header = componentSource.slice(componentSource.indexOf('<!-- Logo/Brand -->'), componentSource.indexOf('<!-- Navigation -->'))
    expect(header).toContain(':src="siteLogo || \'/logo.png\'"')
    expect(header).toMatch(/<RouterLink to="\/home"[^>]*class="sidebar-logo[^>]*@click="closeMobile"/)
    expect(header).not.toMatch(/:compact|VersionBadge|sidebar-brand-title/)
    expect(componentSource).toContain('const siteLogo = computed(() => appStore.siteLogo)')
    expect(componentSource).toContain('--cafe-ink: rgb(var(--color-content-primary));')
    expect(componentSource).toContain('--brand-name-scale: .35;')
    expect(componentSource).toContain('--brand-name-scale: .31;')
  })
})
