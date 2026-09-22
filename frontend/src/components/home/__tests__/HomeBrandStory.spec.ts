import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import HomeBrandStory from '../HomeBrandStory.vue'
import { createWordmark, boundsOf } from '../brandGeometry'
import { createBrandStory, STORY_DURATION } from '../brandStory'
import initialSteam from './fixtures/initialSteam.json'

const mark = createWordmark('Café Shop')!
const story = createBrandStory(mark)!
let wrapper: VueWrapper | undefined
const pause = vi.fn(), resume = vi.fn(), reset = vi.fn()
beforeEach(() => {
  vi.clearAllMocks()
  Object.defineProperties(SVGSVGElement.prototype, {
    pauseAnimations: { value: pause, configurable: true },
    unpauseAnimations: { value: resume, configurable: true },
    setCurrentTime: { value: reset, configurable: true }
  })
})
afterEach(() => {
  wrapper?.unmount()
  for (const key of ['pauseAnimations', 'unpauseAnimations', 'setCurrentTime']) Reflect.deleteProperty(SVGSVGElement.prototype, key)
})
const render = () => {
  wrapper = mount(HomeBrandStory, { props: { wordmark: mark, story, siteName: 'Café Shop', paused: false, reducedMotion: false } })
  return wrapper
}

describe('accent-anchored breathing steam', () => {
  it('derives the steam anchor from the real accent of the é glyph', () => {
    expect(story.e.character).toBe('é')
    const topRing = [...story.e.rings].sort((a, b) => boundsOf(a).top - boundsOf(b).top)[0]
    expect(story.accent).toBe(topRing.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(2)},${y.toFixed(2)}`).join('') + 'Z')
    // The static wordmark keeps the steam in the accent's place, never both at once.
    expect(story.staticD).toContain(story.steam)
    expect(story.staticD).not.toContain(story.accent)
    expect(story.body).not.toContain(story.accent)
  })

  it('keeps the exact accepted steam pose as the anchor of its sway', () => {
    expect(story.steam).toBe(initialSteam.steam)
    const frames = story.steamValues.split(';')
    expect(frames[0]).toBe(initialSteam.steam)
    expect(frames[2]).toBe(initialSteam.steam)
    expect(frames.at(-1)).toBe(initialSteam.steam)
    // Two distinct sway poses, so the loop breathes rather than ping-pongs one shape.
    expect(frames[1]).not.toBe(frames[3])
  })

  it('keeps compatible contours and never lets the steam vanish or transform anything', () => {
    const page = render()
    for (const animation of page.findAll('animate[attributeName="d"]')) {
      const values = animation.attributes('values').split(';')
      const signature = values[0].replace(/-?\d+(?:\.\d+)?/g, '#')
      expect(values.every(value => value.replace(/-?\d+(?:\.\d+)?/g, '#') === signature)).toBe(true)
      expect(values.join('')).not.toMatch(/NaN|Infinity/)
    }
    const accent = page.find('.accent-steam animate').attributes('values').split(';')
    expect(accent[0]).toBe(story.steam)
    expect(accent.at(-1)).toBe(story.steam)
    expect(accent).not.toContain(story.accent)
    const opacity = page.find('.accent-steam animate[attributeName="opacity"]').attributes('values').split(';').map(Number)
    expect(Math.min(...opacity)).toBeGreaterThan(0)
    // The simplified story leaves no trace of the retired cloud, umbrella or pi acts.
    expect(page.find('.steam-cloud, .story-codex, .story-umbrella, .story-drip, .p-vessel, .p-to-pi, .pi-runoff, .impact-letter').exists()).toBe(false)
    expect(page.html()).not.toMatch(/#F09082|#4D9ABF|#F1BE58|<image|<img/)
    // Every letter of the wordmark is still drawn, with the é split into body + steam.
    expect(page.findAll('.brand-wordmark path')).toHaveLength(mark.letters.length)
  })

  it('has matching native timeline values and no per-frame JavaScript timers', () => {
    vi.useFakeTimers()
    const page = render()
    expect(page.findAll('animate, animateTransform').length).toBeGreaterThan(0)
    for (const animation of page.findAll('animate, animateTransform')) {
      expect(animation.attributes('dur')).toBe(STORY_DURATION)
      expect(animation.attributes('repeatCount')).toBe('indefinite')
      expect(animation.attributes('values').split(';')).toHaveLength(animation.attributes('keyTimes').split(';').length)
      const times = animation.attributes('keyTimes').split(';').map(Number)
      expect(times.every((time, i) => i === 0 || time > times[i - 1])).toBe(true)
      // A miscounted keySplines list silently disables the whole animation in browsers.
      if (animation.attributes('calcMode') === 'spline') {
        const splines = animation.attributes('keySplines').split(';')
        expect(splines).toHaveLength(times.length - 1)
        expect(splines.every(spline => spline.trim().split(/\s+/).every(n => Number(n) >= 0 && Number(n) <= 1))).toBe(true)
      }
    }
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('pauses natively, restores a complete wordmark for reduced motion, and resets new targets', async () => {
    const page = render()
    expect(resume).toHaveBeenCalledOnce()
    await page.setProps({ paused: true })
    expect(pause).toHaveBeenCalledOnce()
    await page.setProps({ reducedMotion: true })
    expect(page.find('animate, animateTransform').exists()).toBe(false)
    expect(page.find('path').attributes('d')).toBe(story.staticD)
    await page.setProps({ reducedMotion: false, paused: false })
    expect(resume).toHaveBeenCalledTimes(2)
    const other = createWordmark('Café Shoppe')!
    await page.setProps({ wordmark: other, story: createBrandStory(other)! })
    expect(reset).toHaveBeenCalledWith(0)
  })

  it('never invents steam for names without the real é', () => {
    for (const name of ['Sub2API', '后台站点名', 'Cafe Shop', '']) expect(createBrandStory(createWordmark(name))).toBeNull()
    expect(createBrandStory(createWordmark('Café Shop'))!.accent).toBe(story.accent)
  })
})
