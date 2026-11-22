<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Send } from 'lucide-vue-next'
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

const draft = ref<string>('')
const touched = ref(false)
const quality = computed(() => props.sample?.quality ?? 'STALE')
const writable = computed(() => props.canWrite && props.mapping && props.mapping.accessMode !== 'READ')
const minVal = computed(() => (props.widget.extra?.min !== undefined ? Number(props.widget.extra.min) : undefined))
const maxVal = computed(() => (props.widget.extra?.max !== undefined ? Number(props.widget.extra.max) : undefined))
const stepVal = computed(() => (props.widget.extra?.step !== undefined ? Number(props.widget.extra.step) : 1))

watch(
  () => props.sample?.value,
  (val) => {
    if (!touched.value && val !== null && val !== undefined && Number.isFinite(val)) {
      draft.value = String(val)
    }
  },
  { immediate: true },
)

function submit() {
  if (!writable.value || !props.mapping) return
  const num = Number(draft.value)
  if (!Number.isFinite(num)) return
  if (minVal.value !== undefined && num < minVal.value) return
  if (maxVal.value !== undefined && num > maxVal.value) return
  touched.value = false
  emit('write', props.mapping.id, num)
}
</script>

<template>
  <div
    class="flex flex-col justify-between rounded-xl border p-4 transition"
    :class="
      quality === 'BAD'
        ? 'border-rose-500/40 bg-rose-950/20'
        : quality === 'STALE' && mapping?.accessMode !== 'WRITE'
          ? 'border-amber-500/30 bg-slate-900/60'
          : 'border-slate-800 bg-slate-900/75'
    "
  >
    <div class="flex items-start justify-between gap-2">
      <p class="truncate text-xs font-medium text-slate-300">{{ widget.name }}</p>
      <span
        v-if="mapping?.accessMode !== 'WRITE' && quality !== 'GOOD'"
        class="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase"
        :class="quality === 'BAD' ? 'bg-rose-500/20 text-rose-300' : 'bg-amber-500/20 text-amber-300'"
      >
        {{ quality }}
      </span>
    </div>

    <form class="my-3 flex items-center gap-2" @submit.prevent="submit">
      <input
        v-model="draft"
        type="number"
        :min="minVal"
        :max="maxVal"
        :step="stepVal"
        :disabled="!writable"
        placeholder="Setpoint"
        class="w-full rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-1.5 font-mono text-sm text-white disabled:opacity-40"
        @focus="touched = true"
      />
      <span v-if="mapping?.unit" class="shrink-0 text-xs text-slate-400">{{ mapping.unit }}</span>
      <button
        type="submit"
        :disabled="!writable"
        class="inline-flex shrink-0 items-center gap-1 rounded-lg bg-sky-500 px-2.5 py-1.5 text-xs font-semibold text-slate-950 hover:bg-sky-400 disabled:opacity-40"
      >
        <Send class="h-3.5 w-3.5" />
      </button>
    </form>

    <div class="flex items-center justify-between text-[10px] text-slate-500">
      <span class="truncate">{{ writable ? (mapping?.name || '') : 'Read-only access' }}</span>
      <span v-if="sample?.value !== null && sample?.value !== undefined" class="font-mono text-slate-400">
        cur: {{ sample.value }}
      </span>
    </div>
  </div>
</template>
