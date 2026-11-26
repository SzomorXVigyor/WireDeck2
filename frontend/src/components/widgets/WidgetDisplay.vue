<script setup lang="ts">
import { computed } from 'vue'
import type { FontSizeOption, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import { clampPrecision, formatPrecisionValue } from '../../utils/precision'
import {
  getTileAccentVisual,
  resolveNumericAffixClass,
  resolveNumericValueClass,
  resolveTileAccent,
  resolveTileSize,
} from '../../utils/tiles'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  preview?: boolean
}>()

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
    // Realistic sample value scaled for the active precision
    const p = precision.value
    const sampleRaw = p > 0 ? Math.round(24.5 * Math.pow(10, p)) : p < 0 ? 42 : 42
    return formatPrecisionValue(sampleRaw, p)
  }
  return formatPrecisionValue(undefined, precision.value)
})

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
</script>

<template>
  <div
    class="wd-tile-surface flex h-full w-full flex-col justify-between border transition-colors"
    :class="[surfaceClass, tileSize === 'sm' ? 'p-3' : 'p-3.5 sm:p-4']"
    :style="surfaceStyle"
  >
    <div class="pointer-events-none absolute inset-x-0 top-0 h-[3px]" :class="topBarClass" :style="topBarStyle" />

    <!-- Header -->
    <div class="flex items-start justify-between gap-2 pt-0.5">
      <p
        class="font-semibold leading-snug tracking-tight text-slate-200"
        :class="tileSize === 'sm' ? 'line-clamp-2 text-[11px] sm:text-xs' : 'truncate text-xs sm:text-[13px]'"
      >
        {{ widget.name || 'Untitled Display' }}
      </p>
      <span
        v-if="quality !== 'GOOD'"
        class="shrink-0 rounded px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide"
        :class="quality === 'BAD' ? 'bg-rose-500/20 text-rose-300' : 'bg-amber-500/20 text-amber-300'"
        :title="sample?.error || 'Waiting for fresh telemetry'"
      >
        {{ quality }}
      </span>
    </div>

    <!-- Metric Readout -->
    <div
      class="flex min-w-0 flex-1 flex-col"
      :class="isHero ? 'justify-center py-2' : 'justify-end pt-2'"
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
    </div>

    <!-- Subtle Footer on Tall / Large Tiles -->
    <div
      v-if="isHero"
      class="flex items-center justify-between gap-2 border-t border-slate-800/60 pt-2 text-[10px] text-slate-400"
    >
      <span class="truncate font-mono">{{ mapping?.name || 'Display' }}</span>
      <span v-if="mapping?.protocolAttributes?.dataType" class="shrink-0 font-mono uppercase text-slate-500">
        {{ mapping.protocolAttributes.dataType }}
      </span>
    </div>
  </div>
</template>
