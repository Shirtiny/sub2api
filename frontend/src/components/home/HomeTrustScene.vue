<template>
  <svg
    class="feature-scene trust-scene" data-scene="trust"
    :data-layout="compact ? 'portrait' : 'landscape'"
    :viewBox="compact ? '0 0 600 720' : '0 0 1200 520'"
    preserveAspectRatio="xMidYMid meet"
    fill="none" stroke="currentColor" stroke-width="1.4"
    stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"
  >
    <defs>
      <linearGradient :id="id('paper')" x1="0" y1="0" x2="1" y2="1">
        <stop stop-color="var(--cafe-page, #faf7f2)" />
        <stop offset="1" stop-color="var(--cafe-surface, #f2ece3)" />
      </linearGradient>
      <clipPath :id="id('history')" clipPathUnits="userSpaceOnUse">
        <rect class="history-reveal" x="-142" y="-92" width="284" height="210" />
      </clipPath>
      <clipPath :id="id('print')" clipPathUnits="userSpaceOnUse">
        <rect x="-140" y="-172" width="280" height="326" />
      </clipPath>
      <clipPath v-for="record in records" :id="id(`line-${record.key}`)" :key="record.key" clipPathUnits="userSpaceOnUse">
        <rect class="line-ink" x="-114" y="-22" width="228" height="44" :style="recordStyle(record)" />
      </clipPath>
      <g :id="id('model')" class="model-mark">
        <rect x="-9" y="-9" width="18" height="18" rx="4" />
        <path d="M-4-13v4m8-4v4M-4 9v4m8-4v4M-13-4h4m-4 8h4M9-4h4m-4 8h4" />
        <path d="m-3-3-3 3 3 3m6-6 3 3-3 3" />
      </g>
      <g :id="id('request')">
        <path d="M-8-12H3l7 7v17H-8Zm11 0v7h7M-4 1H5M-4 6H2" />
      </g>
      <g v-for="record in records" :id="id(record.key)" :key="record.key" class="request-fingerprint">
        <rect v-for="(piece, index) in record.fingerprint" :key="index" :x="piece.x" y="-3" :width="piece.width" height="6" rx="2" />
      </g>
    </defs>

    <!-- Three separate requests, not three token categories of one request. -->
    <g :transform="anchor(layout.request)" data-detail="request-window">
      <rect class="soft-shadow" x="-145" y="-129" width="300" height="280" rx="18" />
      <rect class="window-face" x="-150" y="-135" width="300" height="280" rx="18" :fill="paper" />
      <path class="hairline" d="M-150-99H150" />
      <circle v-for="x in [-126, -114, -102]" :key="x" :cx="x" cy="-117" r="2.5" class="window-dot" />
      <path class="window-caption" d="M-50-119H37M-50-112H10" />
      <use :href="`#${id('request')}`" transform="translate(123 -117) scale(.65)" class="quiet-mark" />
      <g :clip-path="`url(#${id('history')})`">
        <g v-for="record in records" :key="record.key" :data-source-request="record.key" :transform="`translate(0 ${record.sourceY})`" :style="recordStyle(record)">
          <rect class="source-highlight" x="-138" y="-25" width="276" height="52" rx="9" />
          <use :href="`#${id('request')}`" x="-117" class="request-icon" />
          <use :href="`#${id(record.key)}`" x="-87" y="-5" data-field="request_id" />
          <path class="request-metadata" :d="`M-87 8h${record.metadataSpan}`" />
          <use :href="`#${id('model')}`" transform="translate(73 -2) scale(.62)" class="quiet-mark" data-field="model" />
          <path class="source-guide" d="M-87 19H106q6 0 6-6V0" />
        </g>
      </g>
    </g>

    <g data-detail="connected-billing-route">
      <path class="connection-track" :d="routes.incoming" />
      <path class="connection-track" :d="routes.outgoing" />
      <g v-for="record in records" :key="record.key" :data-connection="record.key" :style="recordStyle(record)">
        <path class="source-branch" :d="branch(record)" />
        <path class="request-signal" :d="branch(record) + routes.incomingTail" pathLength="1" />
        <path class="charge-signal" :d="routes.outgoing" pathLength="1" />
      </g>
      <circle class="connection-port" :cx="routes.start.x" :cy="routes.start.y" r="3.5" :fill="paper" />
    </g>

    <!-- One shared calculation surface. Its active request identity is carried
         unchanged from history through calculation into exactly one usage row. -->
    <g :transform="anchor(layout.billing)" data-detail="request-billing">
      <rect class="soft-shadow" x="-149" y="-144" width="308" height="300" rx="18" />
      <rect class="billing-face" x="-154" y="-150" width="308" height="300" rx="18" :fill="paper" />
      <path class="hairline" d="M-154-83H154M-136 76H136" />
      <path class="billing-inlet" d="M-154-121v12" />
      <path class="billing-outlet" d="M134 106H154m0-6v12" />
      <g v-for="kind in categories" :key="kind.key" :transform="`translate(0 ${kind.y})`" class="calculation-guide">
        <path :d="kind.icon" transform="translate(-126 0) scale(.75)" />
        <path class="quantity-track" d="M-101 0H-29" />
        <path class="math-sign" d="m-20-4 8 8m-8 0 8-8M36-3h7m-7 6h7" />
        <path class="term-track" d="M56 0H111" />
      </g>
      <path class="cost-collector" d="M121-45h5q7 0 7 7V57q0 7-7 7H-114q-8 0-8 8v17M121-1h12M121 43h12" />
      <path class="sum-mark" d="M-125 96h11l-8 10 8 10h-11" />
      <path class="math-sign" d="m-37 102 8 8m-8 0 8-8M41 103h8m-8 6h8" />

      <g v-for="record in records" :key="record.key" :data-billing-request="record.key" :style="recordStyle(record)" class="billing-run">
        <g class="model-lookup" data-stage="model-pricing">
          <rect class="lookup-highlight" x="-141" y="-136" width="281" height="42" rx="9" />
          <use :href="`#${id('model')}`" x="-117" y="-115" data-field="model" />
          <use :href="`#${id(record.key)}`" x="-87" y="-115" data-field="request_id" />
          <path class="price-link" d="M-24-115H70m-5-4 5 4-5 4" />
          <g transform="translate(101 -115)" class="model-price-card">
            <rect x="-22" y="-15" width="44" height="30" rx="5" />
            <path d="M-12-5H2M-12 2H-2M-12 8H5" />
            <circle cx="12" cy="-4" r="4" />
          </g>
        </g>
        <g v-for="term in record.terms" :key="term.key" :data-term="term.key" :transform="`translate(0 ${term.y})`">
          <g transform="translate(-101 0)" :data-field="term.field">
            <!-- Cache is partitioned inside THIS request, never a separate request. -->
            <g :class="term.key === 'cache' ? 'cache-partition' : 'quantity-build'">
              <rect class="quantity-piece" :class="{ 'cached-piece': term.key === 'cache' }" y="-5" :width="term.span" height="10" rx="2.5" />
            </g>
          </g>
          <g transform="translate(12 0)" class="unit-price" data-stage="unit-price">
            <path :d="`M-8 ${-term.priceDepth / 2}v${term.priceDepth}c0 4 16 4 16 0v-${term.priceDepth}`" />
            <ellipse :cy="-term.priceDepth / 2" rx="8" ry="3" />
          </g>
          <g transform="translate(56 0)" :data-field="term.costField">
            <rect class="term-cost" y="-3.5" :width="term.costSpan" height="7" rx="2" />
          </g>
        </g>
        <g transform="translate(-107 106)" data-field="total_cost">
          <rect class="subtotal-meter" y="-3.5" :width="record.totalSpan" height="7" rx="2" />
        </g>
        <g class="rate-factor" data-field="rate_multiplier">
          <rect class="factor-face" x="-14" y="89" width="45" height="34" rx="7" />
          <path d="m-2 114 18-16" />
          <circle cx="-1" cy="100" r="3" />
          <circle cx="14" cy="112" r="3" />
        </g>
        <g class="calculated-charge" data-field="actual_cost">
          <g transform="translate(60 106)">
            <rect class="actual-meter" y="-4" :width="record.totalSpan" height="8" rx="2.5" />
          </g>
          <path class="cost-coin" d="M119 103v6c0 4 14 4 14 0v-6M119 103c0-4 14-4 14 0s-14 4-14 0Z" />
        </g>
      </g>
    </g>

    <g :transform="anchor(layout.receipt)" data-detail="usage-receipt">
      <g :clip-path="`url(#${id('print')})`">
        <g class="paper-sheet">
          <path class="soft-shadow" :d="receiptPath" transform="translate(4 6)" />
          <path class="receipt-paper" :d="receiptPath" :fill="paper" />
          <use :href="`#${id('request')}`" x="-96" y="-126" class="quiet-mark" />
          <path class="receipt-heading" d="M-68-131H44M-68-121H9" />
          <path class="perforation" d="M-108-97H108M-108 98H108" />
          <!-- UsageLog fields, including this request's already-adjusted cost.
               There is no extra deduction or global multiplier on the receipt. -->
          <g v-for="record in records" :key="record.key" :data-usage-record="record.key" :transform="`translate(0 ${record.receiptY})`" :clip-path="`url(#${id(`line-${record.key}`)})`" class="receipt-entry">
            <use :href="`#${id('model')}`" transform="translate(-99 -3) scale(.7)" class="quiet-mark" data-field="model" />
            <use :href="`#${id(record.key)}`" x="-74" y="-5" data-field="request_id" />
            <g transform="translate(-74 14)" data-field="usage">
              <rect v-for="term in record.terms" :key="term.key" :data-field="term.field" class="record-usage" :class="{ 'cached-piece': term.key === 'cache' }" :x="term.index * 25" y="-2" :width="term.span * .45" height="4" rx="1" />
            </g>
            <g transform="translate(34 -5)" class="entry-cost" data-field="actual_cost">
              <rect class="record-charge" y="-4" :width="record.actualSpan" height="8" rx="2.5" />
              <path class="cost-coin" d="M58-2v6c0 4 14 4 14 0v-6M58-2c0-4 14-4 14 0s-14 4-14 0Z" />
            </g>
            <path class="record-divider" d="M-108 21H108" />
          </g>
          <g class="receipt-audit" data-detail="records-reconciled">
            <path class="audit-trace" d="M-101 118v15m5-15v15m8-15v15m4-15v15m10-15v15m5-15v15m5-15v15m8-15v15M-39 121H39M-39 130H9" />
            <path class="verify-check" d="m69 124 7 7 15-16" pathLength="1" />
          </g>
        </g>
      </g>
      <rect class="printer-shadow" x="-145" y="161" width="300" height="33" rx="12" />
      <rect class="printer-back" x="-150" y="154" width="300" height="34" rx="12" />
      <path class="printer-slot" d="M-128 155H128" />
      <path class="print-head" d="M-116 155H116" />
      <path class="printer-seam" d="M-117 175H83" />
      <circle class="printer-light" cx="118" cy="175" r="2.5" />
      <path class="printer-inlet" :d="compact ? 'M150 168v12' : 'M-150 168v12'" />
    </g>
  </svg>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance } from 'vue'

const props = defineProps<{ compact: boolean }>()
const uid = getCurrentInstance()!.uid
const id = (part: string) => `everyday-trust-${uid}-${part}`
const paper = `url(#${id('paper')})`
// Drawing units only, not real token counts, model prices or account multipliers.
// Field roles follow UsageLog / UsageView: sum category costs, apply the
// effective request multiplier once, then record actual_cost on that request.
const categories = [
  { key: 'input', field: 'input_tokens', costField: 'input_cost', weight: 1, priceDepth: 6, icon: 'M0-12V3m-5-5 5 5 5-5M-12 3v8h24V3' },
  { key: 'cache', field: 'cache_read_tokens', costField: 'cache_read_cost', weight: .2, priceDepth: 2, icon: 'M-3-7v-4h-10V2h4M-8-6H7a3 3 0 0 1 3 3V9H-8Z' },
  { key: 'output', field: 'output_tokens', costField: 'output_cost', weight: 2, priceDepth: 10, icon: 'M0 12V-3m-5 5 5-5 5 5M-12-3v-8h24v8' }
].map((kind, index) => ({ ...kind, index, y: -45 + index * 44 }))
const records = [
  { key: 'request-01', fingerprint: [28, 14], spans: [28, 22, 22], rateScale: .6, metadataSpan: 84 },
  { key: 'request-02', fingerprint: [15, 29], spans: [38, 16, 30], rateScale: .75, metadataSpan: 60 },
  { key: 'request-03', fingerprint: [22, 22], spans: [22, 30, 18], rateScale: .5, metadataSpan: 100 }
].map((record, index) => {
  const terms = categories.map(kind => ({ ...kind, span: record.spans[kind.index], costSpan: record.spans[kind.index] * kind.weight * .6 }))
  const totalSpan = terms.reduce((sum, term) => sum + term.costSpan, 0)
  let x = 0
  const fingerprint = record.fingerprint.map(width => { const piece = { x, width }; x += width + 6; return piece })
  return { ...record, index, fingerprint, terms, totalSpan, actualSpan: totalSpan * record.rateScale, sourceY: -64 + index * 70, receiptY: -62 + index * 62 }
})
type Anchor = { x: number; y: number; scale: number }
const anchor = (point: Anchor) => `translate(${point.x} ${point.y}) scale(${point.scale})`
const at = (point: Anchor, x: number, y: number) => ({ x: point.x + x * point.scale, y: point.y + y * point.scale })
const layout = computed(() => props.compact
  ? { request: { x: 150, y: 152, scale: .8 }, billing: { x: 444, y: 152, scale: .8 }, receipt: { x: 300, y: 510, scale: 1 } }
  : { request: { x: 205, y: 252, scale: 1 }, billing: { x: 594, y: 252, scale: 1 }, receipt: { x: 1000, y: 252, scale: 1 } })
const receiptPath = 'M-134 164V-160' + Array.from({ length: 16 }, (_, i) => `L${-134 + (i + 1) * 16.75} ${i % 2 ? -160 : -156}`).join('') + 'V164Z'
const recordStyle = (record: typeof records[number]) => ({ '--request-delay': `${record.index * 4000}ms`, '--cache-origin': `${record.spans[0] + 6}px`, '--rate-scale': record.rateScale })
const routes = computed(() => {
  const { request, billing, receipt } = layout.value
  const start = at(request, 150, 6)
  const inlet = at(billing, -154, -115)
  const outlet = at(billing, 154, 106)
  const printer = at(receipt, props.compact ? 150 : -150, 174)
  const gap = (inlet.x - start.x) / 2
  const incomingTail = `C${start.x + gap} ${start.y} ${inlet.x - gap} ${inlet.y} ${inlet.x} ${inlet.y}`
  const outgoing = props.compact
    ? `M${outlet.x} ${outlet.y}H572Q580 ${outlet.y} 580 ${outlet.y + 8}V${printer.y - 8}Q580 ${printer.y} 572 ${printer.y}H${printer.x}`
    : `M${outlet.x} ${outlet.y}C${outlet.x + 42} ${outlet.y} ${printer.x - 42} ${printer.y} ${printer.x} ${printer.y}`
  return { start, incomingTail, incoming: `M${start.x} ${start.y}${incomingTail}`, outgoing }
})
function branch(record: typeof records[number]) {
  const source = at(layout.value.request, 112, record.sourceY)
  const { start } = routes.value
  return `M${source.x} ${source.y}C${source.x + 24} ${source.y} ${start.x - 24} ${start.y} ${start.x} ${start.y}`
}
</script>

<style scoped>
.trust-scene { --trust-cycle: 16s; display: block; width: 100%; height: auto; color: var(--cafe-accent, #865630); }
.soft-shadow, .printer-shadow { fill: currentColor; fill-opacity: .045; stroke: none; }
.window-face, .billing-face { stroke-opacity: .3; }
.hairline { stroke-opacity: .14; }
.window-dot { fill: currentColor; stroke: none; opacity: .25; }
.window-caption, .request-metadata { stroke-width: 3; stroke-opacity: .2; }
.model-mark { stroke-width: 1.3; }
.quiet-mark, .request-icon { opacity: .6; }
.request-fingerprint { fill: currentColor; fill-opacity: .58; stroke: none; }
.source-guide { stroke-opacity: .1; }
.source-highlight { fill: currentColor; stroke: none; animation: source-highlight var(--trust-cycle) var(--request-delay) ease infinite both; }
.history-reveal { transform-box: fill-box; transform-origin: top; animation: history-reveal var(--trust-cycle) ease infinite both; }
.connection-track { stroke-width: 2.5; stroke-opacity: .23; }
.source-branch { stroke-opacity: .2; }
.connection-port, .billing-inlet, .billing-outlet, .printer-inlet { stroke-opacity: .5; }
.billing-inlet, .printer-inlet { stroke-width: 3; }
.request-signal, .charge-signal { stroke-width: 2.5; stroke-dasharray: .12 1.88; }
.request-signal { animation: request-transit var(--trust-cycle) var(--request-delay) linear infinite both; }
.charge-signal { animation: charge-transit var(--trust-cycle) var(--request-delay) linear infinite both; }
.calculation-guide { stroke-opacity: .5; }
.quantity-track, .term-track { stroke-width: 4; stroke-opacity: .12; }
.math-sign, .sum-mark { stroke-opacity: .35; }
.cost-collector { stroke-opacity: .18; }
.billing-run { animation: calculate-request var(--trust-cycle) var(--request-delay) linear infinite both; }
.billing-run:last-child { animation-name: calculate-final; }
.lookup-highlight { fill: currentColor; fill-opacity: .05; stroke-opacity: .15; }
.price-link { stroke-opacity: .25; }
.model-price-card { fill: var(--cafe-page, #faf7f2); stroke-opacity: .6; }
.model-lookup, .unit-price { animation: price-lookup var(--trust-cycle) var(--request-delay) ease infinite both; }
.quantity-piece, .term-cost, .subtotal-meter, .actual-meter, .record-charge, .record-usage { fill: currentColor; fill-opacity: .58; stroke: none; }
.cached-piece { fill-opacity: .12; stroke: currentColor; stroke-opacity: .5; }
.quantity-build { transform-box: fill-box; transform-origin: left; animation: quantity-build var(--trust-cycle) var(--request-delay) ease infinite both; }
.cache-partition { animation: cache-partition var(--trust-cycle) var(--request-delay) cubic-bezier(.4, 0, .2, 1) infinite both; }
.unit-price { fill: var(--cafe-page, #faf7f2); stroke-opacity: .65; }
.term-cost { transform-box: fill-box; transform-origin: left; animation: price-terms var(--trust-cycle) var(--request-delay) ease infinite both; }
.subtotal-meter { transform-box: fill-box; transform-origin: left; animation: sum-terms var(--trust-cycle) var(--request-delay) ease infinite both; }
.rate-factor, .calculated-charge { animation: charge-result var(--trust-cycle) var(--request-delay) ease infinite both; }
.factor-face { fill: var(--cafe-surface, #f2ece3); stroke-opacity: .5; }
.actual-meter { transform-box: fill-box; transform-origin: left; transform: scaleX(var(--rate-scale)); animation: apply-rate var(--trust-cycle) var(--request-delay) ease-in-out infinite both; }
.cost-coin { fill: var(--cafe-page, #faf7f2); stroke-opacity: .65; }
.paper-sheet { animation: paper-feed var(--trust-cycle) cubic-bezier(.4, 0, .2, 1) infinite both; }
.line-ink { transform-box: fill-box; transform-origin: left; animation: line-print var(--trust-cycle) var(--request-delay) linear infinite both; }
.receipt-paper { stroke-opacity: .4; }
.receipt-heading { stroke-width: 3; stroke-opacity: .38; }
.perforation { stroke-dasharray: 2 5; stroke-opacity: .23; }
.record-divider { stroke-opacity: .11; }
.receipt-audit { animation: audit-reveal var(--trust-cycle) ease infinite both; }
.audit-trace { stroke-opacity: .3; }
.verify-check { stroke-width: 2; stroke-dasharray: 1; animation: verify-check var(--trust-cycle) ease infinite both; }
.printer-back { fill: var(--cafe-surface, #f2ece3); stroke-opacity: .18; }
.printer-slot { stroke-width: 4; stroke-opacity: .25; }
.printer-seam { stroke-opacity: .15; }
.printer-light { fill: currentColor; stroke: none; opacity: .35; }
.print-head { stroke-width: 2; animation: print-activity var(--trust-cycle) linear infinite both; }
/* Per request: arrive 4–9, price/cache 10–16, sum/rate 16–20,
   deliver charge 21–24, print one UsageLog row 25–30; repeat +25%. */
@keyframes history-reveal { 0% { opacity: 1; transform: scaleY(0); } 4%, 24% { opacity: 1; transform: scaleY(.3333); } 29%, 49% { opacity: 1; transform: scaleY(.6667); } 54%, 94% { opacity: 1; transform: scaleY(1); } 97% { opacity: 0; transform: scaleY(1); } 98%, 100% { opacity: 0; transform: scaleY(0); } }
@keyframes source-highlight { 0%, 4%, 27%, 100% { opacity: 0; } 7%, 23% { opacity: .065; } }
@keyframes request-transit { 0%, 4% { opacity: 0; stroke-dashoffset: .12; } 5%, 8% { opacity: .85; } 9%, 100% { opacity: 0; stroke-dashoffset: -1; } }
@keyframes charge-transit { 0%, 21% { opacity: 0; stroke-dashoffset: .12; } 22%, 23% { opacity: .85; } 24%, 100% { opacity: 0; stroke-dashoffset: -1; } }
@keyframes calculate-request { 0%, 9%, 28%, 100% { opacity: 0; } 10%, 26% { opacity: 1; } }
@keyframes calculate-final { 0%, 9%, 47%, 100% { opacity: 0; } 10%, 44% { opacity: 1; } }
@keyframes price-lookup { 0%, 10% { opacity: .25; } 12%, 100% { opacity: 1; } }
@keyframes quantity-build { 0%, 10% { transform: scaleX(0); } 12%, 100% { transform: scaleX(1); } }
@keyframes cache-partition { 0%, 10% { opacity: 0; transform: translate(var(--cache-origin), -44px); } 12% { opacity: 1; transform: translate(var(--cache-origin), -44px); } 14%, 100% { opacity: 1; transform: translate(0, 0); } }
@keyframes price-terms { 0%, 14% { transform: scaleX(0); } 16%, 100% { transform: scaleX(1); } }
@keyframes sum-terms { 0%, 16% { transform: scaleX(0); } 17%, 100% { transform: scaleX(1); } }
@keyframes charge-result { 0%, 17% { opacity: 0; } 18%, 100% { opacity: 1; } }
@keyframes apply-rate { 0%, 18% { transform: scaleX(1); } 20%, 100% { transform: scaleX(var(--rate-scale)); } }
@keyframes paper-feed {
  0%, 6% { opacity: 1; transform: translateY(320px); }
  12%, 25% { opacity: 1; transform: translateY(234px); }
  30%, 50% { opacity: 1; transform: translateY(172px); }
  55%, 75% { opacity: 1; transform: translateY(110px); }
  80%, 84% { opacity: 1; transform: translateY(48px); }
  88%, 94% { opacity: 1; transform: translateY(0); }
  97% { opacity: 0; transform: translateY(0); }
  98%, 100% { opacity: 0; transform: translateY(320px); }
}
@keyframes line-print { 0%, 25% { transform: scaleX(0); } 30%, 97% { transform: scaleX(1); } 99%, 100% { transform: scaleX(0); } }
@keyframes print-activity { 0%, 6%, 13%, 24%, 31%, 49%, 56%, 74%, 81%, 83%, 89%, 100% { opacity: 0; } 9%, 11%, 26%, 29%, 51%, 54%, 76%, 79%, 85%, 87% { opacity: .55; } }
@keyframes audit-reveal { 0%, 84% { opacity: 0; } 88%, 100% { opacity: 1; } }
@keyframes verify-check { 0%, 88% { stroke-dashoffset: 1; } 92%, 100% { stroke-dashoffset: 0; } }
@media (prefers-reduced-motion: reduce) {
  .trust-scene * { animation: none !important; }
  .request-signal, .charge-signal, .source-highlight, .print-head, .billing-run:not(:last-child) { display: none; }
}
</style>
