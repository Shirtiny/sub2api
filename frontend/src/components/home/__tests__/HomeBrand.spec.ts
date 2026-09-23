import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import HomeBrand from '../HomeBrand.vue'
import source from '../HomeBrand.vue?raw'
import { createWordmark } from '../brandGeometry'
import { createBrandStory } from '../brandStory'

const motion = { visibility: ref('visible'), preference: ref('no-preference') }
vi.mock('@vueuse/core', () => ({
  useDocumentVisibility: () => motion.visibility,
  usePreferredReducedMotion: () => motion.preference
}))
const RouterLinkStub = defineComponent({ props: ['to'], template: '<a :href="to"><slot /></a>' })
let wrapper: VueWrapper | undefined
const render = (siteName = 'Café Shop') => {
  wrapper = mount(HomeBrand, { props: { siteName }, global: { stubs: { RouterLink: RouterLinkStub } } })
  return wrapper
}
beforeEach(() => {
  motion.visibility.value = 'visible'
  motion.preference.value = 'no-preference'
})
afterEach(() => {
  wrapper?.unmount()
  vi.useRealTimers()
})

describe('HomeBrand wordmark and decorative tool marks', () => {
  it('scales the complete vector wordmark down without changing its geometry or header height', () => {
    expect(source).toContain('--brand-name-scale: .7;')
    expect(source).toContain('--brand-name-scale: .62;')
    expect(source).toContain('var(--brand-name-width) * var(--brand-name-scale)')
    expect(source).not.toContain('--slider-header-height')
  })

  it('keeps only the configured name, without an underline or annotation', () => {
    const page = render()
    expect(page.find('.brand-name').text()).toBe('Café Shop')
    expect(page.find('.brand').attributes('aria-label')).toBe('Café Shop')
    expect(page.find('.brand').attributes('href')).toBe('/home')
    expect(page.find('.home-brand').attributes('style')).toContain(`--brand-name-width: ${createBrandStory(createWordmark('Café Shop'))!.width}px`)
    expect(page.findAll('.brand-wordmark')).toHaveLength(1)
    expect(page.find('.brand-rule, .brand-note, .brand-signature').exists()).toBe(false)
    expect(page.find('.brand').find('img, image, text, button').exists()).toBe(false)
    expect(page.text()).not.toMatch(/Codex|\bpi\b/)
  })

  it.each(['Café Shop', 'My Atelier'])('keeps native animations without a hover, focus or click control (%s)', async name => {
    const page = render(name)
    const animation = page.findComponent({ name: name === 'Café Shop' ? 'HomeBrandStory' : 'HomeToolGlyph' })
    expect(page.find('animate').exists()).toBe(true)
    expect(animation.props('paused')).toBe(false)
    await page.trigger('mouseenter')
    await page.trigger('focusin')
    expect(page.find('button, [role="button"], [aria-pressed], .story-pause').exists()).toBe(false)
    expect(animation.props('paused')).toBe(false)
    expect(source).not.toMatch(/:hover|:focus-within|@click|toolLabel/)
    if (name === 'Café Shop') {
      expect(page.find('.brand-tools').exists()).toBe(false)
      expect(page.find('.accent-steam animate[attributeName="d"]').exists()).toBe(true)
    } else {
      const marks = page.find('.brand-tools')
      expect(marks.element.tagName).toBe('SPAN')
      expect(marks.attributes('aria-hidden')).toBe('true')
      expect(marks.attributes('tabindex')).toBeUndefined()
      await marks.trigger('click')
      expect(animation.props('paused')).toBe(false)
    }
  })

  it.each(['Café Shop', 'My Atelier'])('automatically pauses when backgrounded and resumes when visible (%s)', async name => {
    const page = render(name)
    const animation = page.findComponent({ name: name === 'Café Shop' ? 'HomeBrandStory' : 'HomeToolGlyph' })
    motion.visibility.value = 'hidden'
    await nextTick()
    expect(animation.props('paused')).toBe(true)
    motion.visibility.value = 'visible'
    await nextTick()
    expect(animation.props('paused')).toBe(false)
  })

  it.each(['Café Shop', 'My Atelier'])('retains the system reduced-motion fallback without adding controls (%s)', async name => {
    const page = render(name)
    const animation = page.findComponent({ name: name === 'Café Shop' ? 'HomeBrandStory' : 'HomeToolGlyph' })
    motion.preference.value = 'reduce'
    await nextTick()
    expect(animation.props('paused')).toBe(true)
    expect(page.find('button, [role="button"]').exists()).toBe(false)
    expect(page.find('animate, animateTransform').exists()).toBe(false)
    motion.preference.value = 'no-preference'
    await nextTick()
    expect(animation.props('paused')).toBe(false)
    expect(page.find('animate').exists()).toBe(true)
  })

  it('does not replace the site name or start an animation timer over time', async () => {
    vi.useFakeTimers()
    const page = render()
    const initial = page.find('.accent-steam').attributes('d')
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(90000)
    expect(page.find('.accent-steam').attributes('d')).toBe(initial)
    expect(page.find('.brand-name').text()).toBe('Café Shop')
    expect(vi.getTimerCount()).toBe(0)
  })

  it('rebuilds letter targets for backend names and falls back without inventing letters', async () => {
    const page = render()
    const stage = page.find('.brand-stage').element
    const initial = page.find('.story-stage').attributes('viewBox')
    await page.setProps({ siteName: 'Café Shoppe' })
    expect(page.find('.brand-stage').element).toBe(stage)
    expect(page.find('.story-stage').attributes('viewBox')).not.toBe(initial)
    await page.setProps({ siteName: 'My Atelier' })
    expect(page.find('.story-stage').exists()).toBe(false)
    expect(page.find('.tool-stage').exists()).toBe(true)
    expect(page.find('.brand').attributes('aria-label')).toBe('My Atelier')
    expect(page.find('.home-brand').attributes('style')).toContain(`--brand-name-width: ${createWordmark('My Atelier')!.width}px`)
  })

  it('left-aligns light vector lettering within a bounded, stable-height sign', () => {
    const mark = createWordmark('Café Shop')!
    const coordinates = mark.d.match(/-?\d+\.\d+/g)!.map(Number)
    const xs = coordinates.filter((_, i) => i % 2 === 0)
    const ys = coordinates.filter((_, i) => i % 2 === 1)
    expect(Math.min(...xs)).toBe(1)
    expect(Math.max(...xs)).toBeLessThanOrEqual(224)
    // Broader upright letterforms, rather than the previous narrow italic sign.
    expect(Math.max(...xs) - Math.min(...xs)).toBeGreaterThan(205)
    expect(Math.min(...ys)).toBeGreaterThanOrEqual(2)
    expect(Math.max(...ys)).toBeLessThanOrEqual(46)
    expect(mark.viewBox.endsWith(' 48')).toBe(true)
    expect(mark.d).not.toMatch(/NaN|Infinity/)
  })

  it('preserves unsupported scripts using static SVG text and local system fonts', () => {
    const page = render('后台站点名称')
    expect(page.find('text').text()).toBe('后台站点名称')
    expect(page.find('.brand').attributes('aria-label')).toBe('后台站点名称')
    expect(page.find('.brand-wordmark').exists()).toBe(false)
  })

  it('normalizes accents and bounds long decorative names without losing their accessible identity', () => {
    expect(createWordmark('   ')).toBeNull()
    expect(createWordmark('Cafe\u0301 Shop')!.d).toBe(createWordmark('Café Shop')!.d)
    const long = 'A'.repeat(1000)
    expect(createWordmark(long)!.width).toBeLessThanOrEqual(224)
    expect(createWordmark(long)!.d).not.toMatch(/NaN|Infinity/)
    const page = render(long)
    expect(page.find('.brand').attributes('aria-label')).toBe(long)
  })
})
