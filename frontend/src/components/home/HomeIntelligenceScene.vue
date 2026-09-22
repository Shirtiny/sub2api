<template>
  <svg
    class="feature-scene intelligence-scene"
    data-scene="intelligence"
    :data-layout="compact ? 'portrait' : 'landscape'"
    :viewBox="compact ? '0 0 600 720' : '0 0 1200 520'"
    preserveAspectRatio="xMidYMid meet"
    fill="none"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <defs>
      <linearGradient :id="id('glass')" x1="0" y1="0" x2="1" y2="1">
        <stop stop-color="var(--cafe-page, #faf7f2)" stop-opacity=".96" />
        <stop offset="1" stop-color="var(--cafe-surface, #f2ece3)" stop-opacity=".8" />
      </linearGradient>
      <linearGradient :id="id('edge')" x1="0" y1="0" x2="1" y2="1">
        <stop stop-color="var(--cafe-muted, #756456)" stop-opacity=".45" />
        <stop offset="1" stop-color="var(--cafe-line, #e4d9ca)" stop-opacity=".7" />
      </linearGradient>
      <radialGradient :id="id('pool')">
        <stop stop-color="var(--cafe-accent, #865630)" stop-opacity=".07" />
        <stop offset="1" stop-color="var(--cafe-accent, #865630)" stop-opacity="0" />
      </radialGradient>
      <radialGradient :id="id('focus')">
        <stop stop-color="var(--cafe-accent, #865630)" stop-opacity=".24" />
        <stop offset="1" stop-color="var(--cafe-accent, #865630)" stop-opacity="0" />
      </radialGradient>
      <pattern :id="id('grid')" width="32" height="32" patternUnits="userSpaceOnUse">
        <path d="M0 2V0H2" stroke="var(--cafe-muted, #756456)" stroke-width=".8" opacity=".18" />
      </pattern>
    </defs>
    <rect width="100%" height="100%" :fill="`url(#${id('grid')})`" />
    <ellipse :cx="center.x" :cy="center.y + 45" :rx="compact ? 270 : 490" :ry="compact ? 260 : 205" :fill="`url(#${id('pool')})`" />

    <!-- A capability illustration, not a model reasoning transcript.
         Wired geometry stays fixed; only emphasis travels through the system. -->
    <g class="context-links" data-detail="context-links" stroke="var(--cafe-muted, #756456)" stroke-width="1">
      <path v-for="(link, index) in inputLinks" :key="index" :d="link" pathLength="100" />
    </g>
    <path class="output-link" :d="outputLink" stroke="var(--cafe-muted, #756456)" stroke-width="1.1" />

    <g v-for="(input, index) in inputs" :key="input.kind" :transform="`translate(${input.x} ${input.y})`" :data-detail="`input-${input.kind}`">
      <g class="input-card" :class="`input-${index}`" :style="{ '--source-delay': `${index * .32}s` }">
        <rect x="-81" y="-48" width="166" height="103" rx="12" fill="var(--cafe-ink, #382a20)" opacity=".035" />
        <rect x="-84" y="-54" width="168" height="104" rx="12" :fill="`url(#${id('glass')})`" :stroke="`url(#${id('edge')})`" />
        <rect class="input-outline" x="-84" y="-54" width="168" height="104" rx="12" stroke="var(--cafe-accent, #865630)" />
        <path d="M-63-33h12m6 0h4M57-33h6" stroke="var(--cafe-muted, #756456)" stroke-width="1.3" opacity=".6" />
        <path :d="input.glyph" stroke="var(--cafe-accent, #865630)" stroke-width="1.5" />
        <path class="input-content" :d="input.lines" pathLength="100" stroke="var(--cafe-muted, #756456)" stroke-width="1.6" opacity=".55" />
        <path d="M-63 31h34m8 0h15" stroke="var(--cafe-line, #e4d9ca)" stroke-width="2" />
      </g>
    </g>

    <g :transform="`translate(${center.x} ${center.y})`" data-detail="system-structure">
      <g class="foundation-layer">
        <rect x="-168" y="-73" width="336" height="216" rx="20" fill="var(--cafe-ink, #382a20)" opacity=".025" />
        <rect x="-174" y="-84" width="348" height="218" rx="18" :fill="`url(#${id('glass')})`" :stroke="`url(#${id('edge')})`" />
        <path d="M-151 111h302m-280 10h32m174 0h32" stroke="var(--cafe-line, #e4d9ca)" />
      </g>
      <g class="relation-layer">
        <rect x="-179" y="-105" width="358" height="216" rx="16" :fill="`url(#${id('glass')})`" :stroke="`url(#${id('edge')})`" />
        <path d="M-156-83h23m7 0h6M136-83h21M-156 88h35m204 0h74" stroke="var(--cafe-muted, #756456)" opacity=".45" />
        <path :d="secondaryRoutes" class="secondary-routes" stroke="var(--cafe-muted, #756456)" stroke-width="1" />
        <path :d="structureRoutes" class="structure-routes" stroke="var(--cafe-muted, #756456)" stroke-width="1.1" />
        <!-- Relationships unfold between anchored nodes. Fine paths supply depth;
             three travelling accents reveal associations instead of moving ports. -->
        <g class="association-field">
          <g v-for="(branch, index) in associations" :key="branch.key" class="association-branch" :style="{ '--branch-delay': `${index * .42}s`, '--flow-delay': `${index * -.73}s` }">
            <path class="association-trace" :d="branch.path" pathLength="100" stroke="var(--cafe-muted, #756456)" stroke-width=".9" />
            <path class="association-signal" :d="branch.path" pathLength="100" stroke="var(--cafe-accent, #865630)" stroke-width="1.65" />
          </g>
          <path class="association-bridges" :d="associationBridges" pathLength="100" stroke="var(--cafe-muted, #756456)" stroke-width=".75" />
          <g class="association-nodes" fill="var(--cafe-accent, #865630)">
            <circle v-for="(node, index) in associationNodes" :key="`${node.x}:${node.y}`" :cx="node.x" :cy="node.y" r="2.1" :style="{ '--spark-delay': `${index * -.29}s` }" />
          </g>
        </g>
      </g>
    </g>

    <!-- Intermediate alternatives occupy the result area only during association,
         then give way to one structured outcome. These are abstract motifs. -->
    <g class="candidate-field">
      <g v-for="(candidate, index) in candidates" :key="index" :style="{ '--choice-delay': `${index * -1.1}s` }">
        <path class="candidate-link" :d="candidate.link" stroke="var(--cafe-muted, #756456)" stroke-width="1" />
        <path class="candidate-signal" :d="candidate.link" pathLength="100" stroke="var(--cafe-accent, #865630)" stroke-width="1.5" />
        <g class="candidate-card" :transform="`translate(${candidate.x} ${candidate.y})`">
          <rect x="-52" y="-33" width="104" height="66" rx="10" :fill="`url(#${id('glass')})`" :stroke="`url(#${id('edge')})`" />
          <path d="M-35-21h14m4 0h5M27-21h7" stroke="var(--cafe-muted, #756456)" opacity=".4" />
          <path :d="candidate.glyph" stroke="var(--cafe-accent, #865630)" stroke-width="1.2" />
        </g>
      </g>
    </g>

    <g :transform="`translate(${output.x} ${output.y})`" data-detail="resolved-structure">
      <g class="solution-card">
        <rect x="-95" y="-65" width="194" height="138" rx="14" fill="var(--cafe-ink, #382a20)" opacity=".04" />
        <rect x="-98" y="-72" width="196" height="140" rx="14" :fill="`url(#${id('glass')})`" :stroke="`url(#${id('edge')})`" />
        <rect class="solution-outline" x="-98" y="-72" width="196" height="140" rx="14" stroke="var(--cafe-accent, #865630)" />
        <path d="M-76-49h28m5 0h8M62-49h12" stroke="var(--cafe-muted, #756456)" stroke-width="1.4" opacity=".55" />
        <path class="solution-tree" d="M-64-20V34m0-41h20m-20 39h20M-28-7H24M-28 32H37" pathLength="100" stroke="var(--cafe-accent, #865630)" stroke-width="1.4" />
        <path d="M-28 3H53m-81 39H23" stroke="var(--cafe-muted, #756456)" opacity=".45" />
        <rect x="-69" y="-30" width="10" height="10" rx="2" fill="var(--cafe-accent, #865630)" />
        <path d="M54-16h21v21H54Z" stroke="var(--cafe-accent, #865630)" stroke-opacity=".35" />
        <path d="m59-6 4 4 7-9" class="solution-check" pathLength="100" stroke="var(--cafe-accent, #865630)" stroke-width="1.6" />
      </g>
    </g>

    <g class="source-streams" stroke="var(--cafe-accent, #865630)" stroke-width="1.8">
      <path v-for="(route, index) in sourceRoutes" :key="index" class="source-signal" :d="route" pathLength="100" :style="{ '--flow-delay': `${index * -.8}s` }" />
    </g>

    <!-- One unbroken path, in the same coordinate system as every fixed port.
         Both the reveal and travelling highlight traverse input, core and output. -->
    <path :d="selectedRoute" class="selected-route" pathLength="100" stroke="var(--cafe-accent, #865630)" stroke-width="2" />
    <g :transform="`translate(${center.x} ${center.y})`" class="route-focus-layer">
      <g class="system-nodes" fill="var(--cafe-page, #faf7f2)" stroke="var(--cafe-muted, #756456)">
        <rect v-for="node in nodes" :key="node.key" :x="node.x - 6" :y="node.y - 6" width="12" height="12" rx="3" />
      </g>
      <g class="selected-nodes" fill="var(--cafe-accent, #865630)">
        <rect v-for="(node, index) in selectedNodes" :key="node.key" :class="`route-node-${index}`" :x="node.x - 3" :y="node.y - 3" width="6" height="6" rx="1.5" />
      </g>
      <g class="relation-focus" transform="translate(0 5)">
        <circle class="focus-aura" r="60" :fill="`url(#${id('focus')})`" />
        <circle class="focus-ring" r="33" stroke="var(--cafe-accent, #865630)" stroke-width="1.3" />
        <circle r="25" stroke="var(--cafe-accent, #865630)" stroke-opacity=".2" />
        <g class="focus-response" stroke="var(--cafe-accent, #865630)">
          <circle class="focus-echo" r="35" />
          <path class="focus-orbit" d="M0-33A33 33 0 0 1 33 0M0 33A33 33 0 0 1-33 0" pathLength="100" stroke-width="2" />
        </g>
        <path class="focus-mark" d="M-13 0 0-13 13 0 0 13ZM0-7 7 0 0 7-7 0Z" stroke="var(--cafe-accent, #865630)" stroke-width="1.5" />
      </g>
    </g>
    <path :d="selectedRoute" class="route-signal" pathLength="100" stroke="var(--cafe-ink, #382a20)" stroke-width="3" />
  </svg>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance } from 'vue'

const props = defineProps<{ compact: boolean }>()
const uid = getCurrentInstance()!.uid
const id = (part: string) => `intelligence-system-${uid}-${part}`
const center = computed(() => props.compact ? { x: 300, y: 345 } : { x: 622, y: 245 })
const output = computed(() => props.compact ? { x: 300, y: 575 } : { x: 1025, y: 250 })
const inputs = computed(() => [
  { kind: 'code', x: props.compact ? 105 : 170, y: props.compact ? 120 : 125,
    glyph: 'M-51-12-64 0-51 12m16-24 13 12-13 12m-7-28-9 32', lines: 'M0-9h55M0 2h39M0 13h48' },
  { kind: 'constraints', x: props.compact ? 300 : 265, y: props.compact ? 120 : 275,
    glyph: 'M-62-13h19v19h-19Zm4 8 4 4 7-9M-62 14h19', lines: 'M-25-8h71M-25 3h46M-25 14h60' },
  { kind: 'dependencies', x: props.compact ? 495 : 158, y: props.compact ? 120 : 390,
    glyph: 'M-60-15h12v12h-12Zm0 28h12v12h-12ZM-26-1h12v12h-12ZM-48-9h12v14h10m-22 14h12V5', lines: 'M2-8h53M2 4h38M2 16h47' }
])
const inputLinks = computed(() => props.compact ? [
  'M105 170V197Q105 213 121 213H154Q170 213 170 229V240',
  'M300 170V194Q300 210 284 210H268Q252 210 252 226V240',
  'M495 170V197Q495 213 479 213H382Q366 213 366 229V240'
] : [
  'M254 125H334Q351 125 351 142V175Q351 191 367 191H443',
  'M349 275H398Q416 275 416 257V250H443',
  'M242 390H347Q365 390 365 372V327Q365 309 383 309H443'
])
const sourceRoutes = computed(() => inputLinks.value.map(link => `${link}${props.compact ? `V${center.value.y - 54}` : `H${center.value.x - 130}`}`))
const outputTail = computed(() => props.compact
  ? 'H500Q520 350 520 370V453Q520 477 496 477H323Q300 477 300 497V503'
  : `H${output.value.x - 98}`)
const outputLink = computed(() => `M${center.value.x + 179} ${center.value.y + 5}${outputTail.value}`)
const candidates = computed(() => [
  { x: props.compact ? 210 : 985, y: props.compact ? 555 : 120,
    link: props.compact
      ? 'M479 350H500Q520 350 520 370V470Q520 490 500 490H230Q210 490 210 510V522'
      : 'M801 250H824Q842 250 842 232V142Q842 120 864 120H933',
    glyph: 'M-30-5H-10Q0-5 0 5V18M0 5H30M-30 18H-15M-34-8h6v6h-6ZM-3 15h6v6h-6ZM27 2h6v6h-6Z' },
  { x: props.compact ? 390 : 1035, y: props.compact ? 555 : 385,
    link: props.compact
      ? 'M479 350H500Q520 350 520 370V470Q520 490 500 490H410Q390 490 390 510V522'
      : 'M801 250H837Q859 250 859 272V363Q859 385 881 385H983',
    glyph: 'M-30 5H-17L-5-7 7 5-5 17-17 5M7 5H30M-34 2h6v6h-6ZM27 2h6v6h-6ZM-5-2 2 5-5 12' }
])
const nodes = [
  { key: 'code', x: -130, y: -54 }, { key: 'constraint', x: -130, y: 5 }, { key: 'context', x: -130, y: 64 },
  { key: 'upper', x: -48, y: -54 }, { key: 'lower', x: -48, y: 64 },
  { key: 'bridge', x: 66, y: -54 }, { key: 'parallel', x: 66, y: 64 }, { key: 'result', x: 134, y: 5 }
]
const selectedNodes = nodes.filter(node => ['code', 'upper', 'result'].includes(node.key))
interface Point { x: number; y: number }
// Static routes through actual shared junctions, not random decorative spaghetti.
// Cubic segments meet each point exactly, including the input and result nodes.
function associationPath(points: readonly Point[]): string {
  return points.map((point, index) => {
    if (index === 0) return `M${point.x} ${point.y}`
    const previous = points[index - 1]
    const middle = (previous.x + point.x) / 2
    return `C${middle} ${previous.y} ${middle} ${point.y} ${point.x} ${point.y}`
  }).join('')
}
const associations = [
  { key: 'context', points: [nodes[0], { x: -104, y: -78 }, nodes[3], { x: -14, y: -78 }, { x: 26, y: -74 }, nodes[5], { x: 104, y: -22 }, nodes[7]] },
  { key: 'relations', points: [nodes[1], { x: -96, y: -26 }, nodes[3], { x: -16, y: -37 }, { x: 0, y: 5 }, { x: 45, y: -28 }, nodes[5], { x: 100, y: -76 }, nodes[7]] },
  { key: 'synthesis', points: [nodes[2], { x: -105, y: 34 }, nodes[4], { x: -20, y: 43 }, { x: 0, y: 5 }, { x: 40, y: 31 }, nodes[6], { x: 108, y: 84 }, nodes[7]] }
].map(branch => ({ ...branch, path: associationPath(branch.points) }))
const associationNodes = [...new Map(associations.flatMap(branch => branch.points)
  .filter(point => !nodes.some(node => node.x === point.x && node.y === point.y) && !(point.x === 0 && point.y === 5))
  .map(point => [`${point.x}:${point.y}`, point])).values()]
const associationBridges = [
  [{ x: -104, y: -78 }, { x: -96, y: -26 }, { x: -105, y: 34 }],
  [{ x: -14, y: -78 }, { x: -16, y: -37 }, { x: -20, y: 43 }],
  [{ x: 26, y: -74 }, { x: 45, y: -28 }, { x: 40, y: 31 }],
  [{ x: 100, y: -76 }, { x: 104, y: -22 }, { x: 108, y: 84 }]
].map(associationPath).join('')
const structureRoutes = computed(() => (props.compact
  ? 'M-130-105V-54H-48M-48-105V-54M66-105V-54M-130 5H-28M-130 64H-48'
  : 'M-179-54H-48M-179 5H-28M-179 64H-48') + 'M-48-54V-19Q-48 5-28 5M-48 64V27Q-48 5-28 5M-28 5H179M66-54H102Q134-54 134-22V5M66 64H102Q134 64 134 32V5')
const secondaryRoutes = 'M-130-54V64M-48-54H66M-48 64H66M66-54V64'
const selectedRoute = computed(() => {
  const { x, y } = center.value
  const entry = props.compact ? `V${y - 54}H${x - 48}` : `H${x - 48}`
  return `${inputLinks.value[0]}${entry}V${y - 19}Q${x - 48} ${y + 5} ${x - 28} ${y + 5}H${x + 179}${outputTail.value}`
})
</script>

<style scoped>
.intelligence-scene { display: block; width: 100%; height: 100%; contain: paint; background: var(--cafe-page, #faf7f2); }
/* A 14s editorial envelope contains quicker, offset local rhythms. Wired geometry
   stays anchored: only content, signals and non-wired focus ornaments move. */
.context-links, .output-link { opacity: .3; }
.structure-routes { opacity: .32; }
.secondary-routes { opacity: .13; }
.system-nodes { stroke-opacity: .45; }
.relation-layer { animation: network-emerge var(--intelligence-duration, 14s) ease-in-out infinite; }
.input-card { animation: input-emphasis var(--intelligence-duration, 14s) ease-in-out infinite; }
.input-outline { opacity: 0; animation: input-focus var(--intelligence-duration, 14s) ease-in-out infinite; animation-delay: var(--source-delay); }
.input-content { stroke-dasharray: 100; animation: context-read var(--intelligence-duration, 14s) ease-in-out infinite; animation-delay: var(--source-delay); }
.source-streams { animation: sources-active var(--intelligence-duration, 14s) ease-in-out infinite; }
.source-signal { stroke-dasharray: 12 100; animation: source-flow 2.4s linear infinite; animation-delay: var(--flow-delay); }
.association-field { animation: associations-active var(--intelligence-duration, 14s) ease-in-out infinite; }
.association-trace { stroke-dasharray: 100; animation: association-unfold var(--intelligence-duration, 14s) ease-in-out infinite; animation-delay: var(--branch-delay); }
.association-signal { stroke-dasharray: 9 105; animation: association-flow 3.1s linear infinite; animation-delay: var(--flow-delay); }
.association-bridges { stroke-dasharray: 100; animation: bridges-unfold var(--intelligence-duration, 14s) ease-in-out infinite; }
.association-nodes circle { animation: node-spark 2.7s ease-in-out infinite; animation-delay: var(--spark-delay); }
.candidate-field { animation: candidates-active var(--intelligence-duration, 14s) ease-in-out infinite; }
.candidate-link { opacity: .3; }
.candidate-signal { stroke-dasharray: 7 107; animation: candidate-flow 3.4s linear infinite alternate; animation-delay: var(--choice-delay); }
.selected-route { stroke-dasharray: 100; animation: route-resolve var(--intelligence-duration, 14s) linear infinite; }
.route-signal { stroke-dasharray: 2 102; animation: signal-travel var(--intelligence-duration, 14s) linear infinite; }
.selected-nodes rect { opacity: 0; }
.route-node-0 { animation: entry-focus var(--intelligence-duration, 14s) ease-in-out infinite; }
.route-node-1 { animation: junction-focus var(--intelligence-duration, 14s) ease-in-out infinite; }
.route-node-2 { animation: exit-focus var(--intelligence-duration, 14s) ease-in-out infinite; }
.focus-aura { transform-origin: 0 0; animation: core-breathe var(--intelligence-duration, 14s) ease-in-out infinite; }
.focus-ring, .focus-mark { animation: core-focus var(--intelligence-duration, 14s) ease-in-out infinite; }
.focus-response { animation: core-active var(--intelligence-duration, 14s) ease-in-out infinite; }
.focus-echo { transform-origin: 0 0; animation: core-echo 1.9s ease-out infinite; }
.focus-orbit { transform-origin: 0 0; animation: core-orbit 6.4s linear infinite; }
.solution-card { animation: solution-arrive var(--intelligence-duration, 14s) ease-in-out infinite; }
.output-link { animation: output-emerge var(--intelligence-duration, 14s) ease-in-out infinite; }
.solution-outline { animation: result-focus var(--intelligence-duration, 14s) ease-in-out infinite; }
.solution-tree { stroke-dasharray: 100; animation: solution-organize var(--intelligence-duration, 14s) ease-in-out infinite; }
.solution-check { stroke-dasharray: 100; animation: check-resolve var(--intelligence-duration, 14s) ease-in-out infinite; }
@keyframes input-emphasis { 0%, 100% { opacity: .7; } 6%, 24% { opacity: 1; } 42%, 98% { opacity: .48; } }
@keyframes input-focus { 0%, 3%, 33%, 100% { opacity: 0; } 8%, 23% { opacity: .75; } }
@keyframes context-read { 0%, 100% { stroke-dashoffset: 100; } 12%, 98% { stroke-dashoffset: 0; } }
@keyframes sources-active { 0%, 38%, 100% { opacity: 0; } 4%, 25% { opacity: .9; } }
@keyframes source-flow { from { stroke-dashoffset: 12; } to { stroke-dashoffset: -100; } }
@keyframes network-emerge { 0%, 100% { opacity: .2; } 24%, 98% { opacity: 1; } }
@keyframes associations-active {
  0%, 15%, 78%, 100% { opacity: 0; }
  25%, 52% { opacity: 1; }
  65% { opacity: .14; }
}
@keyframes association-unfold { 0%, 16%, 100% { stroke-dashoffset: 100; opacity: 0; } 34%, 76% { stroke-dashoffset: 0; opacity: .42; } }
@keyframes bridges-unfold { 0%, 25%, 100% { stroke-dashoffset: 100; opacity: 0; } 43%, 76% { stroke-dashoffset: 0; opacity: .23; } }
@keyframes association-flow { from { stroke-dashoffset: 9; } to { stroke-dashoffset: -105; } }
@keyframes node-spark { 0%, 100% { opacity: .25; } 32% { opacity: 1; } 54% { opacity: .45; } }
@keyframes candidates-active { 0%, 24%, 63%, 100% { opacity: 0; } 33%, 49% { opacity: .85; } }
@keyframes candidate-flow { from { stroke-dashoffset: 7; } to { stroke-dashoffset: -107; } }
@keyframes core-active { 0%, 19%, 72%, 100% { opacity: 0; } 28%, 54% { opacity: .9; } }
@keyframes core-echo { from { transform: scale(.9); opacity: .55; } to { transform: scale(1.6); opacity: 0; } }
@keyframes core-orbit { to { transform: rotate(360deg); } }
@keyframes route-resolve {
  0%, 50% { stroke-dashoffset: 100; opacity: 0; }
  52% { stroke-dashoffset: 91; opacity: .9; }
  72%, 98% { stroke-dashoffset: 0; opacity: .9; }
  99%, 100% { stroke-dashoffset: 0; opacity: 0; }
}
@keyframes signal-travel {
  0%, 49% { stroke-dashoffset: 2; opacity: 0; }
  50% { stroke-dashoffset: 0; opacity: 1; }
  72% { stroke-dashoffset: -100; opacity: 1; }
  74% { stroke-dashoffset: -102; opacity: 0; }
  75% { stroke-dashoffset: 2; opacity: 0; }
  76% { stroke-dashoffset: 0; opacity: .8; }
  94% { stroke-dashoffset: -100; opacity: .8; }
  96%, 100% { stroke-dashoffset: -102; opacity: 0; }
}
@keyframes entry-focus { 0%, 48%, 100% { opacity: 0; } 56%, 98% { opacity: 1; } }
@keyframes junction-focus { 0%, 54%, 100% { opacity: 0; } 60%, 98% { opacity: 1; } }
@keyframes exit-focus { 0%, 60%, 100% { opacity: 0; } 68%, 98% { opacity: 1; } }
@keyframes core-breathe {
  0%, 20%, 100% { opacity: .12; transform: scale(.85); }
  32% { opacity: .85; transform: scale(1); }
  43% { opacity: 1; transform: scale(1.12); }
  56%, 98% { opacity: .25; transform: scale(.94); }
}
@keyframes core-focus { 0%, 19%, 100% { opacity: .4; } 29%, 49% { opacity: 1; } 65%, 98% { opacity: .6; } }
@keyframes solution-arrive { 0%, 52%, 100% { opacity: 0; } 70%, 98% { opacity: 1; } }
@keyframes output-emerge { 0%, 50%, 100% { opacity: 0; } 63%, 98% { opacity: .3; } }
@keyframes result-focus { 0%, 56%, 100% { opacity: 0; } 72%, 98% { opacity: .8; } }
@keyframes solution-organize { 0%, 58%, 100% { stroke-dashoffset: 100; } 73%, 98% { stroke-dashoffset: 0; } }
@keyframes check-resolve { 0%, 68%, 100% { stroke-dashoffset: 100; opacity: 0; } 75%, 98% { stroke-dashoffset: 0; opacity: 1; } }
@media (prefers-reduced-motion: reduce) {
  .intelligence-scene * { animation: none !important; }
  .selected-nodes rect { opacity: 1; }
  .focus-aura { opacity: .35; }
  .solution-outline { opacity: .6; }
  .association-field { opacity: .35; }
  .association-trace { opacity: .4; }
  .association-bridges { opacity: .2; }
  .input-content, .solution-tree { stroke-dashoffset: 0; }
  .route-signal, .source-streams, .association-signal, .focus-response, .candidate-field { display: none; }
}
</style>
