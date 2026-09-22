import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import source from '../HomeIntelligenceScene.vue?raw'
import HomeIntelligenceScene from '../HomeIntelligenceScene.vue'

const pages: VueWrapper[] = []
function render(compact = false) {
  const page = mount(HomeIntelligenceScene, { props: { compact } })
  pages.push(page)
  return page
}
afterEach(() => pages.splice(0).forEach(page => page.unmount()))

describe('native intelligence system illustration', () => {
  it('keeps one system with code, constraints, dependencies, relationships and a structured result', () => {
    const page = render()
    expect(page.element.tagName).toBe('svg')
    expect(page.attributes('data-scene')).toBe('intelligence')
    expect(page.attributes('aria-hidden')).toBe('true')
    expect(page.findAll('[data-detail]').map(group => group.attributes('data-detail'))).toEqual([
      'context-links', 'input-code', 'input-constraints', 'input-dependencies', 'system-structure', 'resolved-structure'
    ])
    expect(page.findAll('.input-card')).toHaveLength(3)
    expect(page.findAll('.system-nodes rect')).toHaveLength(8)
    expect(page.findAll('.selected-nodes rect')).toHaveLength(3)
    expect(page.findAll('.solution-card')).toHaveLength(1)
    expect(page.find('.selected-route').attributes('d')).toBe(page.find('.route-signal').attributes('d'))
    expect(page.find('text, image, img, canvas, foreignObject, filter, .journey-shot, .flight-layer').exists()).toBe(false)
  })

  it('maintains a bounded vector scene with no particles, raster work or frame-level JavaScript', () => {
    const page = render()
    expect(page.findAll('*').length).toBeLessThan(150)
    expect(page.findAll('.input-card')).toHaveLength(3)
    expect(page.findAll('.route-signal')).toHaveLength(1)
    expect(source).not.toMatch(/feGaussianBlur|feTurbulence|will-change|requestAnimationFrame|setInterval|setTimeout|Math\.random/)
    expect(source).not.toMatch(/v-if|v-show/)
  })

  it('uses the same deterministic diagram and routes for each instance', () => {
    const first = render()
    const second = render()
    expect(first.findAll('path').map(path => path.attributes('d'))).toEqual(second.findAll('path').map(path => path.attributes('d')))
    expect(first.html()).not.toMatch(/NaN|Infinity/)
    expect(first.find('.selected-route').attributes('d')).toMatch(/^M254 125.*H801H927$/)
    expect(first.find('.output-link').attributes('d')).toBe('M801 250H927')
    expect(first.find('[data-detail="system-structure"]').attributes('transform')).toBe('translate(622 245)')
  })

  it('keeps gradient and pattern references unique and local', () => {
    const first = render()
    const second = render()
    const ids = [...first.findAll('[id]'), ...second.findAll('[id]')].map(element => element.attributes('id'))
    expect(new Set(ids).size).toBe(ids.length)
    for (const page of [first, second]) {
      for (const element of page.findAll('[fill], [stroke]')) {
        const references = `${element.attributes('fill')} ${element.attributes('stroke')}`.matchAll(/url\(#([^)]+)\)/g)
        for (const [, reference] of references) expect(page.find(`#${reference}`).exists()).toBe(true)
      }
    }
    expect(source).toContain('var(--cafe-page, #faf7f2)')
    expect(source).toContain('var(--cafe-accent, #865630)')
  })

  it('reflows inputs above the structure and result below it on mobile without cropping', async () => {
    const page = render()
    const count = page.findAll('*').length
    expect(page.attributes('viewBox')).toBe('0 0 1200 520')
    expect(page.attributes('preserveAspectRatio')).toBe('xMidYMid meet')
    const system = page.find('[data-detail="system-structure"]').element
    await page.setProps({ compact: true })
    expect(page.attributes('viewBox')).toBe('0 0 600 720')
    expect(page.find('[data-detail="input-code"]').attributes('transform')).toBe('translate(105 120)')
    expect(page.find('[data-detail="input-constraints"]').attributes('transform')).toBe('translate(300 120)')
    expect(page.find('[data-detail="input-dependencies"]').attributes('transform')).toBe('translate(495 120)')
    expect(page.find('[data-detail="system-structure"]').attributes('transform')).toBe('translate(300 345)')
    expect(page.find('[data-detail="resolved-structure"]').attributes('transform')).toBe('translate(300 575)')
    expect(page.find('.selected-route').attributes('d')).toMatch(/^M105 170.*V291H252V326Q252 350 272 350H479.*V503$/)
    expect(page.find('.output-link').attributes('d')).toMatch(/^M479 350.*V503$/)
    expect(page.findAll('*')).toHaveLength(count)
    expect(page.find('[data-detail="system-structure"]').element).toBe(system)
  })

  it('keeps the main stages synchronized while local detail rhythms continue between stages', () => {
    const animations = source.match(/animation: [^;]+;/g) ?? []
    const active = animations.filter(animation => !animation.includes('none'))
    const detailNames = ['source-flow', 'association-flow', 'candidate-flow', 'node-spark', 'core-echo', 'core-orbit']
    expect(active.every(animation => animation.includes('infinite'))).toBe(true)
    expect(active.filter(animation => !detailNames.some(name => animation.includes(name))).every(animation => animation.includes('var(--intelligence-duration, 14s)'))).toBe(true)
    for (const animation of ['input-emphasis', 'core-focus', 'route-resolve', 'result-focus', 'solution-arrive', 'check-resolve']) {
      expect(source).toContain(`@keyframes ${animation}`)
    }
    expect(source).toContain('animation: signal-travel')
    expect(source).toContain('94% { stroke-dashoffset: -100; opacity: .8; }')
    expect(source).toContain('@media (prefers-reduced-motion: reduce)')
  })

  it.each([false, true])('keeps one continuous input-to-result highlight attached to the ports (compact=%s)', compact => {
    const page = render(compact)
    const route = page.find('.selected-route')
    const d = route.attributes('d')
    const inputLink = page.find('.context-links path').attributes('d')
    const outputTail = page.find('.output-link').attributes('d').replace(/^M[\d.]+ [\d.]+/, '')
    expect(d.startsWith(inputLink)).toBe(true)
    expect(d.endsWith(outputTail)).toBe(true)
    expect(d.match(/[Mm]/g)).toHaveLength(1)
    expect(route.element.parentElement).toBe(page.element)
    expect(page.find('.route-signal').element.parentElement).toBe(page.element)
    expect(page.find('.route-signal').attributes('d')).toBe(d)
    expect(page.find('.route-signal').attributes('pathLength')).toBe('100')
    expect(page.find('.structure-routes').attributes('d')).toContain('M-28 5H179')
    expect(page.find('.relation-focus rect').exists()).toBe(false) // No opaque tile hiding the middle of the route.

    const codePosition = page.find('[data-detail="input-code"]').attributes('transform').match(/-?\d+/g)!.map(Number)
    const outputPosition = page.find('[data-detail="resolved-structure"]').attributes('transform').match(/-?\d+/g)!.map(Number)
    expect(d.startsWith(compact ? `M${codePosition[0]} ${codePosition[1] + 50}` : `M${codePosition[0] + 84} ${codePosition[1]}`)).toBe(true)
    expect(d.endsWith(compact ? `V${outputPosition[1] - 72}` : `H${outputPosition[0] - 98}`)).toBe(true)
    if (!compact) expect(page.find('.output-link').attributes('d')).toContain(` ${outputPosition[1]}H`)
  })

  it('keeps connected geometry anchored while only emphasis and non-wired focus ornaments move', () => {
    const page = render()
    expect(source).not.toMatch(/system-perspective|relation-lift|layer-separate|input-gather|transform: translate/)
    expect(page.find('.focus-aura').exists()).toBe(true)
    expect(page.findAll('.input-0 .input-outline')).toHaveLength(1)
    expect(page.findAll('.solution-outline')).toHaveLength(1)
    expect(source).toContain('.context-links, .output-link { opacity: .3; }')
    expect(source).toContain('.structure-routes { opacity: .32; }')
    expect(source).toContain('.secondary-routes { opacity: .13; }')
    // Backbone geometry does not animate; the denser association layer is gated
    // to its own phase and recedes before the single resolved route takes focus.
    for (const selector of ['context-links', 'output-link', 'structure-routes', 'secondary-routes']) {
      expect(page.find(`.${selector}`).attributes('pathLength')).toBeUndefined()
    }
  })

  it('unfolds multiple shared-junction associations instead of revealing the final answer immediately', () => {
    const page = render()
    const branches = page.findAll('.association-branch')
    expect(branches).toHaveLength(3)
    expect(page.findAll('.association-nodes circle')).toHaveLength(12)
    expect(page.find('.association-bridges').attributes('d').match(/M/g)).toHaveLength(4)
    for (const branch of branches) {
      const route = branch.find('.association-trace').attributes('d')
      expect(branch.find('.association-signal').attributes('d')).toBe(route)
      expect(route.match(/M/g)).toHaveLength(1)
      expect(route.match(/C/g)!.length).toBeGreaterThanOrEqual(7)
      expect(route.endsWith('134 5')).toBe(true)
    }
    const junctions = new Set(page.findAll('.association-nodes circle').map(node => `${node.attributes('cx')}:${node.attributes('cy')}`))
    expect(junctions.size).toBe(12)
    expect(source).toContain('0%, 52%, 100% { opacity: 0; }')
    expect(source).toContain('0%, 50% { stroke-dashoffset: 100; opacity: 0; }')
    expect(source).toContain('65% { opacity: .14; }')
    expect(page.findAll('.input-content')).toHaveLength(3)
    expect(page.find('.solution-tree').attributes('pathLength')).toBe('100')
    expect(page.findAll('.candidate-card')).toHaveLength(2)
    expect(source).toContain('0%, 24%, 63%, 100% { opacity: 0; }')
  })

  it.each([false, true])('feeds all three source streams into actual network junctions (compact=%s)', compact => {
    const page = render(compact)
    const base = page.findAll('.context-links path')
    const streams = page.findAll('.source-signal')
    expect(streams).toHaveLength(3)
    for (const [index, stream] of streams.entries()) {
      const d = stream.attributes('d')
      expect(d.startsWith(base[index].attributes('d'))).toBe(true)
      expect(d.match(/M/g)).toHaveLength(1)
      expect(d.endsWith(compact ? 'V291' : 'H492')).toBe(true)
      expect(stream.attributes('pathLength')).toBe('100')
      expect(stream.attributes('style')).toContain('--flow-delay')
    }
    const alternatives = page.findAll('.candidate-link')
    const cards = page.findAll('.candidate-card')
    const highlights = page.findAll('.candidate-signal')
    for (const [index, route] of alternatives.entries()) {
      const position = cards[index].attributes('transform').match(/-?\d+/g)!.map(Number)
      const d = route.attributes('d')
      expect(d.startsWith(compact ? 'M479 350' : 'M801 250')).toBe(true)
      expect(d.endsWith(compact ? `V${position[1] - 33}` : `H${position[0] - 52}`)).toBe(true)
      expect(highlights[index].attributes('d')).toBe(d)
    }
  })
})
