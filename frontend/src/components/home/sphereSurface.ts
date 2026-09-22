import { geoArea, geoOrthographic, geoPath, type GeoGeometryObjects } from 'd3-geo'
import geography from '@/assets/home/earth-geography.json'

export type SphereKind = 'luna' | 'terra' | 'sol'
type Polygon = { type: 'Polygon'; coordinates: number[][][] }
const radians = Math.PI / 180

// Small, irregular geographic patches. These are surface coordinates, not screen paths.
function patch(lon: number, lat: number, width: number, height: number, seed: number, tilt = 0): Polygon {
  const ring = Array.from({ length: 24 }, (_, i) => {
    const a = i / 24 * Math.PI * 2
    const edge = 1 + .14 * Math.sin(a * 3 + seed) + .09 * Math.cos(a * 5 - seed)
    const x = Math.cos(a) * width * edge, y = Math.sin(a) * height * edge
    const turn = tilt * radians
    return [((lon + x * Math.cos(turn) - y * Math.sin(turn) + 540) % 360) - 180,
      Math.max(-86, Math.min(86, lat + x * Math.sin(turn) + y * Math.cos(turn))) ]
  })
  ring.push([...ring[0]])
  const polygon: Polygon = { type: 'Polygon', coordinates: [ring] }
  if (geoArea(polygon) > Math.PI * 2) ring.reverse()
  return polygon
}
function collection(polygons: Polygon[]): GeoGeometryObjects {
  return { type: 'MultiPolygon', coordinates: polygons.map(p => p.coordinates) }
}
// A handful of large landmark craters makes the small moon immediately legible.
// Distribute them around the sphere so each view stays sparse as it turns.
const craters = [
  { lon: -28, lat: 28, radius: 16 },
  { lon: 15, lat: -20, radius: 12 },
  { lon: 35, lat: 38, radius: 9 },
  { lon: -72, lat: -22, radius: 10 },
  { lon: -130, lat: 30, radius: 14 },
  { lon: -170, lat: -25, radius: 12 },
  { lon: 125, lat: 20, radius: 15 },
  { lon: 82, lat: -40, radius: 10 }
]
function craterPoint(lon: number, lat: number, radius: number, bearing: number) {
  const phi = lat * radians, r = radius * radians, b = bearing * radians
  const latitude = Math.asin(Math.sin(phi) * Math.cos(r) + Math.cos(phi) * Math.sin(r) * Math.cos(b))
  const longitude = lon * radians + Math.atan2(Math.sin(b) * Math.sin(r) * Math.cos(phi), Math.cos(r) - Math.sin(phi) * Math.sin(latitude))
  return [((longitude / radians + 540) % 360) - 180, latitude / radians]
}
function craterPolygon(points: number[][]): Polygon {
  const ring = [...points, points[0]]
  const polygon: Polygon = { type: 'Polygon', coordinates: [ring] }
  if (geoArea(polygon) > Math.PI * 2) ring.reverse()
  return polygon
}
// Break the machined-circle silhouette without adding noisy surface detail.
function craterProfile(c: typeof craters[number], bearing: number) {
  const a = bearing * radians, seed = c.lon * .07
  return 1 + .055 * Math.sin(a * 3 + seed) + .032 * Math.cos(a * 5 - seed) + .018 * Math.sin(a * 11 + seed)
}
function craterOutline(c: typeof craters[number], scale: number): Polygon {
  return craterPolygon(Array.from({ length: 64 }, (_, i) => {
    const bearing = i / 64 * 360
    return craterPoint(c.lon, c.lat, c.radius * scale * craterProfile(c, bearing), bearing)
  }))
}
function craterRim(c: typeof craters[number], start: number, width: number, scale = 1): Polygon {
  const outer = Array.from({ length: 33 }, (_, i) => {
    const bearing = start + i * 4.5
    return craterPoint(c.lon, c.lat, c.radius * scale * craterProfile(c, bearing), bearing)
  })
  const inner = Array.from({ length: 33 }, (_, i) => {
    const t = (32 - i) / 32, bearing = start + t * 144
    const broken = 1 - .62 * Math.max(0, Math.sin(t * Math.PI * 9 + c.lat))
    const thickness = width * Math.pow(Math.sin(t * Math.PI), .8) * broken
    return craterPoint(c.lon, c.lat, c.radius * scale * (craterProfile(c, bearing) - thickness), bearing)
  })
  return craterPolygon([...outer, ...inner])
}
const lunarCraterShapes = craters.map(c => {
  const peakTop = craterPoint(c.lon, c.lat, c.radius * .18, 340)
  const peakLeft = craterPoint(c.lon, c.lat, c.radius * .23, 245)
  const peakRight = craterPoint(c.lon, c.lat, c.radius * .23, 125)
  const peakBase = craterPoint(c.lon, c.lat, c.radius * .12, 180)
  return {
    bowl: craterOutline(c, .96),
    floor: craterOutline(c, .5),
    wall: craterRim(c, 245, .3),
    terrace: craterRim(c, 235, .09, .72),
    rim: craterRim(c, 65, .09),
    peakLight: c.radius >= 14 ? craterPolygon([peakTop, peakLeft, peakBase]) : null,
    peakShadow: c.radius >= 14 ? craterPolygon([peakTop, peakBase, peakRight]) : null
  }
})
const craterFloors = collection(lunarCraterShapes.map(c => c.bowl))
const craterShadows = collection(lunarCraterShapes.map(c => c.wall))
const craterHighlights = collection(lunarCraterShapes.map(c => c.rim))
// Broken, oblique weather systems, not long ellipses aligned with latitude.
// Each cluster has clear gaps, a curved direction and differently sized fragments.
const cloudSystems = [
  [-32, 42, -35], [-12, -12, 30], [45, 18, 55], [78, -36, -25],
  [128, 37, 25], [-142, 25, 55], [-98, 40, -45], [-65, -25, 65],
  [5, 62, 15], [154, -40, 35], [-125, -50, -20], [65, 57, -30]
]
const cloudFragments = cloudSystems.flatMap(([lon, lat, tilt], system) =>
  [-3, -1.8, -.6, .8, 2.2, 3.3].map((t, i) => {
    const x = t * 3.8, y = Math.sin(t * .7 + system) * 4
    const turn = tilt * radians
    return {
      lon: lon + x * Math.cos(turn) - y * Math.sin(turn),
      lat: lat + x * Math.sin(turn) + y * Math.cos(turn),
      width: 2.1 + (i + system) % 3 * .8,
      height: 1.3 + (i * 2 + system) % 4 * .45,
      tilt: tilt + Math.cos(t * .7 + system) * 24,
      seed: system * 7 + i
    }
  }))
const clouds = collection(cloudFragments.map(c =>
  patch(c.lon, c.lat, c.width, c.height, c.seed, c.tilt)))
const cloudVeil = collection(cloudFragments.filter((_, i) => i % 3 !== 0).map(c =>
  patch(c.lon - .8, c.lat + .5, c.width * 1.6, c.height * 1.35, c.seed + 2, c.tilt + 12)))

const plasma = (offset: number) => collection(Array.from({ length: 74 }, (_, i) =>
  patch((i * 137.5 + offset) % 360 - 180, Math.asin(2 * (i + .5) / 74 - 1) / radians,
    7 + i % 7 * 1.7, 4 + i % 4 * 1.5, i + offset)))
const surfaces: Record<SphereKind, GeoGeometryObjects[]> = {
  luna: [craterFloors, craterShadows, craterHighlights],
  terra: [geography as GeoGeometryObjects, cloudVeil, clouds],
  sol: [plasma(0), plasma(63)]
}

export function orthographicView() {
  return geoOrthographic().translate([0, 0]).scale(100).clipAngle(90).precision(.5)
}

/** Orthographic projection clips the rear hemisphere and foreshortens the limbs.
 * Only longitude changes. Pole direction, radius and lighting stay fixed.
 */
export function createSphereProjector(kind: SphereKind) {
  const projection = orthographicView()
  const path = geoPath(projection).digits(1)
  return (angle: number) => surfaces[kind].map((surface, i) => {
    const speed = kind === 'terra' && i > 0 ? 1.18 : kind === 'sol' && i === 1 ? 1.06 : 1
    projection.rotate([angle * speed - (kind === 'terra' ? 18 : 0), kind === 'terra' ? -14 : -6, 0])
    return path(surface) ?? ''
  })
}

// Keep craters separate: object-bounding-box gradients must shade each pit,
// not stretch one gradient across every crater on the whole moon.
export function createLunarCraterProjector() {
  const projection = orthographicView()
  const path = geoPath(projection).digits(1)
  return (angle: number) => {
    projection.rotate([angle, -6, 0])
    return lunarCraterShapes.map(c => ({
      bowl: path(c.bowl) ?? '', floor: path(c.floor) ?? '',
      wall: path(c.wall) ?? '', terrace: path(c.terrace) ?? '', rim: path(c.rim) ?? '',
      peakLight: c.peakLight ? path(c.peakLight) ?? '' : '',
      peakShadow: c.peakShadow ? path(c.peakShadow) ?? '' : ''
    }))
  }
}
