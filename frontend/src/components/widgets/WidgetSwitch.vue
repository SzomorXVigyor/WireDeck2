<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ColorOption, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  canWrite: boolean
}>()

const emit = defineEmits<{
  write: [mappingId: string, value: number]
}>()

const busy = ref(false)
const quality = computed(() => props.sample?.quality ?? 'STALE')
const isOn = computed(() => (props.sample?.value ?? 0) !== 0)
const writable = computed(() => props.canWrite && props.mapping && props.mapping.protocolAttributes?.accessMode === 'READ_WRITE')
const onLabel = computed(() => String(props.widget.extra?.onLabel || 'ON'))
const offLabel = computed(() => String(props.widget.extra?.offLabel || 'OFF'))
const color = computed<ColorOption>(() => (props.widget.style?.color as ColorOption) || 'emerald')

const activeTrackMap: Record<ColorOption, string> = {
  emerald: 'border-emerald-400 bg-emerald-500/80',
  sky: 'border-sky-400 bg-sky-500/80',
  rose: 'border-rose-400 bg-rose-500/80',
  amber: 'border-amber-400 bg-amber-500/80',
  slate: 'border-slate-400 bg-slate-500/80',
}

function toggle() {
  if (!writable.value || !props.mapping || busy.value) return
  busy.value = true
  emit('write', props.mapping.id, isOn.value ? 0 : 1)
  window.setTimeout(() => {
    busy.value = false
  }, 350)
}
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

    <div class="mt-3 mb-1 flex items-center justify-between gap-3">
      <span class="text-sm font-semibold" :class="isOn ? 'text-white' : 'text-slate-400'">
        {{ isOn ? onLabel : offLabel }}
      </span>
      <button
        type="button"
        role="switch"
        :aria-checked="isOn"
        :disabled="!writable || busy"
        class="relative inline-flex h-7 w-13 shrink-0 cursor-pointer items-center rounded-full border transition disabled:cursor-not-allowed disabled:opacity-40"
        :class="isOn ? (activeTrackMap[color] || activeTrackMap.emerald) : 'border-slate-700 bg-slate-800'"
        @click="toggle"
      >
        <span
          class="inline-block h-5 w-5 transform rounded-full bg-white shadow transition"
          :class="isOn ? 'translate-x-6.5' : 'translate-x-1'"
        />
      </button>
    </div>
  </div>
</template>
