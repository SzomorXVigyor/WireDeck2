<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import * as echarts from 'echarts'
import { Calendar, ChevronDown, Download, LineChart, PanelLeftClose, PanelLeftOpen, Pause, Play, RefreshCw, Search } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useAuthStore } from '../../stores/auth'
import { useThemeStore } from '../../stores/theme'
import { useToastStore } from '../../stores/toast'
import type { HistoryKeyInfo, HistoryQueryResponse, HistorySeries } from '../../types/webview'
import { clampPrecision, decodePrecision, formatPrecisionValue } from '../../utils/precision'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const route = useRoute()
const auth = useAuthStore()
const theme = useThemeStore()
const toast = useToastStore()
const siteId = computed(() => String(route.params.siteId || ''))

const SERIES_COLORS = [
  '#38bdf8', // sky
  '#34d399', // emerald
  '#fbbf24', // amber
  '#f472b6', // pink
  '#a78bfa', // violet
  '#fb7185', // rose
  '#2dd4bf', // teal
  '#60a5fa', // blue
  '#f97316', // orange
  '#84cc16', // lime
]

type PresetKey = '15m' | '1h' | '6h' | '24h' | '7d' | '30d' | '90d' | 'custom'
const PRESETS: { key: PresetKey; label: string; ms?: number }[] = [
  { key: '15m', label: '15m', ms: 15 * 60 * 1000 },
  { key: '1h', label: '1h', ms: 60 * 60 * 1000 },
  { key: '6h', label: '6h', ms: 6 * 60 * 60 * 1000 },
  { key: '24h', label: '24h', ms: 24 * 60 * 60 * 1000 },
  { key: '7d', label: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
  { key: '30d', label: '30d', ms: 30 * 24 * 60 * 60 * 1000 },
  { key: '90d', label: '90d', ms: 90 * 24 * 60 * 60 * 1000 },
  { key: 'custom', label: 'Custom' },
]

const keys = ref<HistoryKeyInfo[]>([])
const selectedIds = ref<string[]>([])
const keySearch = ref('')
const preset = ref<PresetKey>('1h')
const customFrom = ref('')
const customTo = ref('')

const loadingKeys = ref(false)
const loadingChart = ref(false)
const exporting = ref(false)
const result = ref<HistoryQueryResponse | null>(null)
const keysPanelOpen = ref(typeof window !== 'undefined' ? window.innerWidth >= 1024 : true)
const autoRefresh = ref(true)
const lastUpdatedAt = ref<Date | null>(null)

const chartEl = ref<HTMLDivElement | null>(null)
let chartInstance: echarts.ECharts | null = null
let resizeObserver: ResizeObserver | null = null
let pollTimer: number | null = null
let backgroundQueryInflight = false

const autoRefreshIntervalMs = computed(() => {
  switch (preset.value) {
    case '15m':
    case '1h':
      return 5000
    case '6h':
    case '24h':
      return 10000
    case 'custom':
      return 0
    default:
      return 20000
  }
})

const isLivePolling = computed(
  () => autoRefresh.value && preset.value !== 'custom' && selectedIds.value.length > 0 && autoRefreshIntervalMs.value > 0,
)

const filteredKeys = computed(() => {
  const q = keySearch.value.trim().toLowerCase()
  if (!q) return keys.value
  return keys.value.filter(
    (k) =>
      k.registerName.toLowerCase().includes(q) ||
      k.deviceName.toLowerCase().includes(q) ||
      k.protocol.toLowerCase().includes(q),
  )
})

function toLocalDatetimeInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function resolveRange(): { from: Date; to: Date } {
  if (preset.value === 'custom' && customFrom.value && customTo.value) {
    const f = new Date(customFrom.value)
    const t = new Date(customTo.value)
    if (!Number.isNaN(f.getTime()) && !Number.isNaN(t.getTime()) && t > f) {
      return { from: f, to: t }
    }
  }
  const found = PRESETS.find((p) => p.key === preset.value)
  const ms = found?.ms ?? 60 * 60 * 1000
  const to = new Date()
  return { from: new Date(to.getTime() - ms), to }
}

function selectPreset(next: PresetKey) {
  preset.value = next
  if (next === 'custom') {
    const { from, to } = resolveRange()
    if (!customFrom.value) customFrom.value = toLocalDatetimeInput(from)
    if (!customTo.value) customTo.value = toLocalDatetimeInput(to)
    return
  }
  void queryHistory()
}

function toggleKey(mappingId: string) {
  if (selectedIds.value.includes(mappingId)) {
    selectedIds.value = selectedIds.value.filter((id) => id !== mappingId)
  } else {
    if (selectedIds.value.length >= 20) return
    selectedIds.value = [...selectedIds.value, mappingId]
  }
  void queryHistory()
}

function selectAllVisible() {
  const merged = new Set(selectedIds.value)
  for (const k of filteredKeys.value) {
    if (merged.size >= 20) break
    merged.add(k.registerMappingId)
  }
  selectedIds.value = Array.from(merged)
  void queryHistory()
}

function clearSelection() {
  selectedIds.value = []
  result.value = null
  renderChart()
}

function colorForMapping(mappingId: string): string {
  const idx = keys.value.findIndex((k) => k.registerMappingId === mappingId)
  return SERIES_COLORS[(idx >= 0 ? idx : 0) % SERIES_COLORS.length]
}

function formatPoll(sec: number): string {
  if (sec < 60) return `${sec}s`
  if (sec < 3600) return `${Math.round(sec / 60)}m`
  return `${Math.round(sec / 3600)}h`
}

interface SeriesSummary {
  id: string
  color: string
  deviceName: string
  registerName: string
  unit: string
  latest: string
  min: string
  max: string
  avg: string
  pointsCount: number
}

const summaries = computed<SeriesSummary[]>(() => {
  if (!result.value) return []
  return result.value.series.map((s: HistorySeries) => {
    const prec = clampPrecision(s.precision)
    const vals: number[] = []
    let minRaw: number | null = null
    let maxRaw: number | null = null
    let latestRaw: number | null = null
    let totalSamples = 0
    for (const pt of s.points) {
      totalSamples += pt.count || 0
      if (pt.value !== null && pt.value !== undefined) {
        vals.push(pt.value)
        latestRaw = pt.value
      }
      const lo = pt.min !== undefined && pt.min !== null ? pt.min : pt.value
      const hi = pt.max !== undefined && pt.max !== null ? pt.max : pt.value
      if (lo !== null && lo !== undefined && (minRaw === null || lo < minRaw)) minRaw = lo
      if (hi !== null && hi !== undefined && (maxRaw === null || hi > maxRaw)) maxRaw = hi
    }
    const avgRaw = vals.length > 0 ? vals.reduce((a, b) => a + b, 0) / vals.length : null
    const suffix = s.unit ? ` ${s.unit}` : ''
    return {
      id: s.registerMappingId,
      color: colorForMapping(s.registerMappingId),
      deviceName: s.deviceName,
      registerName: s.registerName,
      unit: s.unit,
      latest: latestRaw !== null ? `${formatPrecisionValue(latestRaw, prec)}${suffix}` : '-',
      min: minRaw !== null ? `${formatPrecisionValue(minRaw, prec)}${suffix}` : '-',
      max: maxRaw !== null ? `${formatPrecisionValue(maxRaw, prec)}${suffix}` : '-',
      avg: avgRaw !== null ? `${formatPrecisionValue(avgRaw, prec)}${suffix}` : '-',
      pointsCount: totalSamples,
    }
  })
})

const summaryPag = usePagination(summaries)

function renderChart(preserveZoom = false) {
  if (!chartEl.value) return
  if (!chartInstance) {
    chartInstance = echarts.init(chartEl.value)
  }

  let savedZoom: { start?: number; end?: number } | null = null
  if (preserveZoom) {
    const cur = chartInstance.getOption() as { dataZoom?: Array<{ start?: number; end?: number }> } | undefined
    const dz = cur?.dataZoom?.[0]
    if (dz && (dz.start !== 0 || dz.end !== 100)) {
      savedZoom = { start: dz.start, end: dz.end }
    }
  }

  const isDark = theme.resolvedTheme === 'dark'
  const textColor = isDark ? '#94a3b8' : '#475569'
  const gridLine = isDark ? '#1e293b' : '#e2e8f0'
  const tooltipBg = isDark ? '#0f172a' : '#ffffff'
  const tooltipBorder = isDark ? '#334155' : '#cbd5e1'
  const tooltipText = isDark ? '#f1f5f9' : '#0f172a'

  const seriesData = (result.value?.series || []).map((ser) => {
    const prec = clampPrecision(ser.precision)
    const label = `${ser.deviceName} / ${ser.registerName}${ser.unit ? ` (${ser.unit})` : ''}`
    const pts = ser.points.map((pt) => {
      const y = pt.value !== null && pt.value !== undefined ? decodePrecision(pt.value, prec) : null
      return [new Date(pt.time).getTime(), y]
    })
    return {
      name: label,
      type: 'line' as const,
      showSymbol: pts.length <= 40,
      symbolSize: 5,
      smooth: false,
      connectNulls: false,
      lineStyle: { width: 2 },
      itemStyle: { color: colorForMapping(ser.registerMappingId) },
      data: pts,
    }
  })

  const isCompact = (chartEl.value?.clientWidth ?? 800) < 520

  const option: echarts.EChartsOption = {
    animationDuration: preserveZoom ? 0 : 220,
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      textStyle: { color: tooltipText, fontSize: 12 },
      axisPointer: { type: 'cross', lineStyle: { color: textColor } },
    },
    legend: {
      type: 'scroll',
      bottom: 34,
      textStyle: { color: textColor, fontSize: 11 },
      pageTextStyle: { color: textColor },
    },
    grid: {
      top: 18,
      left: isCompact ? 42 : 54,
      right: isCompact ? 12 : 24,
      bottom: 86,
    },
    xAxis: {
      type: 'time',
      axisLine: { lineStyle: { color: gridLine } },
      axisLabel: { color: textColor, fontSize: 11 },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      scale: true,
      axisLine: { show: false },
      axisLabel: { color: textColor, fontSize: 11 },
      splitLine: { lineStyle: { color: gridLine } },
    },
    dataZoom: [
      {
        type: 'inside',
        xAxisIndex: 0,
        ...(savedZoom ? { start: savedZoom.start, end: savedZoom.end } : {}),
      },
      {
        type: 'slider',
        xAxisIndex: 0,
        height: 20,
        bottom: 6,
        borderColor: gridLine,
        fillerColor: isDark ? 'rgba(56,189,248,0.18)' : 'rgba(2,132,199,0.16)',
        textStyle: { color: textColor, fontSize: 10 },
        ...(savedZoom ? { start: savedZoom.start, end: savedZoom.end } : {}),
      },
    ],
    series: seriesData,
  }

  chartInstance.setOption(option, true)
}

async function loadKeys() {
  if (!siteId.value) return
  loadingKeys.value = true
  try {
    const res = await apiRequest<{ keys: HistoryKeyInfo[] }>(`/api/v1/webview/sites/${siteId.value}/history/keys`)
    keys.value = res.keys || []
    const validSet = new Set(keys.value.map((k) => k.registerMappingId))
    selectedIds.value = selectedIds.value.filter((id) => validSet.has(id))
    if (selectedIds.value.length === 0 && keys.value.length > 0) {
      const initial = keys.value.filter((k) => k.isActive).slice(0, 3)
      selectedIds.value = (initial.length > 0 ? initial : keys.value.slice(0, 3)).map((k) => k.registerMappingId)
    }
    if (selectedIds.value.length > 0) {
      await queryHistory()
    } else {
      result.value = null
      renderChart()
    }
  } catch (err) {
    toast.error(err, 'Failed to load history keys')
  } finally {
    loadingKeys.value = false
  }
}

async function queryHistory(silent = false) {
  if (!siteId.value || selectedIds.value.length === 0) {
    result.value = null
    renderChart()
    return
  }
  if (silent && (backgroundQueryInflight || document.hidden)) return
  if (silent) {
    backgroundQueryInflight = true
  } else {
    loadingChart.value = true
  }
  try {
    const { from, to } = resolveRange()
    const params = new URLSearchParams({
      mappingIds: selectedIds.value.join(','),
      from: from.toISOString(),
      to: to.toISOString(),
    })
    const res = await apiRequest<HistoryQueryResponse>(
      `/api/v1/webview/sites/${siteId.value}/history?${params.toString()}`,
    )
    result.value = res
    lastUpdatedAt.value = new Date()
    renderChart(silent)
  } catch (err) {
    if (!silent) {
      toast.error(err, 'Failed to query history')
    }
  } finally {
    if (silent) {
      backgroundQueryInflight = false
    } else {
      loadingChart.value = false
    }
  }
}

function restartAutoRefresh() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
  if (!isLivePolling.value) return
  pollTimer = window.setInterval(() => {
    void queryHistory(true)
  }, autoRefreshIntervalMs.value)
}

function onVisibilityChange() {
  if (!document.hidden && isLivePolling.value) {
    void queryHistory(true)
  }
}

async function exportCsv() {
  if (!siteId.value || selectedIds.value.length === 0) return
  exporting.value = true
  try {
    const { from, to } = resolveRange()
    const params = new URLSearchParams({
      mappingIds: selectedIds.value.join(','),
      from: from.toISOString(),
      to: to.toISOString(),
    })
    let token = auth.accessToken
    if (!token) {
      token = await auth.refresh()
    }
    const resp = await fetch(`/api/v1/webview/sites/${siteId.value}/history/export.csv?${params.toString()}`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      credentials: 'include',
    })
    if (!resp.ok) {
      throw new Error((await resp.text()) || `Export failed (HTTP ${resp.status})`)
    }
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `history-${siteId.value.slice(0, 8)}-${preset.value}.csv`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    toast.success('History CSV exported.')
  } catch (err) {
    toast.error(err, 'CSV export failed')
  } finally {
    exporting.value = false
  }
}

function bindChartResizeObserver(el: HTMLDivElement | null) {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (!el) return
  resizeObserver = new ResizeObserver(() => {
    chartInstance?.resize()
  })
  resizeObserver.observe(el)
}

watch(
  chartEl,
  (el) => {
    bindChartResizeObserver(el)
    if (el && chartInstance) {
      chartInstance.resize()
    }
  },
  { immediate: true },
)

watch(keysPanelOpen, async () => {
  await nextTick()
  chartInstance?.resize()
})

watch([isLivePolling, autoRefreshIntervalMs], restartAutoRefresh, { immediate: true })

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibilityChange)
  void loadKeys()
  restartAutoRefresh()
})

watch(siteId, () => {
  selectedIds.value = []
  void loadKeys()
})

watch(
  () => theme.resolvedTheme,
  () => {
    renderChart()
  },
)

onUnmounted(() => {
  document.removeEventListener('visibilitychange', onVisibilityChange)
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
  resizeObserver?.disconnect()
  chartInstance?.dispose()
  chartInstance = null
})
</script>

<template>
  <div class="w-full space-y-5">
    <!-- Top Header & Actions -->
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="flex flex-wrap items-center gap-2.5">
        <h2 class="text-lg font-semibold tracking-tight text-white sm:text-xl">WebView History</h2>
        <span
          v-if="result?.resolution"
          class="rounded-full border border-slate-700 bg-slate-800 px-2.5 py-0.5 font-mono text-[11px] text-slate-300"
        >
          Resolution: {{ result.resolution }}
        </span>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <button
          v-if="preset !== 'custom' && selectedIds.length > 0"
          type="button"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg border px-3 text-xs font-medium transition"
          :class="
            autoRefresh
              ? 'border-emerald-500/40 bg-emerald-500/15 text-emerald-200 hover:bg-emerald-500/25'
              : 'border-slate-700 bg-slate-900 text-slate-300 hover:bg-slate-800'
          "
          :title="autoRefresh ? `Auto-updating every ${Math.round(autoRefreshIntervalMs / 1000)}s` : 'Auto-update paused'"
          @click="autoRefresh = !autoRefresh"
        >
          <Play v-if="autoRefresh" class="h-3 w-3 fill-current" />
          <Pause v-else class="h-3 w-3" />
          <span>{{ autoRefresh ? 'Live' : 'Paused' }}</span>
        </button>

        <button
          v-if="keys.length > 0"
          type="button"
          class="hidden h-9 items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 text-xs font-medium text-slate-200 transition hover:bg-slate-800 lg:inline-flex"
          @click="keysPanelOpen = !keysPanelOpen"
        >
          <PanelLeftClose v-if="keysPanelOpen" class="h-3.5 w-3.5" />
          <PanelLeftOpen v-else class="h-3.5 w-3.5" />
          {{ keysPanelOpen ? 'Hide Keys' : `Keys (${selectedIds.length})` }}
        </button>
        <button
          type="button"
          :disabled="loadingChart || selectedIds.length === 0"
          class="hidden h-9 items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 text-xs font-medium leading-none text-slate-200 transition hover:bg-slate-800 disabled:opacity-40 sm:inline-flex"
          @click="queryHistory(false)"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loadingChart }" />
          Refresh
        </button>
        <button
          type="button"
          :disabled="exporting || selectedIds.length === 0"
          class="inline-flex h-9 items-center justify-center gap-1.5 rounded-lg bg-sky-500 px-3.5 text-xs font-semibold leading-none text-slate-950 transition hover:bg-sky-400 disabled:opacity-40"
          @click="exportCsv"
        >
          <Download class="h-3.5 w-3.5" />
          {{ exporting ? 'Exporting...' : 'Export CSV' }}
        </button>
      </div>
    </div>

    <!-- Time Range Bar -->
    <div class="flex flex-col gap-3 rounded-xl border border-slate-800 bg-slate-900/60 p-3 sm:px-4">
      <div class="no-scrollbar flex items-center gap-1.5 overflow-x-auto">
        <Calendar class="mr-1 h-4 w-4 shrink-0 text-slate-400" />
        <button
          v-for="p in PRESETS"
          :key="p.key"
          type="button"
          class="shrink-0 rounded-lg border px-2.5 py-1.5 text-xs font-medium transition"
          :class="
            preset === p.key
              ? 'border-sky-500/60 bg-sky-500/20 text-sky-200'
              : 'border-slate-800 bg-slate-950/60 text-slate-400 hover:border-slate-700 hover:text-white'
          "
          @click="selectPreset(p.key)"
        >
          {{ p.label }}
        </button>
      </div>

      <div v-if="preset === 'custom'" class="grid grid-cols-1 gap-2 border-t border-slate-800/80 pt-3 text-xs sm:flex sm:flex-wrap sm:items-center">
        <input
          v-model="customFrom"
          type="datetime-local"
          class="w-full rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-xs text-white sm:w-auto"
        />
        <span class="hidden text-slate-500 sm:inline">to</span>
        <input
          v-model="customTo"
          type="datetime-local"
          class="w-full rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-xs text-white sm:w-auto"
        />
        <button
          type="button"
          class="h-9 rounded-lg bg-sky-500 px-3.5 text-xs font-semibold text-slate-950 transition hover:bg-sky-400 sm:h-8"
          @click="queryHistory(false)"
        >
          Apply Range
        </button>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-if="!loadingKeys && keys.length === 0"
      class="flex flex-col items-center justify-center rounded-xl border border-dashed border-slate-800 bg-slate-900/40 px-6 py-16 text-center"
    >
      <LineChart class="h-9 w-9 text-slate-700" />
      <p class="mt-3 text-sm font-medium text-slate-300">No history keys configured for this site</p>
      <p class="mt-1 max-w-md text-xs text-slate-500">
        Ask a Site Operator to select registers for archival under Manage Site -> WebView & History -> History.
      </p>
    </div>

    <!-- Main History Workspace -->
    <div v-else class="grid gap-5" :class="keysPanelOpen ? 'lg:grid-cols-4' : 'lg:grid-cols-1'">
      <!-- Collapsible Series Selector -->
      <aside
        class="flex flex-col rounded-xl border border-slate-800 bg-slate-900/60"
        :class="keysPanelOpen ? '' : 'lg:hidden'"
      >
        <div class="flex items-center justify-between gap-2 p-3.5 sm:p-4">
          <button
            type="button"
            class="flex flex-1 items-center gap-2 text-left text-xs font-semibold uppercase tracking-wider text-slate-300 lg:cursor-default"
            :aria-expanded="keysPanelOpen"
            @click="keysPanelOpen = !keysPanelOpen"
          >
            <ChevronDown
              class="h-4 w-4 shrink-0 text-slate-400 transition-transform lg:hidden"
              :class="keysPanelOpen ? '' : '-rotate-90'"
            />
            <span>History Keys ({{ selectedIds.length }}/{{ keys.length }})</span>
          </button>
          <div class="flex items-center gap-2.5 text-[11px]">
            <button type="button" class="text-sky-300 hover:underline" @click="selectAllVisible">All</button>
            <button type="button" class="text-slate-400 hover:underline" @click="clearSelection">Clear</button>
          </div>
        </div>

        <div v-if="keysPanelOpen" class="space-y-3 border-t border-slate-800/80 p-3.5 pt-3 sm:p-4 sm:pt-3">
          <div class="relative">
            <Search class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
            <input
              v-model="keySearch"
              type="text"
              placeholder="Filter keys..."
              class="w-full rounded-lg border border-slate-700 bg-slate-950 py-1.5 pl-8 pr-2.5 text-xs text-white focus:border-sky-500 focus:outline-none"
            />
          </div>

          <div class="max-h-[260px] space-y-1.5 overflow-y-auto pr-1 lg:max-h-[420px]">
            <button
              v-for="k in filteredKeys"
              :key="k.registerMappingId"
              type="button"
              class="flex w-full items-start gap-2.5 rounded-lg border px-2.5 py-2 text-left text-xs transition"
              :class="
                selectedIds.includes(k.registerMappingId)
                  ? 'border-sky-500/50 bg-sky-500/15'
                  : 'border-slate-800 bg-slate-950/60 hover:border-slate-700 hover:bg-slate-800/40'
              "
              @click="toggleKey(k.registerMappingId)"
            >
              <span
                class="mt-1 h-2.5 w-2.5 shrink-0 rounded-full"
                :style="{
                  backgroundColor: selectedIds.includes(k.registerMappingId) ? colorForMapping(k.registerMappingId) : '#475569',
                }"
              />
              <div class="min-w-0 flex-1">
                <p class="truncate font-medium text-white">{{ k.registerName }}</p>
                <p class="truncate text-[11px] text-slate-400">{{ k.deviceName }}</p>
                <p class="mt-0.5 font-mono text-[10px] text-slate-500">
                  poll {{ formatPoll(k.pollIntervalSeconds) }} | keep {{ k.retentionDays }}d
                  <span v-if="!k.isActive" class="ml-1 text-amber-400">(paused)</span>
                </p>
              </div>
            </button>
            <p v-if="filteredKeys.length === 0" class="py-4 text-center text-xs text-slate-500">
              No matching keys.
            </p>
          </div>
        </div>
      </aside>

      <!-- Right Chart + Summary Table -->
      <div class="min-w-0 space-y-5" :class="keysPanelOpen ? 'lg:col-span-3' : 'lg:col-span-1'">
        <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-3 sm:p-4">
          <div class="relative h-[280px] w-full sm:h-[380px] lg:h-[420px]">
            <div ref="chartEl" class="h-full w-full" />
            <div
              v-if="selectedIds.length === 0"
              class="pointer-events-none absolute inset-0 flex items-center justify-center px-4 text-center text-xs text-slate-500"
            >
              Select one or more history keys to plot trends.
            </div>
            <div
              v-else-if="!loadingChart && result && result.series.every((s) => s.points.length === 0)"
              class="pointer-events-none absolute inset-0 flex items-center justify-center px-4 text-center text-xs text-slate-500"
            >
              No archived data points recorded in this time window yet.
            </div>
          </div>
        </section>

        <!-- Summary Statistics -->
        <section v-if="summaries.length > 0" class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="px-4 pt-4 pb-2">
            <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">Range Statistics</h3>
          </div>

          <!-- Mobile Stat Cards -->
          <div class="divide-y divide-slate-800/70 md:hidden">
            <div v-for="row in summaryPag.paginatedItems.value" :key="row.id" class="space-y-2 p-3.5 text-xs">
              <div class="flex items-center justify-between gap-2">
                <span class="flex min-w-0 items-center gap-2">
                  <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ backgroundColor: row.color }" />
                  <span class="truncate font-medium text-white">{{ row.registerName }}</span>
                </span>
                <span class="shrink-0 font-mono text-[11px] text-slate-400">{{ row.pointsCount }} pts</span>
              </div>
              <p class="text-[11px] text-slate-500">{{ row.deviceName }}</p>
              <div class="grid grid-cols-2 gap-2 rounded-lg border border-slate-800/80 bg-slate-950/60 p-2.5 font-mono text-[11px] tabular-nums">
                <div><span class="text-slate-500">Latest:</span> <span class="text-slate-100">{{ row.latest }}</span></div>
                <div><span class="text-slate-500">Avg:</span> <span class="text-slate-300">{{ row.avg }}</span></div>
                <div><span class="text-slate-500">Min:</span> <span class="text-slate-300">{{ row.min }}</span></div>
                <div><span class="text-slate-500">Max:</span> <span class="text-slate-300">{{ row.max }}</span></div>
              </div>
            </div>
          </div>

          <!-- Desktop Table -->
          <div class="hidden overflow-x-auto md:block">
            <table class="w-full text-left text-xs">
              <thead class="border-b border-slate-800 text-slate-400">
                <tr>
                  <th class="px-4 py-2.5">Series</th>
                  <th class="px-4 py-2.5">Latest</th>
                  <th class="px-4 py-2.5">Min</th>
                  <th class="px-4 py-2.5">Max</th>
                  <th class="px-4 py-2.5">Average</th>
                  <th class="px-4 py-2.5 text-right">Samples</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-800/70">
                <tr v-for="row in summaryPag.paginatedItems.value" :key="row.id" class="hover:bg-slate-800/30">
                  <td class="px-4 py-2.5">
                    <span class="inline-flex items-center gap-2">
                      <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ backgroundColor: row.color }" />
                      <span class="font-medium text-white">{{ row.registerName }}</span>
                      <span class="text-slate-500">({{ row.deviceName }})</span>
                    </span>
                  </td>
                  <td class="whitespace-nowrap px-4 py-2.5 font-mono tabular-nums text-slate-200">{{ row.latest }}</td>
                  <td class="whitespace-nowrap px-4 py-2.5 font-mono tabular-nums text-slate-300">{{ row.min }}</td>
                  <td class="whitespace-nowrap px-4 py-2.5 font-mono tabular-nums text-slate-300">{{ row.max }}</td>
                  <td class="whitespace-nowrap px-4 py-2.5 font-mono tabular-nums text-slate-300">{{ row.avg }}</td>
                  <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-400">{{ row.pointsCount }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <TablePagination
            v-model:page="summaryPag.page.value"
            v-model:page-size="summaryPag.pageSize.value"
            :total-items="summaryPag.totalItems.value"
          />
        </section>
      </div>
    </div>
  </div>
</template>
