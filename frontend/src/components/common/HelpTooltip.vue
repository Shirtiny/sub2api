<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useTemplateRef, nextTick } from 'vue'

const props = withDefaults(defineProps<{
  content?: string
  trigger?: 'hover' | 'click'
  widthClass?: string
}>(), {
  trigger: 'hover',
  widthClass: 'w-64',
})

const show = ref(false)
const triggerRef = useTemplateRef<HTMLElement>('trigger')
const tooltipRef = useTemplateRef<HTMLElement>('tooltip')
const tooltipStyle = ref({ top: '0px', left: '0px' })
const placement = ref<'top' | 'bottom'>('top')
const arrowLeft = ref('50%')
const openDelay = 80
const closeDelay = 180
let openTimer: number | null = null
let closeTimer: number | null = null

function clearOpenTimer() {
  if (!openTimer) return
  window.clearTimeout(openTimer)
  openTimer = null
}

function clearCloseTimer() {
  if (!closeTimer) return
  window.clearTimeout(closeTimer)
  closeTimer = null
}

function clearHoverTimers() {
  clearOpenTimer()
  clearCloseTimer()
}

function openTooltip() {
  clearHoverTimers()
  show.value = true
  nextTick(updatePosition)
}

function closeTooltip() {
  clearHoverTimers()
  show.value = false
}

function scheduleOpen() {
  clearCloseTimer()
  if (show.value) {
    nextTick(updatePosition)
    return
  }
  clearOpenTimer()
  openTimer = window.setTimeout(() => {
    openTimer = null
    openTooltip()
  }, openDelay)
}

function scheduleClose() {
  clearOpenTimer()
  clearCloseTimer()
  closeTimer = window.setTimeout(() => {
    show.value = false
    closeTimer = null
  }, closeDelay)
}

function onEnter() {
  if (props.trigger !== 'hover') return
  scheduleOpen()
}

function onLeave() {
  if (props.trigger !== 'hover') return
  scheduleClose()
}

function onTooltipEnter() {
  if (props.trigger !== 'hover') return
  clearCloseTimer()
}

function onTooltipLeave() {
  if (props.trigger !== 'hover') return
  scheduleClose()
}

function onClick(event: MouseEvent) {
  event.stopPropagation()
  if (show.value) {
    closeTooltip()
    return
  }
  openTooltip()
}

function onDocumentClick(event: MouseEvent) {
  if (!show.value) return
  const target = event.target as Node | null
  if (!target) return
  if (triggerRef.value?.contains(target) || tooltipRef.value?.contains(target)) return
  closeTooltip()
}

function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeTooltip()
  }
}

function onViewportChange() {
  if (!show.value) return
  updatePosition()
}

function updatePosition() {
  const el = triggerRef.value
  const tooltip = tooltipRef.value
  if (!el || !tooltip) return
  const rect = el.getBoundingClientRect()
  const gap = 8
  const width = tooltip.offsetWidth
  const height = tooltip.offsetHeight
  const center = rect.left + rect.width / 2
  const left = Math.max(gap, Math.min(center - width / 2, window.innerWidth - width - gap))
  placement.value = rect.top >= height + gap * 2 ? 'top' : 'bottom'
  const top = placement.value === 'top' ? rect.top - height - gap : rect.bottom + gap
  // Fixed positioning uses viewport coordinates; do not add page scroll offsets.
  tooltipStyle.value = {
    top: `${Math.max(gap, Math.min(top, window.innerHeight - height - gap))}px`,
    left: `${left}px`,
  }
  arrowLeft.value = `${Math.max(12, Math.min(center - left, width - 12))}px`
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick, true)
  document.addEventListener('keydown', onDocumentKeydown)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})

onBeforeUnmount(() => {
  clearHoverTimers()
  document.removeEventListener('click', onDocumentClick, true)
  document.removeEventListener('keydown', onDocumentKeydown)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
})
</script>

<template>
  <div
    ref="trigger"
    class="group relative ml-1 inline-flex items-center align-middle"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
    @focusin="onEnter"
    @focusout="onLeave"
    @click="onClick"
  >
    <!-- Trigger Icon -->
    <slot name="trigger">
      <svg
        class="h-4 w-4 cursor-help text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    </slot>

    <!-- Teleport to body to escape modal overflow clipping -->
    <Teleport to="body">
      <div
        ref="tooltip"
        v-show="show"
        role="tooltip"
        :class="[
          'fixed z-[99999] max-w-[calc(100vw-1rem)] rounded-lg bg-gray-900 p-3 text-xs leading-relaxed text-white shadow-xl ring-1 ring-white/10 dark:bg-gray-800',
          props.widthClass,
        ]"
        :style="tooltipStyle"
        @mouseenter="onTooltipEnter"
        @mouseleave="onTooltipLeave"
        @click.stop
      >
        <button
          v-if="props.trigger === 'click'"
          type="button"
          class="absolute right-1.5 top-1.5 rounded p-1 text-gray-300 transition-colors hover:bg-white/10 hover:text-white"
          aria-label="Close"
          @click.stop="closeTooltip"
        >
          <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
        <slot>{{ content }}</slot>
        <div
          class="absolute h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900 dark:bg-gray-800"
          :class="placement === 'top' ? '-bottom-1' : '-top-1'"
          :style="{ left: arrowLeft }"
        ></div>
      </div>
    </Teleport>
  </div>
</template>
