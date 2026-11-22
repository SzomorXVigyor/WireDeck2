<script setup lang="ts">
import { computed, ref } from 'vue'
import type { RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'

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
const writable = computed(() => props.canWrite && props.mapping && props.mapping.accessMode !== 'READ')

async function toggle() {
  if (!writable.value || !props.mapping || busy.value) return
  busy.value = true
  try {
    emit('write', props.mapping.id, isOn.value ? 0 : 1)
  } finally {
    window.setTimeout(() => {
      busy.value = false
    }, 350)
  }
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

    <div class="my-4 flex items-center justify-between gap-3">
      <span class="text-sm font-semibold" :class="isOn ? 'text-emerald-300' : 'text-slate-400'">
        {{ isOn ? 'ON' : 'OFF' }}
      </span>
      <button
        type="button"
        role="switch"
        :aria-checked="isOn"
        :disabled="!writable || busy"
        class="relative inline-flex h-7 w-13 shrink-0 cursor-pointer items-center rounded-full border transition disabled:cursor-not-allowed disabled:opacity-40"
        :class="isOn ? 'border-emerald-400 bg-emerald-500/80' : 'border-slate-700 bg-slate-800'"
        @click="toggle"
      >
        <span
          class="inline-block h-5 w-5 transform rounded-full bg-white shadow transition"
          :class="isOn ? 'translate-x-6.5' : 'translate-x-1'"
        />
      </button>
    </div>

    <p class="truncate text-[10px] text-slate-500">
      {{ writable ? (mapping?.name || '') : 'Read-only access' }}
    </p>
  </div>
</template>
