import { boundsOf, outline, type Point, type Wordmark } from './brandGeometry'

export const STORY_DURATION = '14s'
const COUNT = 96
const SOFT = '.42 0 .58 1'

// The steam simply breathes at the accent: two sway poses around the accepted original,
// with a faint opacity swell on the same beat. Nothing condenses, falls or transforms.
export const STEAM = {
  keyTimes: '0;.25;.5;.75;1',
  keySplines: [SOFT, SOFT, SOFT, SOFT].join(';'),
  opacity: { values: '.5;.62;.5;.62;.5', keyTimes: '0;.25;.5;.75;1' }
}

// Equal point counts + cyclic alignment keep the steam contour continuous while it sways.
function align(source: Point[], target: Point[]): Point[] {
  let best = target, score = Infinity
  for (const candidate of [target, [...target].reverse()]) {
    for (let shift = 0; shift < COUNT; shift++) {
      const distance = source.reduce((sum, p, i) => {
        const q = candidate[(i + shift) % COUNT]
        return sum + (p[0] - q[0]) ** 2 + (p[1] - q[1]) ** 2
      }, 0)
      if (distance < score) {
        score = distance
        best = candidate.map((_, i) => candidate[(i + shift) % COUNT])
      }
    }
  }
  return best
}

// Original single, fuller ribbon: the accepted steam pose plus its sway variants.
function ribbon(x: number, bottom: number, top: number, bend: number): Point[] {
  const sides = [-1, 1].map(side => Array.from({ length: COUNT / 2 }, (_, i): Point => {
    const t = i / (COUNT / 2 - 1)
    const width = .45 + Math.sin(Math.PI * t) * .9
    return [x + Math.sin(t * Math.PI * 2) * bend + side * width, bottom + (top - bottom) * t]
  }))
  return [...sides[0], ...sides[1].reverse()]
}

export function createBrandStory(mark: Wordmark | null) {
  if (!mark) return null
  const e = mark.letters.find(l => l.character === 'é')
  // The story needs the real é. Other configured names keep their own identity.
  if (!e || e.rings.length !== 3 || mark.width < 160) return null
  const accent = [...e.rings].sort((a, b) => boundsOf(a).top - boundsOf(b).top)[0]
  const accentBox = boundsOf(accent)

  // Keep the exact geometry of the first accepted steam pose; it now sways in place.
  const lowSteam = align(accent, ribbon(accentBox.cx, accentBox.bottom, -7, 2.8))
  const swayRight = align(lowSteam, ribbon(accentBox.cx + .8, accentBox.bottom, -8, -1.8))
  const swayLeft = align(lowSteam, ribbon(accentBox.cx - .5, accentBox.bottom, -6.2, 3.8))
  const body = outline(e.rings.filter(ring => ring !== accent))
  const steam = outline([lowSteam])
  return {
    width: mark.width, viewBox: mark.viewBox,
    e,
    body, accent: outline([accent]), steam,
    steamValues: [lowSteam, swayRight, lowSteam, swayLeft, lowSteam].map(ring => outline([ring])).join(';'),
    staticD: mark.letters.map(letter => letter.index === e.index ? body + steam : letter.d).join('')
  }
}

export type BrandStory = NonNullable<ReturnType<typeof createBrandStory>>
