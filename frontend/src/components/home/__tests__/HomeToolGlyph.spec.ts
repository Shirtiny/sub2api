import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import HomeToolGlyph from '../HomeToolGlyph.vue'
import codexSource from '@/assets/home/codex-mark.svg?raw'
import piSource from '@/assets/home/pi-mark.svg?raw'

let wrapper: VueWrapper | undefined
const pause = vi.fn(), resume = vi.fn()
const render = (paused = false, reducedMotion = false) => {
  wrapper = mount(HomeToolGlyph, { props: { paused, reducedMotion } })
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  Object.defineProperties(SVGSVGElement.prototype, {
    pauseAnimations: { value: pause, configurable: true },
    unpauseAnimations: { value: resume, configurable: true }
  })
})
afterEach(() => {
  wrapper?.unmount()
  Reflect.deleteProperty(SVGSVGElement.prototype, 'pauseAnimations')
  Reflect.deleteProperty(SVGSVGElement.prototype, 'unpauseAnimations')
})

describe('HomeToolGlyph native SVG choreography', () => {
  it('animates strokes, ink, and individual tiles instead of swapping image elements', () => {
    const page = render()
    expect(page.find('img, image, text').exists()).toBe(false)
    expect(page.findAll('.pi-tile')).toHaveLength(3)
    expect(page.findAll('.pi-tile animateTransform')).toHaveLength(3)
    expect(page.find('.codex-contour animate').attributes('attributeName')).toBe('stroke-dashoffset')
    expect(page.find('.codex-ink animate').attributes('attributeName')).toBe('fill-opacity')
    expect(page.find('.codex-face animate').exists()).toBe(true)
    expect(page.find('.codex-cursor animate').exists()).toBe(true)
    for (const animation of page.findAll('animate, animateTransform')) {
      expect(animation.attributes('dur')).toBe('16s')
      expect(animation.attributes('repeatCount')).toBe('indefinite')
      const times = animation.attributes('keyTimes').split(';').map(Number)
      expect(times).toEqual([...times].sort((a, b) => a - b))
      expect(times.length).toBe(animation.attributes('values').split(';').length)
    }
  })

  it('preserves exact product shapes and uses monochrome pi ink', () => {
    const page = render()
    const parser = new DOMParser()
    const codex = parser.parseFromString(codexSource, 'image/svg+xml').querySelector('path')!
    expect(page.find('.codex-ink').attributes('d')).toBe(codex.getAttribute('d'))
    expect(page.find('.codex-contour').attributes('d')).toBe(codex.getAttribute('d')!.split('z')[0] + 'z')
    expect(codex.getAttribute('d')!.match(/z/g)).toHaveLength(3)
    const pi = [...parser.parseFromString(piSource, 'image/svg+xml').querySelectorAll('path')]
    page.findAll('.pi-tile').forEach((tile, i) => {
      expect(tile.attributes('d')).toBe(pi[i].getAttribute('d'))
      expect(tile.attributes('fill')).toBe('currentColor')
    })
  })

  it('pauses and resumes a shared native SVG clock without resetting its position', async () => {
    const page = render()
    expect(resume).toHaveBeenCalledOnce()
    await page.setProps({ paused: true })
    expect(pause).toHaveBeenCalledOnce()
    await page.setProps({ paused: false })
    expect(resume).toHaveBeenCalledTimes(2)
  })

  it('starts paused when hidden and shows a complete static mark for reduced motion', async () => {
    const page = render(true)
    expect(pause).toHaveBeenCalledOnce()
    expect(resume).not.toHaveBeenCalled()
    await page.setProps({ reducedMotion: true })
    expect(page.findAll('path')).toHaveLength(1)
    expect(page.find('animate, animateTransform').exists()).toBe(false)
    expect(page.find('path').attributes('fill-opacity')).toBeUndefined()
    await page.setProps({ paused: false, reducedMotion: false })
    expect(page.find('animate').exists()).toBe(true)
    expect(resume).toHaveBeenCalledOnce()
  })
})
