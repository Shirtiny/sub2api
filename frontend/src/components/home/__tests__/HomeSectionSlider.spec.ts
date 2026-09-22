import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { usePreferredReducedMotion } from '@vueuse/core'
import HomeSectionSlider from '../HomeSectionSlider.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@vueuse/core', async (original) => {
  const { ref } = await import('vue')
  const motion = ref('no-preference')
  return { ...await original<typeof import('@vueuse/core')>(), usePreferredReducedMotion: () => motion }
})

let wrapper: VueWrapper | undefined
let root: HTMLElement
let scrollTo: ReturnType<typeof vi.fn>

async function render(hash = '') {
  let now = 0
  vi.spyOn(performance, 'now').mockImplementation(() => now)
  window.history.replaceState({}, '', `/home${hash}`)
  wrapper = mount(HomeSectionSlider, {
    attachTo: document.body,
    props: { slides: [{ id: 'one', label: 'One' }, { id: 'two', label: 'Two' }, { id: 'three', label: 'Three' }] },
    slots: {
      header: ({ compact }: { compact: boolean }) => h('header', { class: 'home-header', 'data-compact': compact }, h('a', { href: '#home-main', class: 'skip' }, 'Skip')),
      default: `<div data-home-slide="one"><section id="one"><h1 data-reveal>One</h1><p data-reveal style="--reveal-delay: 330ms">Details</p><a href="#two" class="guide">Read guide</a></section></div>
        <div data-home-slide="two"><section id="two"><h2 data-reveal>Two</h2></section></div>
        <div data-home-slide="three"><section id="three">Three<section id="quick-start">Quick start</section></section></div><a href="#quick-start" class="nested-guide">Quick guide</a>`
    }
  })
  root = wrapper.element as HTMLElement
  Object.defineProperty(root, 'clientHeight', { value: 800, configurable: true })
  Object.defineProperty(root.querySelector('.home-header'), 'offsetHeight', { value: 100, configurable: true })
  vi.spyOn(root, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect)
  // Third panel follows a long second panel, not a fixed index * viewport height.
  const tops = [100, 800, 2000]
  root.querySelectorAll<HTMLElement>('[data-home-slide]').forEach((panel, index) => {
    vi.spyOn(panel, 'getBoundingClientRect').mockImplementation(() => ({ top: tops[index] - root.scrollTop, bottom: (tops[index + 1] ?? 2700) - root.scrollTop } as DOMRect))
    panel.querySelectorAll<HTMLElement>('[data-reveal]').forEach(element => {
      vi.spyOn(element, 'getBoundingClientRect').mockImplementation(() => ({ top: tops[index] + 120 - root.scrollTop, bottom: tops[index] + 180 - root.scrollTop } as DOMRect))
    })
  })
  scrollTo = vi.fn()
  root.scrollTo = scrollTo
  await nextTick()
  await nextTick()
  now = 1000
  await new Promise(resolve => requestAnimationFrame(resolve))
  await nextTick()
  return wrapper
}

beforeEach(() => { usePreferredReducedMotion().value = 'no-preference' })
afterEach(() => {
  wrapper?.unmount()
  window.history.replaceState({}, '', '/')
  vi.restoreAllMocks()
})

describe('homepage section slider', () => {
  it('keeps all sections available and initially selects the first chapter', async () => {
    const page = await render()
    expect(page.findAll('[data-home-slide]')).toHaveLength(3)
    expect(page.find('.chapter-stop').attributes('aria-current')).toBe('step')
    expect(page.attributes('data-ready')).toBe('true')
  })

  it('smoothly travels to the measured section position without blanking main', async () => {
    const page = await render()
    await page.findAll('.chapter-stop')[2].trigger('click')
    expect(page.attributes('data-switching')).toBeUndefined()
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1900, behavior: 'smooth' })
    await page.find('.chapter-next').trigger('click')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 700, behavior: 'smooth' })
  })

  it('updates the current chapter while scrolling and does not skip a long section', async () => {
    const page = await render()
    root.scrollTop = 1300
    await page.trigger('scroll')
    await new Promise(resolve => requestAnimationFrame(resolve))
    await nextTick()
    expect(page.findAll('.chapter-stop')[1].attributes('aria-current')).toBe('step')
    expect(page.findAll('[data-home-slide]')[1].attributes('data-active')).toBe('true')
  })

  it('supports arrows and Home/End without changing global keyboard behavior', async () => {
    const page = await render()
    await page.find('.chapter-stop').trigger('keydown', { key: 'End' })
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1900, behavior: 'smooth' })
    expect(document.activeElement).toBe(page.findAll('.chapter-stop')[2].element)
    await page.findAll('.chapter-stop')[2].trigger('keydown', { key: 'ArrowDown' })
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 0, behavior: 'smooth' })
  })

  it('keeps internal guide and skip links focusable and leaves modifier clicks native', async () => {
    const page = await render()
    await page.find('.guide').trigger('click')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 700, behavior: 'smooth' })
    expect(document.activeElement?.id).toBe('two')
    await page.find('.skip').trigger('click')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 0, behavior: 'instant' })
    expect(document.activeElement?.id).toBe('home-main')
    scrollTo.mockClear()
    // Observe native modifier semantics, then prevent jsdom from performing navigation.
    root.addEventListener('click', event => {
      expect(event.defaultPrevented).toBe(false)
      event.preventDefault()
    }, { once: true })
    await page.find('.guide').trigger('click', { ctrlKey: true })
    expect(scrollTo).not.toHaveBeenCalled()
  })

  it('restores an incoming hash and uses immediate navigation with reduced motion', async () => {
    usePreferredReducedMotion().value = 'reduce'
    const page = await render('#three')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1900, behavior: 'instant' })
    await page.findAll('.chapter-stop')[1].trigger('click')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 700, behavior: 'instant' })
    expect(page.attributes('data-reduced-motion')).toBe('true')
  })

  it('navigates and restores hashes for a guide nested inside another chapter', async () => {
    const page = await render('#quick-start')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1900, behavior: 'instant' })
    await page.find('.nested-guide').trigger('click')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1900, behavior: 'smooth' })
    expect(document.activeElement?.id).toBe('quick-start')
  })

  it('cancels pending scroll work on unmount', async () => {
    const page = await render()
    const cancel = vi.spyOn(window, 'cancelAnimationFrame')
    await page.trigger('scroll')
    page.unmount()
    wrapper = undefined
    expect(cancel).toHaveBeenCalled()
  })

  it('keeps the glass state steady around the threshold and restores it only at the top', async () => {
    const page = await render()
    for (const [top, compact] of [[70, true], [45, true], [30, true], [9, true], [0, false]] as const) {
      root.scrollTop = top
      await page.trigger('scroll')
      await new Promise(resolve => requestAnimationFrame(resolve))
      await nextTick()
      expect(page.find('.home-header').attributes('data-compact')).toBe(String(compact))
    }
  })

  it('keeps revealed content readable inside long chapters', async () => {
    const page = await render()
    expect(page.find('#one [data-reveal]').attributes('data-revealed')).toBe('true')
    expect(page.find('#two [data-reveal]').attributes('data-revealed')).toBeUndefined()
    root.scrollTop = 700
    await page.trigger('scroll')
    await new Promise(resolve => requestAnimationFrame(resolve))
    expect(page.find('#one [data-reveal]').attributes('data-revealed')).toBeUndefined()
    expect(page.find('#two [data-reveal]').attributes('data-revealed')).toBe('true')
    root.scrollTop = 950
    await page.trigger('scroll')
    await new Promise(resolve => requestAnimationFrame(resolve))
    expect(page.find('#two [data-reveal]').attributes('data-revealed')).toBe('true')
  })

  it('cancels native programmatic scrolling without consuming the wheel event', async () => {
    const page = await render()
    await page.findAll('.chapter-stop')[2].trigger('click')
    root.scrollTop = 240
    await page.trigger('wheel')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 240, behavior: 'instant' })
    expect(page.attributes('data-switching')).toBeUndefined()
  })

  it('lets the browser redirect a running smooth scroll to the latest chapter', async () => {
    const page = await render()
    await page.findAll('.chapter-stop')[2].trigger('click')
    await page.findAll('.chapter-stop')[1].trigger('pointerdown')
    // Do not snap to an intermediate chapter before the next click redirects smooth scrolling.
    expect(scrollTo).toHaveBeenCalledTimes(1)
    await page.findAll('.chapter-stop')[1].trigger('click')
    expect(scrollTo).toHaveBeenCalledTimes(2)
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 700, behavior: 'smooth' })
  })
  it('continuously stages elements by viewport coverage and reverses on scrolling back', async () => {
    const page = await render()
    const progress = (selector: string) => Number((page.find(selector).element as HTMLElement).style.getPropertyValue('--reveal-progress'))
    expect(progress('#one h1')).toBe(1)
    root.scrollTop = 350
    await page.trigger('scroll')
    await new Promise(resolve => requestAnimationFrame(resolve))
    const title = progress('#one h1')
    const details = progress('#one p')
    expect(title).toBeGreaterThan(details)
    expect(title).toBeGreaterThan(0)
    expect(title).toBeLessThan(1)
    root.scrollTop = 0
    await page.trigger('scroll')
    await new Promise(resolve => requestAnimationFrame(resolve))
    expect(progress('#one h1')).toBe(1)
    expect(progress('#one p')).toBe(1)
  })

})
