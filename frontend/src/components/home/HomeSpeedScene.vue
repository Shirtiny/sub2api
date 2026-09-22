<template>
  <svg
    class="feature-scene speed-scene"
    data-scene="speed"
    :data-layout="compact ? 'portrait' : 'landscape'"
    :viewBox="compact ? '0 0 600 720' : '0 0 1200 520'"
    preserveAspectRatio="xMidYMid meet"
    fill="none"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <defs>
      <radialGradient :id="id('ocean')" cx="30%" cy="26%" r="78%">
        <stop stop-color="var(--cafe-page, #faf7f2)" />
        <stop offset=".6" stop-color="var(--cafe-surface, #f2ece3)" />
        <stop offset="1" stop-color="var(--cafe-line, #e4d9ca)" />
      </radialGradient>
      <radialGradient :id="id('shade')" cx="31%" cy="28%" r="76%">
        <stop offset=".48" stop-color="var(--cafe-ink, #382a20)" stop-opacity="0" />
        <stop offset="1" stop-color="var(--cafe-ink, #382a20)" stop-opacity=".17" />
      </radialGradient>
      <radialGradient :id="id('halo')">
        <stop offset=".68" stop-color="var(--cafe-accent, #865630)" stop-opacity=".1" />
        <stop offset="1" stop-color="var(--cafe-accent, #865630)" stop-opacity="0" />
      </radialGradient>
      <linearGradient :id="id('glass')" x1="0" y1="0" x2="1" y2="1">
        <stop stop-color="var(--cafe-page, #faf7f2)" />
        <stop offset="1" stop-color="var(--cafe-surface, #f2ece3)" />
      </linearGradient>
      <linearGradient :id="id('current')" gradientUnits="userSpaceOnUse" x1="0" y1="0" :x2="compact ? 0 : stream.period" :y2="compact ? stream.period : 0" spreadMethod="repeat">
        <stop stop-color="var(--cafe-accent, #865630)" stop-opacity=".5" />
        <stop offset=".28" stop-color="var(--cafe-accent, #865630)" stop-opacity=".58" />
        <stop offset=".5" stop-color="var(--cafe-accent, #865630)" stop-opacity=".95" />
        <stop offset=".74" stop-color="var(--cafe-accent, #865630)" stop-opacity=".62" />
        <stop offset="1" stop-color="var(--cafe-accent, #865630)" stop-opacity=".5" />
      </linearGradient>
      <mask :id="id('stream')" maskUnits="userSpaceOnUse" :x="stream.x" :y="stream.y" :width="stream.width" :height="stream.height">
        <path class="stream-front" :d="lanes[1]!.path" pathLength="100" stroke="white" stroke-width="2.8" stroke-linecap="butt" />
      </mask>
      <pattern :id="id('grid')" width="30" height="30" patternUnits="userSpaceOnUse">
        <circle cx="1" cy="1" r=".6" fill="var(--cafe-muted, #756456)" opacity=".16" />
      </pattern>
      <clipPath :id="id('sphere')"><circle :r="speedGlobe.radius" /></clipPath>
      <clipPath :id="id('messages')">
        <rect :x="response.innerLeft - 8" :y="response.top + 39" :width="response.innerWidth + 16" :height="response.editorTop - response.top - 53" />
      </clipPath>
      <clipPath :id="id('client-body')">
        <rect :x="response.innerLeft - 8" :y="response.top + 39" :width="response.innerWidth + 16" :height="response.editorBottom - response.top - 39" />
      </clipPath>
      <clipPath :id="id('compose')">
        <rect class="input-reveal" y="-8" :width="inputWidth" height="16" />
      </clipPath>
    </defs>
    <rect width="100%" height="100%" :fill="`url(#${id('grid')})`" />

    <!-- A fixed orthographic globe: coastlines, ports and routes share the same
         projection. No planar spinning of the world or detached route endpoints. -->
    <g :transform="`translate(${globe.x} ${globe.y})`" data-detail="global-network">
      <ellipse cy="207" rx="151" ry="12" fill="var(--cafe-ink, #382a20)" opacity=".035" />
      <circle class="globe-halo scene-ambient" r="226" :fill="`url(#${id('halo')})`" />
      <circle class="globe-body" :r="speedGlobe.radius" :fill="`url(#${id('ocean')})`" />
      <g :clip-path="`url(#${id('sphere')})`">
        <path class="world-graticule" :d="speedGlobe.graticule" stroke="var(--cafe-muted, #756456)" stroke-width=".7" opacity=".13" />
        <path class="world-land" :d="speedGlobe.land" fill="var(--cafe-accent, #865630)" fill-opacity=".16" stroke="var(--cafe-accent, #865630)" stroke-opacity=".2" stroke-width=".65" />
        <circle :r="speedGlobe.radius" :fill="`url(#${id('shade')})`" />
      </g>
      <circle :r="speedGlobe.radius" stroke="var(--cafe-accent, #865630)" stroke-opacity=".22" stroke-width=".9" />
      <path d="M-151-115A190 190 0 0 1 97-164" stroke="var(--cafe-page, #faf7f2)" stroke-opacity=".85" stroke-width="1.6" />

      <g data-detail="world-routes">
        <g v-for="(route, index) in speedGlobe.routes" :key="route.key" class="world-route" :data-origin="route.key" :data-destination="route.destination" :style="{ '--route-delay': `${index * -.47}s` }">
          <path class="world-route-track" :d="route.path" stroke="var(--cafe-accent, #865630)" stroke-width="1" />
          <path class="world-packet world-request scene-ambient" :d="route.path" pathLength="100" stroke="var(--cafe-accent, #865630)" stroke-width="1.8" />
          <path class="world-packet world-return scene-ambient" :d="route.path" pathLength="100" stroke="var(--cafe-ink, #382a20)" stroke-width="1.6" />
          <g :transform="`translate(${route.origin.x} ${route.origin.y})`" class="world-port">
            <circle class="port-dot" r="2.5" fill="var(--cafe-page, #faf7f2)" stroke="var(--cafe-accent, #865630)" stroke-width="1.1" />
            <circle class="port-echo scene-ambient" r="4" stroke="var(--cafe-accent, #865630)" />
          </g>
        </g>
      </g>
    </g>

    <!-- This client's request terminates in the Netherlands and returns directly.
         The other geographic clients have independent, quieter round trips. -->
    <g data-detail="optimized-lines">
      <g v-for="(lane, index) in lanes" :key="index" class="express-lane" :data-primary="index === 1" data-destination="NL">
        <path class="express-track" :d="lane.path" stroke="var(--cafe-accent, #865630)" stroke-width="1" />
        <template v-if="index === 1">
          <path class="route-highlight" :d="lane.path" stroke="var(--cafe-accent, #865630)" stroke-width="3.5" />
          <g :transform="`translate(${lane.gate.x} ${lane.gate.y}) rotate(${compact ? 90 : 0})`" class="direction-cue" stroke="var(--cafe-accent, #865630)" stroke-width="1.4">
            <path class="send-cue" d="M-7-5-2 0-7 5M2-5 7 0 2 5" />
            <path class="receive-cue" d="M7-5 2 0 7 5M-2-5-7 0-2 5" />
          </g>
          <path class="delivery-packet" :d="lane.path" pathLength="100" stroke="var(--cafe-accent, #865630)" stroke-width="2.8" />
          <path class="response-packet" :d="lane.path" pathLength="100" stroke="var(--cafe-ink, #382a20)" stroke-width="3.2" />
          <!-- The waterline advances from Server, keeps flowing while output
               arrives, then its trailing edge drains into the client. -->
          <g class="stream-connection" :mask="`url(#${id('stream')})`">
            <rect class="stream-surface" :x="stream.x" :y="stream.y" :width="stream.width + (compact ? 0 : stream.period)" :height="stream.height + (compact ? stream.period : 0)" :fill="`url(#${id('current')})`" :style="{ '--flow-x': `${compact ? 0 : -stream.period}px`, '--flow-y': `${compact ? -stream.period : 0}px` }" />
            <path :d="lane.path" stroke="var(--cafe-page, #faf7f2)" stroke-width=".6" opacity=".2" />
          </g>
        </template>
        <g :transform="`translate(${lane.start.x} ${lane.start.y})`" class="client-port">
          <circle r="2.5" fill="var(--cafe-accent, #865630)" />
          <circle v-if="index === 1" class="client-echo" r="5" stroke="var(--cafe-accent, #865630)" />
        </g>
      </g>
    </g>

    <g :transform="`translate(${server.x} ${server.y})`" class="server-node" data-detail="netherlands-server" data-country="NL">
      <path class="server-leader" d="M6-6 23-23H85" stroke="var(--cafe-accent, #865630)" stroke-opacity=".4" />
      <text x="27" y="-30" class="server-label">Server</text>
      <circle class="server-echo" r="6" stroke="var(--cafe-accent, #865630)" />
      <circle r="3.5" fill="var(--cafe-accent, #865630)" />
    </g>

    <!-- An unbranded terminal: one persistent submitted input, progressive
         response chunks, a tool task, then more streaming output and scrolling. -->
    <g :transform="`translate(${response.x} ${response.y})`" data-detail="streaming-response" data-client="terminal" :style="scrollStyle">
      <rect :x="response.left + 9" :y="response.top + 9" :width="response.width" :height="response.height" rx="14" fill="var(--cafe-ink, #382a20)" opacity=".035" />
      <rect class="response-shell" :x="response.left" :y="response.top" :width="response.width" :height="response.height" rx="12" :fill="`url(#${id('glass')})`" stroke="var(--cafe-muted, #756456)" stroke-opacity=".35" />
      <g class="terminal-titlebar">
        <path :d="`M${response.left} ${response.top + 34}h${response.width}`" stroke="var(--cafe-line, #e4d9ca)" />
        <circle v-for="offset in [0, 13, 26]" :key="offset" :cx="response.innerLeft + offset" :cy="response.top + 17" r="3" fill="var(--cafe-muted, #756456)" opacity=".25" />
        <path :d="`M-18 ${response.top + 17}h36`" stroke="var(--cafe-muted, #756456)" stroke-width="2" opacity=".25" />
        <circle class="first-byte" :cx="response.innerRight" :cy="response.top + 17" r="3" fill="var(--cafe-accent, #865630)" data-detail="first-byte" />
      </g>
      <g class="terminal-messages" data-detail="terminal-messages" :clip-path="`url(#${id('messages')})`">
        <g class="task-scroll">
          <rect class="history-surface" :x="response.innerLeft - 8" :y="response.requestY - 17" :width="response.innerWidth + 16" height="34" rx="3" fill="var(--cafe-accent, #865630)" />
          <g class="thinking-state" data-state="thinking" :transform="`translate(${response.innerLeft + 3} ${response.replyY})`" fill="var(--cafe-accent, #865630)">
            <circle v-for="index in [0, 1, 2]" :key="index" class="thinking-dot" :cx="index * 10" r="2.2" :style="{ '--dot-delay': `${index * -.16}s` }" />
          </g>
          <g class="response-content" data-state="reply" fill="var(--cafe-accent, #865630)">
            <g v-for="chunk in replyChunks" :key="chunk.index" class="reply-piece" :data-chunk="chunk.index" :data-batch="chunk.batch" :data-row="chunk.row" :transform="`translate(${chunk.x} ${chunk.y})`" :style="{ '--chunk-delay': `${chunk.delayMs}ms` }">
              <rect class="reply-chunk" y="-2" :width="chunk.width" height="4" rx="2" />
            </g>
          </g>
          <g class="tool-task" data-state="working" :transform="`translate(${response.innerLeft} ${task.toolY})`">
            <rect x="-8" y="-6" :width="response.innerWidth + 16" :height="task.toolHeight + 6" rx="6" fill="var(--cafe-accent, #865630)" fill-opacity=".035" stroke="var(--cafe-accent, #865630)" stroke-opacity=".18" />
            <path :d="`M26 16H${response.innerWidth - 42}`" stroke="var(--cafe-accent, #865630)" stroke-opacity=".2" />
            <g v-for="index in [0, 1, 2]" :key="index" :transform="`translate(${26 + index * (response.innerWidth - 68) / 2} 16)`" :style="{ '--step-delay': `${index * 640}ms` }">
              <circle r="8" fill="var(--cafe-page, #faf7f2)" stroke="var(--cafe-accent, #865630)" stroke-opacity=".3" />
              <path class="tool-check" d="M-3 0-1 2 4-3" stroke="var(--cafe-accent, #865630)" stroke-width="1.3" />
              <path d="M-8 17H14" stroke="var(--cafe-muted, #756456)" stroke-width="2" opacity=".3" />
            </g>
            <rect class="tool-progress" y="46" :width="response.innerWidth" height="2" rx="1" fill="var(--cafe-accent, #865630)" />
          </g>
        </g>
      </g>
      <!-- A single input moves from the editor into the transcript. No second
           copy, text clipping swap, or opacity reset during submission. -->
      <g class="request-stage" :clip-path="`url(#${id('client-body')})`">
        <g class="task-scroll">
          <g :transform="`translate(${response.innerLeft} ${response.requestY})`">
            <g class="request-transfer" :style="{ '--editor-shift': `${response.editorTop + 20 - response.requestY}px`, '--input-width': `${inputWidth + 5}px` }">
              <g class="request-ink" :clip-path="`url(#${id('compose')})`" fill="var(--cafe-accent, #865630)">
                <rect v-for="token in inputTokens" :key="token.x" :x="token.x" y="-2" :width="token.width" height="4" rx="2" />
              </g>
              <rect class="input-caret" y="-6" width="4" height="12" rx="1" fill="var(--cafe-accent, #865630)" />
            </g>
          </g>
        </g>
      </g>
      <g class="terminal-editor" data-detail="terminal-editor">
        <path :d="`M${response.innerLeft - 8} ${response.editorTop}h${response.innerWidth + 16}M${response.innerLeft - 8} ${response.editorBottom}h${response.innerWidth + 16}`" stroke="var(--cafe-accent, #865630)" stroke-opacity=".5" />
        <rect class="editor-caret" :x="response.innerLeft" :y="response.editorTop + 13" width="5" height="12" rx="1" fill="var(--cafe-accent, #865630)" />
      </g>
    </g>
  </svg>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance } from 'vue'
import { speedGlobe } from './speedGlobe'

const props = defineProps<{ compact: boolean }>()
const uid = getCurrentInstance()!.uid
const id = (part: string) => `speed-network-${uid}-${part}`
const globe = computed(() => props.compact ? { x: 300, y: 495 } : { x: 900, y: 258 })
const server = computed(() => ({ x: globe.value.x + speedGlobe.server.x, y: globe.value.y + speedGlobe.server.y }))
const response = computed(() => {
  const width = props.compact ? 424 : 412
  const height = props.compact ? 244 : 302
  return {
    x: props.compact ? 300 : 256, y: props.compact ? 136 : 258,
    width, height, left: -width / 2, right: width / 2, top: -height / 2, bottom: height / 2,
    innerLeft: -width / 2 + 24, innerRight: width / 2 - 24, innerWidth: width - 48,
    requestY: -height / 2 + 61, replyY: -height / 2 + (props.compact ? 104 : 120), rowGap: props.compact ? 24 : 27,
    editorTop: height / 2 - 72, editorBottom: height / 2 - 33
  }
})
const lanes = computed(() => [-1, 0, 1].map(offset => {
  const start = props.compact
    ? { x: response.value.x + offset * 64, y: response.value.y + response.value.bottom }
    : { x: response.value.x + response.value.right, y: response.value.y + offset * 34 }
  const end = server.value
  const gate = props.compact ? { x: 300 + offset * 68, y: 296 } : { x: 570, y: 258 + offset * 54 }
  const path = props.compact
    ? `M${start.x} ${start.y}V${start.y + 8}C${start.x} ${start.y + 20} ${gate.x} ${start.y + 18} ${gate.x} ${start.y + 31}V${gate.y + 17}C${gate.x} ${end.y - 50} ${end.x} ${end.y - 46} ${end.x} ${end.y}`
    : `M${start.x} ${start.y}C${start.x + 28} ${start.y} ${gate.x - 84} ${gate.y} ${gate.x - 52} ${gate.y}H${gate.x + 25}C${gate.x + 112} ${gate.y} ${end.x - 98} ${end.y} ${end.x} ${end.y}`
  return { start, end, gate, path }
}))
const stream = computed(() => {
  const { start, end } = lanes.value[1]!
  return {
    x: Math.min(start.x, end.x) - 6, y: Math.min(start.y, end.y) - 6,
    width: Math.abs(end.x - start.x) + 12, height: Math.abs(end.y - start.y) + 12,
    period: props.compact ? 80 : 208
  }
})
const inputTokens = [34, 49, 23, 43, 55, 25].map((width, index, widths) => ({
  width, x: widths.slice(0, index).reduce((sum, value) => sum + value + 8, 0)
}))
const inputWidth = inputTokens.at(-1)!.x + inputTokens.at(-1)!.width
const task = computed(() => {
  const toolY = response.value.replyY + 3 * response.value.rowGap + 16
  const toolHeight = 54
  const secondReplyY = toolY + toolHeight + 32
  const visibleBottom = response.value.editorTop - 14
  return { toolY, toolHeight, secondReplyY, visibleBottom }
})
const scrollStyle = computed(() => ({
  '--scroll-tool': `${task.value.toolY + task.value.toolHeight - task.value.visibleBottom + 8}px`,
  '--scroll-reply-start': `${task.value.secondReplyY - task.value.visibleBottom + 8}px`,
  '--scroll-reply-middle': `${task.value.secondReplyY + response.value.rowGap - task.value.visibleBottom + 8}px`,
  '--scroll-reply-end': `${task.value.secondReplyY + 2 * response.value.rowGap - task.value.visibleBottom + 8}px`
}))
// Payload fragments accumulate only after the continuous stream reaches the client.
// Two batches share a 16s clock, with a graphical tool task between them.
// Response pacing is illustrative, not a measured token-generation rate.
const replyChunks = computed(() => [0, 1].flatMap(batch => [
  [40, 56, 22, 48, 32, 52], [28, 44, 62, 31, 46, 36], [36, 52, 24, 46, 30, 42]
].flatMap((widths, row) => widths.map((width, column) => {
  const offset = row * 6 + column
  return {
    index: batch * 18 + offset, batch, row,
    x: response.value.innerLeft + widths.slice(0, column).reduce((sum, value) => sum + value + 7, 0),
    y: (batch ? task.value.secondReplyY : response.value.replyY) + row * response.value.rowGap,
    width, delayMs: batch * 5920 + offset * 155
  }
}))))

</script>

<style scoped>
.speed-scene { --speed-cycle: 16s; --network-cycle: 3.6s; display: block; width: 100%; height: 100%; contain: paint; }
.server-label { font-family: ui-sans-serif, system-ui, sans-serif; font-size: 12px; letter-spacing: .6px; fill: var(--cafe-muted, #756456); }
/* Geographic clients keep their own direct, quieter round trips throughout. */
.globe-halo { transform-origin: 0 0; animation: globe-breathe 6.4s ease-in-out infinite; }
.world-route-track { opacity: .19; }
.world-port .port-dot { stroke-opacity: .55; }
.world-packet { stroke-dasharray: 5 110; }
.world-request { animation: world-request var(--network-cycle) var(--route-delay) linear infinite both; }
.world-return { animation: world-return var(--network-cycle) var(--route-delay) linear infinite both; }
.port-echo { transform-origin: 0 0; animation: port-echo var(--network-cycle) var(--route-delay) ease-out infinite both; }
.server-echo { transform-origin: 0 0; animation: server-echo var(--speed-cycle) ease-out infinite both; }
.express-track { opacity: .14; }
.express-lane[data-primary='true'] .express-track { opacity: .38; }
.route-highlight { animation: route-highlight var(--speed-cycle) ease-in-out infinite both; }
.send-cue { animation: send-cue var(--speed-cycle) ease-in-out infinite both; }
.receive-cue { animation: receive-cue var(--speed-cycle) ease-in-out infinite both; }
.delivery-packet { stroke-dasharray: 6 110; animation: express-send var(--speed-cycle) linear infinite both; }
.response-packet { stroke-dasharray: 5 110; animation: express-return var(--speed-cycle) linear infinite both; }
.stream-front { stroke-dasharray: 100 200; stroke-dashoffset: -100; animation: stream-travel var(--speed-cycle) linear infinite both; }
.stream-surface { animation: stream-current 3.2s linear infinite; }
.client-echo { transform-origin: 0 0; animation: client-echo var(--speed-cycle) ease-out infinite both; }
/* Receipt -> graphical thinking -> a sustained stream -> tool work -> more
   streaming. A moving, unbroken current feeds each output interval. */
.first-byte { animation: first-byte var(--speed-cycle) ease-out infinite both; }
.thinking-state { opacity: 0; animation: thinking-state var(--speed-cycle) ease-in-out infinite both; }
.thinking-dot { animation: thinking-dot .72s var(--dot-delay) ease-in-out infinite; }
.response-content { opacity: 0; animation: response-arrive var(--speed-cycle) ease-out infinite both; }
.reply-chunk { transform-origin: 0 0; opacity: 0; animation: chunk-arrive var(--speed-cycle) var(--chunk-delay) linear infinite both; }
.tool-task { opacity: 0; animation: tool-task var(--speed-cycle) ease-out infinite both; }
.tool-check { opacity: 0; animation: tool-check var(--speed-cycle) var(--step-delay) ease-out infinite both; }
.tool-progress { transform-origin: 0 0; animation: tool-progress var(--speed-cycle) ease-in-out infinite both; }
.task-scroll { animation: task-scroll var(--speed-cycle) ease-in-out infinite both; }
.history-surface { opacity: 0; animation: history-surface var(--speed-cycle) ease-out infinite both; }
.request-transfer { opacity: 0; animation: request-transfer var(--speed-cycle) ease-in-out infinite both; }
.input-reveal { transform-origin: 0 0; transform: scaleX(0); animation: input-reveal var(--speed-cycle) steps(14, end) infinite both; }
.input-caret { animation: input-caret var(--speed-cycle) steps(14, end) infinite both; }
.editor-caret { opacity: 0; animation: editor-caret var(--speed-cycle) linear infinite both; }
@keyframes globe-breathe { 0%, 100% { transform: scale(.98); opacity: .6; } 28% { transform: scale(1.035); opacity: 1; } 64% { transform: scale(1); opacity: .7; } }
@keyframes world-request {
  0%, 3% { stroke-dashoffset: 5; opacity: 0; }
  4% { stroke-dashoffset: 0; opacity: .6; }
  38% { stroke-dashoffset: -100; opacity: .6; }
  39%, 100% { stroke-dashoffset: -105; opacity: 0; }
}
@keyframes world-return {
  0%, 49% { stroke-dashoffset: -105; opacity: 0; }
  50% { stroke-dashoffset: -100; opacity: .45; }
  80% { stroke-dashoffset: 0; opacity: .45; }
  81%, 100% { stroke-dashoffset: 5; opacity: 0; }
}
@keyframes port-echo { 0%, 79%, 100% { transform: scale(.7); opacity: 0; } 81% { transform: scale(1); opacity: .4; } 95% { transform: scale(2.6); opacity: 0; } }
@keyframes route-highlight { 0%, 8%, 18%, 43%, 54%, 80%, 100% { opacity: 0; } 11%, 15%, 25%, 38%, 57%, 63%, 76% { opacity: .12; } }
@keyframes send-cue { 0%, 8%, 17%, 54%, 61%, 100% { opacity: 0; transform: translateX(-5px); } 11%, 57% { opacity: .6; transform: translateX(3px); } }
@keyframes receive-cue { 0%, 13%, 18%, 22%, 43%, 59%, 80%, 100% { opacity: 0; } 15%, 25%, 38%, 63%, 76% { opacity: .55; } }
@keyframes express-send {
  0%, 9%, 55% { stroke-dashoffset: 6; opacity: 0; }
  10%, 56% { stroke-dashoffset: 0; opacity: 1; animation-timing-function: cubic-bezier(.55, 0, .75, .5); }
  13%, 58% { stroke-dashoffset: -100; opacity: 1; }
  14%, 59%, 100% { stroke-dashoffset: -106; opacity: 0; }
}
@keyframes server-echo { 0%, 12%, 55%, 100% { transform: scale(.7); opacity: 0; } 14%, 58% { transform: scale(1); opacity: .65; } 20%, 64% { transform: scale(3); opacity: 0; } }
@keyframes express-return {
  0%, 13% { stroke-dashoffset: -105; opacity: 0; }
  14% { stroke-dashoffset: -100; opacity: 1; }
  16% { stroke-dashoffset: 0; opacity: 1; }
  17%, 100% { stroke-dashoffset: 5; opacity: 0; }
}
@keyframes client-echo { 0%, 9%, 100% { transform: scale(.7); opacity: 0; } 10%, 17% { transform: scale(1); opacity: .6; } 14%, 22% { transform: scale(2.8); opacity: 0; } }
@keyframes first-byte { 0%, 16%, 100% { opacity: .12; } 17%, 21%, 80%, 92% { opacity: 1; } 25%, 78% { opacity: .6; } }
@keyframes thinking-state { 0%, 17%, 24%, 100% { opacity: 0; } 18%, 22% { opacity: 1; } }
@keyframes thinking-dot { 0%, 100% { opacity: .2; transform: translateY(0); } 50% { opacity: .9; transform: translateY(-2px); } }
/* The path runs client -> server; increasing the offset sends the fluid back
   toward the client. A path-length dash forms one body, never a packet train.
   The 200-unit gap hides the reset between the two response batches. */
@keyframes stream-travel {
  0%, 20% { stroke-dashoffset: -100; }
  25%, 37.5% { stroke-dashoffset: 0; }
  42.5% { stroke-dashoffset: 100; }
  58% { stroke-dashoffset: 200; }
  62%, 75.5% { stroke-dashoffset: 300; }
  79.5%, 100% { stroke-dashoffset: 400; }
}
@keyframes stream-current { to { transform: translate(var(--flow-x), var(--flow-y)); } }
@keyframes chunk-arrive {
  0%, 24.9% { transform: scaleX(0); opacity: 0; }
  25% { transform: scaleX(.12); opacity: .9; }
  25.75% { transform: scaleX(1); opacity: .9; }
  27%, 100% { transform: scaleX(1); opacity: .58; }
}
@keyframes response-arrive { 0%, 24%, 98%, 100% { opacity: 0; } 25%, 93% { opacity: 1; } }
@keyframes tool-task { 0%, 40%, 98%, 100% { opacity: 0; } 43%, 93% { opacity: 1; } }
@keyframes tool-check { 0%, 46% { opacity: 0; } 48%, 100% { opacity: 1; } }
@keyframes tool-progress { 0%, 43% { transform: scaleX(0); opacity: .5; } 57%, 100% { transform: scaleX(1); opacity: .5; } }
@keyframes task-scroll {
  0%, 40% { transform: translateY(0); }
  46%, 58% { transform: translateY(calc(-1 * var(--scroll-tool))); }
  60% { transform: translateY(calc(-1 * var(--scroll-reply-start))); }
  67% { transform: translateY(calc(-1 * var(--scroll-reply-middle))); }
  75%, 98% { transform: translateY(calc(-1 * var(--scroll-reply-end))); }
  99%, 100% { transform: translateY(0); }
}
@keyframes history-surface { 0%, 8%, 98%, 100% { opacity: 0; } 10%, 93% { opacity: .07; } }
@keyframes request-transfer {
  0% { opacity: 0; transform: translateY(var(--editor-shift)); }
  1%, 8% { opacity: 1; transform: translateY(var(--editor-shift)); }
  10%, 93% { opacity: 1; transform: translateY(0); }
  98%, 100% { opacity: 0; transform: translateY(0); }
}
@keyframes input-reveal { 0%, .5% { transform: scaleX(0); } 4%, 100% { transform: scaleX(1); } }
@keyframes input-caret { 0%, .5% { transform: translateX(0); opacity: 1; } 4%, 8% { transform: translateX(var(--input-width)); opacity: 1; } 9%, 100% { transform: translateX(var(--input-width)); opacity: 0; } }
@keyframes editor-caret { 0%, 10%, 98%, 100% { opacity: 0; } 12%, 93% { opacity: .35; } }
@media (prefers-reduced-motion: reduce) {
  .speed-scene * { animation: none !important; }
  .world-packet, .port-echo, .server-echo, .delivery-packet, .response-packet, .stream-connection,
  .client-echo, .direction-cue, .route-highlight, .thinking-state, .input-caret { display: none; }
  .task-scroll { transform: translateY(calc(-1 * var(--scroll-reply-end))); }
  .request-transfer, .response-content, .tool-task, .tool-check { opacity: 1; }
  .reply-chunk { transform: scaleX(1); opacity: .58; }
  .input-reveal { transform: scaleX(1); }
  .history-surface { opacity: .07; }
  .tool-progress { transform: scaleX(1); opacity: .5; }
  .first-byte, .editor-caret { opacity: .6; }
}
</style>
