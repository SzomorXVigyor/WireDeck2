<script setup lang="ts">
import { computed, ref } from 'vue'
import { Lock } from 'lucide-vue-next'
import type { ColorOption, RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import { resolveStateColorValue, resolveStateVisual, resolveTileAccent, resolveTileSize } from '../../utils/tiles'

const props = defineProps<{
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
  canWrite: boolean
  preview?: boolean
  previewState?: boolean
}>()

const emit = defineEmits<{
  write: [mappingId: string, value: number]
}>()

const busy = ref(false)
const tileSize = computed(() => resolveTileSize(props.widget))
const tileAccent = computed(() => resolveTileAccent(props.widget))

const quality = computed(() => {
  if (props.sample) return props.sample.quality
  return props.preview ? 'GOOD' : 'STALE'
})

const isOn = computed(() => {
  if (props.sample) return (props.sample.value ?? 0) !== 0
  if (props.preview) return props.previewState ?? true
  return false
})

const writable = computed(
  () => !props.preview && props.canWrite && Boolean(props.mapping) && props.mapping?.protocolAttributes?.accessMode === 'READ_WRITE',
)
const onLabel = computed(() => String(props.widget.extra?.onLabel || 'ON'))
const offLabel = computed(() => String(props.widget.extra?.offLabel || 'OFF'))

const onColor = computed<ColorOption>(() => resolveStateColorValue(props.widget.style?.color, 'tile_accent'))
const offColor = computed<ColorOption>(() => resolveStateColorValue(props.widget.style?.offColor, 'tile_accent'))

const activeStateVisual = computed(() =>
  resolveStateVisual(isOn.value ? onColor.value : offColor.value, tileAccent.value, 'tile_accent'),
)

// Both ON and OFF switch state colors override the base tile accent entirely when quality is GOOD.
const surfaceClass = computed(() => {
  if (quality.value === 'BAD') return 'border-rose-500/45 bg-rose-950/25'
  if (quality.value === 'STALE') return 'border-amber-500/35 bg-slate-900/75'
  return activeStateVisual.value.surfaceClass
})

const surfaceStyle = computed(() => (quality.value === 'GOOD' ? activeStateVisual.value.surfaceStyle : undefined))

const topBarClass = computed(() => {
  if (quality.value === 'BAD') return 'bg-rose-500'
  if (quality.value === 'STALE') return 'bg-amber-400/80'
  return activeStateVisual.value.barClass
})

const topBarStyle = computed(() => (quality.value === 'GOOD' ? activeStateVisual.value.barStyle : undefined))

const trackClass = computed(() => {
  if (isOn.value) return ''
  return activeStateVisual.value.isNeutral ? 'border-slate-700 bg-slate-800' : ''
})

const trackStyle = computed(() =>
  isOn.value ? activeStateVisual.value.switchOnTrackStyle : activeStateVisual.value.switchOffTrackStyle,
)

const isHero = computed(() => tileSize.value === 'lg' || tileSize.value === 'tall')

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
        {{ widget.name || 'Untitled Switch' }}
      </p>
      <div class="flex shrink-0 items-center gap-1">
        <Lock
          v-if="!preview && !writable"
          class="h-3.5 w-3.5 text-slate-500"
          title="Read only"
        />
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

    <!-- Switch Control -->
    <div
      class="flex flex-1"
      :class="
        isHero
          ? 'flex-col items-center justify-center gap-4 py-2 text-center'
          : 'items-end justify-between gap-2 pt-2'
      "
    >
      <span
        class="min-w-0 truncate font-bold tracking-tight transition-colors"
        :class="[
          isOn ? 'text-white' : 'text-slate-300',
          tileSize === 'lg'
            ? 'text-xl sm:text-2xl'
            : tileSize === 'tall'
              ? 'text-base sm:text-lg'
              : tileSize === 'sm'
                ? 'text-xs sm:text-sm'
                : 'text-sm sm:text-base',
        ]"
      >
        {{ isOn ? onLabel : offLabel }}
      </span>

      <button
        type="button"
        role="switch"
        :aria-checked="isOn"
        :aria-label="`${widget.name || 'Switch'} (${isOn ? onLabel : offLabel})`"
        :disabled="(!preview && !writable) || busy"
        class="relative inline-flex shrink-0 items-center rounded-full border transition-colors disabled:opacity-45"
        :class="[
          trackClass,
          isHero ? 'h-9 w-16' : 'h-7 w-13',
          preview ? 'pointer-events-none' : '',
        ]"
        :style="trackStyle"
        @click="toggle"
      >
        <span
          class="inline-block transform rounded-full bg-white shadow transition-transform"
          :class="[
            isHero
              ? isOn
                ? 'h-7 w-7 translate-x-8'
                : 'h-7 w-7 translate-x-1'
              : isOn
                ? 'h-5 w-5 translate-x-6.5'
                : 'h-5 w-5 translate-x-1',
          ]"
        />
      </button>
    </div>

    <!-- Subtle Footer on Tall / Large Tiles -->
    <div
      v-if="isHero"
      class="flex items-center justify-between gap-2 border-t border-slate-800/60 pt-2 text-[10px] text-slate-400"
    >
      <span class="truncate font-mono">{{ mapping?.name || 'Toggle Switch' }}</span>
      <span class="shrink-0 font-mono uppercase text-slate-500">
        {{ writable || preview ? 'R/W' : 'R' }}
      </span>
    </div>
  </div>
</template>
