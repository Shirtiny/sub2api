// Input: Natural Earth public-domain ne_110m_land.geojson.
// https://github.com/nvkelso/natural-earth-vector/blob/master/geojson/ne_110m_land.geojson
// Usage from frontend/: node tools/generate-home-earth-geography.mjs /path/to/ne_110m_land.geojson
import { readFileSync, writeFileSync } from 'node:fs'
import { geoArea } from 'd3-geo'
if (!process.argv[2]) throw new Error('Supply the Natural Earth land GeoJSON path')
const raw = JSON.parse(readFileSync(process.argv[2], 'utf8'))
const coordinates = raw.features
  .flatMap(f => f.geometry.type === 'Polygon' ? [f.geometry.coordinates] : f.geometry.coordinates)
  .map(rings => rings.map(ring => ring.map(([x, y]) => [+x.toFixed(2), +y.toFixed(2)])))
for (const rings of coordinates) {
  // D3's spherical winding convention differs from common planar GeoJSON output.
  if (geoArea({ type: 'Polygon', coordinates: rings }) > 2 * Math.PI) {
    for (const ring of rings) ring.reverse()
  }
}
writeFileSync(new URL('../src/assets/home/earth-geography.json', import.meta.url),
  JSON.stringify({ type: 'MultiPolygon', coordinates }) + '\n')
