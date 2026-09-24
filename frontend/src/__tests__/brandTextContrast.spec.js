import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import postcss from 'postcss'
import tailwindcss from 'tailwindcss'
import resolveConfig from 'tailwindcss/resolveConfig'
import { describe, expect, it } from 'vitest'
import config from '../../tailwind.config.js'

const style = postcss.parse(readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../style.css'), 'utf8'))
const tokens = (selector) => {
  const values = {}
  style.walkRules(selector, rule => rule.walkDecls(/^--color-/, decl => {
    values[decl.prop] = decl.value.split(' ').map(Number)
  }))
  return values
}
const light = tokens(':root')
const dark = tokens('.dark')
const shades = Object.keys(config.theme.extend.colors.primary)
const mix = (fg, bg, opacity) => fg.map((v, i) => v * opacity + bg[i] * (1 - opacity))
const luminance = rgb => rgb.reduce((sum, value, i) => {
  const s = value / 255
  return sum + [0.2126, 0.7152, 0.0722][i] * (s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4)
}, 0)
const contrast = (fg, bg) => (luminance(fg) + 0.05) / (luminance(bg) + 0.05)

describe('brand text palette', () => {
  it('keeps brand text readable on dark surfaces and tinted selected states', () => {
    const surfaces = ['surface-page', 'surface-card', 'surface-secondary', 'surface-hover', 'dark-700']
    for (const surface of surfaces) {
      const bg = dark[`--color-${surface}`]
      for (const shade of shades) {
        expect(contrast(dark[`--color-primary-text-${shade}`], bg), `${shade} on ${surface}`).toBeGreaterThanOrEqual(4.5)
      }
    }
    const card = dark['--color-surface-card']
    for (const [fg, bg, opacity] of [[600, 400, 0.35], [300, 900, 0.3], [700, 100, 1]]) {
      expect(contrast(dark[`--color-primary-text-${fg}`], mix(dark[`--color-primary-${bg}`], card, opacity))).toBeGreaterThanOrEqual(4.5)
    }
    for (const [shade, opacity] of [[200, 0.8], [400, 0.9], [600, 0.9]]) {
      expect(contrast(mix(dark[`--color-primary-text-${shade}`], card, opacity), card)).toBeGreaterThanOrEqual(4.5)
    }
  })

  it('preserves light colors and separates text from fills, borders, rings and gradients', () => {
    const { theme } = resolveConfig(config)
    for (const shade of shades) {
      expect(light[`--color-primary-text-${shade}`]).toBeUndefined()
      expect(theme.textColor.primary[shade]).toBe(`rgb(var(--color-primary-text-${shade}, var(--color-primary-${shade})) / <alpha-value>)`)
      expect(theme.textColor.accent[shade]).toBe(theme.textColor.primary[shade])
      for (const property of ['backgroundColor', 'borderColor', 'ringColor', 'gradientColorStops']) {
        expect(theme[property].primary[shade]).toBe(`rgb(var(--color-primary-${shade}) / <alpha-value>)`)
      }
    }
  })

  it('compiles foreground tokens for dark, hover, group-hover, opacity and @apply', async () => {
    const classes = 'text-primary-600 dark:text-primary-400 dark:hover:text-primary-300 group-hover:text-primary-500 text-primary-200/80 text-accent-400 bg-primary-400 border-primary-400 ring-primary-400 from-primary-500 brand-selection'
    const { root } = await postcss([tailwindcss({
      ...config,
      content: [{ raw: `<div class="${classes}"></div>`, extension: 'html' }]
    })]).process('@tailwind components; @tailwind utilities; @layer components { .brand-selection { @apply bg-primary-100 text-primary-700 dark:bg-primary-400/20 dark:text-primary-600; } }', { from: undefined })
    const declarations = (selector, property) => {
      const values = []
      root.walkRules(rule => {
        if (rule.selector.includes(selector)) rule.walkDecls(property, decl => values.push(decl.value))
      })
      return values
    }
    for (const [selector, shade] of [
      ['.text-primary-600', 600], ['.dark\\:text-primary-400', 400],
      ['.dark\\:hover\\:text-primary-300', 300], ['.group-hover\\:text-primary-500', 500],
      ['.text-primary-200\\/80', 200], ['.text-accent-400', 400]
    ]) {
      expect(declarations(selector, 'color').some(v => v.includes(`--color-primary-text-${shade}`)), selector).toBe(true)
    }
    expect(declarations('.brand-selection', 'color').some(v => v.includes('--color-primary-text-600'))).toBe(true)
    expect(declarations('.text-primary-200\\/80', 'color')[0]).toContain('/ 0.8')
    expect(declarations('.bg-primary-400', 'background-color')[0]).toContain('var(--color-primary-400)')
    expect(declarations('.border-primary-400', 'border-color')[0]).toContain('var(--color-primary-400)')
    expect(declarations('.ring-primary-400', '--tw-ring-color')[0]).toContain('var(--color-primary-400)')
    expect(declarations('.from-primary-500', '--tw-gradient-from')[0]).toContain('var(--color-primary-500)')
  })
})
