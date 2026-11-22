<script setup lang="ts">
import { computed } from 'vue'
import type { FontSizeOption, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import { clampPrecision, formatPrecisionValue } from '../../utils/precision'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
}>()

const quality = computed(() => props.sample?.quality ?? 'STALE')
const precision = computed(() => {
  const raw = props.widget.extra?.precision
  if (raw !== undefined && raw !== null && raw !== '') return clampPrecision(raw)
  return clampPrecision(props.mapping?.protocolAttributes?.precision ?? 0)
})
const prefix = computed(() => String(props.widget.extra?.prefix || ''))
const unit = computed(() => {
  const raw = props.widget.extra?.unit
  if (raw !== undefined && raw !== null && String(raw).trim() !== '') return String(raw)
  return String(props.mapping?.protocolAttributes?.unit || '')
})
const formatted = computed(() => formatPrecisionValue(props.sample?.value, precision.value))

const fontSizeMap: Record<FontSizeOption, string> = {
  sm: 'text-xl',
  md: 'text-2xl',
  lg: 'text-3xl',
  xl: 'text-4xl',
}
const sizeClass = computed(() => fontSizeMap[(props.widget.style?.fontSize as FontSizeOption) || 'lg'] || 'text-3xl')
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
      <p class="truncate text-xs font-semibold text-slate-200">{{ widget.name }}</p>
      <span
        v-if="quality !== 'GOOD'"
        class="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase"
        :class="quality === 'BAD' ? 'bg-rose-500/20 text-rose-300' : 'bg-amber-500/20 text-amber-300'"
        :title="sample?.error || 'Waiting for fresh telemetry'"
      >
        {{ quality }}
      </span>
    </div>

    <div class="mt-3 mb-1 flex items-baseline gap-1">
      <span v-if="prefix" :class="sizeClass" class="font-bold leading-none text-slate-300">{{ prefix }}</span>
      <span :class="sizeClass" class="font-mono font-bold tabular-nums leading-none tracking-tight text-white">
        {{ formatted }}
      </span>
      <span v-if="unit" :class="sizeClass" class="ml-0.5 font-bold leading-none text-slate-300">{{ unit }}</span>
    </div>
  </div>
</template>
