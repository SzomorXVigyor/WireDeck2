<script setup lang="ts">
import { computed } from 'vue'
import type { RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
}>()

const quality = computed(() => props.sample?.quality ?? 'STALE')
const decimals = computed(() => {
  const d = Number(props.widget.extra?.decimals)
  return Number.isInteger(d) && d >= 0 && d <= 6 ? d : 2
})
const formatted = computed(() => {
  if (!props.sample || props.sample.value === null || !Number.isFinite(props.sample.value)) {
    return '--'
  }
  const v = props.sample.value
  return Number.isInteger(v) ? String(v) : v.toFixed(decimals.value)
})
</script>

<template>
  <div
    class="flex flex-col justify-between rounded-xl border p-4 transition"
    :class="
      quality === 'BAD'
        ? 'border-rose-500/40 bg-rose-950/20'
        : quality === 'STALE'
          ? 'border-amber-500/30 bg-slate-900/60'
          : 'border-slate-800 bg-slate-900/75'
    "
  >
    <div class="flex items-start justify-between gap-2">
      <p class="truncate text-xs font-medium text-slate-300">{{ widget.name }}</p>
      <span
        v-if="quality !== 'GOOD'"
        class="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase"
        :class="quality === 'BAD' ? 'bg-rose-500/20 text-rose-300' : 'bg-amber-500/20 text-amber-300'"
        :title="sample?.error || 'Waiting for fresh telemetry'"
      >
        {{ quality }}
      </span>
    </div>

    <div class="my-3 flex items-baseline gap-1.5">
      <span class="font-mono text-3xl font-bold tracking-tight text-white">{{ formatted }}</span>
      <span v-if="mapping?.unit" class="text-xs font-medium text-slate-400">{{ mapping.unit }}</span>
    </div>

    <div class="flex items-center justify-between text-[10px] text-slate-500">
      <span class="truncate font-mono">{{ mapping ? mapping.name : 'Unbound' }}</span>
      <span v-if="sample?.timestamp" class="shrink-0">
        {{ new Date(sample.timestamp).toLocaleTimeString() }}
      </span>
    </div>
  </div>
</template>
