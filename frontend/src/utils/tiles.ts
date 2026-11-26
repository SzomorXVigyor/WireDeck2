import type { CSSProperties } from 'vue'
import type {
  ColorOption,
  ColorPresetOption,
  DashboardLayoutConfig,
  FontSizeOption,
  GraphSeriesConfig,
  GraphTimeRange,
  TileAccent,
  TileAccentPreset,
  TileDensity,
  TileSize,
  WebviewWidget,
} from '../types/webview'

export const TILE_SIZE_ORDER: TileSize[] = ['sm', 'wide', 'tall', 'lg', 'xl']

export interface TileSizeMeta {
  value: TileSize
  label: string
  shortLabel: string
  cols: 1 | 2 | 4
  rows: 1 | 2
  description: string
}

export const TILE_SIZES: TileSizeMeta[] = [
  { value: 'sm', label: 'Small', shortLabel: '1x1', cols: 1, rows: 1, description: 'Compact square tile' },
  { value: 'wide', label: 'Wide', shortLabel: '2x1', cols: 2, rows: 1, description: 'Standard wide tile' },
  { value: 'tall', label: 'Tall', shortLabel: '1x2', cols: 1, rows: 2, description: 'Vertical stacked tile' },
  { value: 'lg', label: 'Large', shortLabel: '2x2', cols: 2, rows: 2, description: 'Hero featured tile' },
  { value: 'xl', label: 'Panoramic', shortLabel: '4x2', cols: 4, rows: 2, description: 'Extra-wide graph tile' },
]

export const MAX_GRAPH_SERIES = 5

export const GRAPH_DEFAULT_SERIES_COLORS: string[] = [
  '#38bdf8', // Sky
  '#34d399', // Emerald
  '#fbbf24', // Amber
  '#fb7185', // Rose
  '#a78bfa', // Violet
]

export const GRAPH_TIME_RANGES: { value: GraphTimeRange; label: string; shortLabel: string; ms: number }[] = [
  { value: '5m', label: 'Last 5 minutes', shortLabel: '5m', ms: 5 * 60 * 1000 },
  { value: '15m', label: 'Last 15 minutes', shortLabel: '15m', ms: 15 * 60 * 1000 },
  { value: '1h', label: 'Last 1 hour', shortLabel: '1h', ms: 60 * 60 * 1000 },
  { value: '6h', label: 'Last 6 hours', shortLabel: '6h', ms: 6 * 60 * 60 * 1000 },
  { value: '24h', label: 'Last 24 hours', shortLabel: '24h', ms: 24 * 60 * 60 * 1000 },
  { value: '7d', label: 'Last 7 days', shortLabel: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
]

export interface TileAccentMeta {
  value: TileAccentPreset
  label: string
  hex: string
  swatchClass: string
  barClass: string
  surfaceClass: string
}

export const TILE_ACCENTS: TileAccentMeta[] = [
  {
    value: 'default',
    label: 'Neutral',
    hex: '#475569',
    swatchClass: 'bg-slate-600',
    barClass: 'bg-slate-700/70',
    surfaceClass: 'border-slate-800 bg-slate-900/80',
  },
  {
    value: 'sky',
    label: 'Blue',
    hex: '#0ea5e9',
    swatchClass: 'bg-sky-500',
    barClass: 'bg-sky-400',
    surfaceClass: 'border-sky-500/35 bg-sky-500/[0.08]',
  },
  {
    value: 'emerald',
    label: 'Green',
    hex: '#10b981',
    swatchClass: 'bg-emerald-500',
    barClass: 'bg-emerald-400',
    surfaceClass: 'border-emerald-500/35 bg-emerald-500/[0.08]',
  },
  {
    value: 'amber',
    label: 'Amber',
    hex: '#f59e0b',
    swatchClass: 'bg-amber-500',
    barClass: 'bg-amber-400',
    surfaceClass: 'border-amber-500/35 bg-amber-500/[0.08]',
  },
  {
    value: 'rose',
    label: 'Red',
    hex: '#f43f5e',
    swatchClass: 'bg-rose-500',
    barClass: 'bg-rose-400',
    surfaceClass: 'border-rose-500/35 bg-rose-500/[0.08]',
  },
  {
    value: 'indigo',
    label: 'Indigo',
    hex: '#6366f1',
    swatchClass: 'bg-indigo-500',
    barClass: 'bg-indigo-400',
    surfaceClass: 'border-indigo-500/35 bg-indigo-500/[0.08]',
  },
]

export interface StateColorPresetMeta {
  value: Exclude<ColorPresetOption, 'tile_accent'>
  label: string
  hex: string
}

export const STATE_COLOR_PRESETS: StateColorPresetMeta[] = [
  { value: 'emerald', label: 'Green', hex: '#10b981' },
  { value: 'sky', label: 'Blue', hex: '#0ea5e9' },
  { value: 'amber', label: 'Amber', hex: '#f59e0b' },
  { value: 'rose', label: 'Red', hex: '#f43f5e' },
  { value: 'indigo', label: 'Indigo', hex: '#6366f1' },
  { value: 'slate', label: 'Gray', hex: '#64748b' },
]

const ACCENT_MAP: Record<TileAccentPreset, TileAccentMeta> = Object.fromEntries(
  TILE_ACCENTS.map((a) => [a.value, a]),
) as Record<TileAccentPreset, TileAccentMeta>

const STATE_HEX_MAP: Record<Exclude<ColorPresetOption, 'tile_accent'>, string> = Object.fromEntries(
  STATE_COLOR_PRESETS.map((p) => [p.value, p.hex]),
) as Record<Exclude<ColorPresetOption, 'tile_accent'>, string>

const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/

export function isCustomHexColor(value: string | undefined | null): value is string {
  return typeof value === 'string' && HEX_COLOR_RE.test(value.trim())
}

export function normalizeHexColor(value: string): string {
  return value.trim().toLowerCase()
}

export function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const clean = hex.replace('#', '')
  return {
    r: parseInt(clean.slice(0, 2), 16) || 0,
    g: parseInt(clean.slice(2, 4), 16) || 0,
    b: parseInt(clean.slice(4, 6), 16) || 0,
  }
}

export function hexToRgba(hex: string, alpha: number): string {
  const { r, g, b } = hexToRgb(hex)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function mixHexWithWhite(hex: string, factor: number): string {
  const { r, g, b } = hexToRgb(hex)
  const mr = Math.round(r + (255 - r) * factor)
  const mg = Math.round(g + (255 - g) * factor)
  const mb = Math.round(b + (255 - b) * factor)
  return `rgb(${mr}, ${mg}, ${mb})`
}

/** WCAG relative luminance contrast chooser for solid color buttons */
export function readableTextOnHex(hex: string): string {
  const { r, g, b } = hexToRgb(hex)
  const toLin = (c: number) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  }
  const lum = 0.2126 * toLin(r) + 0.7152 * toLin(g) + 0.0722 * toLin(b)
  return lum > 0.36 ? '#020617' : '#ffffff'
}

export function resolveTileSize(widget: Pick<WebviewWidget, 'style'> | undefined): TileSize {
  const raw = widget?.style?.tileSize
  if (raw === 'sm' || raw === 'wide' || raw === 'tall' || raw === 'lg' || raw === 'xl') return raw
  return 'wide'
}

export function nextTileSize(current: TileSize): TileSize {
  const idx = TILE_SIZE_ORDER.indexOf(current)
  return TILE_SIZE_ORDER[(idx + 1) % TILE_SIZE_ORDER.length]
}

export function tileSizeShortLabel(size: TileSize): string {
  switch (size) {
    case 'sm':
      return '1x1'
    case 'tall':
      return '1x2'
    case 'lg':
      return '2x2'
    case 'xl':
      return '4x2'
    default:
      return '2x1'
  }
}

export function tileSlotClass(size: TileSize): string {
  switch (size) {
    case 'sm':
      return 'wd-tile-sm'
    case 'tall':
      return 'wd-tile-tall'
    case 'lg':
      return 'wd-tile-lg'
    case 'xl':
      return 'wd-tile-xl'
    default:
      return 'wd-tile-wide'
  }
}

export function resolveGraphTimeRange(raw: unknown): GraphTimeRange {
  if (raw === '5m' || raw === '15m' || raw === '1h' || raw === '6h' || raw === '24h' || raw === '7d') {
    return raw
  }
  return '1h'
}

export function graphTimeRangeMs(range: GraphTimeRange): number {
  return GRAPH_TIME_RANGES.find((r) => r.value === range)?.ms ?? 60 * 60 * 1000
}

export function extractGraphSeries(widget: Pick<WebviewWidget, 'extra' | 'registerMappingId'> | undefined): GraphSeriesConfig[] {
  const raw = widget?.extra?.series
  const out: GraphSeriesConfig[] = []
  if (Array.isArray(raw)) {
    for (let i = 0; i < raw.length && out.length < MAX_GRAPH_SERIES; i++) {
      const item = raw[i]
      if (!item || typeof item !== 'object') continue
      const rec = item as Record<string, unknown>
      const id = typeof rec.registerMappingId === 'string' ? rec.registerMappingId.trim() : ''
      if (!id) continue
      const fallbackColor = GRAPH_DEFAULT_SERIES_COLORS[out.length % GRAPH_DEFAULT_SERIES_COLORS.length]
      const color = resolveStateColorValue(rec.color, fallbackColor)
      const label = typeof rec.label === 'string' && rec.label.trim() ? rec.label.trim() : undefined
      const unit = typeof rec.unit === 'string' && rec.unit.trim() ? rec.unit.trim() : undefined
      let precision: number | undefined
      if (rec.precision !== undefined && rec.precision !== null && rec.precision !== '') {
        const n = Number(rec.precision)
        if (Number.isFinite(n)) {
          precision = Math.max(-5, Math.min(5, Math.trunc(n)))
        }
      }
      out.push({ registerMappingId: id, color, label, unit, precision })
    }
  }
  if (out.length === 0 && widget?.registerMappingId) {
    out.push({
      registerMappingId: widget.registerMappingId,
      color: GRAPH_DEFAULT_SERIES_COLORS[0],
    })
  }
  return out
}

export function extractWidgetMappingIds(widget: Pick<WebviewWidget, 'widgetType' | 'registerMappingId' | 'extra'>): string[] {
  if (widget.widgetType === 'graph') {
    return extractGraphSeries(widget).map((s) => s.registerMappingId)
  }
  return widget.registerMappingId ? [widget.registerMappingId] : []
}

export function resolveTileAccent(widget: Pick<WebviewWidget, 'style'> | undefined): TileAccent {
  const raw = widget?.style?.tileColor
  if (typeof raw !== 'string') return 'default'
  const trimmed = raw.trim()
  if (trimmed === 'sky' || trimmed === 'emerald' || trimmed === 'amber' || trimmed === 'rose' || trimmed === 'indigo') {
    return trimmed
  }
  if (isCustomHexColor(trimmed)) {
    return normalizeHexColor(trimmed)
  }
  return 'default'
}

export interface ResolvedTileAccentVisual {
  isCustom: boolean
  isNeutral: boolean
  hex: string
  barClass: string
  surfaceClass: string
  barStyle?: CSSProperties
  surfaceStyle?: CSSProperties
}

export function getTileAccentVisual(accent: TileAccent): ResolvedTileAccentVisual {
  if (isCustomHexColor(accent)) {
    const hex = normalizeHexColor(accent)
    return {
      isCustom: true,
      isNeutral: false,
      hex,
      barClass: '',
      surfaceClass: '',
      barStyle: { backgroundColor: hex },
      surfaceStyle: {
        borderColor: hexToRgba(hex, 0.45),
        backgroundColor: hexToRgba(hex, 0.1),
      },
    }
  }
  const meta = ACCENT_MAP[accent as TileAccentPreset] || ACCENT_MAP.default
  return {
    isCustom: false,
    isNeutral: meta.value === 'default',
    hex: meta.hex,
    barClass: meta.barClass,
    surfaceClass: meta.surfaceClass,
  }
}

export function resolveTileAccentHex(accent: TileAccent): string {
  if (isCustomHexColor(accent)) return normalizeHexColor(accent)
  return (ACCENT_MAP[accent as TileAccentPreset] || ACCENT_MAP.default).hex
}

export function resolveStateColorValue(raw: unknown, fallback: ColorOption = 'tile_accent'): ColorOption {
  if (typeof raw !== 'string') return fallback
  const trimmed = raw.trim()
  if (!trimmed) return fallback
  if (trimmed === 'tile_accent' || trimmed === 'default') return 'tile_accent'
  if (
    trimmed === 'emerald' ||
    trimmed === 'sky' ||
    trimmed === 'amber' ||
    trimmed === 'rose' ||
    trimmed === 'slate' ||
    trimmed === 'indigo'
  ) {
    return trimmed
  }
  if (isCustomHexColor(trimmed)) {
    return normalizeHexColor(trimmed)
  }
  return fallback
}

export interface ResolvedStateVisual {
  hex: string
  isNeutral: boolean
  surfaceClass: string
  surfaceStyle?: CSSProperties
  barClass: string
  barStyle?: CSSProperties
  lampLitDotStyle: CSSProperties
  lampDimDotStyle: CSSProperties
  switchOnTrackStyle: CSSProperties
  switchOffTrackStyle: CSSProperties
  buttonStyle: CSSProperties
}

/**
 * Resolves any tile state color (`tile_accent`, preset name, or `#rrggbb`) into
 * full tile-override visuals (surface tint + border + top bar + lamp/switch/button styles).
 */
export function resolveStateVisual(
  stateColor: ColorOption | undefined,
  tileAccent: TileAccent,
  fallback: ColorOption = 'tile_accent',
): ResolvedStateVisual {
  const effective = resolveStateColorValue(stateColor, fallback)
  let isNeutral = false
  let hex = '#475569'

  if (effective === 'tile_accent') {
    if (tileAccent === 'default') {
      isNeutral = true
      hex = ACCENT_MAP.default.hex
    } else {
      hex = resolveTileAccentHex(tileAccent)
    }
  } else if (isCustomHexColor(effective)) {
    hex = normalizeHexColor(effective)
  } else if (effective === 'slate') {
    isNeutral = true
    hex = STATE_HEX_MAP.slate
  } else if (effective in STATE_HEX_MAP) {
    hex = STATE_HEX_MAP[effective as Exclude<ColorPresetOption, 'tile_accent'>]
  } else {
    isNeutral = true
    hex = ACCENT_MAP.default.hex
  }

  const surfaceClass = isNeutral ? 'border-slate-800 bg-slate-900/80' : ''
  const surfaceStyle: CSSProperties | undefined = isNeutral
    ? undefined
    : {
        borderColor: hexToRgba(hex, 0.48),
        backgroundColor: hexToRgba(hex, 0.11),
      }

  const barClass = isNeutral ? 'bg-slate-600/80' : ''
  const barStyle: CSSProperties | undefined = isNeutral ? undefined : { backgroundColor: hex }

  return {
    hex,
    isNeutral,
    surfaceClass,
    surfaceStyle,
    barClass,
    barStyle,
    lampLitDotStyle: {
      backgroundColor: hex,
      borderColor: mixHexWithWhite(hex, 0.55),
      boxShadow: `0 0 18px ${hexToRgba(hex, 0.72)}`,
    },
    lampDimDotStyle: {
      backgroundColor: isNeutral ? 'rgba(30, 41, 59, 0.9)' : hexToRgba(hex, 0.2),
      borderColor: isNeutral ? 'rgba(100, 116, 139, 0.8)' : hexToRgba(hex, 0.55),
    },
    switchOnTrackStyle: {
      backgroundColor: hexToRgba(hex, 0.88),
      borderColor: mixHexWithWhite(hex, 0.3),
    },
    switchOffTrackStyle: isNeutral
      ? {}
      : {
          backgroundColor: hexToRgba(hex, 0.3),
          borderColor: hexToRgba(hex, 0.65),
        },
    buttonStyle: {
      backgroundColor: hex,
      color: readableTextOnHex(hex),
    },
  }
}

export function resolveDashboardLayout(layout: Record<string, unknown> | undefined): Required<DashboardLayoutConfig> {
  const d = layout?.density
  const density: TileDensity = d === 'compact' || d === 'spacious' ? d : 'comfortable'
  return { density }
}

/**
 * Responsive typography scale for numeric Display & Number Input tiles.
 * Combines the user's chosen fontSize with the tile's grid footprint so
 * 1x1 tiles stay legible without clipping and 2x2 hero tiles command attention.
 */
export function resolveNumericValueClass(fontSize: FontSizeOption, tileSize: TileSize): string {
  if (tileSize === 'lg') {
    switch (fontSize) {
      case 'sm':
        return 'text-2xl sm:text-3xl'
      case 'md':
        return 'text-3xl sm:text-4xl'
      case 'xl':
        return 'text-4xl sm:text-5xl'
      default:
        return 'text-3xl sm:text-5xl'
    }
  }
  if (tileSize === 'sm') {
    switch (fontSize) {
      case 'sm':
        return 'text-base sm:text-lg'
      case 'md':
        return 'text-lg sm:text-xl'
      case 'xl':
        return 'text-xl sm:text-2xl'
      default:
        return 'text-lg sm:text-2xl'
    }
  }
  if (tileSize === 'tall') {
    switch (fontSize) {
      case 'sm':
        return 'text-lg sm:text-xl'
      case 'md':
        return 'text-xl sm:text-2xl'
      case 'xl':
        return 'text-2xl sm:text-3xl'
      default:
        return 'text-xl sm:text-3xl'
    }
  }
  // wide (2x1)
  switch (fontSize) {
    case 'sm':
      return 'text-lg sm:text-xl'
    case 'md':
      return 'text-xl sm:text-2xl'
    case 'xl':
      return 'text-3xl sm:text-4xl'
    default:
      return 'text-2xl sm:text-3xl'
  }
}

export function resolveNumericAffixClass(fontSize: FontSizeOption, tileSize: TileSize): string {
  if (tileSize === 'lg') {
    switch (fontSize) {
      case 'sm':
        return 'text-base sm:text-lg'
      case 'md':
        return 'text-lg sm:text-xl'
      case 'xl':
        return 'text-2xl sm:text-3xl'
      default:
        return 'text-xl sm:text-2xl'
    }
  }
  if (tileSize === 'sm') {
    return fontSize === 'xl' ? 'text-sm sm:text-base' : 'text-xs sm:text-sm'
  }
  switch (fontSize) {
    case 'sm':
      return 'text-sm sm:text-base'
    case 'md':
      return 'text-base sm:text-lg'
    case 'xl':
      return 'text-xl sm:text-2xl'
    default:
      return 'text-lg sm:text-xl'
  }
}
