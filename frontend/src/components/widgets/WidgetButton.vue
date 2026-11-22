<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { Zap } from 'lucide-vue-next'
import type { ColorOption, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  canWrite: boolean
}>()

const emit = defineEmits<{
  write: [mappingId: string, value: number, pulseMs?: number]
}>()

const pulsing = ref(false)
let pulseTimer: number | null = null

const label = computed(() => String(props.widget.extra?.label || props.widget.name))
const triggerTimeMs = computed(() => {
  const v = Number(props.widget.extra?.triggerTimeMs)
  return Number.isFinite(v) && v > 0 ? Math.min(60000, Math.round(v)) : 1000
})
const color = computed<ColorOption>(() => (props.widget.style?.color as ColorOption) || 'sky')
const writable = computed(() => props.canWrite && props.mapping && props.mapping.protocolAttributes?.accessMode === 'READ_WRITE')

const btnColorMap: Record<ColorOption, string> = {
  sky: 'bg-sky-500 hover:bg-sky-400 text-slate-950',
  emerald: 'bg-emerald-500 hover:bg-emerald-400 text-slate-950',
  rose: 'bg-rose-500 hover:bg-rose-400 text-white',
  amber: 'bg-amber-500 hover:bg-amber-400 text-slate-950',
  slate: 'bg-slate-700 hover:bg-slate-600 text-white',
}

function trigger() {
  if (!writable.value || !props.mapping || pulsing.value) return
  pulsing.value = true
  emit('write', props.mapping.id, 1, triggerTimeMs.value)
  pulseTimer = window.setTimeout(() => {
    pulsing.value = false
    pulseTimer = null
  }, triggerTimeMs.value)
}

onUnmounted(() => {
  if (pulseTimer !== null) window.clearTimeout(pulseTimer)
})
</script>

<template>
  <div class="flex flex-col justify-between rounded-xl border border-slate-800 bg-slate-900/75 p-4">
    <div class="flex items-start justify-between gap-2">
      <p class="truncate text-xs font-semibold text-slate-200">{{ widget.name }}</p>
      <span class="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] text-slate-400">
        {{ triggerTimeMs }} ms
      </span>
    </div>

    <div class="my-3">
      <button
        type="button"
        :disabled="!writable || pulsing"
        class="inline-flex w-full items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-xs font-semibold transition active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-50"
        :class="btnColorMap[color] || btnColorMap.sky"
        @click="trigger"
      >
        <Zap class="h-3.5 w-3.5" />
        {{ pulsing ? 'Triggered...' : label }}
      </button>
    </div>

    <p class="truncate text-[10px] text-slate-500">
      {{ writable ? (mapping?.name || '') : 'Read-only access' }}
    </p>
  </div>
</template>
