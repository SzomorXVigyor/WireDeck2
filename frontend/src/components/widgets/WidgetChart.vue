<script setup lang="ts">
import { computed } from 'vue'
import type { LivePoint, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  points: LivePoint[]
}>()

const quality = computed(() => props.sample?.quality ?? 'STALE')

const chartData = computed(() => {
  const pts = props.points || []
  if (pts.length === 0) {
    return { path: '', min: 0, max: 0, last: null as number | null }
  }
  let min = pts[0].value
  let max = pts[0].value
  for (const p of pts) {
    if (p.value < min) min = p.value
    if (p.value > max) max = p.value
  }
  const span = max - min || 1
  const w = 320
  const h = 96
  const pad = 6
  const coords = pts.map((p, idx) => {
    const x = pts.length === 1 ? w / 2 : (idx / (pts.length - 1)) * w
    const y = h - pad - ((p.value - min) / span) * (h - pad * 2)
    return `${idx === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
  })
  return {
    path: coords.join(' '),
    min,
    max,
    last: pts[pts.length - 1].value,
  }
})
</script>

<template>
  <div
    class="flex flex-col justify-between rounded-xl border p-4 sm:col-span-2"
    :class="
      quality === 'BAD'
        ? 'border-rose-500/40 bg-rose-950/20'
        : quality === 'STALE'
          ? 'border-amber-500/30 bg-slate-900/60'
          : 'border-slate-800 bg-slate-900/75'
    "
  >
    <div class="flex items-center justify-between gap-2">
      <div class="min-w-0">
        <p class="truncate text-xs font-medium text-slate-300">{{ widget.name }}</p>
        <p class="text-[10px] text-slate-500">Live buffer ({{ points.length }} / 900 pts)</p>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="chartData.last !== null" class="font-mono text-sm font-semibold text-white">
          {{ Number.isInteger(chartData.last) ? chartData.last : chartData.last.toFixed(2) }}
          <span v-if="mapping?.unit" class="text-xs font-normal text-slate-400">{{ mapping.unit }}</span>
        </span>
        <span
          v-if="quality !== 'GOOD'"
          class="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase"
          :class="quality === 'BAD' ? 'bg-rose-500/20 text-rose-300' : 'bg-amber-500/20 text-amber-300'"
        >
          {{ quality }}
        </span>
      </div>
    </div>

    <div class="my-2 h-24 w-full rounded-lg border border-slate-800/80 bg-slate-950/70 p-1">
      <svg v-if="chartData.path" viewBox="0 0 320 96" preserveAspectRatio="none" class="h-full w-full">
        <path
          :d="chartData.path"
          fill="none"
          stroke="#38bdf8"
          stroke-width="2"
          vector-effect="non-scaling-stroke"
        />
      </svg>
      <div v-else class="flex h-full items-center justify-center text-[11px] text-slate-600">
        Waiting for live samples...
      </div>
    </div>

    <div class="flex items-center justify-between font-mono text-[10px] text-slate-500">
      <span>min: {{ points.length ? chartData.min.toFixed(2) : '--' }}</span>
      <span>max: {{ points.length ? chartData.max.toFixed(2) : '--' }}</span>
    </div>
  </div>
</template>
