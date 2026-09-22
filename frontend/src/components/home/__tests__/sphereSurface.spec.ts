import { describe, expect, it } from 'vitest'
import { geoPath, type GeoGeometryObjects } from 'd3-geo'
import { createLunarCraterProjector, createSphereProjector, orthographicView } from '../sphereSurface'

const patch: GeoGeometryObjects = {
  type: 'Polygon', coordinates: [[[-6, -6], [-6, 6], [6, 6], [6, -6], [-6, -6]]]
}
describe('spherical surface rotation', () => {
  it('compresses a surface feature toward the limb instead of rotating it flat', () => {
    const view = orthographicView()
    const path = geoPath(view)
    view.rotate([0, 0, 0])
    const center = path.bounds(patch)
    view.rotate([70, 0, 0])
    const limb = path.bounds(patch)
    expect(limb[1][0] - limb[0][0]).toBeLessThan((center[1][0] - center[0][0]) * .4)
    expect(limb[1][1] - limb[0][1]).toBeCloseTo(center[1][1] - center[0][1], 1)
    expect(limb[0][0]).toBeGreaterThan(85)
  })

  it('hides the rear hemisphere, and brings it back after a full turn', () => {
    const view = orthographicView(), path = geoPath(view)
    const initial = path(patch)
    view.rotate([180, 0, 0])
    expect(path(patch)).toBeNull()
    view.rotate([360, 0, 0])
    expect(path(patch)).toBe(initial)
  })

  it('keeps Earth clouds in small separated fragments instead of globe-spanning bands', () => {
    const render = createSphereProjector('terra')
    for (const angle of [0, 90, 180, 270]) {
      const layers = render(angle)
      expect(layers).toHaveLength(3) // land, thin veil, denser cloud fragments
      for (const layer of layers.slice(1)) {
        const fragments = layer.split('M').filter(Boolean)
        expect(fragments.length).toBeGreaterThan(8)
        for (const fragment of fragments) {
          const coordinates = fragment.match(/-?\d+(?:\.\d+)?/g)!.map(Number)
          const xs = coordinates.filter((_, i) => i % 2 === 0)
          expect(Math.max(...xs) - Math.min(...xs)).toBeLessThan(45)
        }
      }
    }
  })

  it('keeps a few large recognizable lunar craters without extra surface layers', () => {
    const render = createSphereProjector('luna')
    for (const angle of [0, 90, 180, 270]) {
      const layers = render(angle)
      expect(layers).toHaveLength(3)
      for (const layer of layers) {
        const count = layer.split('M').filter(Boolean).length
        expect(count).toBeGreaterThan(2)
        expect(count).toBeLessThan(10)
      }
    }
    expect(render(20).slice(1)).not.toEqual(render(0).slice(1))
  })

  it('projects individual crater bowls for local lighting and clips their hidden faces', () => {
    const render = createLunarCraterProjector()
    const front = render(0)
    expect(front).toHaveLength(8)
    expect(front.some(crater => !crater.bowl)).toBe(true)
    expect(front.filter(crater => crater.bowl).length).toBeGreaterThan(2)
    expect(front[0].terrace).toBeTruthy()
    expect(front[0].floor).not.toBe(front[0].bowl)
    expect(front[0].peakLight).toBeTruthy()
    expect(front[0].peakShadow).toBeTruthy()
    expect(front[2].peakLight).toBe('') // small pits do not all get a central peak
    expect(front[2].peakShadow).toBe('')
    for (const crater of front.filter(c => c.bowl && c.wall && c.rim)) {
      expect(crater.bowl).not.toBe(crater.wall)
      expect(crater.rim).not.toBe(crater.wall)
    }
    expect(render(35)).not.toEqual(front)
    expect(render(360)).toEqual(front)
    for (const angle of [0, 90, 180, 270]) {
      for (const crater of render(angle)) {
        for (const layer of Object.values(crater)) {
          expect(layer).not.toMatch(/NaN|Infinity/)
          const coordinates = layer.match(/-?\d+(?:\.\d+)?/g)?.map(Number) ?? []
          expect(coordinates.every(n => Math.abs(n) <= 100)).toBe(true)
        }
      }
    }
  })

  it.each(['luna', 'terra', 'sol'] as const)('projects %s at different longitudes with a fixed radius', kind => {
    const render = createSphereProjector(kind)
    const first = render(0), next = render(35)
    expect(next).not.toEqual(first)
    for (const angle of [0, 90, 180, 270]) {
      for (const path of render(angle)) {
        expect(path).not.toMatch(/NaN|Infinity/)
        const values = path.match(/-?\d+(?:\.\d+)?/g)!.map(Number)
        expect(Math.max(...values.map(Math.abs))).toBeLessThanOrEqual(100)
      }
    }
  })
})
