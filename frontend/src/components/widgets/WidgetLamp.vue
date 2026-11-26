<script setup lang="ts">
import { computed } from 'vue'
import type { ColorOption, LampMode, LampStateConfig, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import { resolveStateColorValue, resolveStateVisual, resolveTileAccent, resolveTileSize } from '../../utils/tiles'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  preview?: boolean
  previewState?: boolean
}>()

const tileSize = computed(() => resolveTileSize(props.widget))
const tileAccent = computed(() => resolveTileAccent(props.widget))

const quality = computed(() => {
  if (props.sample) return props.sample.quality
  return props.preview ? 'GOOD' : 'STALE'
})

const hasData = computed(() => Boolean(props.sample) || Boolean(props.preview))

const isOn = computed(() => {
  if (props.sample) return (props.sample.value ?? 0) !== 0
  if (props.preview) return props.previewState ?? true
  return false
})

function readStateCfg(key: 'on' | 'off'): LampStateConfig {
  const raw = (props.widget.extra?.[key] as Partial<LampStateConfig> | undefined) || {}
  if (key === 'on') {
    return {
      mode: (raw.mode as LampMode) || 'on',
      color: resolveStateColorValue(raw.color, 'tile_accent'),
      text: raw.text ?? String(props.widget.extra?.onLabel || 'ON'),
    }
  }
  return {
    mode: (raw.mode as LampMode) || 'off',
    color: resolveStateColorValue(raw.color, 'tile_accent'),
    text: raw.text ?? String(props.widget.extra?.offLabel || 'OFF'),
  }
}

const activeState = computed<LampStateConfig>(() => readStateCfg(isOn.value ? 'on' : 'off'))
const stateVisual = computed(() => resolveStateVisual(activeState.value.color as ColorOption, tileAccent.value, 'tile_accent'))
const isLit = computed(() => quality.value === 'GOOD' && hasData.value && activeState.value.mode !== 'off')

const dotClass = computed(() => {
  if (quality.value === 'BAD') return 'bg-rose-500/30 border-rose-500'
  if (quality.value === 'STALE' || !hasData.value) return 'bg-amber-500/20 border-amber-500/60'
  if (activeState.value.mode === 'blink') return 'wd-tile-blink animate-pulse'
  return ''
})

const dotStyle = computed(() => {
  if (quality.value !== 'GOOD' || !hasData.value) return undefined
  return activeState.value.mode === 'off' ? stateVisual.value.lampDimDotStyle : stateVisual.value.lampLitDotStyle
})

// Both ON and OFF lamp state colors override the base tile accent entirely when data is GOOD.
const surfaceClass = computed(() => {
  if (quality.value === 'BAD') return 'border-rose-500/45 bg-rose-950/25'
  if (quality.value === 'STALE') return 'border-amber-500/35 bg-slate-900/75'
  return stateVisual.value.surfaceClass
})

const surfaceStyle = computed(() => (quality.value === 'GOOD' ? stateVisual.value.surfaceStyle : undefined))

const topBarClass = computed(() => {
  if (quality.value === 'BAD') return 'bg-rose-500'
  if (quality.value === 'STALE') return 'bg-amber-400/80'
  return stateVisual.value.barClass
})

const topBarStyle = computed(() => (quality.value === 'GOOD' ? stateVisual.value.barStyle : undefined))

const isHero = computed(() => tileSize.value === 'lg' || tileSize.value === 'tall')
</script>

<template>
  <div
    class="wd-tile-surface flex h-full w-full flex-col justify-between border transition-colors"
    :class="[surfaceClass, tileSize === 'sm' ? 'p-3' : 'p-3.5 sm:p-4']"
    :style="surfaceStyle"
  >
    <div class="pointer-events-none absolute inset-x-0 top-0 h-[3px] transition-colors" :class="topBarClass" :style="topBarStyle" />

    <!-- Header -->
    <div class="flex items-start justify-between gap-2 pt-0.5">
      <p
        class="font-semibold leading-snug tracking-tight text-slate-200"
        :class="tileSize === 'sm' ? 'line-clamp-2 text-[11px] sm:text-xs' : 'truncate text-xs sm:text-[13px]'"
      >
        {{ widget.name || 'Untitled Lamp' }}
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

    <!-- Lamp Beacon + Status Text -->
    <div
      class="flex flex-1"
      :class="
        isHero
          ? 'flex-col items-center justify-center gap-3 py-2 text-center'
          : tileSize === 'sm'
            ? 'items-end gap-2.5 pt-2'
            : 'items-end gap-3 pt-2'
      "
    >
      <div
        class="shrink-0 rounded-full border-2 transition-all duration-200"
        :class="[
          dotClass,
          tileSize === 'lg'
            ? 'h-14 w-14'
            : tileSize === 'tall'
              ? 'h-12 w-12'
              : tileSize === 'sm'
                ? 'h-6 w-6'
                : 'h-8 w-8',
        ]"
        :style="dotStyle"
      />
      <span
        class="min-w-0 truncate font-bold tracking-tight"
        :class="[
          isLit ? 'text-white' : 'text-slate-300',
          tileSize === 'lg'
            ? 'text-xl sm:text-2xl'
            : tileSize === 'tall'
              ? 'text-base sm:text-lg'
              : tileSize === 'sm'
                ? 'text-sm sm:text-base'
                : 'text-base sm:text-lg',
        ]"
      >
        {{ hasData ? activeState.text : 'No data' }}
      </span>
    </div>

    <!-- Subtle Footer on Tall / Large Tiles -->
    <div
      v-if="isHero"
      class="flex items-center justify-between gap-2 border-t border-slate-800/60 pt-2 text-[10px] text-slate-400"
    >
      <span class="truncate font-mono">{{ mapping?.name || 'Status Lamp' }}</span>
      <span class="shrink-0 font-mono uppercase text-slate-500">
        {{ activeState.mode }}
      </span>
    </div>
  </div>
</template>
