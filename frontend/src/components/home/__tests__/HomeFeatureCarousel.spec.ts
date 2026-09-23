import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import source from '../HomeFeatureCarousel.vue?raw'
import { useDocumentVisibility, usePreferredReducedMotion } from '@vueuse/core'
import HomeFeatureCarousel from '../HomeFeatureCarousel.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

const state = vi.hoisted(() => ({ locale: 'zh', notify: (_visible: boolean, _ratio?: number) => {} }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      let value: unknown = state.locale === 'zh' ? zh : en
      for (const part of key.split('.')) value = (value as Record<string, unknown>)?.[part]
      return typeof value === 'string' ? value : key
    }
  })
}))
vi.mock('@vueuse/core', async importOriginal => {
  const { ref } = await import('vue')
  const visibility = ref('visible')
  const motion = ref('no-preference')
  return {
    ...await importOriginal<typeof import('@vueuse/core')>(),
    useDocumentVisibility: () => visibility,
    usePreferredReducedMotion: () => motion,
    useIntersectionObserver: (_target: unknown, callback: (entries: { isIntersecting: boolean; intersectionRatio: number }[]) => void) => {
      state.notify = (visible, ratio = visible ? 1 : 0) => callback([{ isIntersecting: visible, intersectionRatio: ratio }])
    }
  }
})

let wrapper: VueWrapper | undefined
beforeEach(() => {
  state.locale = 'zh'
  useDocumentVisibility().value = 'visible'
  usePreferredReducedMotion().value = 'no-preference'
})
afterEach(() => wrapper?.unmount())
const render = () => (wrapper = mount(HomeFeatureCarousel, { attachTo: document.body }))
const current = (page: VueWrapper) => page.find('.feature-frame.is-current').attributes('data-feature')
async function enter() { state.notify(true); await nextTick() }
async function finishClock(page: VueWrapper) { await page.find('.feature-clock').trigger('animationend') }
async function finishEntry(page: VueWrapper) {
  if (page.find('.feature-clock').attributes('data-phase') === 'enter') await finishClock(page)
}

describe('everyday SVG feature carousel', () => {
  it('adds three original SVG scenes without remote assets or fabricated account figures', () => {
    const page = render()
    expect(page.attributes('role')).toBe('region')
    expect(page.attributes('aria-roledescription')).toBe('轮播')
    expect(page.findAll('svg.feature-scene').map(svg => svg.attributes('data-scene'))).toEqual(['intelligence', 'speed', 'trust'])
    expect(page.find('img, image, canvas, iframe').exists()).toBe(false)
    expect(page.findAll('.feature-selector').map(button => button.text())).toEqual(['原生智能 · Intelligence', '快速响应 · Speed', '真实可信 · Trust'])
    expect(page.find('.feature-grid p').exists()).toBe(false)
    expect(page.findAll('.feature-description')).toHaveLength(3)
    expect(page.find('.feature-presentation h4').exists()).toBe(false)
    expect(page.find('[data-scene="speed"]').text()).toBe('Server')
    expect(page.find('[data-scene="intelligence"] text').exists()).toBe(false)
    expect(page.find('[data-scene="trust"]').text()).toBe('')
    expect(page.find('[data-scene="trust"] text').exists()).toBe(false)
    expect(page.find('[data-feature="trust"] .feature-art > .sr-only').text()).toBe(zh.home.landing.featureShowcase.billingStory.summary)
    expect(page.find('.feature-story, .story-kicker, .story-footnote').exists()).toBe(false)
    expect(page.find('.astra-signature').text()).toBe('ASTRA')
    expect(page.findAll('.feature')[1].text()).not.toContain('WS 首字节')
    expect(page.find('[data-feature="speed"] .feature-description').text()).toContain('WS 首字节约 0.3 秒')
    expect(page.findAll('.feature-description').map(copy => copy.text())).toEqual([
      zh.home.landing.features.connect.description,
      zh.home.landing.features.create.description,
      zh.home.landing.features.manage.description
    ])
    expect(page.find('.feature-presentation').text()).not.toMatch(/让每一次思考，\s*都全力以赴|完整能力 · 稳定发挥|Intelligence \/ 01/)
    expect(page.text()).not.toMatch(/¥|\$|99\.9|100%/)
    for (const feature of ['intelligence', 'speed', 'trust']) expect(page.find(`[data-scene="${feature}"]`).text()).not.toContain('$')
    expect(page.findAll('.feature-frame[aria-hidden="false"]')).toHaveLength(1)
    expect(page.find('.feature-progress, .feature-playback').exists()).toBe(false)
    expect(page.findAll('.feature-presentation button')).toHaveLength(0)
    expect(page.findAll('.intelligence-caption')).toHaveLength(3)
    expect(page.find('.feature-copy.sr-only').exists()).toBe(false)
    for (const button of page.findAll('.feature-selector')) {
      expect(page.find(`#${button.attributes('aria-controls')}`).exists()).toBe(true)
    }
    expect(page.findAll('.feature-grid .feature-selector')).toHaveLength(3)
    expect(page.findAll('.feature-selector')).toHaveLength(3)
    expect(page.find('.feature-controls, .feature-selectors').exists()).toBe(false)
    for (const button of page.findAll('.feature-selector')) {
      const description = page.find(`#${button.attributes('aria-describedby')}`)
      expect(description.text()).not.toBe('')
      expect(description.element.closest('.feature-frame')?.id).toBe(button.attributes('aria-controls'))
    }
    const ids = page.findAll('[id]').map(element => element.attributes('id'))
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('keeps every original description visible below its animation, not inside the timed caption overlay', async () => {
    const page = render()
    const descriptions = [zh.home.landing.features.connect.description, zh.home.landing.features.create.description, zh.home.landing.features.manage.description]
    for (let index = 0; index < 3; index++) {
      await page.findAll('.feature-selector')[index].trigger('click')
      const frame = page.find('.feature-frame.is-current')
      const copy = frame.find('.feature-copy')
      expect(copy.text()).toBe(descriptions[index])
      expect(copy.isVisible()).toBe(true)
      expect(copy.classes()).not.toContain('sr-only')
      expect(copy.attributes('aria-hidden')).not.toBe('true')
      expect(frame.find('.feature-art').element.nextElementSibling).toBe(copy.element)
      expect(frame.find('.feature-art .feature-description').exists()).toBe(false)
    }
    await page.findAll('.feature-selector')[0].trigger('click')
    expect(page.find('.feature-art .intelligence-story').exists()).toBe(true)
    expect(source).toContain('grid-template-rows: minmax(200px, 1fr) auto')
    expect(source).toContain('.feature-copy { position: relative; z-index: 1; display: flex; justify-content: flex-end; margin-top: 0; padding: 16px 32px 24px;')
    expect(source).toContain('border-top: 1px solid var(--cafe-line)')
    expect(source).toContain('width: 100%; min-width: 0; max-width: none; margin: 0; padding-right: 0;')
    expect(source).toContain('text-align: right;')
    expect(source).not.toContain("'sr-only': slide.key")
  })

  it('uses the full desktop footer width without truncating copy or forcing narrow screens onto one line', () => {
    expect(source).not.toContain('820px')
    expect(source).toContain('font-size: clamp(13px, 1.3vw, 14px); line-height: 1.7;')
    expect(source).toContain('.feature-copy { padding: 12px 28px; }')
    expect(source).not.toMatch(/white-space: nowrap|text-overflow: ellipsis|line-clamp/)
    // At the desktop breakpoints, 60 full-width Chinese glyphs fit the available
    // container minus frame border and footer padding. Not a browser layout test.
    for (const viewport of [1024, 1080, 1280, 1440, 1920]) {
      const available = Math.min(1280, viewport - 160) - 2 - 56
      const fontSize = Math.min(14, Math.max(13, viewport * .013))
      expect(zh.home.landing.features.connect.description.length * fontSize).toBeLessThan(available)
    }
  })

  it('uses the original first-tab frame, canvas and footer for every tab at every breakpoint', () => {
    const css = source.split('<style scoped>')[1]!
    const frame = css.match(/\.feature-frame \{([^}]+)\}/)![1]!
    expect(frame).toContain('grid-template-rows: minmax(320px, 1fr) auto;')
    expect(frame).toContain('padding: 0;')
    expect(source).toContain('.feature-stage { display: grid; isolation: isolate; overflow: hidden; background: var(--cafe-page); }')
    expect(source).toContain('.feature-art { grid-area: 1 / 1; position: relative; width: 100%; min-height: 0; overflow: hidden; }')
    expect(source).toContain('.feature-frame { grid-template-rows: minmax(400px, 1fr) auto; }')
    expect(source).toContain('.feature-frame { min-height: 0; grid-template-rows: minmax(200px, 1fr) auto; }')
    // No feature-specific wrapper overrides or negative-margin compensation.
    expect(css).not.toContain('[data-feature')
    expect(css).not.toMatch(/margin-(inline|bottom):\s*-/)
    expect(source).toContain('margin-top: 0; padding: 16px 32px 24px; border-top: 1px solid var(--cafe-line);')
    expect(source).toContain('.feature-copy { padding: 12px 28px; }')
    expect(source).toContain('.feature-copy { padding: 14px 18px 20px; }')
  })

  it('keeps the same full-width canvas followed by a separate description in each frame', () => {
    for (const frame of render().findAll('.feature-frame')) {
      expect(Array.from(frame.element.children).map(child => child.className)).toEqual(['feature-art', 'feature-copy'])
      const canvas = frame.find('.feature-art > svg.feature-scene')
      expect(canvas.attributes('viewBox')).toBe('0 0 1200 520')
      expect(canvas.attributes('preserveAspectRatio')).toBe('xMidYMid meet')
      expect(frame.find('.feature-copy > .feature-description').exists()).toBe(true)
    }
  })

  it('uses three short, understated asides without repeated headline blocks or notes', () => {
    const page = render()
    expect(page.findAll('.intelligence-line').map(line => line.text())).toEqual(['1. 理解全貌', '2. 深入关联', '3. 清晰落地'])
    expect(page.find('.intelligence-note, .intelligence-caption br').exists()).toBe(false)
    expect(source).not.toContain('.intelligence-caption::before')
    expect(source).toContain('font-size: clamp(13px, 1.25vw, 16px)')
    expect(source).toContain('.intelligence-caption { position: absolute; right: 6%; bottom: 7%;')
    expect(source).toContain('max-width: 75%; text-align: right;')
    expect(source).not.toMatch(/intelligence-caption[^{}]*\{[^}]*left:/)
    expect(page.findAll('.feature-description')).toHaveLength(3)
  })

  it('automatically cycles through all three completed scenes with a 1.5s final hold', async () => {
    const page = render()
    expect(page.attributes('data-animated')).toBe('false')
    expect(page.attributes('data-autoplay')).toBe('true')
    await enter()
    expect(page.attributes('data-animated')).toBe('true')
    expect(page.find('.feature-stage').attributes('aria-live')).toBe('off')
    const firstScene = page.find('svg.intelligence-scene').element
    const timings = [13580, 14720, 14880]
    for (const [index, kind] of ['intelligence', 'speed', 'trust'].entries()) {
      expect(current(page)).toBe(kind)
      expect(page.findAll('.feature-clock')).toHaveLength(1)
      expect(page.find('.feature-clock').attributes('style')).toBe(`--phase-duration: ${timings[index]}ms;`)
      await finishClock(page)
      expect(current(page)).toBe(kind)
      expect(page.find('.feature-frame.is-current').attributes('data-holding')).toBe('true')
      expect(page.find('.feature-clock').attributes('data-phase')).toBe('hold')
      expect(page.find('.feature-clock').attributes('style')).toBe('--phase-duration: 1500ms;')
      await finishClock(page)
      expect(page.findAll('.feature-selector[aria-pressed="true"]')).toHaveLength(1)
      expect(page.find('.feature-clock').attributes('data-phase')).toBe('enter')
      expect(page.find('.feature-clock').attributes('style')).toBe('--phase-duration: 900ms;')
      await finishEntry(page)
      expect(page.find('.feature-clock').attributes('data-phase')).toBe('play')
      expect(page.find('.feature-frame.is-current').attributes('data-holding')).toBe('false')
    }
    expect(current(page)).toBe('intelligence')
    expect(page.find('svg.intelligence-scene').element).not.toBe(firstScene)
    expect(source).toContain('--intelligence-duration: 14s')
    expect(source).not.toMatch(/setInterval|setTimeout|feature-playback|feature-countdown/)
    for (const shot of ['understand', 'connect', 'resolve', 'finale']) {
      expect(source).toContain(`animation: intelligence-${shot} var(--intelligence-duration) ease-in-out infinite`)
    }
  })

  it('ignores other illustration animation events instead of advancing early', async () => {
    const page = render()
    await enter()
    await page.find('.intelligence-finale').trigger('animationend')
    await page.find('.intelligence-finale').trigger('animationiteration')
    expect(current(page)).toBe('intelligence')
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('play')
  })

  it('keeps ambient world traffic moving during the completed-task hold, but not offscreen', () => {
    const page = render()
    const speed = page.find('.speed-scene')
    const routes = speed.findAll('.world-request, .world-return, .port-echo, .globe-halo')
    expect(routes.length).toBeGreaterThan(1)
    expect(routes.every(route => route.classes().includes('scene-ambient'))).toBe(true)
    expect(speed.find('.response-content').classes()).not.toContain('scene-ambient')
    expect(source).toContain(".feature-carousel[data-animated='true'] .feature-frame.is-current[data-holding='true'] :deep(.scene-ambient) { animation-play-state: running !important; }")
  })

  it.each([
    [0, 'intelligence', 420], [1, 'speed', 1280], [2, 'trust', 1120]
  ] as const)('manual selection of tab %s permanently stops rotation but keeps its extended animation looping', async (index, kind, outro) => {
    const page = render()
    await enter()
    await page.findAll('.feature-selector')[index].trigger('click')
    expect(page.attributes('data-autoplay')).toBe('false')
    await finishEntry(page)
    const scene = page.find(`svg[data-scene="${kind}"]`).element
    await finishClock(page)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('hold')
    expect(page.find('.feature-clock').attributes('style')).toBe('--phase-duration: 1500ms;')
    await finishClock(page)
    expect(current(page)).toBe(kind)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('outro')
    expect(page.find('.feature-clock').attributes('style')).toBe(`--phase-duration: ${outro}ms;`)
    await finishClock(page)
    expect(current(page)).toBe(kind)
    expect(page.find(`svg[data-scene="${kind}"]`).element).not.toBe(scene)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('play')
    state.notify(false)
    await nextTick()
    await enter()
    expect(page.attributes('data-autoplay')).toBe('false')
  })

  it('manual selection during a final hold starts a fresh scene and cancels rotation', async () => {
    const page = render()
    await enter()
    await finishClock(page)
    await page.findAll('.feature-selector')[1].trigger('click')
    expect(current(page)).toBe('speed')
    expect(page.attributes('data-autoplay')).toBe('false')
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('enter')
    await finishEntry(page)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('play')
    expect(page.findAll('[data-holding="true"]')).toHaveLength(0)
  })

  it('pauses the native phase clock offscreen and in the background, including during the final hold', async () => {
    const page = render()
    await finishClock(page)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('play')
    await enter()
    await finishClock(page)
    useDocumentVisibility().value = 'hidden'
    await nextTick()
    await finishClock(page)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('hold')
    useDocumentVisibility().value = 'visible'
    state.notify(false)
    await nextTick()
    await finishClock(page)
    expect(current(page)).toBe('intelligence')
    await enter()
    await finishClock(page)
    expect(current(page)).toBe('speed')
    expect(source).toContain(".feature-carousel[data-animated='false'] .feature-clock { animation-play-state: paused !important; }")
    expect(source).toContain(".feature-frame[data-holding='true'] :deep(.feature-scene *)")
    expect(source).toContain(".feature-frame[data-holding='true'] .intelligence-story :deep(*)")
  })

  it('crossfades mounted scenes and starts the incoming timeline only after its entrance finishes', async () => {
    const page = render()
    await enter()
    const outgoing = page.find('svg.intelligence-scene').element
    await page.findAll('.feature-selector')[1].trigger('click')
    expect(page.find('svg.intelligence-scene').element).toBe(outgoing)
    expect(page.find('.feature-frame.is-current').attributes('data-entering')).toBe('true')
    expect(page.find('.feature-clock').attributes('style')).toBe('--phase-duration: 900ms;')
    await finishEntry(page)
    expect(page.find('.feature-frame.is-current').attributes('data-entering')).toBe('false')
    expect(page.find('.feature-clock').attributes('style')).toBe('--phase-duration: 14720ms;')
    expect(source).toContain(".feature-frame[data-entering='true'] :deep(.feature-scene *)")
    expect(source).toContain(".feature-frame[data-entering='true'] .intelligence-story :deep(*)")
    expect(source).toContain('transition: opacity .9s cubic-bezier(.4, 0, .2, 1), visibility 0s .9s;')
    expect(source).toContain('transform: translateY(12px) scale(.985);')
    expect(source).toContain('animation: feature-art-enter .9s cubic-bezier(.22, 1, .36, 1) both;')
    expect(source).toContain(".feature-carousel[data-animated='false'] :deep(.feature-scene),")
    expect(source).toContain('background: transparent;')
    const frame = source.match(/\.feature-frame \{([^}]+)\}/)![1]!
    expect(frame).not.toContain('transform:')
    expect(frame).not.toContain('background:')
  })

  it('does not reset the current illustration when its selector only cancels autoplay', async () => {
    const page = render()
    await enter()
    await finishClock(page)
    const scene = page.find('svg.intelligence-scene').element
    await page.findAll('.feature-selector')[0].trigger('click')
    expect(page.attributes('data-autoplay')).toBe('false')
    expect(page.find('svg.intelligence-scene').element).toBe(scene)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('hold')
  })

  it('keeps entrance playback paused in the background and honors the latest quick selection', async () => {
    const page = render()
    await enter()
    await page.findAll('.feature-selector')[1].trigger('click')
    await page.findAll('.feature-selector')[2].trigger('click')
    expect(current(page)).toBe('trust')
    expect(page.findAll('.feature-clock')).toHaveLength(1)
    useDocumentVisibility().value = 'hidden'
    await nextTick()
    await finishClock(page)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('enter')
    useDocumentVisibility().value = 'visible'
    await nextTick()
    await finishEntry(page)
    expect(page.find('.feature-clock').attributes('style')).toBe('--phase-duration: 14880ms;')
    expect(page.attributes('data-autoplay')).toBe('false')
  })

  it('pauses below the viewport threshold and when the document is hidden', async () => {
    const page = render()
    state.notify(true, .1)
    await nextTick()
    expect(page.attributes('data-running')).toBe('false')
    await enter()
    expect(page.attributes('data-running')).toBe('true')
    useDocumentVisibility().value = 'hidden'
    await nextTick()
    expect(current(page)).toBe('intelligence')
    expect(page.attributes('data-running')).toBe('false')
    useDocumentVisibility().value = 'visible'
    await nextTick()
    expect(page.attributes('data-running')).toBe('true')
    state.notify(false)
    await nextTick()
    expect(page.attributes('data-running')).toBe('false')
  })

  it('keeps playing during hover and focus without needing a resume button', async () => {
    const page = render()
    await enter()
    await page.trigger('mouseenter')
    ;(page.find('.feature-selector').element as HTMLButtonElement).focus()
    await nextTick()
    expect(page.attributes('data-animated')).toBe('true')
    expect(current(page)).toBe('intelligence')
    await page.trigger('mouseleave')
    expect(page.attributes('data-animated')).toBe('true')
  })

  it('switches manually and restarts the entire synchronized illustration on returning to intelligence', async () => {
    const page = render()
    await enter()
    const film = page.find('svg.intelligence-scene').element
    const story = page.find('.intelligence-story').element
    await page.findAll('.feature-selector')[2].trigger('click')
    expect(current(page)).toBe('trust')
    expect(page.find('.feature-frame.is-current .feature-description').text()).toBe(zh.home.landing.features.manage.description)
    expect(page.findAll('.feature-frame[aria-hidden="false"]')).toHaveLength(1)
    await page.findAll('.feature-selector')[0].trigger('click')
    expect(current(page)).toBe('intelligence')
    expect(page.find('svg.intelligence-scene').element).not.toBe(film)
    expect(page.find('.intelligence-story').element).not.toBe(story)
    expect(page.attributes('data-animated')).toBe('true')
  })

  it('supports arrow, Home and End keys without changing the outer chapter', async () => {
    const page = render()
    await enter()
    const buttons = page.findAll('.feature-selector')
    await buttons[0].trigger('keydown', { key: 'ArrowLeft' })
    expect(current(page)).toBe('trust')
    expect(page.attributes('data-autoplay')).toBe('false')
    expect(document.activeElement).toBe(buttons[2].element)
    expect(page.attributes('data-animated')).toBe('true')
    await buttons[2].trigger('keydown', { key: 'ArrowRight' })
    expect(current(page)).toBe('intelligence')
    await buttons[0].trigger('keydown', { key: 'End' })
    expect(current(page)).toBe('trust')
    await buttons[2].trigger('keydown', { key: 'Home' })
    expect(current(page)).toBe('intelligence')
    await buttons[0].trigger('keydown', { key: 'ArrowDown' })
    expect(current(page)).toBe('intelligence')
  })

  it('disables automatic motion but preserves manual browsing for reduced motion', async () => {
    usePreferredReducedMotion().value = 'reduce'
    const page = render()
    await enter()
    expect(page.attributes('data-running')).toBe('false')
    expect(page.attributes('data-reduced-motion')).toBe('true')
    await finishClock(page)
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('play')
    await page.findAll('.feature-selector')[1].trigger('click')
    expect(current(page)).toBe('speed')
    expect(page.find('.feature-clock').attributes('data-phase')).toBe('play')
  })

  it('preserves the localized original feature titles without a second selector row', () => {
    state.locale = 'en'
    const page = render()
    expect(page.findAll('.feature-selector').map(button => button.text())).toEqual(['Native intelligence', 'Fast response', 'Verifiable trust'])
    expect(page.text()).not.toMatch(/home\.landing|智能|速度|可信/)
    expect(page.find('.feature-grid p').exists()).toBe(false)
    expect(page.find('.feature-frame .feature-description').text()).toBe(en.home.landing.features.connect.description)
    expect(page.findAll('.intelligence-line').map(line => line.text())).toEqual(['1. See the whole picture', '2. Explore deeper connections', '3. Turn insight into action'])
  })
})
