import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { useDocumentVisibility, usePreferredReducedMotion } from '@vueuse/core'
import HomeModelShowcase from '../HomeModelShowcase.vue'
import source from '../HomeModelShowcase.vue?raw'

const observer = vi.hoisted(() => ({ notify: (_visible: boolean) => {} }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@vueuse/core', async (importOriginal) => {
  const { ref } = await import('vue')
  const visibility = ref('visible')
  const motion = ref('no-preference')
  return {
    ...await importOriginal<typeof import('@vueuse/core')>(),
    useDocumentVisibility: () => visibility,
    usePreferredReducedMotion: () => motion,
    useIntersectionObserver: (_target: unknown, callback: (entries: { isIntersecting: boolean }[]) => void) => {
      observer.notify = visible => callback([{ isIntersecting: visible }])
    }
  }
})

let wrapper: VueWrapper | undefined
beforeEach(() => {
  useDocumentVisibility().value = 'visible'
  usePreferredReducedMotion().value = 'no-preference'
})
afterEach(() => wrapper?.unmount())
const render = () => (wrapper = mount(HomeModelShowcase))

describe('model brand gallery', () => {
  it('anchors the compact brand at the top inset without changing the centered full-size stage', () => {
    const brandRule = source.match(/\.model-brand \{([^}]+)\}/)?.[1] ?? ''
    expect(brandRule).toContain('transform-origin: top center;')
    expect(brandRule).toContain('top: var(--model-brand-top);')
    expect(brandRule).toContain('translateX(-50%) scale(var(--model-brand-scale, .44))')
    expect(source).toContain('.model-manual-brand .model-brand { top: 50%; transform: translate(-50%, -50%) scale(1); }')
    expect(source).toContain('from { top: 50%; transform: translate(-50%, -50%) scale(1); }')
    expect(source).toContain('to { top: var(--model-brand-top); transform: translate(-50%, 0) scale(var(--model-brand-scale, .44)); }')
  })

  it('keeps the scaled logo and wordmark above the cards on both sides of the short-screen breakpoint', () => {
    // Geometry contract, not a jsdom/browser layout measurement. Include the
    // entire SVG box and wordmark, not just the visible logo paths.
    expect(source).toContain('.model-art { width: 240px;')
    expect(source).toContain('margin-top: -16px;')
    expect(source).toContain('font-size: 44px; font-weight: 500; line-height: 1.15;')
    expect(source).toContain('--model-brand-top: 44px;')
    expect(source).toContain('padding: 220px 24px 48px;')
    expect(source).toContain('@media (min-width: 1024px) and (min-height: 700px)')
    expect(source).toContain('--model-brand-scale: .38;')
    expect(source).toContain('--model-brand-top: 12px;')
    expect(source).toContain('padding: clamp(124px, 15dvh, 164px) 24px 16px;')
    const unscaledHeight = 240 - 16 + 44 * 1.15
    for (const height of [600, 699, 700, 720, 768, 900, 1080]) {
      const fitted = height >= 700
      const inset = fitted ? 12 : 44
      const scale = fitted ? .38 : .44
      const cardsTop = fitted ? Math.min(164, Math.max(124, height * .15)) : 220
      expect(inset + unscaledHeight * scale).toBeLessThanOrEqual(cardsTop - 7)
      // With the old center origin the retained unscaled half-height overlaps
      // even the largest reserved header band in these desktop layouts.
      expect(inset + unscaledHeight * (1 + scale) / 2).toBeGreaterThan(cardsTop)
    }
  })

  it('shows a section heading, a rotating ChatGPT mark and its four groups', () => {
    const page = render()
    expect(page.findAll('.model-name').map(name => name.text())).toEqual(['ChatGPT'])
    expect(page.find('[data-model="chatgpt"]').exists()).toBe(true)
    expect(page.find('h2#models-title').text()).toBe('home.landing.models.title')
    expect(page.find('h2#models-title').element.parentElement).toBe(page.element)
    expect(page.findAll('.model-group h3').map(title => title.text())).toEqual(['Luna', 'Terra', 'Sol', 'Astra'])
    expect(page.findAll('.model-group').map(group => group.attributes('style'))).toEqual([
      '--group-delay: 2.45s;', '--group-delay: 2.59s;', '--group-delay: 2.73s;', '--group-delay: 2.87s;'
    ])
    expect(page.find('.models-description, .model-gallery, .model-orbit').exists()).toBe(false)
    expect(page.find('button').exists()).toBe(false)
    expect(page.find('.model-logo-turn').exists()).toBe(true)
    expect(page.text()).not.toMatch(/Claude|Grok|Gemini|The model collection/)
    expect(page.find('.model-solid path').exists()).toBe(true)
    expect(page.findAll('.model-echo')).toHaveLength(2)
    expect(page.findAll('.celestial-scene').map(scene => scene.attributes('data-scene'))).toEqual(['luna', 'terra', 'sol', 'astra'])
    expect(page.findAll('.model-group-caption p').map(version => version.text())).toEqual(['GPT-5.6', 'GPT-5.6', 'GPT-5.6', 'GPT-6'])
  })

  it('starts the reveal on first entry without replaying it on every scroll', async () => {
    const page = render()
    expect(page.classes()).not.toContain('model-entered')
    observer.notify(true)
    await nextTick()
    expect(page.classes()).toContain('model-entered')
    expect(page.classes()).toContain('model-sequence-active')
    expect(page.attributes('data-stage')).toBe('intro')
    observer.notify(false)
    await nextTick()
    expect(page.classes()).toContain('model-entered')
    observer.notify(true)
    await nextTick()
    expect(page.classes()).toContain('model-entered')
  })

  it('rotates native SVG paths rather than an HTML snapshot at the compact size', async () => {
    const page = render()
    const svg = page.find('svg.model-logo')
    const rotor = svg.find('g.model-logo-turn')
    expect(page.find('span.model-logo-turn').exists()).toBe(false)
    expect(rotor.element.namespaceURI).toBe('http://www.w3.org/2000/svg')
    expect(svg.attributes('shape-rendering')).toBe('geometricPrecision')
    const rotation = rotor.find('animateTransform')
    expect(rotation.attributes()).toMatchObject({
      attributeName: 'transform', type: 'rotate', from: '0 12 12', to: '360 12 12', dur: '36s', repeatCount: 'indefinite'
    })
    expect(rotor.find('.model-solid path').exists()).toBe(true)
    observer.notify(true)
    await nextTick()
    await page.findAll('.model-group')[3].trigger('animationend')
    await page.find('.model-art').trigger('click')
    await page.find('.model-art').trigger('click')
    expect(page.find('svg.model-logo').element).toBe(svg.element)
    expect(page.find('animateTransform').element).toBe(rotation.element)
  })

  it('pauses and resumes the native SVG clock without resetting its angle', async () => {
    const page = render()
    const svg = Object.assign(page.find('svg.model-logo').element, {
      pauseAnimations: vi.fn(), unpauseAnimations: vi.fn(), setCurrentTime: vi.fn()
    })
    observer.notify(false)
    await nextTick()
    expect(svg.pauseAnimations).toHaveBeenCalled()
    observer.notify(true)
    await nextTick()
    expect(svg.unpauseAnimations).toHaveBeenCalled()
    svg.pauseAnimations.mockClear()
    useDocumentVisibility().value = 'hidden'
    await nextTick()
    expect(svg.pauseAnimations).toHaveBeenCalledOnce()
    useDocumentVisibility().value = 'visible'
    await nextTick()
    svg.pauseAnimations.mockClear()
    usePreferredReducedMotion().value = 'reduce'
    await nextTick()
    expect(svg.pauseAnimations).toHaveBeenCalledOnce()
    expect(svg.setCurrentTime).not.toHaveBeenCalled()
  })

  it('keeps a manually selected brand visible until the visitor switches back', async () => {
    const page = render()
    observer.notify(true)
    await nextTick()
    const control = page.find('.model-art')
    await page.findAll('.model-group')[3].trigger('animationend')
    await nextTick()
    expect(page.classes()).not.toContain('model-sequence-active')
    expect(page.attributes('data-stage')).toBe('models')

    await control.trigger('click')
    expect(page.attributes('data-stage')).toBe('brand')
    expect(page.classes()).toContain('model-manual-brand')
    expect(control.attributes('title')).toBeUndefined()
    await nextTick()
    expect(page.attributes('data-stage')).toBe('brand')

    await control.trigger('click')
    expect(page.attributes('data-stage')).toBe('models')
    expect(page.classes()).not.toContain('model-sequence-active')
  })

  it('pauses outside the viewport and when the tab is hidden', async () => {
    const page = render()
    observer.notify(false)
    await nextTick()
    expect(page.attributes('data-paused')).toBe('true')
    observer.notify(true)
    await nextTick()
    expect(page.attributes('data-paused')).toBe('false')
    useDocumentVisibility().value = 'hidden'
    await nextTick()
    expect(page.attributes('data-paused')).toBe('true')
  })

  it('respects reduced motion and cannot be forced to animate', async () => {
    usePreferredReducedMotion().value = 'reduce'
    const page = render()
    expect(page.attributes('data-paused')).toBe('true')
    expect(page.attributes('data-reduced-motion')).toBe('true')
    expect(page.findAll('.model-group')).toHaveLength(4)
    expect(page.find('button').exists()).toBe(false)
    observer.notify(true)
    await nextTick()
    expect(page.attributes('data-paused')).toBe('true')
  })
})
