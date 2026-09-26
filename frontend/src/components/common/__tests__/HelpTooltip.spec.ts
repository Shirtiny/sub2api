import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('opens hover tooltips by tap or focus and dismisses them with Escape', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { content: 'rules' },
      slots: { trigger: '<button type="button">?</button>' },
    })
    const tooltip = getTooltipElement()
    await wrapper.get('button').trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')
    await wrapper.get('.group').trigger('focusin')
    await vi.advanceTimersByTimeAsync(80)
    expect(tooltip.style.display).not.toBe('none')
    await wrapper.get('.group').trigger('focusout')
    await vi.advanceTimersByTimeAsync(180)
    expect(tooltip.style.display).toBe('none')
    wrapper.unmount()
  })

  it('keeps the existing hover interaction by default', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('mouseenter')
    expect(tooltip.style.display).toBe('none')
    await vi.advanceTimersByTimeAsync(80)
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    expect(tooltip.style.display).not.toBe('none')
    await vi.advanceTimersByTimeAsync(180)
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('flips below a high trigger and keeps narrow-screen tips inside the viewport', async () => {
    const wrapper = mount(HelpTooltip, { attachTo: document.body, props: { content: 'rules' } })
    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({ top: 80, bottom: 100, left: 10, width: 20 } as DOMRect)
    Object.defineProperty(tooltip, 'offsetWidth', { configurable: true, value: 256 })
    Object.defineProperty(tooltip, 'offsetHeight', { configurable: true, value: 260 })
    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.top).toBe('108px')
    expect(tooltip.style.left).toBe('8px')
    expect(tooltip.querySelector('.absolute')?.classList.contains('-top-1')).toBe(true)
    wrapper.unmount()
  })

  it('supports click-to-toggle details and closes on outside click', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'click details',
        trigger: 'click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('click details')

    const closeButton = tooltip.querySelector('button[aria-label="Close"]')
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('close button not found')
    }
    closeButton.click()
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })
})
