import vectors from '@/assets/home/brand-vectors.json'

export type Point = [number, number]
interface Glyph { advance: number; rings: number[] }
export interface Letter {
  character: string
  index: number
  rings: Point[][]
  d: string
  bounds: ReturnType<typeof boundsOf>
}
const glyphs: Record<string, Glyph> = vectors.glyphs
const decoded = new Map<number, Point[]>()

function decode(index: number): Point[] {
  if (decoded.has(index)) return decoded.get(index)!
  const values = vectors.rings[index]
  const points: Point[] = []
  let x = 0, y = 0
  for (let i = 0; i < values.length; i += 2) {
    x += values[i] / 2
    y += values[i + 1] / 2
    points.push([x, y])
  }
  decoded.set(index, points)
  return points
}

export function boundsOf(points: Point[]) {
  const xs = points.map(p => p[0]), ys = points.map(p => p[1])
  const left = Math.min(...xs), right = Math.max(...xs), top = Math.min(...ys), bottom = Math.max(...ys)
  return { left, right, top, bottom, width: right - left, height: bottom - top, cx: (left + right) / 2, cy: (top + bottom) / 2 }
}

export function outline(rings: Point[][]) {
  return rings.map(ring => ring.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(2)},${y.toFixed(2)}`).join('') + 'Z').join('')
}

// Keep glyph geometry available for the letter-anchored choreography; no font request.
export function createWordmark(name: string) {
  const characters = Array.from(name.normalize('NFC').trim())
  const display = characters.length > 48 ? [...characters.slice(0, 47), '…'] : characters
  if (!display.length || display.some(c => !glyphs[c])) return null

  let advance = 0
  const raw: { character: string; rings: Point[][] }[] = []
  for (const character of display) {
    const glyph = glyphs[character]
    raw.push({ character, rings: glyph.rings.map(index => decode(index).map(([x, y]): Point => [x + advance, y])) })
    advance += glyph.advance + 32
  }
  const rings = raw.flatMap(letter => letter.rings)
  if (!rings.length) return null
  const points = rings.flat()
  const left = Math.min(...points.map(p => p[0])), right = Math.max(...points.map(p => p[0]))
  const top = Math.min(...points.map(p => p[1])), bottom = Math.max(...points.map(p => p[1]))
  const scale = Math.min(222 / (right - left || 1), 44 / (bottom - top || 1), .065)
  const width = (right - left) * scale + 2
  const letters: Letter[] = raw.flatMap((letter, index) => {
    if (!letter.rings.length) return []
    const transformed = letter.rings.map(ring => ring.map(([x, y]): Point => [1 + (x - left) * scale, 24 + (y - (top + bottom) / 2) * scale]))
    return [{ character: letter.character, index, rings: transformed, d: outline(transformed), bounds: boundsOf(transformed.flat()) }]
  })
  return { d: letters.map(letter => letter.d).join(''), width, letters, viewBox: `0 0 ${width.toFixed(2)} 48` }
}

export type Wordmark = NonNullable<ReturnType<typeof createWordmark>>
