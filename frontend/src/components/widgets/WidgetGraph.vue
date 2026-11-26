<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { apiRequest } from '../../services/api'
import { useThemeStore } from '../../stores/theme'
import type {
  GraphSeriesConfig,
  HistoryQueryResponse,
  RegisterMapping,
  TelemetrySample,
  WebviewWidget,
} from '../../types/webview'
import { clampPrecision, decodePrecision, formatPrecisionValue } from '../../utils/precision'
import {
  extractGraphSeries,
  getTileAccentVisual,
  graphTimeRangeMs,
  resolveGraphTimeRange,
  resolveStateVisual,
  resolveTileAccent,
  resolveTileSize,
} from '../../utils/tiles'

const MAX_POINTS_PER_SERIES = 1500
const HISTORY_REFRESH_MS = 60_000

const props = defineProps<{
  widget: WebviewWidget
  siteId?: string
  mappingsById?: Record<string, RegisterMapping>
  samples?: Record<string, TelemetrySample>
  preview?: boolean
}>()

const theme = useThemeStore()

const tileSize = computed(() => resolveTileSize(props.widget))
const tileAccent = computed(() => resolveTileAccent(props.widget))
const accentVisual = computed(() => getTileAccentVisual(tileAccent.value))

const seriesConfigs = computed<GraphSeriesConfig[]>(() => extractGraphSeries(props.widget))
const timeRange = computed(() => resolveGraphTimeRange(props.widget.extra?.timeRange))
const windowMs = computed(() => graphTimeRangeMs(timeRange.value))
const smoothLine = computed(() => Boolean(props.widget.extra?.smooth))

const yMin = computed<number | null>(() => {
  const raw = props.widget.extra?.yMin
  if (raw === undefined || raw === null || raw === '') return null
  const n = Number(raw)
  return Number.isFinite(n) ? n : null
})

const yMax = computed<number | null>(() => {
  const raw = props.widget.extra?.yMax
  if (raw === undefined || raw === null || raw === '') return null
  const n = Number(raw)
  return Number.isFinite(n) ? n : null
})

interface ResolvedSeriesMeta {
  registerMappingId: string
  label: string
  colorHex: string
  unit: string
  precision: number
}

const resolvedSeries = computed<ResolvedSeriesMeta[]>(() =>
  seriesConfigs.value.map((cfg, idx) => {
    const mapping = props.mappingsById?.[cfg.registerMappingId]
    const label = cfg.label || mapping?.name || `Register ${idx + 1}`
    const colorHex = resolveStateVisual(cfg.color, tileAccent.value).hex
    const unit =
      cfg.unit !== undefined && cfg.unit.trim() !== ''
        ? cfg.unit.trim()
        : String(mapping?.protocolAttributes?.unit || '')
    const precision =
      cfg.precision !== undefined
        ? clampPrecision(cfg.precision)
        : clampPrecision(mapping?.protocolAttributes?.precision ?? 0)
    return {
      registerMappingId: cfg.registerMappingId,
      label,
      colorHex,
      unit,
      precision,
    }
  }),
)

const seriesKey = computed(() =>
  resolvedSeries.value.map((s) => s.registerMappingId).join(','),
)

// Points stored per registerMappingId as [timestampMs, decodedNumber | null]
const seriesBuffers = ref<Record<string, Array<[number, number | null]>>>({})
const historyBacked = ref(false)
const historyLoaded = ref(false)
const historyError = ref<string | null>(null)
const noDataGraceExpired = ref(false)
const lastSeenTs: Record<string, string> = {}

const chartEl = ref<HTMLDivElement | null>(null)
let chartInstance: echarts.ECharts | null = null
let resizeObserver: ResizeObserver | null = null
let historyTimer: number | null = null
let noDataTimer: number | null = null
let historyFetchSeq = 0

function formatDecoded(val: number | null | undefined, precision: number, unit: string): string {
  if (val === null || val === undefined || !Number.isFinite(val)) return '-'
  const digits = precision > 0 ? precision : 0
  const txt = digits > 0 ? val.toFixed(digits) : String(Math.round(val * 100) / 100)
  return unit ? `${txt} ${unit}` : txt
}

function buildPreviewPoints(idx: number): Array<[number, number]> {
  const count = 24
  const now = Date.now()
  const step = Math.max(1000, Math.round(windowMs.value / (count - 1)))
  const lo = yMin.value ?? 15
  const hi = yMax.value ?? 85
  const span = Math.max(1, hi - lo)
  const base = lo + span * (0.28 + ((idx * 0.14) % 0.45))
  const amp = span * (0.12 + (idx % 3) * 0.04)
  const pts: Array<[number, number]> = []
  for (let i = 0; i < count; i++) {
    const t = now - (count - 1 - i) * step
    const wave = Math.sin(i * 0.55 + idx * 1.3) * amp + Math.cos(i * 0.25 - idx * 0.8) * (amp * 0.35)
    const clamped = Math.max(lo, Math.min(hi, base + wave))
    pts.push([t, Number(clamped.toFixed(2))])
  }
  return pts
}

const legendItems = computed(() =>
  resolvedSeries.value.map((s, idx) => {
    const live = props.samples?.[s.registerMappingId]
    let latestText = '-'
    if (live && live.quality === 'GOOD' && live.value !== null && live.value !== undefined) {
      const formatted = formatPrecisionValue(live.value, s.precision)
      latestText = s.unit ? `${formatted} ${s.unit}` : formatted
    } else if (props.preview) {
      const pts = buildPreviewPoints(idx)
      const last = pts[pts.length - 1]?.[1]
      latestText = formatDecoded(last, s.precision, s.unit)
    } else {
      const buf = seriesBuffers.value[s.registerMappingId]
      if (buf && buf.length > 0) {
        for (let i = buf.length - 1; i >= 0; i--) {
          if (buf[i][1] !== null) {
            latestText = formatDecoded(buf[i][1], s.precision, s.unit)
            break
          }
        }
      }
    }
    return {
      ...s,
      latestText,
    }
  }),
)

const hasAnyData = computed(() => {
  if (props.preview) return resolvedSeries.value.length > 0
  return resolvedSeries.value.some((s) => (seriesBuffers.value[s.registerMappingId]?.length ?? 0) > 0)
})

const badSampleInfo = computed<{ label: string; error: string } | null>(() => {
  if (props.preview || !props.samples) return null
  for (const s of resolvedSeries.value) {
    const sample = props.samples[s.registerMappingId]
    if (sample && sample.quality === 'BAD') {
      return {
        label: s.label,
        error: sample.error?.trim() || 'Register read failed',
      }
    }
  }
  return null
})

const chartErrorText = computed<string | null>(() => {
  if (props.preview) return null
  if (resolvedSeries.value.length === 0) {
    return 'No registers bound to this graph.'
  }
  if (badSampleInfo.value) {
    return `${badSampleInfo.value.label}: ${badSampleInfo.value.error}`
  }
  if (!hasAnyData.value && historyError.value) {
    return `No data: ${historyError.value}`
  }
  if (!hasAnyData.value && historyLoaded.value && noDataGraceExpired.value) {
    return 'No telemetry data received.'
  }
  return null
})

const isErrorState = computed(() => Boolean(chartErrorText.value))

const surfaceClass = computed(() => {
  if (isErrorState.value) return 'border-rose-500/45 bg-rose-950/25'
  return accentVisual.value.surfaceClass
})

const surfaceStyle = computed(() => (isErrorState.value ? undefined : accentVisual.value.surfaceStyle))

const topBarClass = computed(() => {
  if (isErrorState.value) return 'bg-rose-500'
  return accentVisual.value.barClass
})

const topBarStyle = computed(() => (isErrorState.value ? undefined : accentVisual.value.barStyle))

function armNoDataGraceTimer() {
  if (noDataTimer !== null) {
    window.clearTimeout(noDataTimer)
    noDataTimer = null
  }
  noDataGraceExpired.value = false
  if (props.preview) return
  noDataTimer = window.setTimeout(() => {
    noDataGraceExpired.value = true
    renderChart()
  }, 2000)
}

function pruneWindow(pts: Array<[number, number | null]>, cutoffMs: number): Array<[number, number | null]> {
  let start = 0
  while (start < pts.length && pts[start][0] < cutoffMs) {
    start++
  }
  const sliced = start > 0 ? pts.slice(start) : pts
  return sliced.length > MAX_POINTS_PER_SERIES ? sliced.slice(sliced.length - MAX_POINTS_PER_SERIES) : sliced
}

async function fetchHistory() {
  if (props.preview || !props.siteId || resolvedSeries.value.length === 0) {
    historyLoaded.value = true
    return
  }
  const seq = ++historyFetchSeq
  const ids = resolvedSeries.value.map((s) => s.registerMappingId)
  const to = new Date()
  const from = new Date(to.getTime() - windowMs.value)

  try {
    const params = new URLSearchParams({
      mappingIds: ids.join(','),
      from: from.toISOString(),
      to: to.toISOString(),
    })
    const res = await apiRequest<HistoryQueryResponse>(
      `/api/v1/webview/sites/${props.siteId}/history?${params.toString()}`,
    )
    if (seq !== historyFetchSeq) return

    const nextBuf: Record<string, Array<[number, number | null]>> = { ...seriesBuffers.value }
    const cutoffMs = from.getTime()
    let foundHistory = false

    for (const ser of res.series || []) {
      const meta = resolvedSeries.value.find((m) => m.registerMappingId === ser.registerMappingId)
      const prec = meta ? meta.precision : clampPrecision(ser.precision)
      const histPts: Array<[number, number | null]> = (ser.points || []).map((pt) => {
        const y = pt.value !== null && pt.value !== undefined ? decodePrecision(pt.value, prec) : null
        return [new Date(pt.time).getTime(), y]
      })
      if (histPts.length > 0) {
        foundHistory = true
      }

      // Preserve any live points newer than the last historical sample
      const existing = nextBuf[ser.registerMappingId] || []
      const lastHistTs = histPts.length > 0 ? histPts[histPts.length - 1][0] : 0
      const newerLive = existing.filter((p) => p[0] > lastHistTs && p[0] >= cutoffMs)
      nextBuf[ser.registerMappingId] = pruneWindow([...histPts, ...newerLive], cutoffMs)
    }

    historyError.value = null
    historyLoaded.value = true
    historyBacked.value = foundHistory
    seriesBuffers.value = nextBuf
    renderChart()
  } catch (err) {
    if (seq !== historyFetchSeq) return
    historyLoaded.value = true
    historyError.value = err instanceof Error ? err.message : 'Unable to load historical data'
    renderChart()
  }
}

function ingestLiveSamples() {
  if (props.preview || !props.samples || resolvedSeries.value.length === 0) return
  const now = Date.now()
  const cutoffMs = now - windowMs.value
  let changed = false
  const nextBuf: Record<string, Array<[number, number | null]>> = { ...seriesBuffers.value }

  for (const s of resolvedSeries.value) {
    const sample = props.samples[s.registerMappingId]
    if (!sample || sample.quality !== 'GOOD' || sample.value === null || sample.value === undefined) continue
    const tsKey = sample.timestamp || ''
    if (tsKey && lastSeenTs[s.registerMappingId] === tsKey) continue
    lastSeenTs[s.registerMappingId] = tsKey

    const tMs = tsKey ? new Date(tsKey).getTime() : now
    const safeTs = Number.isFinite(tMs) ? tMs : now
    const decoded = decodePrecision(sample.value, s.precision)
    const cur = nextBuf[s.registerMappingId] ? [...nextBuf[s.registerMappingId]] : []

    if (cur.length > 0 && cur[cur.length - 1][0] >= safeTs) {
      cur[cur.length - 1] = [safeTs, decoded]
    } else {
      cur.push([safeTs, decoded])
    }
    nextBuf[s.registerMappingId] = pruneWindow(cur, cutoffMs)
    changed = true
  }

  if (changed) {
    seriesBuffers.value = nextBuf
    renderChart()
  }
}

function renderChart() {
  if (!chartEl.value) return
  if (!chartInstance) {
    chartInstance = echarts.init(chartEl.value)
  }

  const isDark = theme.resolvedTheme === 'dark'
  const errVisual = isErrorState.value && !hasAnyData.value
  const textColor = errVisual
    ? isDark
      ? '#fda4af'
      : '#be123c'
    : isDark
      ? '#94a3b8'
      : '#475569'
  const gridLine = errVisual
    ? isDark
      ? 'rgba(244, 63, 94, 0.22)'
      : 'rgba(225, 29, 72, 0.18)'
    : isDark
      ? 'rgba(148, 163, 184, 0.14)'
      : 'rgba(15, 23, 42, 0.1)'
  const tooltipBg = isDark ? '#0f172a' : '#ffffff'
  const tooltipBorder = isDark ? '#334155' : '#cbd5e1'
  const tooltipText = isDark ? '#f1f5f9' : '#0f172a'

  const compact = tileSize.value === 'sm' || tileSize.value === 'wide' || (chartEl.value.clientHeight || 0) < 120

  const chartSeries: echarts.SeriesOption[] = resolvedSeries.value.map((s, idx) => {
    const pts = props.preview ? buildPreviewPoints(idx) : seriesBuffers.value[s.registerMappingId] || []
    return {
      name: s.label,
      type: 'line',
      showSymbol: pts.length <= 2,
      symbolSize: 4,
      smooth: smoothLine.value ? 0.3 : false,
      connectNulls: false,
      lineStyle: { width: 2, color: s.colorHex },
      itemStyle: { color: s.colorHex },
      areaStyle:
        resolvedSeries.value.length === 1
          ? {
              color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: `${s.colorHex}33` },
                { offset: 1, color: `${s.colorHex}02` },
              ]),
            }
          : undefined,
      data: pts,
    }
  })

  const metaByName = new Map(resolvedSeries.value.map((s) => [s.label, s]))

  const option: echarts.EChartsOption = {
    animation: false,
    backgroundColor: 'transparent',
    grid: {
      top: 6,
      right: 6,
      bottom: 2,
      left: 2,
      containLabel: true,
    },
    tooltip: props.preview
      ? { show: false }
      : {
          trigger: 'axis',
          confine: true,
          backgroundColor: tooltipBg,
          borderColor: tooltipBorder,
          textStyle: { color: tooltipText, fontSize: 11 },
          axisPointer: { type: 'line', lineStyle: { color: textColor, width: 1, type: 'dashed' } },
          formatter: (params: unknown) => {
            const list = Array.isArray(params) ? params : [params]
            if (list.length === 0) return ''
            const first = list[0] as { value?: [number, number | null] }
            const ts = first?.value?.[0]
            const header = ts ? new Date(ts).toLocaleTimeString() : ''
            const rows = list.map((item) => {
              const p = item as { marker?: string; seriesName?: string; value?: [number, number | null] }
              const meta = p.seriesName ? metaByName.get(p.seriesName) : undefined
              const valStr = formatDecoded(p.value?.[1], meta?.precision ?? 0, meta?.unit ?? '')
              return `<div style="display:flex;align-items:center;justify-content:space-between;gap:10px;">
                <span>${p.marker || ''}${p.seriesName || ''}</span>
                <strong style="font-family:monospace;">${valStr}</strong>
              </div>`
            })
            return `<div style="font-size:10px;opacity:0.75;margin-bottom:3px;">${header}</div>${rows.join('')}`
          },
        },
    xAxis: {
      type: 'time',
      boundaryGap: [0, 0],
      splitNumber: compact ? 3 : 4,
      axisLine: { lineStyle: { color: gridLine } },
      axisTick: { show: false },
      axisLabel: {
        color: textColor,
        fontSize: 9,
        hideOverlap: true,
      },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      scale: yMin.value === null && yMax.value === null,
      min: yMin.value !== null ? yMin.value : undefined,
      max: yMax.value !== null ? yMax.value : undefined,
      splitNumber: compact ? 2 : 4,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: textColor,
        fontSize: 9,
        margin: 4,
      },
      splitLine: { lineStyle: { color: gridLine } },
    },
    series: chartSeries,
  }

  chartInstance.setOption(option, true)
}

function resetHistoryPolling() {
  if (historyTimer !== null) {
    window.clearInterval(historyTimer)
    historyTimer = null
  }
  if (!props.preview && props.siteId && resolvedSeries.value.length > 0) {
    void fetchHistory()
    historyTimer = window.setInterval(() => {
      if (!document.hidden) {
        void fetchHistory()
      }
    }, HISTORY_REFRESH_MS)
  }
}

watch(
  () => [props.siteId, seriesKey.value, timeRange.value, props.preview] as const,
  () => {
    historyBacked.value = false
    historyLoaded.value = false
    historyError.value = null
    for (const k of Object.keys(lastSeenTs)) {
      delete lastSeenTs[k]
    }
    armNoDataGraceTimer()
    resetHistoryPolling()
    void nextTick(() => {
      ingestLiveSamples()
      renderChart()
    })
  },
)

watch(
  () => props.samples,
  () => {
    ingestLiveSamples()
  },
)

watch(
  () => [
    theme.resolvedTheme,
    yMin.value,
    yMax.value,
    smoothLine.value,
    tileSize.value,
    tileAccent.value,
    isErrorState.value,
    resolvedSeries.value.map((s) => `${s.label}:${s.colorHex}:${s.unit}:${s.precision}`).join('|'),
  ],
  () => {
    renderChart()
  },
)

onMounted(() => {
  void nextTick(() => {
    if (chartEl.value) {
      resizeObserver = new ResizeObserver(() => {
        chartInstance?.resize()
        renderChart()
      })
      resizeObserver.observe(chartEl.value)
    }
    armNoDataGraceTimer()
    ingestLiveSamples()
    renderChart()
    resetHistoryPolling()
  })
})

onUnmounted(() => {
  if (historyTimer !== null) {
    window.clearInterval(historyTimer)
    historyTimer = null
  }
  if (noDataTimer !== null) {
    window.clearTimeout(noDataTimer)
    noDataTimer = null
  }
  resizeObserver?.disconnect()
  resizeObserver = null
  chartInstance?.dispose()
  chartInstance = null
})
</script>

<template>
  <div
    class="wd-tile-surface flex h-full w-full flex-col border transition-colors"
    :class="[surfaceClass, tileSize === 'sm' ? 'px-2 py-2' : 'px-2.5 py-2.5']"
    :style="surfaceStyle"
  >
    <div
      class="pointer-events-none absolute inset-x-0 top-0 h-[3px]"
      :class="topBarClass"
      :style="topBarStyle"
    />

    <!-- Header -->
    <div class="flex shrink-0 items-center justify-between gap-2 pt-0.5">
      <p class="truncate text-xs font-semibold leading-snug tracking-tight text-slate-200 sm:text-[13px]">
        {{ widget.name || 'Register Graph' }}
      </p>
      <div class="flex shrink-0 items-center gap-1">
        <span
          v-if="!preview && isErrorState"
          class="rounded bg-rose-500/20 px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide text-rose-300"
          :title="chartErrorText || 'No data'"
        >
          {{ badSampleInfo ? 'BAD' : 'NO DATA' }}
        </span>
        <span
          v-else-if="!preview && historyBacked"
          class="rounded bg-sky-500/15 px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide text-sky-300"
          title="Historical archive + live updates"
        >
          HIST
        </span>
        <span class="rounded bg-slate-800/85 px-1.5 py-0.5 font-mono text-[9px] font-semibold text-slate-300">
          {{ timeRange }}
        </span>
      </div>
    </div>

    <!-- Compact Multi-Variable Legend Strip -->
    <div
      v-if="legendItems.length > 0 && tileSize !== 'sm'"
      class="mt-1.5 flex shrink-0 flex-wrap items-center gap-x-2.5 gap-y-1 text-[10px]"
    >
      <div
        v-for="item in legendItems"
        :key="item.registerMappingId"
        class="inline-flex min-w-0 max-w-full items-center gap-1"
      >
        <span
          class="h-2 w-2 shrink-0 rounded-full"
          :style="{ backgroundColor: item.colorHex }"
        />
        <span class="truncate text-slate-400">{{ item.label }}</span>
        <span class="shrink-0 font-mono font-semibold tabular-nums text-slate-200">{{ item.latestText }}</span>
      </div>
    </div>

    <!-- Chart Canvas -->
    <div class="relative mt-1 min-h-0 flex-1 w-full">
      <div ref="chartEl" class="h-full w-full" />
      <div
        v-if="!hasAnyData && !preview && !isErrorState"
        class="pointer-events-none absolute inset-0 flex items-center justify-center text-center text-[11px] text-slate-500"
      >
        Waiting for telemetry samples...
      </div>
    </div>
  </div>
</template>
