<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import * as echarts from 'echarts'
import { Calendar, Download, LineChart, RefreshCw, Search } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useAuthStore } from '../../stores/auth'
import { useThemeStore } from '../../stores/theme'
import type { HistoryKeyInfo, HistoryQueryResponse, HistorySeries } from '../../types/webview'
import { clampPrecision, decodePrecision, formatPrecisionValue } from '../../utils/precision'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const route = useRoute()
const auth = useAuthStore()
const theme = useThemeStore()
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
const errMsg = ref<string | null>(null)
const result = ref<HistoryQueryResponse | null>(null)

const chartEl = ref<HTMLDivElement | null>(null)
let chartInstance: echarts.ECharts | null = null
let resizeObserver: ResizeObserver | null = null

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

function renderChart() {
  if (!chartEl.value) return
  if (!chartInstance) {
    chartInstance = echarts.init(chartEl.value)
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

  const option: echarts.EChartsOption = {
    animationDuration: 220,
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
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
      left: 54,
      right: 24,
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
      { type: 'inside', xAxisIndex: 0 },
      {
        type: 'slider',
        xAxisIndex: 0,
        height: 20,
        bottom: 6,
        borderColor: gridLine,
        fillerColor: isDark ? 'rgba(56,189,248,0.18)' : 'rgba(2,132,199,0.16)',
        textStyle: { color: textColor, fontSize: 10 },
      },
    ],
    series: seriesData,
  }

  chartInstance.setOption(option, true)
}

async function loadKeys() {
  if (!siteId.value) return
  loadingKeys.value = true
  errMsg.value = null
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
    errMsg.value = err instanceof Error ? err.message : 'Failed to load history keys'
  } finally {
    loadingKeys.value = false
  }
}

async function queryHistory() {
  if (!siteId.value || selectedIds.value.length === 0) {
    result.value = null
    renderChart()
    return
  }
  loadingChart.value = true
  errMsg.value = null
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
    renderChart()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to query history'
  } finally {
    loadingChart.value = false
  }
}

async function exportCsv() {
  if (!siteId.value || selectedIds.value.length === 0) return
  exporting.value = true
  errMsg.value = null
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
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'CSV export failed'
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  if (chartEl.value) {
    resizeObserver = new ResizeObserver(() => {
      chartInstance?.resize()
    })
    resizeObserver.observe(chartEl.value)
  }
  void loadKeys()
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
  resizeObserver?.disconnect()
  chartInstance?.dispose()
  chartInstance = null
})
</script>

<template>
  <div class="space-y-5">
    <!-- Top Header & Actions -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2.5">
        <h2 class="text-lg font-semibold text-white">WebView History</h2>
        <span
          v-if="result?.resolution"
          class="rounded-full border border-slate-700 bg-slate-800 px-2.5 py-0.5 font-mono text-[11px] text-slate-300"
        >
          Resolution: {{ result.resolution }}
        </span>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          :disabled="loadingChart || selectedIds.length === 0"
          class="inline-flex items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 hover:bg-slate-800 disabled:opacity-40"
          @click="queryHistory"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loadingChart }" />
          Refresh
        </button>
        <button
          type="button"
          :disabled="exporting || selectedIds.length === 0"
          class="inline-flex items-center gap-1.5 rounded-lg bg-sky-500 px-3 py-1.5 text-xs font-semibold text-slate-950 hover:bg-sky-400 disabled:opacity-40"
          @click="exportCsv"
        >
          <Download class="h-3.5 w-3.5" />
          {{ exporting ? 'Exporting...' : 'Export CSV' }}
        </button>
      </div>
    </div>

    <!-- Time Range Bar -->
    <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-800 bg-slate-900/60 px-4 py-3">
      <div class="flex flex-wrap items-center gap-1.5">
        <Calendar class="mr-1 h-4 w-4 text-slate-400" />
        <button
          v-for="p in PRESETS"
          :key="p.key"
          type="button"
          class="rounded-lg border px-2.5 py-1 text-xs font-medium transition"
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

      <div v-if="preset === 'custom'" class="flex flex-wrap items-center gap-2 text-xs">
        <input
          v-model="customFrom"
          type="datetime-local"
          class="rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-1 text-xs text-white"
        />
        <span class="text-slate-500">to</span>
        <input
          v-model="customTo"
          type="datetime-local"
          class="rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-1 text-xs text-white"
        />
        <button
          type="button"
          class="rounded-lg bg-sky-500 px-3 py-1 text-xs font-semibold text-slate-950 hover:bg-sky-400"
          @click="queryHistory"
        >
          Apply
        </button>
      </div>
    </div>

    <p v-if="errMsg" class="rounded-lg border border-rose-500/40 bg-rose-500/10 px-4 py-2.5 text-xs text-rose-200">
      {{ errMsg }}
    </p>

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
    <div v-else class="grid gap-5 lg:grid-cols-4">
      <!-- Left Series Selector -->
      <aside class="flex flex-col rounded-xl border border-slate-800 bg-slate-900/60 p-4 space-y-3">
        <div class="flex items-center justify-between">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">
            History Keys ({{ selectedIds.length }}/{{ keys.length }})
          </h3>
          <div class="flex items-center gap-2 text-[11px]">
            <button type="button" class="text-sky-300 hover:underline" @click="selectAllVisible">All</button>
            <button type="button" class="text-slate-400 hover:underline" @click="clearSelection">Clear</button>
          </div>
        </div>

        <div class="relative">
          <Search class="pointer-events-none absolute left-2.5 top-2 h-3.5 w-3.5 text-slate-500" />
          <input
            v-model="keySearch"
            type="text"
            placeholder="Filter keys..."
            class="w-full rounded-lg border border-slate-700 bg-slate-950 py-1.5 pl-8 pr-2.5 text-xs text-white focus:border-sky-500 focus:outline-none"
          />
        </div>

        <div class="max-h-[420px] space-y-1.5 overflow-y-auto pr-1">
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
      </aside>

      <!-- Right Chart + Summary Table -->
      <div class="space-y-5 lg:col-span-3">
        <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4">
          <div class="relative h-[380px] w-full">
            <div ref="chartEl" class="h-full w-full" />
            <div
              v-if="selectedIds.length === 0"
              class="pointer-events-none absolute inset-0 flex items-center justify-center text-xs text-slate-500"
            >
              Select one or more history keys to plot trends.
            </div>
            <div
              v-else-if="!loadingChart && result && result.series.every((s) => s.points.length === 0)"
              class="pointer-events-none absolute inset-0 flex items-center justify-center text-xs text-slate-500"
            >
              No archived data points recorded in this time window yet.
            </div>
          </div>
        </section>

        <!-- Summary Statistics Table -->
        <section v-if="summaries.length > 0" class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="px-4 pt-4 pb-2">
            <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">Range Statistics</h3>
          </div>
          <div class="overflow-x-auto">
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
                      <span class="h-2.5 w-2.5 rounded-full" :style="{ backgroundColor: row.color }" />
                      <span class="font-medium text-white">{{ row.registerName }}</span>
                      <span class="text-slate-500">({{ row.deviceName }})</span>
                    </span>
                  </td>
                  <td class="px-4 py-2.5 font-mono text-slate-200">{{ row.latest }}</td>
                  <td class="px-4 py-2.5 font-mono text-slate-300">{{ row.min }}</td>
                  <td class="px-4 py-2.5 font-mono text-slate-300">{{ row.max }}</td>
                  <td class="px-4 py-2.5 font-mono text-slate-300">{{ row.avg }}</td>
                  <td class="px-4 py-2.5 text-right font-mono text-slate-400">{{ row.pointsCount }}</td>
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
