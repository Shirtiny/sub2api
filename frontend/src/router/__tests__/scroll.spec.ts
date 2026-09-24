import { describe, expect, it } from 'vitest'
import router from '@/router'

describe('router scroll behavior', () => {
  const scroll = router.options.scrollBehavior!

  it.each([
    ['/presale', '/presale?plan=2&multiplier=2'],
    ['/presale?plan=2&multiplier=2', '/presale'],
    ['/presale?plan=2&multiplier=1', '/presale?plan=2&multiplier=2'],
    ['/presale#presale-plans', '/presale?plan=2#presale-plans'],
    ['/presale?plan=2#presale-plans', '/presale#presale-plans'],
  ])('keeps scroll for inline checkout changes from %s to %s', async (from, to) => {
    expect(await scroll(router.resolve(to), router.resolve(from), null)).toBe(false)
  })

  it('still restores browser back/forward positions', async () => {
    const saved = { left: 0, top: 1200 }
    expect(await scroll(router.resolve('/presale'), router.resolve('/presale?plan=2'), saved)).toEqual(saved)
  })

  it.each([
    ['/login', '/presale?plan=2'],
    ['/presale?plan=2', '/payment/result'],
    ['/orders?page=1', '/orders?page=2'],
    ['/presale', '/presale#presale-plans'],
  ])('preserves existing navigation behavior from %s to %s', async (from, to) => {
    expect(await scroll(router.resolve(to), router.resolve(from), null)).toEqual({ top: 0 })
  })
})
