import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import HomeTrustScene from '../HomeTrustScene.vue'
import source from '../HomeTrustScene.vue?raw'
import carouselSource from '../HomeFeatureCarousel.vue?raw'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

const pages: VueWrapper[] = []
const render = (compact = false) => {
  const page = mount(HomeTrustScene, { props: { compact } })
  pages.push(page)
  return page
}
afterEach(() => pages.splice(0).forEach(page => page.unmount()))
const numbers = (value: string) => value.match(/-?[\d.]+/g)!.map(Number)
const frames = (name: string) => source.split(`@keyframes ${name} {`)[1].split('@keyframes')[0]
const requestKeys = ['request-01', 'request-02', 'request-03']

describe('per-request billing and printed usage records', () => {
  it.each([false, true])('draws connected request history, calculation and receipt without quotes or prose (%s)', compact => {
    const page = render(compact)
    expect(page.attributes('data-scene')).toBe('trust')
    expect(page.attributes('aria-hidden')).toBe('true')
    expect(page.text()).toBe('')
    expect(page.find('text, image, foreignObject, filter, canvas').exists()).toBe(false)
    for (const detail of ['request-window', 'request-billing', 'usage-receipt', 'connected-billing-route']) {
      expect(page.find(`[data-detail="${detail}"]`).exists()).toBe(true)
    }
    expect(page.find('.usage-slip, [data-transfer], [data-source-row], [data-receipt-row]').exists()).toBe(false)
    expect(page.findAll('.entry-cost')).toHaveLength(3)
    expect(page.findAll('.verify-check')).toHaveLength(1)
    expect(page.findAll('*').length).toBeLessThan(320)
  })

  it('maps exactly one request to one calculation and one usage record, retaining its identity and model', () => {
    const page = render()
    for (const attribute of ['data-source-request', 'data-billing-request', 'data-usage-record']) {
      expect(page.findAll(`[${attribute}]`).map(row => row.attributes(attribute))).toEqual(requestKeys)
    }
    const fingerprints = new Set<string>()
    for (const key of requestKeys) {
      const parts = ['source-request', 'billing-request', 'usage-record'].map(part => page.find(`[data-${part}="${key}"]`))
      for (const field of ['request_id', 'model']) {
        const references = parts.map(part => part.find(`[data-field="${field}"]`).attributes('href'))
        expect(new Set(references).size).toBe(1)
        expect(page.find(references[0]).exists()).toBe(true)
        if (field === 'request_id') fingerprints.add(references[0])
      }
    }
    expect(fingerprints.size).toBe(3)
    expect(page.find('[data-source-request="input"], [data-usage-record="cache"], [data-usage-record="output"]').exists()).toBe(false)
  })

  it.each(requestKeys)('shows all pricing inputs inside %s, rather than treating token categories as requests', key => {
    const page = render()
    const calculation = page.find(`[data-billing-request="${key}"]`)
    expect(calculation.findAll('[data-term]').map(term => term.attributes('data-term'))).toEqual(['input', 'cache', 'output'])
    expect(calculation.find('[data-stage="model-pricing"]').exists()).toBe(true)
    expect(calculation.findAll('[data-stage="unit-price"]')).toHaveLength(3)
    const receipt = page.find(`[data-usage-record="${key}"]`)
    for (const field of ['input_tokens', 'cache_read_tokens', 'output_tokens']) {
      expect(calculation.findAll(`[data-field="${field}"]`)).toHaveLength(1)
      expect(receipt.findAll(`[data-field="${field}"]`)).toHaveLength(1)
      expect(Number(receipt.find(`[data-field="${field}"]`).attributes('width')))
        .toBeCloseTo(Number(calculation.find(`[data-field="${field}"] .quantity-piece`).attributes('width')) * .45)
    }
  })

  it('sums the request terms and applies one multiplier before copying the final charge to its usage row', () => {
    const page = render()
    const charges: number[] = []
    for (const key of requestKeys) {
      const calculation = page.find(`[data-billing-request="${key}"]`)
      const terms = calculation.findAll('.term-cost').reduce((sum, rect) => sum + Number(rect.attributes('width')), 0)
      const subtotal = Number(calculation.find('[data-field="total_cost"] rect').attributes('width'))
      const multiplier = Number(calculation.attributes('style').match(/--rate-scale:\s*([\d.]+)/)![1])
      expect(subtotal).toBeCloseTo(terms)
      expect(calculation.findAll('[data-field="rate_multiplier"]')).toHaveLength(1)
      expect(Number(calculation.find('.actual-meter').attributes('width'))).toBeCloseTo(subtotal)
      const printed = Number(page.find(`[data-usage-record="${key}"] .record-charge`).attributes('width'))
      expect(printed).toBeCloseTo(subtotal * multiplier)
      charges.push(printed)
    }
    expect(new Set(charges).size).toBe(3)
    const receipt = page.find('[data-detail="usage-receipt"]')
    expect(receipt.findAll('[data-field="actual_cost"]')).toHaveLength(3)
    expect(receipt.find('[data-field="rate_multiplier"], .actual-meter, .rate-tag, .total-amount, [data-detail="single-settlement"]').exists()).toBe(false)
    expect(receipt.find('[data-detail="records-reconciled"] [data-field="actual_cost"]').exists()).toBe(false)
  })

  it('partitions cached input only inside its request and uses a separate cached unit-price term', () => {
    const page = render()
    expect(page.find('[data-detail="request-window"] .cache-partition').exists()).toBe(false)
    for (const key of requestKeys) {
      const calculation = page.find(`[data-billing-request="${key}"]`)
      expect(calculation.findAll('.cache-partition')).toHaveLength(1)
      expect(calculation.findAll('[data-term="cache"] .cached-piece')).toHaveLength(1)
      expect(calculation.find('[data-term="input"] .cached-piece').exists()).toBe(false)
      const input = Number(calculation.find('[data-term="input"] .quantity-piece').attributes('width'))
      expect(calculation.attributes('style')).toContain(`--cache-origin: ${input + 6}px`)
      const cachedPrice = Number(calculation.find('[data-term="cache"] .unit-price ellipse').attributes('cy'))
      const inputPrice = Number(calculation.find('[data-term="input"] .unit-price ellipse').attributes('cy'))
      expect(Math.abs(cachedPrice)).toBeLessThan(Math.abs(inputPrice))
    }
    expect(frames('cache-partition')).toContain('translate(var(--cache-origin), -44px)')
    expect(frames('cache-partition')).toContain('14%, 100% { opacity: 1; transform: translate(0, 0); }')
  })

  it.each([false, true])('routes requests into the calculator, and ONLY calculated charges into the printer (%s)', compact => {
    const page = render(compact)
    const [sx, sy, ss] = numbers(page.find('[data-detail="request-window"]').attributes('transform'))
    const [bx, by, bs] = numbers(page.find('[data-detail="request-billing"]').attributes('transform'))
    const [rx, ry] = numbers(page.find('[data-detail="usage-receipt"]').attributes('transform'))
    const [incoming, outgoing] = page.findAll('.connection-track').map(path => path.attributes('d'))
    expect(numbers(incoming).slice(0, 2)).toEqual([sx + 150 * ss, sy + 6 * ss])
    expect(numbers(incoming).slice(-2)).toEqual([bx - 154 * bs, by - 115 * bs])
    expect(numbers(outgoing).slice(0, 2)).toEqual([bx + 154 * bs, by + 106 * bs])
    if (compact) expect(outgoing.endsWith(`Q580 ${ry + 174} 572 ${ry + 174}H${rx + 150}`)).toBe(true)
    else expect(numbers(outgoing).slice(-2)).toEqual([rx - 150, ry + 174])
    for (const connection of page.findAll('[data-connection]')) {
      const key = connection.attributes('data-connection')
      const [, rowY] = numbers(page.find(`[data-source-request="${key}"]`).attributes('transform'))
      const branch = connection.find('.source-branch').attributes('d')
      expect(numbers(branch).slice(0, 2)).toEqual([sx + 112 * ss, sy + rowY * ss])
      expect(numbers(branch).slice(-2)).toEqual(numbers(incoming).slice(0, 2))
      expect(connection.find('.request-signal').attributes('d')).toBe(branch + incoming.replace(/^M[\d.]+ [\d.]+/, ''))
      expect(connection.find('.charge-signal').attributes('d')).toBe(outgoing)
      for (const signal of connection.findAll('.request-signal, .charge-signal')) {
        expect(signal.attributes('d').match(/M/g)).toHaveLength(1)
        expect(signal.attributes('pathLength')).toBe('1')
      }
    }
  })

  it('recomposes all three objects within the phone canvas without rebuilding records or scaling moving paper', async () => {
    const page = render()
    const window = page.find('.window-face').element
    await page.setProps({ compact: true })
    expect(page.attributes('viewBox')).toBe('0 0 600 720')
    expect(page.attributes('preserveAspectRatio')).toBe('xMidYMid meet')
    expect(page.find('.window-face').element).toBe(window)
    expect(page.find('[data-detail="request-window"]').attributes('transform')).toBe('translate(150 152) scale(0.8)')
    expect(page.find('[data-detail="request-billing"]').attributes('transform')).toBe('translate(444 152) scale(0.8)')
    expect(page.find('[data-detail="usage-receipt"]').attributes('transform')).toBe('translate(300 510) scale(1)')
    expect(444 + 154 * .8).toBeLessThan(580) // Right-hand cable stays outside the calculation.
    expect(510 + 194).toBeLessThan(720)
    expect(page.find('.paper-sheet').attributes('transform')).toBeUndefined()
    for (const charge of page.findAll('.actual-meter')) expect(charge.attributes('x')).toBeUndefined()
  })

  it('prints one complete usage record at each feed, hiding future records inside the stationary printer', () => {
    const page = render()
    const clip = page.find('clipPath rect[height="326"]')
    const slotY = Number(clip.attributes('y')) + Number(clip.attributes('height'))
    expect(slotY).toBe(154)
    expect(clip.attributes('style')).toBeUndefined()
    const rows = page.findAll('[data-usage-record]')
    for (const [index, feed] of [234, 172, 110, 48].entries()) {
      expect(rows.filter(row => numbers(row.attributes('transform'))[1] + feed + 18 < slotY)).toHaveLength(index)
      if (index < 3) expect(numbers(rows[index].attributes('transform'))[1] + feed - 14).toBeGreaterThan(slotY)
      expect(frames('paper-feed')).toContain(`translateY(${feed}px)`)
    }
    expect(frames('paper-feed')).not.toContain('scale')
    expect(frames('paper-feed')).toContain('97% { opacity: 0; transform: translateY(0); }')
    expect(frames('paper-feed')).toContain('98%, 100% { opacity: 0; transform: translateY(320px); }')
    for (const [index, row] of rows.entries()) {
      const reference = row.attributes('clip-path').match(/url\(#([^)]+)\)/)![1]
      expect(page.find(`#${reference} .line-ink`).attributes('style')).toContain(`--request-delay: ${index * 4000}ms`)
      for (const field of ['request_id', 'model', 'usage', 'actual_cost']) expect(row.find(`[data-field="${field}"]`).exists()).toBe(true)
    }
  })

  it('orders each request through pricing, cache, sum, multiplier, delivery and printing on one clock', () => {
    expect(source).toContain('--trust-cycle: 16s')
    expect(frames('request-transit')).toContain('9%, 100% { opacity: 0; stroke-dashoffset: -1; }')
    expect(frames('price-lookup')).toContain('12%, 100% { opacity: 1; }')
    expect(frames('cache-partition')).toContain('14%, 100%')
    expect(frames('price-terms')).toContain('16%, 100% { transform: scaleX(1); }')
    expect(frames('sum-terms')).toContain('17%, 100% { transform: scaleX(1); }')
    expect(frames('apply-rate')).toContain('20%, 100% { transform: scaleX(var(--rate-scale)); }')
    expect(frames('charge-transit')).toContain('0%, 21% { opacity: 0; stroke-dashoffset: .12; }')
    expect(frames('charge-transit')).toContain('24%, 100% { opacity: 0; stroke-dashoffset: -1; }')
    expect(frames('line-print')).toContain('0%, 25% { transform: scaleX(0); }')
    expect(frames('line-print')).toContain('30%, 97% { transform: scaleX(1); }')
    expect(.30 * 16000 + 2 * 4000).toBeLessThan(.84 * 16000)
    // Consecutive calculation windows never overlap; the final one holds for audit.
    expect(frames('calculate-request')).toContain('0%, 9%, 28%, 100% { opacity: 0; }')
    expect(.28 * 16000).toBeLessThan(.09 * 16000 + 4000)
    expect(frames('calculate-final')).toContain('10%, 44% { opacity: 1; }')
  })

  it('isolates SVG resources and preserves native pause/static behavior without real account calls', () => {
    const mounts = [render(), render()]
    const ids = mounts.flatMap(page => page.findAll('[id]').map(node => node.attributes('id')))
    expect(new Set(ids).size).toBe(ids.length)
    for (const page of mounts) {
      for (const node of page.findAll('[fill], [clip-path], use')) {
        const reference = `${node.attributes('fill')} ${node.attributes('clip-path')}`.match(/url\(#([^)]+)\)/)?.[1] ?? node.attributes('href')?.slice(1)
        if (reference) expect(page.find(`#${reference}`).exists()).toBe(true)
      }
      expect(page.html()).not.toMatch(/NaN|Infinity/)
    }
    expect(source).not.toMatch(/setInterval|setTimeout|requestAnimationFrame|fetch\(|https?:\/\/|<animate|<filter|<button/)
    expect(carouselSource).toContain('.feature-frame:not(.is-current) :deep(.feature-scene *)')
    expect(carouselSource).toContain('animation-play-state: paused !important')
    expect(source).toContain('@media (prefers-reduced-motion: reduce)')
    expect(source).toContain('.billing-run:not(:last-child) { display: none; }')
  })

  it('explains the per-request calculation and one-to-one usage log in both accessible summaries', () => {
    expect(zh.home.landing.featureShowcase.billingStory.summary).toContain('一条请求对应一行记录')
    expect(zh.home.landing.featureShowcase.billingStory.summary).toContain('不代表真实请求')
    expect(en.home.landing.featureShowcase.billingStory.summary).toContain('One request corresponds to one receipt row')
    expect(en.home.landing.featureShowcase.billingStory.summary).toContain('not real requests')
  })
})
