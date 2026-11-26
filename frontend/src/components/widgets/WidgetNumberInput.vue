<script setup lang="ts">
import { computed, ref } from 'vue'
import { Lock, Pencil } from 'lucide-vue-next'
import type { FontSizeOption, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import { clampPrecision, formatPrecisionValue } from '../../utils/precision'
import {
  getTileAccentVisual,
  resolveNumericAffixClass,
  resolveNumericValueClass,
  resolveTileAccent,
  resolveTileSize,
} from '../../utils/tiles'
import NumberInputModal from './NumberInputModal.vue'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  canWrite: boolean
  preview?: boolean
}>()

const emit = defineEmits<{
  write: [mappingId: string, value: number]
}>()

const modalOpen = ref(false)
const tileSize = computed(() => resolveTileSize(props.widget))
const accentVisual = computed(() => getTileAccentVisual(resolveTileAccent(props.widget)))

const quality = computed(() => {
  if (props.sample) return props.sample.quality
  return props.preview ? 'GOOD' : 'STALE'
})

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

const displayValue = computed(() => {
  if (props.sample) {
    return formatPrecisionValue(props.sample.value, precision.value)
  }
  if (props.preview) {
    const p = precision.value
    const sampleRaw = p > 0 ? Math.round(50.0 * Math.pow(10, p)) : 50
    return formatPrecisionValue(sampleRaw, p)
  }
  return formatPrecisionValue(undefined, precision.value)
})

const writable = computed(
  () => !props.preview && props.canWrite && Boolean(props.mapping) && props.mapping?.protocolAttributes?.accessMode === 'READ_WRITE',
)

const fontSize = computed<FontSizeOption>(() => (props.widget.style?.fontSize as FontSizeOption) || 'lg')
const valueClass = computed(() => resolveNumericValueClass(fontSize.value, tileSize.value))
const affixClass = computed(() => resolveNumericAffixClass(fontSize.value, tileSize.value))

const surfaceClass = computed(() => {
  if (quality.value === 'BAD') return 'border-rose-500/45 bg-rose-950/25'
  if (quality.value === 'STALE') return 'border-amber-500/35 bg-slate-900/75'
  return accentVisual.value.surfaceClass
})

const surfaceStyle = computed(() => (quality.value === 'GOOD' ? accentVisual.value.surfaceStyle : undefined))

const topBarClass = computed(() => {
  if (quality.value === 'BAD') return 'bg-rose-500'
  if (quality.value === 'STALE') return 'bg-amber-400/80'
  return accentVisual.value.barClass
})

const topBarStyle = computed(() => (quality.value === 'GOOD' ? accentVisual.value.barStyle : undefined))

const isHero = computed(() => tileSize.value === 'lg' || tileSize.value === 'tall')

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
    class="wd-tile-surface group flex h-full w-full flex-col justify-between border transition-colors"
    :class="[
      surfaceClass,
      tileSize === 'sm' ? 'p-3' : 'p-3.5 sm:p-4',
      writable ? 'cursor-pointer hover:border-sky-500/60' : '',
    ]"
    :style="surfaceStyle"
    @click="openModal"
  >
    <div class="pointer-events-none absolute inset-x-0 top-0 h-[3px]" :class="topBarClass" :style="topBarStyle" />

    <!-- Header -->
    <div class="flex items-start justify-between gap-2 pt-0.5">
      <p
        class="font-semibold leading-snug tracking-tight text-slate-200"
        :class="tileSize === 'sm' ? 'line-clamp-2 text-[11px] sm:text-xs' : 'truncate text-xs sm:text-[13px]'"
      >
        {{ widget.name || 'Untitled Input' }}
      </p>
      <div class="flex shrink-0 items-center gap-1">
        <Lock v-if="!preview && !writable" class="h-3.5 w-3.5 text-slate-500" title="Read only" />
        <span
          v-if="quality !== 'GOOD'"
          class="rounded px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide"
          :class="quality === 'BAD' ? 'bg-rose-500/20 text-rose-300' : 'bg-amber-500/20 text-amber-300'"
          :title="sample?.error || 'Waiting for fresh telemetry'"
        >
          {{ quality }}
        </span>
      </div>
    </div>

    <!-- Value + Set Value Affordance -->
    <div
      class="flex min-w-0 flex-1 items-end justify-between gap-2"
      :class="isHero ? 'items-center py-2' : 'pt-2'"
    >
      <div class="flex min-w-0 flex-wrap items-baseline gap-x-1 gap-y-0.5">
        <span v-if="prefix" :class="affixClass" class="shrink-0 font-semibold leading-none text-slate-400">
          {{ prefix }}
        </span>
        <span
          :class="valueClass"
          class="min-w-0 break-all font-mono font-bold tabular-nums leading-none tracking-tight text-white"
        >
          {{ displayValue }}
        </span>
        <span v-if="unit" :class="affixClass" class="shrink-0 font-semibold leading-none text-slate-300">
          {{ unit }}
        </span>
      </div>

      <button
        v-if="writable || preview"
        type="button"
        title="Set value"
        aria-label="Set value"
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-700/70 bg-slate-800/70 text-slate-300 transition hover:border-sky-500/50 hover:bg-sky-500/15 hover:text-sky-300 group-hover:border-sky-500/40 group-hover:text-sky-300"
        :class="preview ? 'pointer-events-none' : ''"
        @click.stop="openModal"
      >
        <Pencil class="h-3.5 w-3.5" />
      </button>
    </div>

    <!-- Subtle Footer on Tall / Large Tiles -->
    <div
      v-if="isHero"
      class="flex items-center justify-between gap-2 border-t border-slate-800/60 pt-2 text-[10px] text-slate-400"
    >
      <span class="truncate font-mono">{{ mapping?.name || 'Setpoint' }}</span>
      <span v-if="widget.extra?.min !== undefined || widget.extra?.max !== undefined" class="shrink-0 font-mono text-slate-500">
        {{ widget.extra?.min ?? '*' }}..{{ widget.extra?.max ?? '*' }}
      </span>
    </div>

    <NumberInputModal
      v-if="!preview"
      :open="modalOpen"
      :widget="widget"
      :mapping="mapping"
      :sample="sample"
      @close="modalOpen = false"
      @submit="handleSubmit"
    />
  </div>
</template>
