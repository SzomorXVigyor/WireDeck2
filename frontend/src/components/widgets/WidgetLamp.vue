<script setup lang="ts">
import { computed } from 'vue'
import type { RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
}>()

const isOn = computed(() => (props.sample?.value ?? 0) !== 0)
const quality = computed(() => props.sample?.quality ?? 'STALE')
const onLabel = computed(() => String(props.widget.extra?.onLabel || 'ACTIVE'))
const offLabel = computed(() => String(props.widget.extra?.offLabel || 'INACTIVE'))
const onColor = computed(() => String(props.widget.style?.onColor || 'emerald'))

const dotClass = computed(() => {
  if (quality.value === 'BAD') return 'bg-rose-500/30 border-rose-500'
  if (quality.value === 'STALE') return 'bg-amber-500/20 border-amber-500/60'
  if (!isOn.value) return 'bg-slate-800 border-slate-600'
  if (onColor.value === 'rose') return 'bg-rose-500 border-rose-300 shadow-[0_0_16px_rgba(244,63,94,0.65)]'
  if (onColor.value === 'amber') return 'bg-amber-400 border-amber-200 shadow-[0_0_16px_rgba(251,191,36,0.65)]'
  if (onColor.value === 'sky') return 'bg-sky-400 border-sky-200 shadow-[0_0_16px_rgba(56,189,248,0.65)]'
  return 'bg-emerald-400 border-emerald-200 shadow-[0_0_16px_rgba(52,211,153,0.65)]'
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

    <div class="my-4 flex items-center gap-3">
      <div class="h-7 w-7 rounded-full border-2 transition" :class="dotClass" />
      <span class="text-base font-semibold tracking-tight text-white">
        {{ sample ? (isOn ? onLabel : offLabel) : 'No data' }}
      </span>
    </div>

    <p class="truncate font-mono text-[10px] text-slate-500">
      {{ mapping ? `${mapping.name} @ ${mapping.registerAddress}` : 'Unbound mapping' }}
    </p>
  </div>
</template>
