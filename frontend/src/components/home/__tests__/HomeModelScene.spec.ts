import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import HomeModelScene from '../HomeModelScene.vue'

const props = { kind: 'luna' as const, label: 'Luna', active: true, reducedMotion: false }

describe('vector celestial scenes', () => {
  it.each(['luna', 'terra', 'sol', 'astra'] as const)('renders %s using only inline vector artwork', kind => {
    const page = mount(HomeModelScene, { props: { ...props, kind, label: kind } })
    expect(page.attributes('role')).toBe('img')
    expect(page.attributes('aria-label')).toBe(kind)
    expect(page.find('svg').attributes('viewBox')).toBe('0 0 320 280')
    expect(page.find('canvas, img, image, button').exists()).toBe(false)
    expect(page.findAll('.motion, .sphere-surface').length).toBeGreaterThan(0)
    page.unmount()
  })

  it('makes Luna smaller than Terra and Terra smaller than Sol on the same scale', () => {
    const pages = (['luna', 'terra', 'sol'] as const).map(kind => mount(HomeModelScene, { props: { ...props, kind } }))
    expect(pages.map(page => Number(page.find('.planet-body').attributes('r')))).toEqual([26, 48, 80])
    expect(pages.map(page => Number(page.find('.planet-body').attributes('cx')))).toEqual([160, 160, 160])
    expect(pages.map(page => Number(page.find('clipPath circle').attributes('cx')))).toEqual([160, 160, 160])
    expect(pages.map(page => page.find('svg').attributes('viewBox'))).toEqual(Array(3).fill('0 0 320 280'))
    pages.forEach(page => page.unmount())
  })

  it('uses unique clip and gradient IDs when instances are repeated', () => {
    const a = mount(HomeModelScene, { props }), b = mount(HomeModelScene, { props })
    const ids = [...a.findAll('[id]'), ...b.findAll('[id]')].map(node => node.attributes('id'))
    expect(new Set(ids).size).toBe(ids.length)
    for (const page of [a, b]) {
      const clip = page.find('clipPath').attributes('id')
      expect(page.find('[clip-path]').attributes('clip-path')).toBe(`url(#${clip})`)
      page.unmount()
    }
  })

  it('keeps the official reference cues instead of diagram rings and a generic star icon', () => {
    const lunar = mount(HomeModelScene, { props })
    expect(lunar.find('.lunar-surface path').exists()).toBe(true)
    expect(lunar.find('.lunar-grain, .lunar-mare').exists()).toBe(false)
    expect(lunar.find('.lunar-finish').exists()).toBe(true)
    expect(Number(lunar.find('.lunar-finish').attributes('opacity'))).toBeLessThanOrEqual(.15)
    expect(lunar.find('.lunar-crater-floors').exists()).toBe(true)
    expect(lunar.find('.lunar-crater-shadows').exists()).toBe(true)
    expect(lunar.find('.lunar-crater-rims').exists()).toBe(true)
    expect(lunar.findAll('.lunar-crater')).toHaveLength(8)
    expect(lunar.find('.lunar-crater-floors').attributes('fill')).toMatch(/^url\(#celestial-crater-bowl-/)
    expect(lunar.find('.lunar-crater-shadows').attributes('fill')).toMatch(/^url\(#celestial-crater-wall-/)
    expect(lunar.find('.lunar-crater-basin').exists()).toBe(true)
    expect(lunar.find('.lunar-crater-terrace').exists()).toBe(true)
    expect(lunar.find('.lunar-crater-peak-light').exists()).toBe(true)
    expect(lunar.find('.lunar-crater-peak-shadow').exists()).toBe(true)
    // Lighting stays fixed while the geographic surface turns.
    expect(lunar.find('.lunar-shadow').classes()).not.toContain('motion')
    expect(lunar.find('.orbit-line, .scene-guides').exists()).toBe(false)
    lunar.unmount()
    const earth = mount(HomeModelScene, { props: { ...props, kind: 'terra' } })
    expect(earth.find('.terra-clouds path').exists()).toBe(true)
    expect(earth.find('.terra-grid').exists()).toBe(false)
    earth.unmount()
    const sun = mount(HomeModelScene, { props: { ...props, kind: 'sol' } })
    expect(sun.find('.solar-prominences path').exists()).toBe(true)
    expect(sun.find('.solar-filaments').exists()).toBe(false)
    expect(sun.find('.solar-plasma').exists()).toBe(true)
    expect(sun.find('feTurbulence').exists()).toBe(true)
    expect(sun.find('feColorMatrix').attributes('values')).toBe('0')
    expect(sun.find('.solar-surface ellipse').exists()).toBe(false)
    expect(sun.find('[stroke]').exists()).toBe(false)
    sun.unmount()
    const galaxy = mount(HomeModelScene, { props: { ...props, kind: 'astra' } })
    expect(galaxy.findAll('.galaxy-arms circle').length).toBeGreaterThan(100)
    galaxy.unmount()
  })

  it('keeps Astra round instead of compressing its spiral into a flat ellipse', () => {
    const page = mount(HomeModelScene, { props: { ...props, kind: 'astra' } })
    const stars = page.findAll('.galaxy-arms circle')
    const xs = stars.map(star => Number(star.attributes('cx')))
    const ys = stars.map(star => Number(star.attributes('cy')))
    const ratio = (Math.max(...ys) - Math.min(...ys)) / (Math.max(...xs) - Math.min(...xs))
    expect(stars).toHaveLength(720)
    expect(Math.max(...xs) - Math.min(...xs)).toBeGreaterThan(210)
    expect(ratio).toBeGreaterThan(.9)
    expect(ratio).toBeLessThan(1.1)
    page.unmount()
  })

  it('pauses offscreen and respects reduced motion without a control button', async () => {
    const page = mount(HomeModelScene, { props })
    expect(page.attributes('data-paused')).toBe('false')
    await page.setProps({ active: false })
    expect(page.attributes('data-paused')).toBe('true')
    await page.setProps({ active: true, reducedMotion: true })
    expect(page.attributes('data-paused')).toBe('true')
    expect(page.attributes('data-reduced-motion')).toBe('true')
    expect(page.find('button').exists()).toBe(false)
    page.unmount()
  })
})
