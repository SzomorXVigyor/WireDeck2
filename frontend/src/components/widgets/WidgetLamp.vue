<script setup lang="ts">
import { computed } from 'vue'
import type { ColorOption, LampMode, LampStateConfig, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
}>()

const quality = computed(() => props.sample?.quality ?? 'STALE')
const isOn = computed(() => (props.sample?.value ?? 0) !== 0)

function readStateCfg(key: 'on' | 'off'): LampStateConfig {
  const raw = (props.widget.extra?.[key] as Partial<LampStateConfig> | undefined) || {}
  if (key === 'on') {
    return {
      mode: (raw.mode as LampMode) || 'on',
      color: (raw.color as ColorOption) || 'emerald',
      text: raw.text ?? String(props.widget.extra?.onLabel || 'ON'),
    }
  }
  return {
    mode: (raw.mode as LampMode) || 'off',
    color: (raw.color as ColorOption) || 'slate',
    text: raw.text ?? String(props.widget.extra?.offLabel || 'OFF'),
  }
}

const activeState = computed<LampStateConfig>(() => readStateCfg(isOn.value ? 'on' : 'off'))

const litColorMap: Record<ColorOption, string> = {
  emerald: 'bg-emerald-400 border-emerald-200 shadow-[0_0_16px_rgba(52,211,153,0.7)]',
  sky: 'bg-sky-400 border-sky-200 shadow-[0_0_16px_rgba(56,189,248,0.7)]',
  rose: 'bg-rose-500 border-rose-200 shadow-[0_0_16px_rgba(244,63,94,0.7)]',
  amber: 'bg-amber-400 border-amber-200 shadow-[0_0_16px_rgba(251,191,36,0.7)]',
  slate: 'bg-slate-400 border-slate-200 shadow-[0_0_10px_rgba(148,163,184,0.5)]',
}

const dimColorMap: Record<ColorOption, string> = {
  emerald: 'bg-emerald-950/60 border-emerald-800/80',
  sky: 'bg-sky-950/60 border-sky-800/80',
  rose: 'bg-rose-950/60 border-rose-800/80',
  amber: 'bg-amber-950/60 border-amber-800/80',
  slate: 'bg-slate-800 border-slate-600',
}

const dotClass = computed(() => {
  if (quality.value === 'BAD') return 'bg-rose-500/30 border-rose-500'
  if (quality.value === 'STALE' || !props.sample) return 'bg-amber-500/20 border-amber-500/60'
  const st = activeState.value
  if (st.mode === 'off') return dimColorMap[st.color] || dimColorMap.slate
  if (st.mode === 'blink') return `${litColorMap[st.color] || litColorMap.emerald} animate-pulse`
  return litColorMap[st.color] || litColorMap.emerald
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

    <div class="my-4 flex items-center gap-3">
      <div class="h-7 w-7 shrink-0 rounded-full border-2 transition" :class="dotClass" />
      <span class="truncate text-base font-semibold tracking-tight text-white">
        {{ sample ? activeState.text : 'No data' }}
      </span>
    </div>

    <p class="truncate font-mono text-[10px] text-slate-500">
      {{ mapping ? mapping.name : 'Unbound' }}
    </p>
  </div>
</template>
