import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import HomeFeatureScene from '../HomeFeatureScene.vue'
vi.mock('@vueuse/core', async () => {
  const { ref } = await import('vue')
  const compact = ref(false)
  return { useMediaQuery: () => compact }
})

let page: VueWrapper | undefined
const media = () => useMediaQuery('(max-width: 639px)')
beforeEach(() => { media().value = false })
afterEach(() => page?.unmount())

describe('proportional SVG feature tableaux', () => {
  it.each(['speed', 'trust'] as const)('draws a complete %s scene with no raster assets or extra marketing headlines', kind => {
    page = mount(HomeFeatureScene, { props: { kind } })
    expect(page.attributes('viewBox')).toBe('0 0 1200 520')
    expect(page.attributes('preserveAspectRatio')).toBe('xMidYMid meet')
    expect(page.attributes('aria-hidden')).toBe('true')
    expect(page.find('image, foreignObject').exists()).toBe(false)
    if (kind === 'speed') {
      expect(page.findAll('[data-state="thinking"] circle')).toHaveLength(3)
      expect(page.find('.server-label').text()).toBe('Server')
    } else {
      expect(page.text()).toBe('')
      expect(page.findAll('.receipt-entry')).toHaveLength(3)
    }
    expect(page.findAll('[data-detail]').length).toBeGreaterThanOrEqual(3)
    expect(page.findAll(kind === 'speed' ? '.world-packet' : '.charge-signal')).toHaveLength(kind === 'speed' ? 14 : 3)
    for (const element of page.findAll('[fill], [clip-path]')) {
      const reference = `${element.attributes('fill') ?? ''} ${element.attributes('clip-path') ?? ''}`.match(/url\(#([^)]+)\)/)?.[1]
      if (reference) expect(page.find(`#${reference}`).exists()).toBe(true)
    }
  })

  it.each(['speed', 'trust'] as const)('recomposes all %s details into a portrait without cropping side panels', async kind => {
    page = mount(HomeFeatureScene, { props: { kind } })
    media().value = true
    await nextTick()
    expect(page.attributes('viewBox')).toBe('0 0 600 720')
    expect(page.attributes('data-layout')).toBe('portrait')
    if (kind === 'speed') {
      expect(page.find('[data-detail="global-network"]').attributes('transform')).toBe('translate(300 495)')
      expect(page.find('[data-detail="streaming-response"]').attributes('transform')).toBe('translate(300 136)')
      expect(page.findAll('.express-lane')).toHaveLength(3)
    } else {
      expect(page.find('[data-detail="request-window"]').attributes('transform')).toBe('translate(150 152) scale(0.8)')
      expect(page.find('[data-detail="request-billing"]').attributes('transform')).toBe('translate(444 152) scale(0.8)')
      expect(page.find('[data-detail="usage-receipt"]').attributes('transform')).toBe('translate(300 510) scale(1)')
      expect(page.findAll('.receipt-entry')).toHaveLength(3)
    }
    media().value = false
    await nextTick()
    expect(page.attributes('data-layout')).toBe('landscape')
  })

  it('uses feature-specific details rather than three variations of the same icon', async () => {
    page = mount(HomeFeatureScene, { props: { kind: 'intelligence' } })
    expect(page.find('[data-detail="system-structure"]').exists()).toBe(true)
    expect(page.findAll('.input-card')).toHaveLength(3)
    expect(page.find('.neural-signal, [data-detail="work-inputs"]').exists()).toBe(false)
    await page.setProps({ kind: 'speed' })
    expect(page.findAll('.express-lane')).toHaveLength(3)
    expect(page.findAll('.world-route')).toHaveLength(7)
    expect(page.findAll('.world-route[data-destination="NL"]')).toHaveLength(7)
    expect(page.find('.request-ink').exists()).toBe(true)
    expect(page.find('.response-content').exists()).toBe(true)
    expect(page.find('[data-detail="streaming-response"]').exists()).toBe(true)
    await page.setProps({ kind: 'trust' })
    expect(page.findAll('.verify-check')).toHaveLength(1)
    expect(page.find('[data-detail="usage-receipt"]').exists()).toBe(true)
    expect(page.findAll('[data-source-request]')).toHaveLength(3)
    expect(page.findAll('[data-billing-request]')).toHaveLength(3)
  })

  it('recomposes the intelligence system when switching to a portrait viewport', async () => {
    page = mount(HomeFeatureScene, { props: { kind: 'intelligence' } })
    expect(page.attributes('viewBox')).toBe('0 0 1200 520')
    media().value = true
    await nextTick()
    expect(page.attributes('viewBox')).toBe('0 0 600 720')
    expect(page.attributes('data-layout')).toBe('portrait')
    expect(page.findAll('.input-card')).toHaveLength(3)
  })
})
