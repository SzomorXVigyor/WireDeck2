<script setup lang="ts">
import { computed, ref } from 'vue'
import { Pencil } from 'lucide-vue-next'
import type { FontSizeOption, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import { clampPrecision, formatPrecisionValue } from '../../utils/precision'
import NumberInputModal from './NumberInputModal.vue'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  canWrite: boolean
}>()

const emit = defineEmits<{
  write: [mappingId: string, value: number]
}>()

const modalOpen = ref(false)
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
const writable = computed(() => props.canWrite && props.mapping && props.mapping.protocolAttributes?.accessMode === 'READ_WRITE')

const fontSizeMap: Record<FontSizeOption, string> = {
  sm: 'text-xl',
  md: 'text-2xl',
  lg: 'text-3xl',
  xl: 'text-4xl',
}
const sizeClass = computed(() => fontSizeMap[(props.widget.style?.fontSize as FontSizeOption) || 'lg'] || 'text-3xl')

function openModal() {
  if (!writable.value) return
  modalOpen.value = true
}

function handleSubmit(rawValue: number) {
  if (!props.mapping) return
  emit('write', props.mapping.id, rawValue)
}
</script>

<template>
  <div
    class="group flex flex-col justify-between rounded-xl border p-4 transition"
    :class="[
      quality === 'BAD'
        ? 'border-rose-500/40 bg-rose-950/20'
        : quality === 'STALE'
          ? 'border-amber-500/30 bg-slate-900/60'
          : 'border-slate-800 bg-slate-900/75',
      writable ? 'cursor-pointer hover:border-sky-500/50 hover:bg-slate-900' : '',
    ]"
    @click="openModal"
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

    <!-- Same visual layout as Display card + edit pencil affordance -->
    <div class="mt-3 mb-1 flex items-baseline justify-between gap-2">
      <div class="flex min-w-0 items-baseline gap-1">
        <span v-if="prefix" :class="sizeClass" class="shrink-0 font-bold leading-none text-slate-300">{{ prefix }}</span>
        <span :class="sizeClass" class="truncate font-mono font-bold tabular-nums leading-none tracking-tight text-white">
          {{ formatted }}
        </span>
        <span v-if="unit" :class="sizeClass" class="ml-0.5 shrink-0 font-bold leading-none text-slate-300">{{ unit }}</span>
      </div>

      <button
        v-if="writable"
        type="button"
        title="Set value"
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-500 transition hover:bg-sky-500/15 hover:text-sky-300 group-hover:text-sky-400"
        @click.stop="openModal"
      >
        <Pencil class="h-4 w-4" />
      </button>
    </div>

    <NumberInputModal
      :open="modalOpen"
      :widget="widget"
      :mapping="mapping"
      :sample="sample"
      @close="modalOpen = false"
      @submit="handleSubmit"
    />
  </div>
</template>
