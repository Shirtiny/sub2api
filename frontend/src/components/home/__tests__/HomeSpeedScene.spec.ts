import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import HomeSpeedScene from '../HomeSpeedScene.vue'
import source from '../HomeSpeedScene.vue?raw'
import geometrySource from '../speedGlobe.ts?raw'
import { speedGlobe } from '../speedGlobe'
const pages: VueWrapper[] = []
function render(compact = false) {
  const page = mount(HomeSpeedScene, { props: { compact } })
  pages.push(page)
  return page
}
afterEach(() => pages.splice(0).forEach(page => page.unmount()))

describe('global speed illustration', () => {
  it('shows a geographic globe, worldwide routes, three optimized lanes and a streaming first byte', () => {
    const page = render()
    expect(page.attributes('data-scene')).toBe('speed')
    expect(page.attributes('aria-hidden')).toBe('true')
    expect(page.findAll('[data-detail]').map(group => group.attributes('data-detail'))).toEqual(expect.arrayContaining([
      'global-network', 'world-routes', 'optimized-lines', 'netherlands-server', 'streaming-response', 'first-byte', 'terminal-messages', 'terminal-editor'
    ]))
    expect(page.findAll('.world-route')).toHaveLength(7)
    expect(page.findAll('.express-lane')).toHaveLength(3)
    expect(page.findAll('.reply-chunk')).toHaveLength(36)
    expect(page.find('.first-byte').exists()).toBe(true)
    expect(page.find('image, foreignObject, canvas, filter').exists()).toBe(false)
    expect(page.text()).not.toMatch(/0\.3|99\.9|100%|¥|\$/)
    expect(page.findAll('*').length).toBeLessThan(250)
    expect(page.find('.world-land').attributes('d').length).toBeGreaterThan(1000)
    expect(page.find('.world-graticule').attributes('d').length).toBeGreaterThan(100)
  })

  it('projects raised great circles with exact on-sphere endpoints and no hidden rear-side routes', () => {
    expect(speedGlobe.routes).toHaveLength(7)
    expect(speedGlobe.server.country).toBe('NL')
    expect(speedGlobe.server.coordinate).toEqual([5.3, 52.2])
    for (const route of speedGlobe.routes) {
      expect(route.path.match(/M/g)).toHaveLength(1)
      expect(route.points).toHaveLength(49)
      expect(route.points[0].x).toBeCloseTo(route.origin.x, 8)
      expect(route.points[0].y).toBeCloseTo(route.origin.y, 8)
      expect(Math.hypot(route.origin.x, route.origin.y, route.origin.z)).toBeCloseTo(speedGlobe.radius, 8)
      const last = route.points.at(-1)!
      expect(route.destination).toBe('NL')
      expect(last.x).toBeCloseTo(speedGlobe.server.x, 8)
      expect(last.y).toBeCloseTo(speedGlobe.server.y, 8)
      for (const point of route.points) {
        expect(point.z).toBeGreaterThan(0)
        expect(Math.hypot(point.x, point.y, point.z)).toBeGreaterThanOrEqual(speedGlobe.radius - 1e-8)
        expect(Math.hypot(point.x, point.y, point.z)).toBeLessThanOrEqual(speedGlobe.radius * 1.2)
      }
      expect(Math.hypot(route.points[24].x, route.points[24].y, route.points[24].z)).toBeGreaterThan(speedGlobe.radius * 1.09)
    }
  })

  it.each([false, true])('connects the terminal client directly to the Netherlands with no onward hop (compact=%s)', compact => {
    const page = render(compact)
    const position = page.find('[data-detail="global-network"]').attributes('transform').match(/-?[\d.]+/g)!.map(Number)
    const response = page.find('[data-detail="streaming-response"]').attributes('transform').match(/-?[\d.]+/g)!.map(Number)
    expect(page.find('.server-node').attributes('transform')).toBe(`translate(${position[0] + speedGlobe.server.x} ${position[1] + speedGlobe.server.y})`)
    for (const [index, lane] of page.findAll('.express-lane').entries()) {
      const d = lane.find('.express-track').attributes('d')
      const port = lane.find('.client-port').attributes('transform').match(/-?[\d.]+/g)!.map(Number)
      expect(d.startsWith(`M${port[0]} ${port[1]}`)).toBe(true)
      expect(d.endsWith(`${position[0] + speedGlobe.server.x} ${position[1] + speedGlobe.server.y}`)).toBe(true)
      expect(lane.attributes('data-destination')).toBe('NL')
      expect(d.match(/M/g)).toHaveLength(1)
      expect(port[0]).toBe(compact ? response[0] + (index - 1) * 64 : response[0] + 206)
      expect(port[1]).toBe(compact ? response[1] + 122 : response[1] + (index - 1) * 34)
      if (index === 1) {
        expect(lane.attributes('data-primary')).toBe('true')
        expect(lane.find('.delivery-packet').attributes('d')).toBe(d)
        expect(lane.find('.response-packet').attributes('d')).toBe(d)
      } else {
        expect(lane.find('.delivery-packet, .response-packet, .direction-cue').exists()).toBe(false)
      }
    }
  })

  it('places the globe on the right, then follows the same client-first order vertically on mobile', async () => {
    const page = render()
    const globe = page.find('.world-land').element
    const geography = page.find('.world-land').attributes('d')
    const count = page.findAll('*').length
    expect(page.attributes('viewBox')).toBe('0 0 1200 520')
    expect(page.find('[data-detail="global-network"]').attributes('transform')).toBe('translate(900 258)')
    expect(page.find('[data-detail="streaming-response"]').attributes('transform')).toBe('translate(256 258)')
    await page.setProps({ compact: true })
    expect(page.attributes('viewBox')).toBe('0 0 600 720')
    expect(page.attributes('preserveAspectRatio')).toBe('xMidYMid meet')
    expect(page.find('[data-detail="global-network"]').attributes('transform')).toBe('translate(300 495)')
    expect(page.find('[data-detail="streaming-response"]').attributes('transform')).toBe('translate(300 136)')
    expect(page.find('.world-land').element).toBe(globe)
    expect(page.find('.world-land').attributes('d')).toBe(geography)
    expect(page.findAll('*')).toHaveLength(count)
  })

  it('keeps all gradient, pattern, mask and clip references local to each component', () => {
    const pages = [render(), render()]
    const ids = pages.flatMap(page => page.findAll('[id]').map(node => node.attributes('id')))
    expect(new Set(ids).size).toBe(ids.length)
    for (const page of pages) {
      for (const node of page.findAll('[fill], [clip-path], [mask]')) {
        const references = `${node.attributes('fill')} ${node.attributes('clip-path')} ${node.attributes('mask')}`.matchAll(/url\(#([^)]+)\)/g)
        for (const [, reference] of references) expect(page.find(`#${reference}`).exists()).toBe(true)
      }
      expect(page.html()).not.toMatch(/NaN|Infinity/)
    }
  })

  it('keeps all seven geographic round trips animated independently of the foreground client', () => {
    const page = render()
    expect(page.findAll('.world-packet')).toHaveLength(14)
    const routes = page.findAll('.world-route')
    expect(new Set(routes.map(route => route.attributes('style'))).size).toBe(7)
    for (const route of routes) {
      expect(route.attributes('data-destination')).toBe('NL')
      expect(route.find('.world-request').attributes('d')).toBe(route.find('.world-return').attributes('d'))
      expect(route.find('.port-echo').exists()).toBe(true)
    }
    expect(page.findAll('.server-node')).toHaveLength(1)
    expect(page.find('.server-node').attributes('data-country')).toBe('NL')
    expect(page.find('.server-label').text()).toBe('Server')
    expect(source).toContain('world-request var(--network-cycle) var(--route-delay)')
    expect(source).toContain('world-return var(--network-cycle) var(--route-delay)')
    expect(source).not.toContain('activeRoute')
    expect(source).toContain('38% { stroke-dashoffset: -100; opacity: .6; }')
    expect(source).toContain('80% { stroke-dashoffset: 0; opacity: .45; }')
  })

  it.each([false, true])('keeps the terminal layout and scrolls a long task inside its message viewport (compact=%s)', compact => {
    const page = render(compact)
    const client = page.find('[data-client="terminal"]')
    const children = [...client.element.children]
    expect(children.indexOf(client.find('.terminal-messages').element)).toBeLessThan(children.indexOf(client.find('.terminal-editor').element))
    expect(children.at(-1)).toBe(client.find('.terminal-editor').element)
    const viewportId = client.find('.terminal-messages').attributes('clip-path').match(/url\(#([^)]+)\)/)![1]
    const viewport = page.find(`#${viewportId} rect`)
    const top = Number(viewport.attributes('y'))
    const bottom = top + Number(viewport.attributes('height'))
    const scroll = Number(client.attributes('style').match(/--scroll-reply-end: ([\d.]+)px/)![1])
    for (const piece of client.findAll('.reply-piece[data-batch="1"]')) {
      const [x, y] = piece.attributes('transform').match(/-?[\d.]+/g)!.map(Number)
      expect(y - scroll - 2).toBeGreaterThan(top)
      expect(y - scroll + 2).toBeLessThan(bottom)
      expect(x + Number(piece.find('rect').attributes('width'))).toBeLessThan(Number(viewport.attributes('x')) + Number(viewport.attributes('width')))
    }
    expect(Number(client.find('.response-shell').attributes('width'))).toBeGreaterThan(Number(client.find('.response-shell').attributes('height')))
    expect(client.find('[data-state="working"]').exists()).toBe(true)
    expect(client.findAll('.tool-check')).toHaveLength(3)
    expect(client.find('.terminal-footer, .task-progress, .task-complete, [data-detail="task-progress"]').exists()).toBe(false)
    expect(client.find('.tool-progress').exists()).toBe(true)
    expect(source).toContain('46%, 58% { transform: translateY(calc(-1 * var(--scroll-tool))); }')
    expect(source).toContain('75%, 98% { transform: translateY(calc(-1 * var(--scroll-reply-end))); }')
  })

  it('has only the Server label, no product branding, server glyph or example copy', () => {
    const page = render()
    expect(page.findAll('text').map(text => text.text())).toEqual(['Server'])
    expect(page.find('[data-client="terminal"]').text()).toBe('')
    expect(page.find('.server-node').findAll('path')).toHaveLength(1)
    expect(page.find('.server-node path').classes()).toContain('server-leader')
    expect(page.find('.pi-mark, .terminal-brand, .thinking-label, .received-state').exists()).toBe(false)
    expect(source).not.toMatch(/useI18n|piTiles|replyKeys|ASTRA|Thinking|思考中/)
    expect(page.findAll('.thinking-dot')).toHaveLength(3)
  })

  it('moves one input from editor to history without an opacity swap or a one-frame duplicate', () => {
    const page = render()
    expect(page.findAll('.request-ink')).toHaveLength(1)
    expect(page.findAll('.request-transfer')).toHaveLength(1)
    expect(page.findAll('.request-ink rect')).toHaveLength(6)
    expect(page.find('.editor-request, .request-prompt').exists()).toBe(false)
    const input = page.find('.request-transfer').element
    expect(input.closest('.request-stage')?.hasAttribute('clip-path')).toBe(true)
    expect(source).toContain('1%, 8% { opacity: 1; transform: translateY(var(--editor-shift)); }')
    expect(source).toContain('10%, 93% { opacity: 1; transform: translateY(0); }')
    expect(source).toContain('4%, 100% { transform: scaleX(1); }')
    expect(source).toContain('.input-reveal { transform-origin: 0 0; transform: scaleX(0);')
    expect(source).toContain('.request-transfer { opacity: 0; animation: request-transfer var(--speed-cycle) ease-in-out infinite both; }')
  })

  it.each([false, true])('advances one unbroken current from server to client (compact=%s)', compact => {
    const page = render(compact)
    const stream = page.find('.stream-connection')
    const front = page.find('.stream-front')
    const surface = stream.find('.stream-surface')
    const maskId = stream.attributes('mask').match(/url\(#([^)]+)\)/)![1]
    const mask = page.find(`#${maskId}`)
    const gradientId = surface.attributes('fill').match(/url\(#([^)]+)\)/)![1]
    const gradient = page.find(`#${gradientId}`)
    expect(page.findAll('.stream-connection')).toHaveLength(1)
    expect(page.findAll('.stream-front')).toHaveLength(1)
    expect(page.find('.stream-packet').exists()).toBe(false)
    expect(source).not.toMatch(/chunk-flight|stream-open/)
    expect(front.element.parentElement).toBe(mask.element)
    expect(front.attributes('d')).toBe(page.find('.express-lane[data-primary="true"] .express-track').attributes('d'))
    expect(front.attributes('pathLength')).toBe('100')
    expect(front.attributes('stroke-linecap')).toBe('butt')
    expect(source).toContain('stroke-dasharray: 100 200; stroke-dashoffset: -100; animation: stream-travel var(--speed-cycle) linear infinite both;')
    // Surface brightness moves without ever cutting holes in the fluid body.
    const opacity = gradient.findAll('stop').map(stop => Number(stop.attributes('stop-opacity')))
    expect(Math.min(...opacity)).toBeGreaterThanOrEqual(.5)
    expect(opacity[0]).toBe(opacity.at(-1))
    expect(gradient.attributes('spreadMethod')).toBe('repeat')
    const period = Number(gradient.attributes(compact ? 'y2' : 'x2'))
    const flowX = Number(surface.attributes('style').match(/--flow-x: (-?[\d.]+)px/)![1])
    const flowY = Number(surface.attributes('style').match(/--flow-y: (-?[\d.]+)px/)![1])
    expect(flowX).toBe(compact ? 0 : -period)
    expect(flowY).toBe(compact ? -period : 0)
    expect(Number(surface.attributes('width')) + flowX).toBeCloseTo(Number(mask.attributes('width')))
    expect(Number(surface.attributes('height')) + flowY).toBeCloseTo(Number(mask.attributes('height')))
    expect(surface.attributes('x')).toBe(mask.attributes('x'))
    expect(surface.attributes('y')).toBe(mask.attributes('y'))
    expect(source).toContain('stream-current 3.2s linear infinite')
  })

  it('fills directionally before output, keeps flowing, then drains without a backwards reset', () => {
    const page = render()
    const travel = source.match(/@keyframes stream-travel \{([\s\S]*?)\n\}/)![1]
    expect(travel).not.toContain('opacity')
    const stops = [...travel.matchAll(/([\d.%, ]+)\{ stroke-dashoffset: (-?\d+); \}/g)]
      .flatMap(([, times, value]) => [...times.matchAll(/([\d.]+)%/g)].map(time => ({ time: Number(time[1]), value: Number(value) })))
      .sort((a, b) => a.time - b.time)
    const visibleAt = (time: number) => {
      const end = stops.findIndex(stop => stop.time >= time)
      const a = stops[Math.max(0, end - 1)]!, b = stops[end]!
      const offset = a.time === b.time ? a.value : a.value + (b.value - a.value) * (time - a.time) / (b.time - a.time)
      // Sample the 100-unit path's actual dash pattern, not a separate clock.
      return Array.from({ length: 100 }, (_, index) => index + .5)
        .filter(position => ((position + offset) % 300 + 300) % 300 < 100)
    }
    for (const [frontAt, arrivalAt, sustainedAt, tailAt] of [[22.5, 25, 37, 40], [60, 62, 75, 77.5]]) {
      const front = visibleAt(frontAt)
      expect(front).toHaveLength(50)
      expect(front[0]).toBe(50.5) // Starts at the server end (100).
      expect(front.at(-1)).toBe(99.5)
      expect(visibleAt(arrivalAt)).toHaveLength(100)
      expect(visibleAt(sustainedAt)).toHaveLength(100)
      const tail = visibleAt(tailAt)
      expect(tail).toHaveLength(50)
      expect(tail[0]).toBe(.5) // Last fluid drains into the client (0).
      expect(tail.at(-1)).toBe(49.5)
    }
    for (const time of [0, 20, 43, 44, 50, 57, 58, 80, 90, 100]) expect(visibleAt(time)).toHaveLength(0)
    const pieces = page.findAll('.reply-piece')
    expect(pieces).toHaveLength(36)
    const delays = pieces.map(piece => Number(piece.attributes('style').match(/--chunk-delay: ([\d.]+)ms/)![1]))
    for (const [index, piece] of pieces.entries()) {
      expect(piece.attributes('data-batch')).toBe(String(index < 18 ? 0 : 1))
      expect(delays[index]).toBe((index < 18 ? 0 : 5920) + index % 18 * 155)
    }
    const cycle = Number(source.match(/--speed-cycle: ([\d.]+)s/)![1]) * 1000
    // Output starts on first arrival and finishes before the tail reaches the client.
    expect(cycle * .25 + delays[0]).toBe(cycle * .25)
    expect(cycle * .2575 + delays[17]).toBeLessThan(cycle * .425)
    expect(cycle * .25 + delays[18]).toBe(cycle * .62)
    expect(cycle * .2575 + delays[35]).toBeLessThan(cycle * .795)
    // Slow generation/current, not the initial request or first-byte receipt.
    expect((stops.find(stop => stop.value === 0)!.time - stops.filter(stop => stop.value === -100).at(-1)!.time) / 100 * cycle).toBe(800)
    expect(delays[1] - delays[0]).toBe(155)
    expect(source).toContain('13%, 58% { stroke-dashoffset: -100; opacity: 1; }')
    expect(source).toContain('16% { stroke-dashoffset: 0; opacity: 1; }')
  })

  it('receives before thinking, keeps working through a long task and loops with CSS only', () => {
    const page = render()
    const animations = source.match(/animation: [^;]+;/g) ?? []
    expect(animations.filter(animation => !animation.includes('none')).every(animation => animation.includes('infinite'))).toBe(true)
    expect(source).toContain('--speed-cycle: 16s')
    expect(page.findAll('.delivery-packet')).toHaveLength(1)
    expect(page.findAll('.response-packet')).toHaveLength(1)
    expect(source).toContain('16% { stroke-dashoffset: 0; opacity: 1; }')
    expect(source).toContain('18%, 22% { opacity: 1; }')
    expect(source).toContain('25%, 37.5% { stroke-dashoffset: 0; }')
    expect(source).toContain('43%, 93% { opacity: 1; }')
    expect(source).toContain('57%, 100% { transform: scaleX(1); opacity: .5; }')
    expect(source).toContain('@media (prefers-reduced-motion: reduce)')
    expect(source).toContain('.reply-chunk { transform: scaleX(1); opacity: .58; }')
    expect(source + geometrySource).not.toMatch(/requestAnimationFrame|useRafFn|setInterval|setTimeout|feGaussianBlur|Math\.random|rotate\(360deg\)/)
    expect(geometrySource).toContain('geoInterpolate')
    expect(geometrySource).toContain("@/assets/home/earth-geography.json")
  })
})
