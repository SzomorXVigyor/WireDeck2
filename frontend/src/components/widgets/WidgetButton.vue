<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { Lock, Zap } from 'lucide-vue-next'
import type { ColorOption, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import {
  getTileAccentVisual,
  resolveStateColorValue,
  resolveStateVisual,
  resolveTileAccent,
  resolveTileSize,
} from '../../utils/tiles'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  canWrite: boolean
  preview?: boolean
  previewTriggered?: boolean
}>()

const emit = defineEmits<{
  write: [mappingId: string, value: number, pulseMs?: number]
}>()

const pulsing = ref(false)
let pulseTimer: number | null = null

const tileSize = computed(() => resolveTileSize(props.widget))
const accent = computed(() => resolveTileAccent(props.widget))
const accentVisual = computed(() => getTileAccentVisual(accent.value))

const label = computed(() => String(props.widget.extra?.label || props.widget.name || 'Trigger'))
const triggeredLabel = computed(() => {
  const custom = typeof props.widget.extra?.triggeredLabel === 'string' ? props.widget.extra.triggeredLabel.trim() : ''
  return custom || 'Triggered...'
})
const triggerTimeMs = computed(() => {
  const v = Number(props.widget.extra?.triggerTimeMs)
  return Number.isFinite(v) && v > 0 ? Math.min(60000, Math.round(v)) : 1000
})
const color = computed<ColorOption>(() => resolveStateColorValue(props.widget.style?.color, 'tile_accent'))
const buttonVisual = computed(() => resolveStateVisual(color.value, accent.value, 'tile_accent'))

const isTriggered = computed(() => (props.preview ? Boolean(props.previewTriggered) : pulsing.value))
const writable = computed(
  () => !props.preview && props.canWrite && Boolean(props.mapping) && props.mapping?.protocolAttributes?.accessMode === 'READ_WRITE',
)

const isHero = computed(() => tileSize.value === 'lg' || tileSize.value === 'tall')

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
  <div
    class="wd-tile-surface flex h-full w-full flex-col justify-between border transition-colors"
    :class="[accentVisual.surfaceClass, tileSize === 'sm' ? 'p-3' : 'p-3.5 sm:p-4']"
    :style="accentVisual.surfaceStyle"
  >
    <div
      class="pointer-events-none absolute inset-x-0 top-0 h-[3px]"
      :class="accent === 'default' ? buttonVisual.barClass : accentVisual.barClass"
      :style="accent === 'default' ? buttonVisual.barStyle : accentVisual.barStyle"
    />

    <!-- Header -->
    <div class="flex items-start justify-between gap-2 pt-0.5">
      <p
        class="font-semibold leading-snug tracking-tight text-slate-200"
        :class="tileSize === 'sm' ? 'line-clamp-2 text-[11px] sm:text-xs' : 'truncate text-xs sm:text-[13px]'"
      >
        {{ widget.name || 'Untitled Button' }}
      </p>
      <div class="flex shrink-0 items-center gap-1">
        <Lock v-if="!preview && !writable" class="h-3.5 w-3.5 text-slate-500" title="Read only" />
        <span class="rounded bg-slate-800/90 px-1.5 py-0.5 font-mono text-[10px] text-slate-400">
          {{ triggerTimeMs }}ms
        </span>
      </div>
    </div>

    <!-- Action Button -->
    <div class="flex flex-1 flex-col justify-end pt-2.5">
      <button
        type="button"
        :disabled="(!preview && !writable) || pulsing"
        class="inline-flex w-full items-center justify-center gap-1.5 rounded-lg font-semibold transition hover:brightness-110 active:scale-[0.985] disabled:opacity-55"
        :class="[
          isHero
            ? 'h-full min-h-[68px] flex-col gap-2 px-4 py-4 text-sm sm:text-base'
            : tileSize === 'sm'
              ? 'min-h-[38px] px-2.5 py-2 text-xs'
              : 'min-h-[44px] px-3.5 py-2.5 text-xs sm:text-sm',
          preview ? 'pointer-events-none' : '',
          isTriggered ? 'ring-2 ring-white/40 brightness-95' : '',
        ]"
        :style="buttonVisual.buttonStyle"
        @click="trigger"
      >
        <Zap v-if="isHero" class="h-5 w-5 shrink-0" :class="isTriggered ? 'animate-pulse' : ''" />
        <span class="truncate">{{ isTriggered ? triggeredLabel : label }}</span>
      </button>
    </div>
  </div>
</template>
