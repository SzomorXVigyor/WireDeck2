<script setup lang="ts">
import { computed, ref } from 'vue'
import { Zap } from 'lucide-vue-next'
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

const sending = ref(false)
const pressValue = computed(() => {
  const v = Number(props.widget.extra?.pressValue)
  return Number.isFinite(v) ? v : 1
})
const buttonLabel = computed(() => String(props.widget.extra?.buttonLabel || 'Trigger'))
const writable = computed(() => props.canWrite && props.mapping && props.mapping.accessMode !== 'READ')

function trigger() {
  if (!writable.value || !props.mapping || sending.value) return
  sending.value = true
  emit('write', props.mapping.id, pressValue.value)
  window.setTimeout(() => {
    sending.value = false
  }, 400)
}
</script>

<template>
  <div class="flex flex-col justify-between rounded-xl border border-slate-800 bg-slate-900/75 p-4">
    <div class="flex items-start justify-between gap-2">
      <p class="truncate text-xs font-medium text-slate-300">{{ widget.name }}</p>
      <span class="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] text-slate-400">
        val={{ pressValue }}
      </span>
    </div>

    <div class="my-3">
      <button
        type="button"
        :disabled="!writable || sending"
        class="inline-flex w-full items-center justify-center gap-1.5 rounded-lg bg-sky-500 px-3 py-2 text-xs font-semibold text-slate-950 transition hover:bg-sky-400 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-40"
        @click="trigger"
      >
        <Zap class="h-3.5 w-3.5" />
        {{ sending ? 'Sending...' : buttonLabel }}
      </button>
    </div>

    <p class="truncate text-[10px] text-slate-500">
      {{ writable ? (mapping?.name || '') : 'Read-only access' }}
    </p>
  </div>
</template>
