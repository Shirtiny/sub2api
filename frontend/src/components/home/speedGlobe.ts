import { geoGraticule, geoInterpolate, geoOrthographic, geoPath, geoRotation, type GeoGeometryObjects } from 'd3-geo'
import geography from '@/assets/home/earth-geography.json'

type Coordinate = [number, number]
const radians = Math.PI / 180
const radius = 190
const rotation: [number, number, number] = [-20, -22, -10]
const turn = geoRotation(rotation)
const projection = geoOrthographic().translate([0, 0]).scale(radius).rotate(rotation).clipAngle(90).precision(.65)
const path = geoPath(projection).digits(2)
// The owner identifies the server country as the Netherlands. Use a country-
// level marker, not an invented datacenter address or another transit node.
const serverCoordinate: Coordinate = [5.3, 52.2]

function project(coordinate: Coordinate, lift = 0) {
  const [longitude, latitude] = turn(coordinate)
  const lon = longitude * radians, lat = latitude * radians
  const scale = radius * (1 + lift)
  return { x: scale * Math.cos(lat) * Math.sin(lon), y: -scale * Math.sin(lat), z: scale * Math.cos(lat) * Math.cos(lon) }
}

// These are illustrative client origins, not operating PoPs or live telemetry.
// Every raised great circle goes directly to the same Netherlands server.
const origins: { key: string; coordinate: Coordinate }[] = [
  { key: 'north-america', coordinate: [-74, 40.7] },
  { key: 'south-america', coordinate: [-46.6, -23.6] },
  { key: 'europe', coordinate: [24.9, 60.2] },
  { key: 'africa', coordinate: [28, -26] },
  { key: 'west-asia', coordinate: [55.3, 25.2] },
  { key: 'south-asia', coordinate: [72.9, 19.1] },
  { key: 'southeast-asia', coordinate: [103.8, 1.3] }
]
const routes = origins.map((origin, index) => {
  const interpolate = geoInterpolate(origin.coordinate, serverCoordinate)
  const points = Array.from({ length: 49 }, (_, step) => {
    const progress = step / 48
    return project(interpolate(progress), Math.sin(progress * Math.PI) * (.1 + index % 3 * .045))
  })
  return {
    key: origin.key,
    destination: 'NL' as const,
    origin: project(origin.coordinate),
    points,
    path: points.map((point, step) => `${step ? 'L' : 'M'}${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join('')
  }
})

// Generated once. CSS moves the light along these paths; no per-frame projection.
export const speedGlobe = {
  radius,
  land: path(geography as GeoGeometryObjects) ?? '',
  graticule: path(geoGraticule().step([30, 30])()) ?? '',
  server: { country: 'NL' as const, coordinate: serverCoordinate, ...project(serverCoordinate) },
  routes
}
