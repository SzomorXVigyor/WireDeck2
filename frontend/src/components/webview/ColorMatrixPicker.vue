<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Check, Link2, Palette, X } from 'lucide-vue-next'
import type { ColorOption, TileAccent } from '../../types/webview'
import {
  isCustomHexColor,
  normalizeHexColor,
  resolveStateVisual,
  resolveTileAccentHex,
} from '../../utils/tiles'

const CACHE_STORAGE_KEY = 'wd2_tile_color_cache'
const MAX_CACHED_COLORS = 9

const props = withDefaults(
  defineProps<{
    modelValue: TileAccent | ColorOption
    mode?: 'accent' | 'state'
    tileAccent?: TileAccent
  }>(),
  {
    mode: 'accent',
    tileAccent: 'default',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

// Multiple predefined color lines (4 curated rows x 9 columns = 36 colors)
const MATRIX_LINES: { label: string; colors: string[] }[] = [
  {
    label: 'Bright',
    colors: ['#38bdf8', '#22d3ee', '#34d399', '#a3e635', '#facc15', '#fb923c', '#fb7185', '#c084fc', '#818cf8'],
  },
  {
    label: 'Vivid',
    colors: ['#0284c7', '#0891b2', '#059669', '#65a30d', '#d97706', '#ea580c', '#e11d48', '#9333ea', '#4f46e5'],
  },
  {
    label: 'Deep',
    colors: ['#0369a1', '#155e75', '#047857', '#3f6212', '#b45309', '#c2410c', '#be123c', '#7e22ce', '#3730a3'],
  },
  {
    label: 'Neutral',
    colors: ['#f8fafc', '#cbd5e1', '#94a3b8', '#64748b', '#475569', '#78716c', '#a8a29e', '#71717a', '#334155'],
  },
]

// --- Cached custom colors (localStorage) ---
function readCachedColors(): string[] {
  try {
    const raw = window.localStorage.getItem(CACHE_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((c): c is string => isCustomHexColor(c)).map(normalizeHexColor).slice(0, MAX_CACHED_COLORS)
  } catch {
    return []
  }
}

function writeCachedColors(list: string[]) {
  try {
    window.localStorage.setItem(CACHE_STORAGE_KEY, JSON.stringify(list.slice(0, MAX_CACHED_COLORS)))
  } catch {
    // ignore storage errors
  }
}

const cachedColors = ref<string[]>(readCachedColors())

const resetTarget = computed(() => (props.mode === 'state' ? 'tile_accent' : 'default'))
const isDefaultSelected = computed(() => props.modelValue === resetTarget.value)
const defaultOptionMeta = computed(() =>
  props.mode === 'state'
    ? {
        value: 'tile_accent',
        label: 'Default',
        title: 'Default (Tile Accent)',
        hex: resolveTileAccentHex(props.tileAccent),
        isTileAccentRef: true,
      }
    : {
        value: 'default',
        label: 'Neutral',
        title: 'Neutral (Default)',
        hex: resolveTileAccentHex('default'),
        isTileAccentRef: false,
      },
)

function pushColorToCache(hex: string) {
  if (!isCustomHexColor(hex)) return
  const norm = normalizeHexColor(hex)
  const next = [norm, ...cachedColors.value.filter((c) => c !== norm)].slice(0, MAX_CACHED_COLORS)
  cachedColors.value = next
  writeCachedColors(next)
}

// --- Color conversion helpers (HEX <-> RGB <-> HSV) ---
function hexToRgbLocal(hex: string): { r: number; g: number; b: number } {
  const clean = hex.replace('#', '')
  return {
    r: parseInt(clean.slice(0, 2), 16) || 0,
    g: parseInt(clean.slice(2, 4), 16) || 0,
    b: parseInt(clean.slice(4, 6), 16) || 0,
  }
}

function rgbToHex(r: number, g: number, b: number): string {
  const clamp = (n: number) => Math.max(0, Math.min(255, Math.round(n)))
  return (
    '#' +
    [clamp(r), clamp(g), clamp(b)]
      .map((v) => v.toString(16).padStart(2, '0'))
      .join('')
  )
}

function rgbToHsv(r: number, g: number, b: number): { h: number; s: number; v: number } {
  const rn = r / 255
  const gn = g / 255
  const bn = b / 255
  const max = Math.max(rn, gn, bn)
  const min = Math.min(rn, gn, bn)
  const d = max - min
  let h = 0
  if (d !== 0) {
    if (max === rn) h = ((gn - bn) / d) % 6
    else if (max === gn) h = (bn - rn) / d + 2
    else h = (rn - gn) / d + 4
    h *= 60
    if (h < 0) h += 360
  }
  const s = max === 0 ? 0 : d / max
  return { h, s, v: max }
}

function hsvToRgb(h: number, s: number, v: number): { r: number; g: number; b: number } {
  const c = v * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = v - c
  let rp = 0
  let gp = 0
  let bp = 0
  if (h >= 0 && h < 60) {
    rp = c
    gp = x
  } else if (h >= 60 && h < 120) {
    rp = x
    gp = c
  } else if (h >= 120 && h < 180) {
    gp = c
    bp = x
  } else if (h >= 180 && h < 240) {
    gp = x
    bp = c
  } else if (h >= 240 && h < 300) {
    rp = x
    bp = c
  } else {
    rp = c
    bp = x
  }
  return {
    r: Math.round((rp + m) * 255),
    g: Math.round((gp + m) * 255),
    b: Math.round((bp + m) * 255),
  }
}

// --- Popover state & positioning ---
const popoverOpen = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const popoverRef = ref<HTMLElement | null>(null)
const popoverStyle = ref<{ top: string; left: string }>({ top: '0px', left: '0px' })

// HSV + text input state
const hue = ref(200)
const sat = ref(0.85)
const val = ref(0.9)
const hexInput = ref('#0ea5e9')
const rInput = ref('14')
const gInput = ref('165')
const bInput = ref('233')

const isCustomSelected = computed(() => !isDefaultSelected.value)
const activeHex = computed(() => {
  if (props.mode === 'state') {
    return resolveStateVisual(props.modelValue as ColorOption, props.tileAccent).hex
  }
  return resolveTileAccentHex(props.modelValue as TileAccent)
})

const pureHueHex = computed(() => {
  const { r, g, b } = hsvToRgb(hue.value, 1, 1)
  return rgbToHex(r, g, b)
})

function syncInternalFromHex(hex: string) {
  const norm = isCustomHexColor(hex) ? normalizeHexColor(hex) : '#0ea5e9'
  const { r, g, b } = hexToRgbLocal(norm)
  const hsv = rgbToHsv(r, g, b)
  if (hsv.s > 0.001) {
    hue.value = hsv.h
  }
  sat.value = hsv.s
  val.value = hsv.v
  hexInput.value = norm
  rInput.value = String(r)
  gInput.value = String(g)
  bInput.value = String(b)
}

function applyHsv(h: number, s: number, v: number) {
  hue.value = ((h % 360) + 360) % 360
  sat.value = Math.max(0, Math.min(1, s))
  val.value = Math.max(0, Math.min(1, v))
  const { r, g, b } = hsvToRgb(hue.value, sat.value, val.value)
  const hex = rgbToHex(r, g, b)
  hexInput.value = hex
  rInput.value = String(r)
  gInput.value = String(g)
  bInput.value = String(b)
  emit('update:modelValue', hex)
}

function selectPreset(value: string) {
  emit('update:modelValue', value)
  const nextHex =
    props.mode === 'state'
      ? resolveStateVisual(value as ColorOption, props.tileAccent).hex
      : resolveTileAccentHex(value as TileAccent)
  syncInternalFromHex(nextHex)
}

function selectMatrixHex(hex: string) {
  const norm = normalizeHexColor(hex)
  syncInternalFromHex(norm)
  emit('update:modelValue', norm)
  pushColorToCache(norm)
}

function selectCachedHex(hex: string) {
  const norm = normalizeHexColor(hex)
  syncInternalFromHex(norm)
  emit('update:modelValue', norm)
  pushColorToCache(norm)
}

function onHexFieldInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value.trim()
  hexInput.value = raw
  const candidate = raw.startsWith('#') ? raw : `#${raw}`
  if (isCustomHexColor(candidate)) {
    const norm = normalizeHexColor(candidate)
    const { r, g, b } = hexToRgbLocal(norm)
    const hsv = rgbToHsv(r, g, b)
    if (hsv.s > 0.001) hue.value = hsv.h
    sat.value = hsv.s
    val.value = hsv.v
    rInput.value = String(r)
    gInput.value = String(g)
    bInput.value = String(b)
    emit('update:modelValue', norm)
  }
}

function onHexFieldBlur() {
  hexInput.value = activeHex.value
}

function onRgbFieldInput(channel: 'r' | 'g' | 'b', e: Event) {
  const digits = (e.target as HTMLInputElement).value.replace(/[^0-9]/g, '').slice(0, 3)
  if (channel === 'r') rInput.value = digits
  else if (channel === 'g') gInput.value = digits
  else bInput.value = digits

  const r = Math.max(0, Math.min(255, parseInt(rInput.value || '0', 10) || 0))
  const g = Math.max(0, Math.min(255, parseInt(gInput.value || '0', 10) || 0))
  const b = Math.max(0, Math.min(255, parseInt(bInput.value || '0', 10) || 0))
  const hex = rgbToHex(r, g, b)
  const hsv = rgbToHsv(r, g, b)
  if (hsv.s > 0.001) hue.value = hsv.h
  sat.value = hsv.s
  val.value = hsv.v
  hexInput.value = hex
  emit('update:modelValue', hex)
}

function onRgbFieldBlur() {
  const { r, g, b } = hexToRgbLocal(activeHex.value)
  rInput.value = String(r)
  gInput.value = String(g)
  bInput.value = String(b)
}

// --- 2D Saturation/Value Plane & Hue Bar Pointer Dragging ---
const svPlaneRef = ref<HTMLElement | null>(null)
const hueBarRef = ref<HTMLElement | null>(null)
const draggingSv = ref(false)
const draggingHue = ref(false)

function updateSvFromPointer(e: PointerEvent) {
  const el = svPlaneRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return
  const x = Math.max(0, Math.min(rect.width, e.clientX - rect.left))
  const y = Math.max(0, Math.min(rect.height, e.clientY - rect.top))
  applyHsv(hue.value, x / rect.width, 1 - y / rect.height)
}

function onSvPointerDown(e: PointerEvent) {
  draggingSv.value = true
  ;(e.currentTarget as HTMLElement | null)?.setPointerCapture?.(e.pointerId)
  updateSvFromPointer(e)
}

function onSvPointerMove(e: PointerEvent) {
  if (!draggingSv.value) return
  updateSvFromPointer(e)
}

function onSvPointerUp() {
  if (draggingSv.value) {
    draggingSv.value = false
    pushColorToCache(activeHex.value)
  }
}

function updateHueFromPointer(e: PointerEvent) {
  const el = hueBarRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  if (rect.width <= 0) return
  const x = Math.max(0, Math.min(rect.width, e.clientX - rect.left))
  applyHsv((x / rect.width) * 360, sat.value, val.value)
}

function onHuePointerDown(e: PointerEvent) {
  draggingHue.value = true
  ;(e.currentTarget as HTMLElement | null)?.setPointerCapture?.(e.pointerId)
  updateHueFromPointer(e)
}

function onHuePointerMove(e: PointerEvent) {
  if (!draggingHue.value) return
  updateHueFromPointer(e)
}

function onHuePointerUp() {
  if (draggingHue.value) {
    draggingHue.value = false
    pushColorToCache(activeHex.value)
  }
}

// --- Popover Open / Close & Anchoring ---
async function positionPopover() {
  await nextTick()
  const trigger = triggerRef.value
  const pop = popoverRef.value
  if (!trigger || !pop) return
  const tRect = trigger.getBoundingClientRect()
  const pWidth = pop.offsetWidth || 320
  const pHeight = pop.offsetHeight || 420
  const vw = window.innerWidth
  const vh = window.innerHeight
  const pad = 12

  let left = tRect.left
  if (left + pWidth > vw - pad) {
    left = vw - pad - pWidth
  }
  if (left < pad) left = pad

  let top = tRect.bottom + 8
  if (top + pHeight > vh - pad) {
    top = Math.max(pad, tRect.top - pHeight - 8)
  }
  if (top + pHeight > vh - pad) {
    top = Math.max(pad, vh - pad - pHeight)
  }

  popoverStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
  }
}

function togglePopover() {
  if (popoverOpen.value) {
    closePopover()
    return
  }
  cachedColors.value = readCachedColors()
  syncInternalFromHex(activeHex.value)
  popoverOpen.value = true
  void positionPopover()
}

function closePopover() {
  if (!popoverOpen.value) return
  if (!isDefaultSelected.value) {
    pushColorToCache(activeHex.value)
  }
  popoverOpen.value = false
}

function onGlobalPointerDown(e: MouseEvent) {
  if (!popoverOpen.value) return
  const target = e.target as Node | null
  if (popoverRef.value?.contains(target) || triggerRef.value?.contains(target)) return
  closePopover()
}

function onGlobalKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && popoverOpen.value) {
    e.stopPropagation()
    closePopover()
  }
}

function onWindowResize() {
  if (popoverOpen.value) void positionPopover()
}

watch(
  () => [props.modelValue, props.tileAccent] as const,
  () => {
    if (!draggingSv.value && !draggingHue.value) {
      syncInternalFromHex(activeHex.value)
    }
  },
  { immediate: true },
)

onMounted(() => {
  document.addEventListener('mousedown', onGlobalPointerDown)
  document.addEventListener('keydown', onGlobalKeydown, true)
  window.addEventListener('resize', onWindowResize)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', onGlobalPointerDown)
  document.removeEventListener('keydown', onGlobalKeydown, true)
  window.removeEventListener('resize', onWindowResize)
})
</script>

<template>
  <div class="space-y-1.5">
    <!-- Compact Trigger Row: Neutral / Default + Color Modal Trigger -->
    <div class="flex flex-wrap items-center gap-2">
      <button
        type="button"
        :title="defaultOptionMeta.title"
        :aria-label="defaultOptionMeta.title"
        class="inline-flex h-8 items-center gap-2 rounded-lg border px-2.5 text-xs font-medium transition"
        :class="
          isDefaultSelected
            ? 'border-amber-400/80 bg-slate-800 text-white ring-1 ring-amber-400/40'
            : 'border-slate-700/80 bg-slate-900/90 text-slate-300 hover:border-slate-500 hover:text-white'
        "
        @click="selectPreset(defaultOptionMeta.value)"
      >
        <span
          class="relative flex h-4 w-4 shrink-0 items-center justify-center rounded-xs border border-white/25"
          :style="{ backgroundColor: defaultOptionMeta.hex }"
        >
          <Check
            v-if="isDefaultSelected"
            class="h-2.5 w-2.5 text-white drop-shadow-[0_1px_2px_rgba(0,0,0,0.9)]"
          />
          <Link2
            v-else-if="defaultOptionMeta.isTileAccentRef"
            class="h-2.5 w-2.5 text-white/90 drop-shadow-[0_1px_2px_rgba(0,0,0,0.9)]"
          />
        </span>
        <span>{{ defaultOptionMeta.label }}</span>
      </button>

      <!-- Color Modal Trigger -->
      <button
        ref="triggerRef"
        type="button"
        title="Open color selector"
        aria-label="Open color selector"
        :aria-expanded="popoverOpen"
        class="relative inline-flex h-8 items-center gap-2 rounded-lg border px-2.5 text-xs font-medium transition"
        :class="
          popoverOpen || isCustomSelected
            ? 'border-amber-400/80 bg-slate-800 text-white ring-1 ring-amber-400/40'
            : 'border-dashed border-slate-600 bg-slate-900/90 text-slate-300 hover:border-slate-400 hover:text-white'
        "
        @click="togglePopover"
      >
        <span
          class="h-4 w-4 shrink-0 rounded-xs border border-white/25"
          :style="
            isCustomSelected
              ? { backgroundColor: activeHex }
              : {
                  backgroundImage:
                    'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #06b6d4, #3b82f6, #8b5cf6, #ef4444)',
                }
          "
        />
        <span v-if="isCustomSelected" class="font-mono text-[11px]">{{ activeHex }}</span>
        <span v-else>Select Color</span>
        <Palette class="h-3.5 w-3.5 shrink-0 opacity-75" />
      </button>
    </div>

    <!-- Custom In-App Color Matrix & Generic HSV + HEX/RGB Popover (zero native browser picker) -->
    <Teleport to="body">
      <div
        v-if="popoverOpen"
        ref="popoverRef"
        :style="popoverStyle"
        class="fixed z-[75] w-[320px] max-w-[calc(100vw-24px)] rounded-xl border border-slate-700 bg-slate-900 p-3.5 shadow-2xl space-y-3"
      >
        <div class="flex items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <span
              class="h-4 w-4 rounded-sm border border-white/25"
              :style="{ backgroundColor: activeHex }"
            />
            <span class="text-xs font-semibold text-white">Color Selector</span>
          </div>
          <button
            type="button"
            aria-label="Close color selector"
            class="flex h-6 w-6 items-center justify-center rounded-md text-slate-400 hover:bg-slate-800 hover:text-white"
            @click="closePopover"
          >
            <X class="h-3.5 w-3.5" />
          </button>
        </div>

        <!-- Multiple Predefined Color Lines (Color Matrix) -->
        <div class="space-y-1.5">
          <div
            v-for="line in MATRIX_LINES"
            :key="line.label"
            class="grid grid-cols-9 gap-1"
            :title="line.label"
          >
            <button
              v-for="hex in line.colors"
              :key="hex"
              type="button"
              :title="`${line.label} ${hex}`"
              :aria-label="`Select color ${hex}`"
              class="relative flex h-6 w-full items-center justify-center rounded-md border transition hover:scale-110 hover:z-10"
              :class="
                isCustomSelected && activeHex.toLowerCase() === hex.toLowerCase()
                  ? 'border-white ring-1 ring-amber-400'
                  : 'border-black/25'
              "
              :style="{ backgroundColor: hex }"
              @click="selectMatrixHex(hex)"
            >
              <Check
                v-if="isCustomSelected && activeHex.toLowerCase() === hex.toLowerCase()"
                class="h-3 w-3 text-white drop-shadow-[0_1px_2px_rgba(0,0,0,0.9)]"
              />
            </button>
          </div>

          <!-- Extra Row: Last Used Colors -->
          <div class="pt-1.5">
            <p class="mb-1 text-[10px] font-medium uppercase tracking-wider text-slate-500">Last Used</p>
            <div class="grid grid-cols-9 gap-1">
              <template v-for="slot in MAX_CACHED_COLORS" :key="slot">
                <button
                  v-if="cachedColors[slot - 1]"
                  type="button"
                  :title="`Recent ${cachedColors[slot - 1]}`"
                  :aria-label="`Select recent color ${cachedColors[slot - 1]}`"
                  class="relative flex h-6 w-full items-center justify-center rounded-md border transition hover:scale-110 hover:z-10"
                  :class="
                    isCustomSelected && activeHex.toLowerCase() === cachedColors[slot - 1]
                      ? 'border-white ring-1 ring-amber-400'
                      : 'border-slate-700/80'
                  "
                  :style="{ backgroundColor: cachedColors[slot - 1] }"
                  @click="selectCachedHex(cachedColors[slot - 1]!)"
                >
                  <Check
                    v-if="isCustomSelected && activeHex.toLowerCase() === cachedColors[slot - 1]"
                    class="h-3 w-3 text-white drop-shadow-[0_1px_2px_rgba(0,0,0,0.9)]"
                  />
                </button>
                <span
                  v-else
                  aria-hidden="true"
                  class="h-6 w-full rounded-md border border-dashed border-slate-800 bg-slate-950/50"
                />
              </template>
            </div>
          </div>
        </div>

        <!-- Generic 2D Saturation / Brightness Plane -->
        <div class="space-y-2 border-t border-slate-800 pt-2.5">
          <div
            ref="svPlaneRef"
            class="relative h-32 w-full cursor-crosshair touch-none select-none overflow-hidden rounded-lg border border-slate-700"
            :style="{
              backgroundColor: pureHueHex,
              backgroundImage:
                'linear-gradient(to top, #000000, transparent), linear-gradient(to right, #ffffff, transparent)',
            }"
            @pointerdown="onSvPointerDown"
            @pointermove="onSvPointerMove"
            @pointerup="onSvPointerUp"
            @pointercancel="onSvPointerUp"
          >
            <span
              class="pointer-events-none absolute h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1px_rgba(0,0,0,0.75)]"
              :style="{
                left: `${Math.round(sat * 100)}%`,
                top: `${Math.round((1 - val) * 100)}%`,
                backgroundColor: activeHex,
              }"
            />
          </div>

          <!-- Hue Spectrum Bar -->
          <div
            ref="hueBarRef"
            class="relative h-3.5 w-full cursor-ew-resize touch-none select-none rounded-full border border-slate-700"
            style="background: linear-gradient(to right, #ff0000 0%, #ffff00 17%, #00ff00 33%, #00ffff 50%, #0000ff 67%, #ff00ff 83%, #ff0000 100%)"
            @pointerdown="onHuePointerDown"
            @pointermove="onHuePointerMove"
            @pointerup="onHuePointerUp"
            @pointercancel="onHuePointerUp"
          >
            <span
              class="pointer-events-none absolute top-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1px_rgba(0,0,0,0.75)]"
              :style="{
                left: `${(hue / 360) * 100}%`,
                backgroundColor: pureHueHex,
              }"
            />
          </div>
        </div>

        <!-- HEX & RGB Inputs -->
        <div class="grid grid-cols-12 gap-1.5 border-t border-slate-800 pt-2.5">
          <div class="col-span-5">
            <label class="mb-0.5 block text-[10px] font-medium uppercase text-slate-400">HEX</label>
            <input
              :value="hexInput"
              type="text"
              maxlength="7"
              spellcheck="false"
              placeholder="#0ea5e9"
              class="w-full rounded-md border border-slate-700 bg-slate-950 px-2 py-1.5 font-mono text-[11px] text-white"
              @input="onHexFieldInput"
              @blur="onHexFieldBlur"
            />
          </div>
          <div class="col-span-7 grid grid-cols-3 gap-1">
            <div>
              <label class="mb-0.5 block text-[10px] font-medium uppercase text-slate-400">R</label>
              <input
                :value="rInput"
                type="text"
                inputmode="numeric"
                maxlength="3"
                class="w-full rounded-md border border-slate-700 bg-slate-950 px-1.5 py-1.5 text-center font-mono text-[11px] text-white"
                @input="onRgbFieldInput('r', $event)"
                @blur="onRgbFieldBlur"
              />
            </div>
            <div>
              <label class="mb-0.5 block text-[10px] font-medium uppercase text-slate-400">G</label>
              <input
                :value="gInput"
                type="text"
                inputmode="numeric"
                maxlength="3"
                class="w-full rounded-md border border-slate-700 bg-slate-950 px-1.5 py-1.5 text-center font-mono text-[11px] text-white"
                @input="onRgbFieldInput('g', $event)"
                @blur="onRgbFieldBlur"
              />
            </div>
            <div>
              <label class="mb-0.5 block text-[10px] font-medium uppercase text-slate-400">B</label>
              <input
                :value="bInput"
                type="text"
                inputmode="numeric"
                maxlength="3"
                class="w-full rounded-md border border-slate-700 bg-slate-950 px-1.5 py-1.5 text-center font-mono text-[11px] text-white"
                @input="onRgbFieldInput('b', $event)"
                @blur="onRgbFieldBlur"
              />
            </div>
          </div>
        </div>

        <div class="flex items-center justify-end gap-1.5 pt-0.5">
          <button
            type="button"
            class="h-7 rounded-md border border-slate-700 px-2.5 text-[11px] font-medium text-slate-300 hover:bg-slate-800"
            @click="selectPreset(resetTarget); closePopover()"
          >
            {{ mode === 'state' ? 'Use Tile Accent' : 'Reset' }}
          </button>
          <button
            type="button"
            class="h-7 rounded-md bg-amber-500 px-3 text-[11px] font-semibold text-slate-950 hover:bg-amber-400"
            @click="closePopover"
          >
            Done
          </button>
        </div>
      </div>
    </Teleport>
  </div>
</template>
