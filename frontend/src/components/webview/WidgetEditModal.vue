<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Pencil, Plus, Trash2, X } from 'lucide-vue-next'
import type {
  ColorOption,
  FontSizeOption,
  GraphSeriesConfig,
  GraphTimeRange,
  LampMode,
  LampStateConfig,
  RegisterMapping,
  TelemetrySample,
  TileAccent,
  TileSize,
  WebviewDevice,
  WebviewWidget,
  WidgetType,
} from '../../types/webview'
import { clampPrecision, effectiveDisplayBounds } from '../../utils/precision'
import {
  extractGraphSeries,
  GRAPH_DEFAULT_SERIES_COLORS,
  GRAPH_TIME_RANGES,
  MAX_GRAPH_SERIES,
  resolveGraphTimeRange,
  resolveStateColorValue,
  resolveStateVisual,
  resolveTileAccent,
  resolveTileAccentHex,
  resolveTileSize,
  TILE_SIZES,
  tileSizeShortLabel,
} from '../../utils/tiles'
import {
  firstError,
  validateIntegerRange,
  validateNumberRange,
  validateOptionalText,
  validateRequiredText,
} from '../../utils/validation'
import { useToastStore } from '../../stores/toast'
import CustomSelect, { type SelectOption } from '../CustomSelect.vue'
import NumericTextInput from '../NumericTextInput.vue'
import RegisterPicker from '../RegisterPicker.vue'
import WidgetTileRenderer from '../widgets/WidgetTileRenderer.vue'
import ColorMatrixPicker from './ColorMatrixPicker.vue'

export interface WidgetDraftPayload {
  id?: string
  name: string
  widgetType: WidgetType
  registerMappingId?: string
  style: Record<string, unknown>
  extra: Record<string, unknown>
}

const props = defineProps<{
  open: boolean
  widget: WebviewWidget | null
  devices: WebviewDevice[]
  mappings: RegisterMapping[]
  samples?: Record<string, TelemetrySample>
  submitLabel?: string
}>()

const emit = defineEmits<{
  close: []
  save: [payload: WidgetDraftPayload]
  delete: [widget: WebviewWidget]
}>()

const toast = useToastStore()

const TILE_TYPE_OPTIONS: SelectOption<WidgetType>[] = [
  { value: 'display', label: 'Display' },
  { value: 'lamp', label: 'Lamp' },
  { value: 'switch', label: 'Switch' },
  { value: 'button', label: 'Button' },
  { value: 'number_input', label: 'Number Input' },
  { value: 'graph', label: 'Graph' },
]

const GRAPH_RANGE_OPTIONS: SelectOption<GraphTimeRange>[] = GRAPH_TIME_RANGES.map((r) => ({
  value: r.value,
  label: r.label,
}))

const FONT_SIZE_OPTIONS: SelectOption<FontSizeOption>[] = [
  { value: 'sm', label: 'Small' },
  { value: 'md', label: 'Medium' },
  { value: 'lg', label: 'Large' },
  { value: 'xl', label: 'Extra Large' },
]

const LAMP_MODE_OPTIONS: SelectOption<LampMode>[] = [
  { value: 'on', label: 'On' },
  { value: 'blink', label: 'Blink' },
  { value: 'off', label: 'Off' },
]

const widName = ref('')
const widType = ref<WidgetType>('display')
const widMappingId = ref('')
const widTileSize = ref<TileSize>('wide')
const widTileColor = ref<TileAccent>('default')

// Display & Number Input
const widFontSize = ref<FontSizeOption>('lg')
const widPrefix = ref('')
const widUnit = ref('')
const widPrecision = ref('')
const widMin = ref('')
const widMax = ref('')

// Lamp
const widLampOn = ref<LampStateConfig>({ mode: 'on', color: 'tile_accent', text: 'ON' })
const widLampOff = ref<LampStateConfig>({ mode: 'off', color: 'tile_accent', text: 'OFF' })

// Switch (ON & OFF color overrides)
const widSwitchColor = ref<ColorOption>('tile_accent')
const widSwitchOffColor = ref<ColorOption>('tile_accent')
const widSwitchOnLabel = ref('ON')
const widSwitchOffLabel = ref('OFF')

// Button (Idle + Triggered labels)
const widBtnColor = ref<ColorOption>('tile_accent')
const widBtnLabel = ref('Trigger')
const widBtnTriggeredLabel = ref('Triggered...')
const widBtnTriggerMs = ref('1000')

// Graph (up to 5 registers, Y-axis scale, time range, per-variable color/unit/precision)
interface GraphSeriesDraft {
  uid: number
  registerMappingId: string
  color: ColorOption
  label: string
  unit: string
  precision: string
}
let graphSeriesSeq = 1
const widGraphSeries = ref<GraphSeriesDraft[]>([])
const widGraphTimeRange = ref<GraphTimeRange>('1h')
const widGraphYMin = ref('')
const widGraphYMax = ref('')
const widGraphSmooth = ref(false)

// Separate Graph Register Edit Modal state
const seriesModalOpen = ref(false)
const editingSeriesUid = ref<number | null>(null)
const seriesDraftMappingId = ref('')
const seriesDraftLabel = ref('')
const seriesDraftColor = ref<ColorOption>(GRAPH_DEFAULT_SERIES_COLORS[0])
const seriesDraftUnit = ref('')
const seriesDraftPrecision = ref('')
const seriesDraftSubmitted = ref(false)
const seriesDraftTouched = ref<Record<string, boolean>>({})

const seriesDraftMapping = computed(() =>
  seriesDraftMappingId.value ? mappingsById.value[seriesDraftMappingId.value] ?? null : null,
)

const seriesDraftErrors = computed(() => {
  let mappingErr: string | null = null
  if (!seriesDraftMappingId.value) {
    mappingErr = 'Please select a register.'
  } else {
    const dup = widGraphSeries.value.some(
      (s) => s.uid !== editingSeriesUid.value && s.registerMappingId === seriesDraftMappingId.value,
    )
    if (dup) {
      mappingErr = 'This register is already added to the graph.'
    }
  }
  return {
    mappingId: mappingErr,
    label: validateOptionalText(seriesDraftLabel.value, 'Legend label', 64),
    unit: validateOptionalText(seriesDraftUnit.value, 'Unit', 32),
    precision: validateIntegerRange(seriesDraftPrecision.value, 'Decimal places', -5, 5, { required: false }),
  }
})

function showSeriesDraftErr(field: 'mappingId' | 'label' | 'unit' | 'precision'): string | null {
  return seriesDraftSubmitted.value || seriesDraftTouched.value[field] ? seriesDraftErrors.value[field] : null
}

function openAddGraphSeriesModal() {
  if (widGraphSeries.value.length >= MAX_GRAPH_SERIES) return
  const usedIds = new Set(widGraphSeries.value.map((s) => s.registerMappingId).filter(Boolean))
  editingSeriesUid.value = null
  seriesDraftMappingId.value = props.mappings.find((m) => !usedIds.has(m.id))?.id || ''
  seriesDraftLabel.value = ''
  seriesDraftColor.value =
    GRAPH_DEFAULT_SERIES_COLORS[widGraphSeries.value.length % GRAPH_DEFAULT_SERIES_COLORS.length]
  seriesDraftUnit.value = ''
  seriesDraftPrecision.value = ''
  seriesDraftSubmitted.value = false
  seriesDraftTouched.value = {}
  seriesModalOpen.value = true
}

function openEditGraphSeriesModal(row: GraphSeriesDraft) {
  editingSeriesUid.value = row.uid
  seriesDraftMappingId.value = row.registerMappingId
  seriesDraftLabel.value = row.label
  seriesDraftColor.value = row.color
  seriesDraftUnit.value = row.unit
  seriesDraftPrecision.value = row.precision
  seriesDraftSubmitted.value = false
  seriesDraftTouched.value = {}
  seriesModalOpen.value = true
}

function saveGraphSeriesModal() {
  seriesDraftSubmitted.value = true
  const err = firstError(seriesDraftErrors.value)
  if (err) {
    toast.error(err, 'Validation Error')
    return
  }
  const normalizedPrecision =
    seriesDraftPrecision.value.trim() !== '' ? String(clampPrecision(seriesDraftPrecision.value)) : ''
  if (editingSeriesUid.value !== null) {
    widGraphSeries.value = widGraphSeries.value.map((s) =>
      s.uid === editingSeriesUid.value
        ? {
            ...s,
            registerMappingId: seriesDraftMappingId.value,
            label: seriesDraftLabel.value.trim(),
            color: seriesDraftColor.value,
            unit: seriesDraftUnit.value.trim(),
            precision: normalizedPrecision,
          }
        : s,
    )
  } else {
    widGraphSeries.value = [
      ...widGraphSeries.value,
      {
        uid: graphSeriesSeq++,
        registerMappingId: seriesDraftMappingId.value,
        label: seriesDraftLabel.value.trim(),
        color: seriesDraftColor.value,
        unit: seriesDraftUnit.value.trim(),
        precision: normalizedPrecision,
      },
    ]
  }
  seriesModalOpen.value = false
}

function removeGraphSeriesRow(uid: number) {
  if (widGraphSeries.value.length <= 1) return
  widGraphSeries.value = widGraphSeries.value.filter((s) => s.uid !== uid)
}

// Preview interactive state for Lamp, Switch & Button
const previewBoolState = ref(true)
const previewBtnTriggered = ref(false)
const submitted = ref(false)
const touched = ref<Record<string, boolean>>({})

const isEditing = computed(() => Boolean(props.widget))
const mappingsById = computed<Record<string, RegisterMapping>>(() => {
  const out: Record<string, RegisterMapping> = {}
  for (const m of props.mappings) {
    out[m.id] = m
  }
  return out
})
const boundMapping = computed(() => props.mappings.find((m) => m.id === widMappingId.value) ?? undefined)
const boundSample = computed(() => (widMappingId.value && props.samples ? props.samples[widMappingId.value] : undefined))
const colorPickerHex = computed(() => resolveTileAccentHex(widTileColor.value))

const effectivePrecision = computed(() => {
  if (widPrecision.value.trim() !== '') return clampPrecision(widPrecision.value)
  return clampPrecision(boundMapping.value?.protocolAttributes?.precision ?? 0)
})

const boundTypeBounds = computed(() => {
  const dt = boundMapping.value?.protocolAttributes?.dataType
  return effectiveDisplayBounds(dt, effectivePrecision.value)
})

// --- Runtime + On-Save Validation ---
const fieldErrors = computed<Record<string, string | null>>(() => {
  const errs: Record<string, string | null> = {
    name: validateRequiredText(widName.value, 'Tile title', { min: 1, max: 100 }),
    precision: null,
    prefix: null,
    unit: null,
    min: null,
    max: null,
    triggerMs: null,
    graphSeries: null,
    graphYMin: null,
    graphYMax: null,
  }

  if (widType.value === 'display' || widType.value === 'number_input') {
    errs.precision = validateIntegerRange(widPrecision.value, 'Decimal places', -5, 5, { required: false })
    errs.prefix = validateOptionalText(widPrefix.value, 'Prefix', 32)
    errs.unit = validateOptionalText(widUnit.value, 'Unit', 32)
  }

  if (widType.value === 'number_input') {
    const bounds = boundTypeBounds.value
    errs.min = validateNumberRange(widMin.value, 'Min value', bounds.typeMin, bounds.typeMax, { required: false })
    errs.max = validateNumberRange(widMax.value, 'Max value', bounds.typeMin, bounds.typeMax, { required: false })
    if (!errs.min && !errs.max && widMin.value.trim() !== '' && widMax.value.trim() !== '') {
      if (Number(widMin.value) > Number(widMax.value)) {
        errs.min = 'Min value cannot exceed max value.'
      }
    }
  }

  if (widType.value === 'button') {
    errs.triggerMs = validateIntegerRange(widBtnTriggerMs.value, 'Trigger time', 100, 60000, { required: true })
  }

  if (widType.value === 'graph') {
    if (widGraphSeries.value.length === 0 || widGraphSeries.value.length > MAX_GRAPH_SERIES) {
      errs.graphSeries = `Select between 1 and ${MAX_GRAPH_SERIES} registers.`
    } else {
      const seen = new Set<string>()
      for (let i = 0; i < widGraphSeries.value.length; i++) {
        const row = widGraphSeries.value[i]
        if (!row.registerMappingId) {
          errs.graphSeries = `Select a register for variable #${i + 1}.`
          break
        }
        if (seen.has(row.registerMappingId)) {
          errs.graphSeries = 'Each variable in the graph must use a different register.'
          break
        }
        seen.add(row.registerMappingId)
        if (row.label.trim().length > 64) {
          errs.graphSeries = `Variable #${i + 1} label must be 64 characters or fewer.`
          break
        }
      }
    }
    errs.graphYMin = validateNumberRange(widGraphYMin.value, 'Y-axis min', undefined, undefined, { required: false })
    errs.graphYMax = validateNumberRange(widGraphYMax.value, 'Y-axis max', undefined, undefined, { required: false })
    if (!errs.graphYMin && !errs.graphYMax && widGraphYMin.value.trim() !== '' && widGraphYMax.value.trim() !== '') {
      if (Number(widGraphYMin.value) >= Number(widGraphYMax.value)) {
        errs.graphYMin = 'Y-axis min must be less than max.'
      }
    }
  }

  return errs
})

function showErr(field: string): string | null {
  return submitted.value || touched.value[field] ? fieldErrors.value[field] || null : null
}

function markTouched(field: string) {
  touched.value = { ...touched.value, [field]: true }
}

function buildStyleAndExtra(): { style: Record<string, unknown>; extra: Record<string, unknown> } {
  const style: Record<string, unknown> = {
    tileSize: widTileSize.value,
  }
  if (widTileColor.value !== 'default') {
    style.tileColor = widTileColor.value
  }
  const extra: Record<string, unknown> = {}

  if (widType.value === 'display') {
    style.fontSize = widFontSize.value
    extra.prefix = widPrefix.value
    if (widUnit.value.trim() !== '') extra.unit = widUnit.value.trim()
    if (widPrecision.value.trim() !== '') extra.precision = clampPrecision(widPrecision.value)
  } else if (widType.value === 'number_input') {
    style.fontSize = widFontSize.value
    extra.prefix = widPrefix.value
    if (widUnit.value.trim() !== '') extra.unit = widUnit.value.trim()
    if (widPrecision.value.trim() !== '') extra.precision = clampPrecision(widPrecision.value)
    if (widMin.value.trim() !== '' && Number.isFinite(Number(widMin.value))) {
      extra.min = Number(widMin.value)
    }
    if (widMax.value.trim() !== '' && Number.isFinite(Number(widMax.value))) {
      extra.max = Number(widMax.value)
    }
  } else if (widType.value === 'lamp') {
    extra.on = { ...widLampOn.value }
    extra.off = { ...widLampOff.value }
  } else if (widType.value === 'switch') {
    style.color = widSwitchColor.value
    style.offColor = widSwitchOffColor.value
    extra.onLabel = widSwitchOnLabel.value || 'ON'
    extra.offLabel = widSwitchOffLabel.value || 'OFF'
  } else if (widType.value === 'button') {
    style.color = widBtnColor.value
    extra.label = widBtnLabel.value || widName.value || 'Trigger'
    if (widBtnTriggeredLabel.value.trim() !== '') {
      extra.triggeredLabel = widBtnTriggeredLabel.value.trim()
    }
    extra.triggerTimeMs = Math.max(100, Math.min(60000, Math.round(Number(widBtnTriggerMs.value) || 1000)))
  } else if (widType.value === 'graph') {
    const series: GraphSeriesConfig[] = widGraphSeries.value
      .filter((s) => Boolean(s.registerMappingId))
      .slice(0, MAX_GRAPH_SERIES)
      .map((s) => ({
        registerMappingId: s.registerMappingId,
        color: s.color,
        ...(s.label.trim() ? { label: s.label.trim() } : {}),
        ...(s.unit.trim() ? { unit: s.unit.trim() } : {}),
        ...(s.precision.trim() !== '' ? { precision: clampPrecision(s.precision) } : {}),
      }))
    extra.series = series
    extra.timeRange = widGraphTimeRange.value
    extra.smooth = widGraphSmooth.value
    if (widGraphYMin.value.trim() !== '' && Number.isFinite(Number(widGraphYMin.value))) {
      extra.yMin = Number(widGraphYMin.value)
    }
    if (widGraphYMax.value.trim() !== '' && Number.isFinite(Number(widGraphYMax.value))) {
      extra.yMax = Number(widGraphYMax.value)
    }
  }

  return { style, extra }
}

const previewWidget = computed<WebviewWidget>(() => {
  const { style, extra } = buildStyleAndExtra()
  const primaryMappingId =
    widType.value === 'graph'
      ? widGraphSeries.value[0]?.registerMappingId || undefined
      : widMappingId.value || undefined
  return {
    id: props.widget?.id || 'preview',
    dashboardId: props.widget?.dashboardId || 'preview',
    registerMappingId: primaryMappingId,
    name: widName.value.trim() || defaultPlaceholderName(widType.value),
    widgetType: widType.value,
    sortOrder: props.widget?.sortOrder ?? 0,
    style,
    extra,
  }
})

function defaultPlaceholderName(t: WidgetType): string {
  switch (t) {
    case 'lamp':
      return 'Pump Status'
    case 'switch':
      return 'Main Valve'
    case 'button':
      return 'Reset Alarm'
    case 'number_input':
      return 'Target Setpoint'
    case 'graph':
      return 'Process Trend'
    default:
      return 'Water Temperature'
  }
}

const previewBoxStyle = computed(() => {
  switch (widTileSize.value) {
    case 'sm':
      return { width: '156px', height: '138px' }
    case 'tall':
      return { width: '156px', height: '288px' }
    case 'lg':
      return { width: '324px', height: '288px' }
    case 'xl':
      return { width: '100%', maxWidth: '616px', aspectRatio: '660 / 288' }
    default:
      return { width: '324px', height: '138px' }
  }
})

function hydrateFromProps() {
  submitted.value = false
  touched.value = {}
  previewBoolState.value = true
  previewBtnTriggered.value = false
  const w = props.widget
  if (!w) {
    widName.value = ''
    widType.value = 'display'
    widMappingId.value = props.mappings[0]?.id || ''
    widTileSize.value = 'wide'
    widTileColor.value = 'default'
    widFontSize.value = 'lg'
    widPrefix.value = ''
    widUnit.value = ''
    widPrecision.value = ''
    widMin.value = ''
    widMax.value = ''
    widLampOn.value = { mode: 'on', color: 'tile_accent', text: 'ON' }
    widLampOff.value = { mode: 'off', color: 'tile_accent', text: 'OFF' }
    widSwitchColor.value = 'tile_accent'
    widSwitchOffColor.value = 'tile_accent'
    widSwitchOnLabel.value = 'ON'
    widSwitchOffLabel.value = 'OFF'
    widBtnColor.value = 'tile_accent'
    widBtnLabel.value = 'Trigger'
    widBtnTriggeredLabel.value = 'Triggered...'
    widBtnTriggerMs.value = '1000'
    widGraphSeries.value = props.mappings[0]
      ? [
          {
            uid: graphSeriesSeq++,
            registerMappingId: props.mappings[0].id,
            color: GRAPH_DEFAULT_SERIES_COLORS[0],
            label: '',
            unit: '',
            precision: '',
          },
        ]
      : []
    seriesModalOpen.value = false
    widGraphTimeRange.value = '1h'
    widGraphYMin.value = ''
    widGraphYMax.value = ''
    widGraphSmooth.value = false
    return
  }

  widName.value = w.name
  widType.value = w.widgetType
  widMappingId.value = w.registerMappingId || ''
  widTileSize.value = resolveTileSize(w)
  widTileColor.value = resolveTileAccent(w)
  widFontSize.value = (w.style?.fontSize as FontSizeOption) || 'lg'
  widPrefix.value = String(w.extra?.prefix ?? '')
  widUnit.value = w.extra?.unit !== undefined && w.extra?.unit !== null ? String(w.extra.unit) : ''
  widPrecision.value =
    w.extra?.precision !== undefined && w.extra?.precision !== null && w.extra?.precision !== ''
      ? String(clampPrecision(w.extra.precision))
      : ''
  widMin.value = w.extra?.min !== undefined && w.extra?.min !== null ? String(w.extra.min) : ''
  widMax.value = w.extra?.max !== undefined && w.extra?.max !== null ? String(w.extra.max) : ''

  const onCfg = (w.extra?.on as Partial<LampStateConfig> | undefined) || {}
  const offCfg = (w.extra?.off as Partial<LampStateConfig> | undefined) || {}
  widLampOn.value = {
    mode: (onCfg.mode as LampMode) || 'on',
    color: resolveStateColorValue(onCfg.color, 'tile_accent'),
    text: onCfg.text ?? 'ON',
  }
  widLampOff.value = {
    mode: (offCfg.mode as LampMode) || 'off',
    color: resolveStateColorValue(offCfg.color, 'tile_accent'),
    text: offCfg.text ?? 'OFF',
  }

  widSwitchColor.value = resolveStateColorValue(w.style?.color, 'tile_accent')
  widSwitchOffColor.value = resolveStateColorValue(w.style?.offColor, 'tile_accent')
  widSwitchOnLabel.value = String(w.extra?.onLabel ?? 'ON')
  widSwitchOffLabel.value = String(w.extra?.offLabel ?? 'OFF')

  widBtnColor.value = resolveStateColorValue(w.style?.color, 'tile_accent')
  widBtnLabel.value = String(w.extra?.label ?? w.name)
  widBtnTriggeredLabel.value = String(w.extra?.triggeredLabel ?? 'Triggered...')
  widBtnTriggerMs.value = String(Number(w.extra?.triggerTimeMs ?? 1000))

  const existingSeries = extractGraphSeries(w)
  widGraphSeries.value =
    existingSeries.length > 0
      ? existingSeries.map((s) => ({
          uid: graphSeriesSeq++,
          registerMappingId: s.registerMappingId,
          color: s.color,
          label: s.label || '',
          unit: s.unit || '',
          precision: s.precision !== undefined ? String(s.precision) : '',
        }))
      : w.registerMappingId || props.mappings[0]?.id
        ? [
            {
              uid: graphSeriesSeq++,
              registerMappingId: w.registerMappingId || props.mappings[0]?.id || '',
              color: GRAPH_DEFAULT_SERIES_COLORS[0],
              label: '',
              unit: '',
              precision: '',
            },
          ]
        : []
  seriesModalOpen.value = false
  widGraphTimeRange.value = resolveGraphTimeRange(w.extra?.timeRange)
  widGraphYMin.value = w.extra?.yMin !== undefined && w.extra?.yMin !== null ? String(w.extra.yMin) : ''
  widGraphYMax.value = w.extra?.yMax !== undefined && w.extra?.yMax !== null ? String(w.extra.yMax) : ''
  widGraphSmooth.value = Boolean(w.extra?.smooth)
}

watch(widType, (next, prev) => {
  if (next === 'graph') {
    if (widGraphSeries.value.length === 0) {
      const firstId = widMappingId.value || props.mappings[0]?.id || ''
      if (firstId) {
        widGraphSeries.value = [
          {
            uid: graphSeriesSeq++,
            registerMappingId: firstId,
            color: GRAPH_DEFAULT_SERIES_COLORS[0],
            label: '',
            unit: '',
            precision: '',
          },
        ]
      }
    }
    if (!isEditing.value && prev !== 'graph' && (widTileSize.value === 'sm' || widTileSize.value === 'wide')) {
      widTileSize.value = 'lg'
    }
  }
})

watch(
  () => [props.open, props.widget] as const,
  ([isOpen]) => {
    if (isOpen) hydrateFromProps()
  },
  { immediate: true },
)

function handleSubmit() {
  submitted.value = true
  const err = firstError(fieldErrors.value)
  if (err) {
    toast.error(err, 'Validation Error')
    return
  }
  const { style, extra } = buildStyleAndExtra()
  const primaryMappingId =
    widType.value === 'graph'
      ? widGraphSeries.value[0]?.registerMappingId || undefined
      : widMappingId.value || undefined
  emit('save', {
    id: props.widget?.id,
    name: widName.value.trim(),
    widgetType: widType.value,
    registerMappingId: primaryMappingId,
    style,
    extra,
  })
}

function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape' || !props.open) return
  if (seriesModalOpen.value) {
    seriesModalOpen.value = false
    return
  }
  emit('close')
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-3 backdrop-blur-xs sm:p-4"
      @click.self="emit('close')"
    >
      <form
        novalidate
        class="flex max-h-[92dvh] w-full flex-col overflow-hidden rounded-2xl border border-slate-700 bg-slate-900 shadow-2xl transition-[max-width] duration-200"
        :class="widTileSize === 'xl' ? 'max-w-7xl' : 'max-w-6xl'"
        @submit.prevent="handleSubmit"
      >
        <!-- Header -->
        <div class="flex shrink-0 items-center justify-between border-b border-slate-800 px-5 py-4">
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-white">
              {{ isEditing ? 'Edit Tile' : 'Add Tile' }}
            </h3>
            <p class="mt-0.5 text-[11px] text-slate-400">
              Configure parameters on the left and inspect the end-result tile live on the right.
            </p>
          </div>
          <button
            type="button"
            aria-label="Close modal"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
            @click="emit('close')"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <!-- Split Body: Parameters (Left) + Live Preview (Right) -->
        <div class="flex min-h-0 flex-1 flex-col overflow-y-auto lg:flex-row lg:overflow-hidden">
          <!-- LEFT COLUMN: Tile Parameters -->
          <div class="min-w-0 flex-1 space-y-5 px-5 py-4 lg:overflow-y-auto">
            <!-- BASIC IDENTITY -->
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Tile Title</label>
                <input
                  v-model="widName"
                  :placeholder="defaultPlaceholderName(widType)"
                  class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-xs text-white focus:outline-none"
                  :class="showErr('name') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
                  @blur="markTouched('name')"
                />
                <p v-if="showErr('name')" class="mt-1 text-[11px] text-rose-400">{{ showErr('name') }}</p>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Tile Type</label>
                <CustomSelect
                  v-model="widType"
                  :options="TILE_TYPE_OPTIONS"
                  aria-label="Tile type"
                />
              </div>
            </div>

            <!-- TILE GEOMETRY & ACCENT -->
            <div class="grid gap-4 rounded-xl border border-slate-800 bg-slate-950/60 p-4 sm:grid-cols-2">
              <div>
                <label class="mb-2 block text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                  Tile Size
                </label>
                <div class="grid grid-cols-5 gap-1.5">
                  <button
                    v-for="sz in TILE_SIZES"
                    :key="sz.value"
                    type="button"
                    :title="`${sz.label} (${sz.shortLabel}) - ${sz.description}`"
                    class="flex flex-col items-center justify-center gap-1.5 rounded-lg border px-1.5 py-2 text-[11px] font-medium transition"
                    :class="
                      widTileSize === sz.value
                        ? 'border-amber-500/70 bg-amber-500/15 text-amber-200'
                        : 'border-slate-800 bg-slate-900/80 text-slate-400 hover:border-slate-700 hover:text-white'
                    "
                    @click="widTileSize = sz.value"
                  >
                    <span
                      v-if="sz.cols === 4"
                      class="grid h-5 w-7 grid-cols-4 grid-rows-2 gap-0.5"
                    >
                      <span
                        v-for="cell in 8"
                        :key="cell"
                        class="rounded-[2px]"
                        :class="widTileSize === sz.value ? 'bg-amber-400' : 'bg-slate-500'"
                      />
                    </span>
                    <span v-else class="grid h-5 w-5 grid-cols-2 grid-rows-2 gap-0.5">
                      <span
                        class="rounded-[2px]"
                        :class="widTileSize === sz.value ? 'bg-amber-400' : 'bg-slate-500'"
                      />
                      <span
                        class="rounded-[2px]"
                        :class="
                          sz.cols === 2
                            ? widTileSize === sz.value
                              ? 'bg-amber-400'
                              : 'bg-slate-500'
                            : 'bg-slate-800'
                        "
                      />
                      <span
                        class="rounded-[2px]"
                        :class="
                          sz.rows === 2
                            ? widTileSize === sz.value
                              ? 'bg-amber-400'
                              : 'bg-slate-500'
                            : 'bg-slate-800'
                        "
                      />
                      <span
                        class="rounded-[2px]"
                        :class="
                          sz.cols === 2 && sz.rows === 2
                            ? widTileSize === sz.value
                              ? 'bg-amber-400'
                              : 'bg-slate-500'
                            : 'bg-slate-800'
                        "
                      />
                    </span>
                    <span class="font-mono text-[10px]">{{ sz.shortLabel }}</span>
                  </button>
                </div>
              </div>

              <div>
                <label class="mb-2 block text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                  Tile Accent
                </label>
                <ColorMatrixPicker v-model="widTileColor" mode="accent" />
              </div>
            </div>

            <!-- REGISTER BINDING (single-register tiles) -->
            <RegisterPicker
              v-if="widType !== 'graph'"
              v-model="widMappingId"
              :devices="devices"
              :mappings="mappings"
              register-label="Bound Register"
              placeholder="Search by register name, address, or type..."
            />

            <!-- DISPLAY & NUMBER INPUT EXTRA PARAMS -->
            <div v-if="widType === 'display' || widType === 'number_input'" class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
              <p class="text-[11px] font-semibold uppercase tracking-wider text-sky-300">
                {{ widType === 'display' ? 'Display Options' : 'Number Input Options' }}
              </p>
              <div class="grid gap-3 sm:grid-cols-2">
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Value Text Scale</label>
                  <CustomSelect
                    v-model="widFontSize"
                    :options="FONT_SIZE_OPTIONS"
                    aria-label="Value text scale"
                    trigger-class="bg-slate-900"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Decimal Places (10^-x)</label>
                  <NumericTextInput
                    v-model="widPrecision"
                    :min="-5"
                    :max="5"
                    allow-negative
                    :placeholder="`Default: ${boundMapping?.protocolAttributes?.precision ?? 0}`"
                    input-class="bg-slate-900"
                    :external-error="showErr('precision')"
                    @blur="markTouched('precision')"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Prefix</label>
                  <input
                    v-model="widPrefix"
                    placeholder="e.g. ~"
                    class="w-full rounded-lg border bg-slate-900 px-3 py-2 text-xs text-white focus:outline-none"
                    :class="showErr('prefix') ? 'border-rose-500' : 'border-slate-700 focus:border-amber-500'"
                    @blur="markTouched('prefix')"
                  />
                  <p v-if="showErr('prefix')" class="mt-1 text-[11px] text-rose-400">{{ showErr('prefix') }}</p>
                </div>
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Unit (Postfix)</label>
                  <input
                    v-model="widUnit"
                    :placeholder="boundMapping?.protocolAttributes?.unit ? `Default: ${boundMapping.protocolAttributes.unit}` : 'e.g. C, bar, %'"
                    class="w-full rounded-lg border bg-slate-900 px-3 py-2 text-xs text-white focus:outline-none"
                    :class="showErr('unit') ? 'border-rose-500' : 'border-slate-700 focus:border-amber-500'"
                    @blur="markTouched('unit')"
                  />
                  <p v-if="showErr('unit')" class="mt-1 text-[11px] text-rose-400">{{ showErr('unit') }}</p>
                </div>
              </div>
              <p class="text-[11px] text-slate-500">
                Leave Unit or Decimal Places empty to inherit from the bound register (unit: {{ boundMapping?.protocolAttributes?.unit || 'none' }}, precision: {{ boundMapping?.protocolAttributes?.precision ?? 0 }}).
              </p>

              <div v-if="widType === 'number_input'" class="space-y-2 border-t border-slate-800 pt-3">
                <div class="grid gap-3 sm:grid-cols-2">
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Min Value (optional)</label>
                    <NumericTextInput
                      v-model="widMin"
                      allow-decimal
                      allow-negative
                      :min="boundTypeBounds.typeMin"
                      :max="boundTypeBounds.typeMax"
                      :placeholder="String(boundTypeBounds.typeMin)"
                      input-class="bg-slate-900"
                      :external-error="showErr('min')"
                      @blur="markTouched('min')"
                    />
                  </div>
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Max Value (optional)</label>
                    <NumericTextInput
                      v-model="widMax"
                      allow-decimal
                      allow-negative
                      :min="boundTypeBounds.typeMin"
                      :max="boundTypeBounds.typeMax"
                      :placeholder="String(boundTypeBounds.typeMax)"
                      input-class="bg-slate-900"
                      :external-error="showErr('max')"
                      @blur="markTouched('max')"
                    />
                  </div>
                </div>
                <p class="text-[11px] text-slate-500">
                  Register type limit ({{ boundMapping?.protocolAttributes?.dataType || 'UINT16' }}):
                  {{ boundTypeBounds.typeMin }} to {{ boundTypeBounds.typeMax }}
                </p>
              </div>
            </div>

            <!-- LAMP EXTRA PARAMS (ON & OFF states override tile accent) -->
            <div v-else-if="widType === 'lamp'" class="space-y-4 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
              <div class="space-y-3">
                <p class="text-[11px] font-semibold uppercase tracking-wider text-emerald-300">ON State (value != 0)</p>
                <div class="grid gap-3 sm:grid-cols-2">
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Mode</label>
                    <CustomSelect
                      v-model="widLampOn.mode"
                      :options="LAMP_MODE_OPTIONS"
                      aria-label="Lamp ON mode"
                      trigger-class="bg-slate-900"
                    />
                  </div>
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Shown Text</label>
                    <input v-model="widLampOn.text" placeholder="RUNNING" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white focus:border-amber-500 focus:outline-none" />
                  </div>
                </div>
                <div>
                  <label class="mb-1.5 block text-xs text-slate-400">ON State Color (overrides tile accent)</label>
                  <ColorMatrixPicker v-model="widLampOn.color" mode="state" :tile-accent="widTileColor" />
                </div>
              </div>

              <div class="space-y-3 border-t border-slate-800 pt-3">
                <p class="text-[11px] font-semibold uppercase tracking-wider text-slate-400">OFF State (value = 0)</p>
                <div class="grid gap-3 sm:grid-cols-2">
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Mode</label>
                    <CustomSelect
                      v-model="widLampOff.mode"
                      :options="LAMP_MODE_OPTIONS"
                      aria-label="Lamp OFF mode"
                      trigger-class="bg-slate-900"
                    />
                  </div>
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Shown Text</label>
                    <input v-model="widLampOff.text" placeholder="STOPPED" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white focus:border-amber-500 focus:outline-none" />
                  </div>
                </div>
                <div>
                  <label class="mb-1.5 block text-xs text-slate-400">OFF State Color (overrides tile accent)</label>
                  <ColorMatrixPicker v-model="widLampOff.color" mode="state" :tile-accent="widTileColor" />
                </div>
              </div>
            </div>

            <!-- SWITCH EXTRA PARAMS (ON & OFF color overrides) -->
            <div v-else-if="widType === 'switch'" class="space-y-4 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
              <p class="text-[11px] font-semibold uppercase tracking-wider text-emerald-300">Switch Options</p>
              <div class="grid gap-3 sm:grid-cols-2">
                <div>
                  <label class="mb-1 block text-xs text-slate-400">ON Label</label>
                  <input v-model="widSwitchOnLabel" placeholder="ON" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white focus:border-amber-500 focus:outline-none" />
                </div>
                <div>
                  <label class="mb-1 block text-xs text-slate-400">OFF Label</label>
                  <input v-model="widSwitchOffLabel" placeholder="OFF" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white focus:border-amber-500 focus:outline-none" />
                </div>
              </div>
              <div>
                <label class="mb-1.5 block text-xs text-slate-400">ON State Color (overrides tile accent)</label>
                <ColorMatrixPicker v-model="widSwitchColor" mode="state" :tile-accent="widTileColor" />
              </div>
              <div class="border-t border-slate-800 pt-3">
                <label class="mb-1.5 block text-xs text-slate-400">OFF State Color (overrides tile accent)</label>
                <ColorMatrixPicker v-model="widSwitchOffColor" mode="state" :tile-accent="widTileColor" />
              </div>
            </div>

            <!-- BUTTON EXTRA PARAMS (Idle + Triggered Labels + Color Matrix) -->
            <div v-else-if="widType === 'button'" class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
              <p class="text-[11px] font-semibold uppercase tracking-wider text-sky-300">Button Options</p>
              <div class="grid gap-3 sm:grid-cols-3">
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Button Label</label>
                  <input v-model="widBtnLabel" placeholder="Start Pump" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white focus:border-amber-500 focus:outline-none" />
                </div>
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Triggered Label</label>
                  <input v-model="widBtnTriggeredLabel" placeholder="Triggered..." class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white focus:border-amber-500 focus:outline-none" />
                </div>
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Trigger Time (ms)</label>
                  <NumericTextInput
                    v-model="widBtnTriggerMs"
                    :min="100"
                    :max="60000"
                    required
                    placeholder="1000"
                    input-class="bg-slate-900"
                    :external-error="showErr('triggerMs')"
                    @blur="markTouched('triggerMs')"
                  />
                </div>
              </div>
              <div>
                <label class="mb-1.5 block text-xs text-slate-400">Button Color</label>
                <ColorMatrixPicker v-model="widBtnColor" mode="state" :tile-accent="widTileColor" />
              </div>
              <p class="text-[11px] text-slate-500">
                Sends 1 immediately, then automatically resets and sends 0 after the trigger time.
              </p>
            </div>

            <!-- GRAPH EXTRA PARAMS (Scale & Compact Register List + Separate Register Edit Modal) -->
            <div v-else-if="widType === 'graph'" class="space-y-4">
              <!-- Graph Scale & Axes -->
              <div class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
                <p class="text-[11px] font-semibold uppercase tracking-wider text-sky-300">Graph Scale & Window</p>
                <div class="grid gap-3 sm:grid-cols-3">
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Time Scale</label>
                    <CustomSelect
                      v-model="widGraphTimeRange"
                      :options="GRAPH_RANGE_OPTIONS"
                      aria-label="Graph time window"
                      trigger-class="bg-slate-900"
                    />
                  </div>
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Y-Axis Min (optional)</label>
                    <NumericTextInput
                      v-model="widGraphYMin"
                      allow-decimal
                      allow-negative
                      placeholder="Auto"
                      input-class="bg-slate-900"
                      :external-error="showErr('graphYMin')"
                      @blur="markTouched('graphYMin')"
                    />
                  </div>
                  <div>
                    <label class="mb-1 block text-xs text-slate-400">Y-Axis Max (optional)</label>
                    <NumericTextInput
                      v-model="widGraphYMax"
                      allow-decimal
                      allow-negative
                      placeholder="Auto"
                      input-class="bg-slate-900"
                      :external-error="showErr('graphYMax')"
                      @blur="markTouched('graphYMax')"
                    />
                  </div>
                </div>

                <label class="inline-flex items-center gap-2 pt-0.5 text-xs text-slate-300">
                  <input v-model="widGraphSmooth" type="checkbox" />
                  Smooth trend lines
                </label>
              </div>

              <!-- Compact Register List (1 to 5 Registers) -->
              <div class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
                <div class="flex items-center justify-between gap-2">
                  <p class="text-[11px] font-semibold uppercase tracking-wider text-emerald-300">
                    Registers ({{ widGraphSeries.length }} / {{ MAX_GRAPH_SERIES }})
                  </p>
                  <button
                    v-if="widGraphSeries.length < MAX_GRAPH_SERIES"
                    type="button"
                    class="inline-flex h-8 items-center gap-1 rounded-lg border border-amber-500/45 bg-amber-500/15 px-2.5 text-[11px] font-semibold text-amber-200 transition hover:bg-amber-500/25"
                    @click="openAddGraphSeriesModal"
                  >
                    <Plus class="h-3.5 w-3.5" />
                    Add Register
                  </button>
                </div>

                <p v-if="showErr('graphSeries')" class="text-[11px] text-rose-400">
                  {{ showErr('graphSeries') }}
                </p>

                <div v-if="widGraphSeries.length > 0" class="divide-y divide-slate-800/80 overflow-hidden rounded-lg border border-slate-800 bg-slate-900/70">
                  <div
                    v-for="(row, idx) in widGraphSeries"
                    :key="row.uid"
                    class="flex items-center justify-between gap-2.5 px-3 py-2.5"
                  >
                    <div class="flex min-w-0 items-center gap-2.5">
                      <span
                        class="h-3.5 w-3.5 shrink-0 rounded-full border border-white/25"
                        :style="{ backgroundColor: resolveStateVisual(row.color, widTileColor).hex }"
                      />
                      <div class="min-w-0">
                        <p class="truncate text-xs font-medium text-white">
                          {{ row.label || mappingsById[row.registerMappingId]?.name || `Variable #${idx + 1}` }}
                        </p>
                        <div class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-slate-400">
                          <span class="truncate font-mono">
                            {{ mappingsById[row.registerMappingId]?.name || 'Unbound register' }}
                          </span>
                          <span
                            v-if="row.unit || mappingsById[row.registerMappingId]?.protocolAttributes?.unit"
                            class="rounded bg-slate-800 px-1 py-0.2 font-mono text-slate-300"
                          >
                            {{ row.unit || mappingsById[row.registerMappingId]?.protocolAttributes?.unit }}
                          </span>
                          <span class="font-mono text-slate-500">
                            10^-{{ row.precision !== '' ? row.precision : (mappingsById[row.registerMappingId]?.protocolAttributes?.precision ?? 0) }}
                          </span>
                        </div>
                      </div>
                    </div>

                    <div class="flex shrink-0 items-center gap-1">
                      <button
                        type="button"
                        title="Edit register"
                        aria-label="Edit register"
                        class="inline-flex h-7 items-center gap-1 rounded-md border border-slate-700 bg-slate-800 px-2 text-[11px] font-medium text-slate-200 transition hover:bg-slate-700"
                        @click="openEditGraphSeriesModal(row)"
                      >
                        <Pencil class="h-3 w-3" />
                        Edit
                      </button>
                      <button
                        v-if="widGraphSeries.length > 1"
                        type="button"
                        title="Remove register"
                        aria-label="Remove register"
                        class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-rose-500/30 bg-rose-500/10 text-rose-300 transition hover:bg-rose-500/20"
                        @click="removeGraphSeriesRow(row.uid)"
                      >
                        <Trash2 class="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
                <p v-else class="rounded-lg border border-dashed border-slate-800 px-3 py-4 text-center text-xs text-slate-500">
                  No registers added yet. Click Add Register to bind up to {{ MAX_GRAPH_SERIES }} variables.
                </p>
              </div>
            </div>
          </div>

          <!-- RIGHT COLUMN: Sticky End-Result Live Tile Preview -->
          <aside
            class="flex shrink-0 flex-col justify-between gap-4 border-t border-slate-800 bg-slate-950/60 p-5 transition-[width] duration-200 lg:border-t-0 lg:border-l"
            :class="widTileSize === 'xl' ? 'lg:w-[540px] xl:w-[660px]' : 'lg:w-[420px] xl:w-[460px]'"
          >
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="flex items-center gap-2">
                <span class="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                  Live Preview
                </span>
                <span class="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] text-amber-300">
                  {{ tileSizeShortLabel(widTileSize) }}
                </span>
              </div>

              <div v-if="widType === 'lamp' || widType === 'switch'" class="inline-flex rounded-lg border border-slate-800 bg-slate-900 p-0.5 text-[11px]">
                <button
                  type="button"
                  class="rounded-md px-2.5 py-1 font-medium transition"
                  :class="previewBoolState ? 'bg-sky-500/20 text-sky-200' : 'text-slate-400 hover:text-white'"
                  @click="previewBoolState = true"
                >
                  ON
                </button>
                <button
                  type="button"
                  class="rounded-md px-2.5 py-1 font-medium transition"
                  :class="!previewBoolState ? 'bg-sky-500/20 text-sky-200' : 'text-slate-400 hover:text-white'"
                  @click="previewBoolState = false"
                >
                  OFF
                </button>
              </div>

              <div v-else-if="widType === 'button'" class="inline-flex rounded-lg border border-slate-800 bg-slate-900 p-0.5 text-[11px]">
                <button
                  type="button"
                  class="rounded-md px-2.5 py-1 font-medium transition"
                  :class="!previewBtnTriggered ? 'bg-sky-500/20 text-sky-200' : 'text-slate-400 hover:text-white'"
                  @click="previewBtnTriggered = false"
                >
                  Idle
                </button>
                <button
                  type="button"
                  class="rounded-md px-2.5 py-1 font-medium transition"
                  :class="previewBtnTriggered ? 'bg-sky-500/20 text-sky-200' : 'text-slate-400 hover:text-white'"
                  @click="previewBtnTriggered = true"
                >
                  Triggered
                </button>
              </div>
            </div>

            <!-- Centered Preview Stage -->
            <div class="flex flex-1 items-center justify-center rounded-xl border border-dashed border-slate-800/90 bg-slate-900/35 p-4">
              <div
                class="max-w-full transition-all duration-200"
                :style="previewBoxStyle"
              >
                <WidgetTileRenderer
                  :widget="previewWidget"
                  :mapping="boundMapping"
                  :mappings-by-id="mappingsById"
                  :sample="widType === 'lamp' || widType === 'switch' ? undefined : boundSample"
                  :samples="samples"
                  :can-write="true"
                  :preview="true"
                  :preview-state="previewBoolState"
                  :preview-triggered="previewBtnTriggered"
                />
              </div>
            </div>

            <!-- Live Spec Summary -->
            <div class="space-y-1.5 rounded-lg border border-slate-800/80 bg-slate-900/55 px-3 py-2.5 text-[11px]">
              <div class="flex items-center justify-between gap-2">
                <span class="text-slate-500">{{ widType === 'graph' ? 'Registers' : 'Register' }}</span>
                <span class="truncate font-mono text-slate-300">
                  {{
                    widType === 'graph'
                      ? `${widGraphSeries.filter((s) => s.registerMappingId).length} bound (${widGraphTimeRange})`
                      : boundMapping?.name || 'Unbound'
                  }}
                </span>
              </div>
              <div class="flex items-center justify-between gap-2">
                <span class="text-slate-500">Tile Accent</span>
                <span class="inline-flex items-center gap-1.5 font-mono text-slate-300">
                  <span
                    class="h-2.5 w-2.5 rounded-xs border border-white/20"
                    :style="{ backgroundColor: colorPickerHex }"
                  />
                  {{ widTileColor === 'default' ? 'Neutral' : widTileColor }}
                </span>
              </div>
            </div>
          </aside>
        </div>

        <!-- Footer -->
        <div class="flex shrink-0 items-center justify-between gap-2 border-t border-slate-800 px-5 py-3.5">
          <div>
            <button
              v-if="widget"
              type="button"
              class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-rose-500/35 bg-rose-500/10 px-3 text-xs font-medium text-rose-300 transition hover:bg-rose-500/20"
              @click="emit('delete', widget)"
            >
              <Trash2 class="h-3.5 w-3.5" />
              Remove
            </button>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="h-9 rounded-lg border border-slate-700 px-3.5 text-xs font-medium text-slate-300 transition hover:bg-slate-800"
              @click="emit('close')"
            >
              Cancel
            </button>
            <button
              type="submit"
              class="h-9 rounded-lg bg-amber-500 px-4 text-xs font-semibold text-slate-950 transition hover:bg-amber-400"
            >
              {{ submitLabel || (isEditing ? 'Save Tile' : 'Add Tile') }}
            </button>
          </div>
        </div>
      </form>
    </div>

    <!-- SEPARATE MODAL: ADD / EDIT GRAPH REGISTER VARIABLE -->
    <div
      v-if="open && seriesModalOpen"
      class="fixed inset-0 z-[60] flex items-center justify-center bg-black/75 p-4 backdrop-blur-xs"
      @click.self="seriesModalOpen = false"
    >
      <form
        novalidate
        class="max-h-[90dvh] w-full max-w-lg overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6"
        @submit.prevent="saveGraphSeriesModal"
      >
        <div class="flex items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-white">
            {{ editingSeriesUid !== null ? 'Edit Graph Register' : 'Add Graph Register' }}
          </h3>
          <button
            type="button"
            aria-label="Close modal"
            class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
            @click="seriesModalOpen = false"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div>
          <RegisterPicker
            v-model="seriesDraftMappingId"
            :devices="devices"
            :mappings="mappings"
            register-label="Register"
            placeholder="Search register..."
          />
          <p v-if="showSeriesDraftErr('mappingId')" class="mt-1 text-[11px] text-rose-400">
            {{ showSeriesDraftErr('mappingId') }}
          </p>
        </div>

        <div>
          <label class="mb-1 block text-xs text-slate-400">Legend Label (optional)</label>
          <input
            v-model="seriesDraftLabel"
            maxlength="64"
            :placeholder="seriesDraftMapping?.name || 'Defaults to register name'"
            class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-xs text-white focus:outline-none"
            :class="showSeriesDraftErr('label') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
            @blur="seriesDraftTouched.label = true"
          />
          <p v-if="showSeriesDraftErr('label')" class="mt-1 text-[11px] text-rose-400">
            {{ showSeriesDraftErr('label') }}
          </p>
        </div>

        <div>
          <label class="mb-1.5 block text-xs text-slate-400">Line Color</label>
          <ColorMatrixPicker v-model="seriesDraftColor" mode="state" :tile-accent="widTileColor" />
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs text-slate-400">Unit Override (optional)</label>
            <input
              v-model="seriesDraftUnit"
              maxlength="32"
              :placeholder="seriesDraftMapping?.protocolAttributes?.unit ? `Default: ${seriesDraftMapping.protocolAttributes.unit}` : 'e.g. C, bar, %'"
              class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-xs text-white focus:outline-none"
              :class="showSeriesDraftErr('unit') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
              @blur="seriesDraftTouched.unit = true"
            />
            <p v-if="showSeriesDraftErr('unit')" class="mt-1 text-[11px] text-rose-400">
              {{ showSeriesDraftErr('unit') }}
            </p>
          </div>
          <div>
            <label class="mb-1 block text-xs text-slate-400">Decimal Places (10^-x)</label>
            <NumericTextInput
              v-model="seriesDraftPrecision"
              :min="-5"
              :max="5"
              allow-negative
              :placeholder="`Default: ${seriesDraftMapping?.protocolAttributes?.precision ?? 0}`"
              input-class="bg-slate-950"
              :external-error="showSeriesDraftErr('precision')"
              @blur="seriesDraftTouched.precision = true"
            />
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button
            type="button"
            class="h-9 rounded-lg border border-slate-700 px-3.5 text-xs font-medium text-slate-300 hover:bg-slate-800"
            @click="seriesModalOpen = false"
          >
            Cancel
          </button>
          <button
            type="submit"
            class="h-9 rounded-lg bg-amber-500 px-4 text-xs font-semibold text-slate-950 hover:bg-amber-400"
          >
            {{ editingSeriesUid !== null ? 'Save Register' : 'Add Register' }}
          </button>
        </div>
      </form>
    </div>
  </Teleport>
</template>
