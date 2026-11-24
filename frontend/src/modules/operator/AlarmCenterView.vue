<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  AlertOctagon,
  AlertTriangle,
  Bell,
  CheckCircle2,
  Info,
  Mail,
  MailWarning,
  RefreshCw,
  Search,
  WifiOff,
  X,
} from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useAlarmStore } from '../../stores/alarms'
import { useSiteStore } from '../../stores/sites'
import { useWebviewStore } from '../../stores/webview'
import type { AlarmCondition, AlarmEvent, AlarmEventsResponse, AlarmSummary } from '../../types/webview'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const route = useRoute()
const siteStore = useSiteStore()
const webviewStore = useWebviewStore()
const alarmStore = useAlarmStore()

const routeSiteId = computed(() => (typeof route.params.siteId === 'string' ? route.params.siteId : ''))
const isGlobal = computed(() => !routeSiteId.value)

const loading = ref(false)
const errMsg = ref<string | null>(null)
const events = ref<AlarmEvent[]>([])
const summary = ref<AlarmSummary>({
  activeCount: 0,
  activeCritical: 0,
  activeWarning: 0,
  activeInfo: 0,
  unacknowledgedCount: 0,
})

// Filters
const stateFilter = ref<'ACTIVE' | 'CLEARED' | 'ALL'>('ACTIVE')
const severityFilter = ref<'ALL' | 'CRITICAL' | 'WARNING' | 'INFO'>('ALL')
const ackFilter = ref<'ALL' | 'false' | 'true'>('ALL')
const siteFilter = ref<string>('')
const searchQuery = ref('')

// Acknowledge modal state
const ackModalOpen = ref(false)
const ackTarget = ref<AlarmEvent | null>(null)
const ackNote = ref('')
const ackSubmitting = ref(false)

let unsubAlarmWS: (() => void) | null = null
let refreshTimer: number | null = null

const filteredEvents = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return events.value
  return events.value.filter((ev) => {
    return (
      ev.ruleName.toLowerCase().includes(q) ||
      ev.registerName.toLowerCase().includes(q) ||
      ev.siteName.toLowerCase().includes(q) ||
      (ev.acknowledgedByEmail || '').toLowerCase().includes(q) ||
      (ev.ackNote || '').toLowerCase().includes(q)
    )
  })
})

const eventsPag = usePagination(filteredEvents)

watch([stateFilter, severityFilter, ackFilter, siteFilter, routeSiteId], () => {
  eventsPag.resetPage()
  void loadAlarms()
})

watch(searchQuery, () => {
  eventsPag.resetPage()
})

function conditionSymbol(c: AlarmCondition): string {
  switch (c) {
    case 'GT':
      return '>'
    case 'GTE':
      return '>='
    case 'LT':
      return '<'
    case 'LTE':
      return '<='
    case 'EQ':
      return '=='
    case 'NEQ':
      return '!='
    default:
      return c
  }
}

function formatNumber(val: number | undefined | null, unit?: string): string {
  if (val === undefined || val === null || !Number.isFinite(val)) return '-'
  const txt = Number(val.toFixed(5)).toString()
  return unit ? `${txt} ${unit}` : txt
}

function formatTime(iso?: string): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

function severityClasses(sev: string): string {
  switch (sev) {
    case 'CRITICAL':
      return 'border-rose-500/40 bg-rose-500/15 text-rose-300'
    case 'WARNING':
      return 'border-amber-500/40 bg-amber-500/15 text-amber-300'
    default:
      return 'border-sky-500/40 bg-sky-500/15 text-sky-300'
  }
}

async function loadAlarms(silent = false) {
  if (!silent) {
    loading.value = true
  }
  errMsg.value = null
  try {
    const params = new URLSearchParams()
    params.set('state', stateFilter.value)
    if (severityFilter.value !== 'ALL') {
      params.set('severity', severityFilter.value)
    }
    if (ackFilter.value !== 'ALL') {
      params.set('acknowledged', ackFilter.value)
    }
    params.set('limit', '500')

    let url: string
    if (isGlobal.value) {
      if (siteFilter.value) {
        params.set('siteId', siteFilter.value)
      }
      url = `/api/v1/webview/alarms?${params.toString()}`
    } else {
      url = `/api/v1/webview/sites/${routeSiteId.value}/alarms?${params.toString()}`
    }

    const res = await apiRequest<AlarmEventsResponse>(url)
    events.value = res.events || []
    summary.value = res.summary || {
      activeCount: 0,
      activeCritical: 0,
      activeWarning: 0,
      activeInfo: 0,
      unacknowledgedCount: 0,
    }
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to load alarms'
  } finally {
    loading.value = false
  }
}

function openAckModal(ev: AlarmEvent) {
  ackTarget.value = ev
  ackNote.value = ev.ackNote || ''
  ackModalOpen.value = true
}

async function submitAck() {
  if (!ackTarget.value) return
  ackSubmitting.value = true
  errMsg.value = null
  try {
    const updated = await apiRequest<AlarmEvent>(
      `/api/v1/webview/sites/${ackTarget.value.siteId}/alarms/${ackTarget.value.id}/ack`,
      {
        method: 'POST',
        body: JSON.stringify({ note: ackNote.value.trim() }),
      },
    )
    const idx = events.value.findIndex((e) => e.id === updated.id)
    if (idx >= 0) {
      const next = [...events.value]
      next[idx] = updated
      events.value = next
    }
    ackModalOpen.value = false
    ackTarget.value = null
    void loadAlarms(true)
    void alarmStore.fetchSummary()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to acknowledge alarm'
  } finally {
    ackSubmitting.value = false
  }
}

function bindLiveUpdates() {
  if (unsubAlarmWS) {
    unsubAlarmWS()
    unsubAlarmWS = null
  }
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }

  if (routeSiteId.value) {
    // Open lightweight site WebSocket (no register subscriptions needed for alarm push)
    webviewStore.connect(routeSiteId.value, [])
    unsubAlarmWS = webviewStore.subscribeAlarmEvents((ev) => {
      if (ev.siteId === routeSiteId.value) {
        void loadAlarms(true)
      }
    })
  } else {
    refreshTimer = window.setInterval(() => {
      if (!document.hidden) {
        void loadAlarms(true)
      }
    }, 15000)
  }
}

watch(routeSiteId, () => {
  bindLiveUpdates()
})

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && ackModalOpen.value) {
    ackModalOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  void loadAlarms()
  bindLiveUpdates()
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  if (unsubAlarmWS) {
    unsubAlarmWS()
    unsubAlarmWS = null
  }
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
})
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <!-- Header -->
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h2 class="flex items-center gap-2 text-lg font-semibold tracking-tight text-white sm:text-xl">
          <Bell class="h-5 w-5 text-sky-400" />
          {{ isGlobal ? 'Global Alarm Center' : 'Site Alarm Center' }}
        </h2>
        <p class="mt-0.5 text-xs text-slate-400">
          {{
            isGlobal
              ? 'Cross-site latched alarm feed across all accessible sites.'
              : 'Live latched alarms, stale outage freezes, and acknowledgement log for this site.'
          }}
        </p>
      </div>

      <div class="flex items-center gap-2">
        <span
          v-if="!isGlobal && webviewStore.connectionState === 'connected'"
          class="inline-flex h-8 items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 text-[11px] font-medium text-emerald-300"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-emerald-400" />
          Live
        </span>
        <button
          type="button"
          class="hidden h-9 items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3.5 text-xs font-medium leading-none text-slate-200 transition hover:bg-slate-800 sm:inline-flex"
          :disabled="loading"
          @click="loadAlarms()"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
          Refresh
        </button>
      </div>
    </div>

    <!-- Summary KPI Strip (2x2 on mobile, 4-up on desktop) -->
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <div class="rounded-xl border border-rose-500/30 bg-rose-500/10 p-3.5 sm:p-4">
        <div class="flex items-center justify-between gap-2">
          <span class="text-xs font-medium text-rose-200">Active Critical</span>
          <AlertOctagon class="h-4 w-4 shrink-0 text-rose-400" />
        </div>
        <p class="mt-2 text-2xl font-bold tabular-nums text-white">{{ summary.activeCritical }}</p>
      </div>
      <div class="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3.5 sm:p-4">
        <div class="flex items-center justify-between gap-2">
          <span class="text-xs font-medium text-amber-200">Active Warning</span>
          <AlertTriangle class="h-4 w-4 shrink-0 text-amber-400" />
        </div>
        <p class="mt-2 text-2xl font-bold tabular-nums text-white">{{ summary.activeWarning }}</p>
      </div>
      <div class="rounded-xl border border-sky-500/30 bg-sky-500/10 p-3.5 sm:p-4">
        <div class="flex items-center justify-between gap-2">
          <span class="text-xs font-medium text-sky-200">Active Info</span>
          <Info class="h-4 w-4 shrink-0 text-sky-400" />
        </div>
        <p class="mt-2 text-2xl font-bold tabular-nums text-white">{{ summary.activeInfo }}</p>
      </div>
      <div class="rounded-xl border border-slate-800 bg-slate-900/70 p-3.5 sm:p-4">
        <div class="flex items-center justify-between gap-2">
          <span class="text-xs font-medium text-slate-300">Unacked Active</span>
          <CheckCircle2 class="h-4 w-4 shrink-0 text-slate-400" />
        </div>
        <p class="mt-2 text-2xl font-bold tabular-nums text-white">{{ summary.unacknowledgedCount }}</p>
      </div>
    </div>

    <!-- Filters -->
    <div class="grid grid-cols-1 gap-2.5 rounded-xl border border-slate-800 bg-slate-900/60 p-3 sm:flex sm:flex-wrap sm:items-center sm:gap-3">
      <!-- State segmented pills -->
      <div class="grid grid-cols-3 rounded-lg border border-slate-700 bg-slate-950 p-0.5 text-xs sm:inline-flex">
        <button
          v-for="st in (['ACTIVE', 'CLEARED', 'ALL'] as const)"
          :key="st"
          type="button"
          class="rounded-md px-3 py-1.5 font-medium transition sm:py-1"
          :class="stateFilter === st ? 'bg-sky-500/20 text-sky-200' : 'text-slate-400 hover:text-white'"
          @click="stateFilter = st"
        >
          {{ st === 'ACTIVE' ? 'Active' : st === 'CLEARED' ? 'Cleared' : 'All' }}
        </button>
      </div>

      <div class="grid grid-cols-2 gap-2 sm:contents">
        <select
          v-model="severityFilter"
          class="rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-2 text-xs text-white sm:py-1.5"
        >
          <option value="ALL">All Severities</option>
          <option value="CRITICAL">Critical</option>
          <option value="WARNING">Warning</option>
          <option value="INFO">Info</option>
        </select>

        <select
          v-model="ackFilter"
          class="rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-2 text-xs text-white sm:py-1.5"
        >
          <option value="ALL">All Ack States</option>
          <option value="false">Unacknowledged</option>
          <option value="true">Acknowledged</option>
        </select>
      </div>

      <select
        v-if="isGlobal"
        v-model="siteFilter"
        class="w-full rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-2 text-xs text-white sm:w-auto sm:py-1.5"
      >
        <option value="">All Accessible Sites</option>
        <option v-for="s in siteStore.sites" :key="s.id" :value="s.id">
          {{ s.name }}
        </option>
      </select>

      <div class="relative w-full sm:ml-auto sm:min-w-[200px] sm:w-auto sm:flex-initial">
        <Search class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search rule, register, note..."
          class="w-full rounded-lg border border-slate-700 bg-slate-950 py-2 pl-8 pr-3 text-xs text-white placeholder-slate-500 sm:py-1.5"
        />
      </div>
    </div>

    <p v-if="errMsg" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-xs text-rose-200">
      {{ errMsg }}
    </p>

    <!-- Events Container (Mobile Cards + Desktop Table) -->
    <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <!-- Mobile Stacked Alarm Cards -->
      <div class="divide-y divide-slate-800/70 md:hidden">
        <div
          v-for="ev in eventsPag.paginatedItems.value"
          :key="ev.id"
          class="space-y-3 p-4 text-xs"
          :class="ev.state === 'ACTIVE' && !ev.acknowledgedAt ? 'bg-rose-500/[0.04]' : ''"
        >
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex flex-wrap items-center gap-1.5">
              <span
                class="inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
                :class="severityClasses(ev.severity)"
              >
                {{ ev.severity }}
              </span>
              <span
                class="inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
                :class="
                  ev.state === 'ACTIVE'
                    ? 'border-rose-500/40 bg-rose-500/20 text-rose-200'
                    : 'border-emerald-500/30 bg-emerald-500/15 text-emerald-300'
                "
              >
                {{ ev.state }}
              </span>
              <span
                v-if="ev.isStale"
                class="inline-flex items-center gap-1 rounded-full border border-amber-500/40 bg-amber-500/15 px-2 py-0.5 text-[10px] font-semibold text-amber-200"
              >
                <WifiOff class="h-2.5 w-2.5" /> STALE
              </span>
            </div>

            <span v-if="ev.notifyFailed" class="inline-flex items-center gap-1 text-[11px] text-rose-300">
              <MailWarning class="h-3.5 w-3.5" /> Failed
            </span>
            <span v-else-if="ev.notifiedAt" class="inline-flex items-center gap-1 text-[11px] text-emerald-300">
              <Mail class="h-3.5 w-3.5" /> Sent
            </span>
          </div>

          <div>
            <p class="text-sm font-semibold text-white">{{ ev.ruleName }}</p>
            <p class="mt-0.5 text-[11px] text-slate-400">
              <RouterLink v-if="isGlobal" :to="`/sites/${ev.siteId}/alarms`" class="font-medium text-sky-300 hover:underline">
                {{ ev.siteName }}
              </RouterLink>
              <span v-if="isGlobal"> / </span>
              {{ ev.registerName }}
            </p>
          </div>

          <div class="rounded-lg border border-slate-800/80 bg-slate-950/60 px-3 py-2 font-mono text-[11px] space-y-1">
            <p class="text-slate-100">
              {{ formatNumber(ev.triggeredValue, ev.unit) }}
              <span class="text-slate-400">
                ({{ conditionSymbol(ev.condition) }} {{ formatNumber(ev.threshold, ev.unit) }})
              </span>
            </p>
            <p v-if="ev.state === 'CLEARED' && ev.clearedValue !== undefined" class="text-emerald-300">
              Cleared at {{ formatNumber(ev.clearedValue, ev.unit) }}
            </p>
            <p class="text-[10px] text-slate-400">Triggered: {{ formatTime(ev.triggeredAt) }}</p>
            <p v-if="ev.clearedAt" class="text-[10px] text-slate-500">Cleared: {{ formatTime(ev.clearedAt) }}</p>
          </div>

          <div class="pt-0.5">
            <div v-if="ev.acknowledgedAt" class="space-y-0.5 text-[11px]">
              <span class="inline-flex items-center gap-1 font-medium text-emerald-300">
                <CheckCircle2 class="h-3.5 w-3.5" />
                {{ ev.acknowledgedByEmail || 'Acknowledged' }}
              </span>
              <p class="text-[10px] text-slate-500">{{ formatTime(ev.acknowledgedAt) }}</p>
              <p v-if="ev.ackNote" class="text-slate-300">"{{ ev.ackNote }}"</p>
            </div>
            <button
              v-else
              type="button"
              class="inline-flex h-9 w-full items-center justify-center gap-1.5 rounded-lg border border-sky-500/40 bg-sky-500/15 px-3 text-xs font-medium text-sky-200 transition hover:bg-sky-500/25"
              @click="openAckModal(ev)"
            >
              <CheckCircle2 class="h-3.5 w-3.5" />
              Acknowledge Alarm
            </button>
          </div>
        </div>

        <div v-if="!loading && filteredEvents.length === 0" class="px-4 py-10 text-center text-xs text-slate-500">
          No alarms match the current filters.
        </div>
      </div>

      <!-- Desktop Table -->
      <div class="hidden overflow-x-auto md:block">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-4 py-3">Severity</th>
              <th class="px-4 py-3">State</th>
              <th v-if="isGlobal" class="px-4 py-3">Site</th>
              <th class="px-4 py-3">Rule & Register</th>
              <th class="px-4 py-3">Value / Threshold</th>
              <th class="px-4 py-3">Triggered / Cleared</th>
              <th class="px-4 py-3">Notify</th>
              <th class="px-4 py-3 text-right">Acknowledgement</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr
              v-for="ev in eventsPag.paginatedItems.value"
              :key="ev.id"
              class="hover:bg-slate-800/30"
              :class="ev.state === 'ACTIVE' && !ev.acknowledgedAt ? 'bg-rose-500/[0.03]' : ''"
            >
              <td class="whitespace-nowrap px-4 py-3">
                <span
                  class="inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
                  :class="severityClasses(ev.severity)"
                >
                  {{ ev.severity }}
                </span>
              </td>
              <td class="whitespace-nowrap px-4 py-3">
                <div class="flex items-center gap-1.5">
                  <span
                    class="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
                    :class="
                      ev.state === 'ACTIVE'
                        ? 'border border-rose-500/40 bg-rose-500/20 text-rose-200'
                        : 'border border-emerald-500/30 bg-emerald-500/15 text-emerald-300'
                    "
                  >
                    {{ ev.state }}
                  </span>
                  <span
                    v-if="ev.isStale"
                    class="inline-flex items-center gap-1 rounded-full border border-amber-500/40 bg-amber-500/15 px-2 py-0.5 text-[10px] font-semibold text-amber-200"
                    title="Site or device is currently offline; alarm state is frozen until fresh readings arrive"
                  >
                    <WifiOff class="h-2.5 w-2.5" /> STALE
                  </span>
                </div>
              </td>
              <td v-if="isGlobal" class="whitespace-nowrap px-4 py-3">
                <RouterLink
                  :to="`/sites/${ev.siteId}/alarms`"
                  class="font-medium text-sky-300 hover:underline"
                >
                  {{ ev.siteName }}
                </RouterLink>
              </td>
              <td class="px-4 py-3">
                <p class="font-medium text-white">{{ ev.ruleName }}</p>
                <p class="text-[11px] text-slate-400">{{ ev.registerName }}</p>
              </td>
              <td class="whitespace-nowrap px-4 py-3 font-mono tabular-nums">
                <p class="text-slate-100">
                  {{ formatNumber(ev.triggeredValue, ev.unit) }}
                  <span class="text-slate-400">
                    ({{ conditionSymbol(ev.condition) }} {{ formatNumber(ev.threshold, ev.unit) }})
                  </span>
                </p>
                <p v-if="ev.state === 'CLEARED' && ev.clearedValue !== undefined" class="text-[11px] text-emerald-300">
                  Cleared at {{ formatNumber(ev.clearedValue, ev.unit) }}
                </p>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-[11px] tabular-nums text-slate-300">
                <p>{{ formatTime(ev.triggeredAt) }}</p>
                <p v-if="ev.clearedAt" class="text-slate-500">Cleared: {{ formatTime(ev.clearedAt) }}</p>
              </td>
              <td class="whitespace-nowrap px-4 py-3">
                <span
                  v-if="ev.notifyFailed"
                  class="inline-flex items-center gap-1 text-rose-300"
                  :title="ev.notifyError || 'Email notification failed'"
                >
                  <MailWarning class="h-3.5 w-3.5" /> Failed
                </span>
                <span
                  v-else-if="ev.notifiedAt"
                  class="inline-flex items-center gap-1 text-emerald-300"
                  :title="`Sent at ${formatTime(ev.notifiedAt)}`"
                >
                  <Mail class="h-3.5 w-3.5" /> Sent
                </span>
                <span v-else class="text-slate-500">-</span>
              </td>
              <td class="px-4 py-3 text-right">
                <div v-if="ev.acknowledgedAt" class="space-y-0.5 text-right">
                  <span class="inline-flex items-center gap-1 text-[11px] font-medium text-emerald-300">
                    <CheckCircle2 class="h-3.5 w-3.5" />
                    {{ ev.acknowledgedByEmail || 'Acknowledged' }}
                  </span>
                  <p class="text-[10px] text-slate-500">{{ formatTime(ev.acknowledgedAt) }}</p>
                  <p v-if="ev.ackNote" class="ml-auto max-w-xs truncate text-[11px] text-slate-300" :title="ev.ackNote">
                    "{{ ev.ackNote }}"
                  </p>
                </div>
                <button
                  v-else
                  type="button"
                  class="inline-flex items-center gap-1 rounded-lg border border-sky-500/40 bg-sky-500/15 px-2.5 py-1 text-[11px] font-medium text-sky-200 transition hover:bg-sky-500/25"
                  @click="openAckModal(ev)"
                >
                  <CheckCircle2 class="h-3.5 w-3.5" />
                  Acknowledge
                </button>
              </td>
            </tr>
            <tr v-if="!loading && filteredEvents.length === 0">
              <td :colspan="isGlobal ? 8 : 7" class="px-4 py-10 text-center text-slate-500">
                No alarms match the current filters.
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <TablePagination
        v-model:page="eventsPag.page.value"
        v-model:page-size="eventsPag.pageSize.value"
        :total-items="eventsPag.totalItems.value"
      />
    </div>

    <!-- ACKNOWLEDGE MODAL -->
    <Teleport to="body">
      <div
        v-if="ackModalOpen && ackTarget"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
        @click.self="ackModalOpen = false"
      >
        <form
          class="max-h-[90dvh] w-full max-w-md overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6"
          @submit.prevent="submitAck"
        >
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">Acknowledge Alarm</h3>
            <button
              type="button"
              aria-label="Close modal"
              class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
              @click="ackModalOpen = false"
            >
              <X class="h-4 w-4" />
            </button>
          </div>

          <div class="rounded-lg border border-slate-800 bg-slate-950/60 p-3 text-xs space-y-1">
            <p class="font-medium text-white">{{ ackTarget.ruleName }}</p>
            <p class="text-slate-400">
              {{ ackTarget.siteName }} / {{ ackTarget.registerName }}
            </p>
            <p class="font-mono text-slate-300">
              Triggered: {{ formatNumber(ackTarget.triggeredValue, ackTarget.unit) }}
              ({{ conditionSymbol(ackTarget.condition) }} {{ formatNumber(ackTarget.threshold, ackTarget.unit) }})
            </p>
          </div>

          <div>
            <label class="mb-1 block text-xs text-slate-400">Operator Note (optional)</label>
            <textarea
              v-model="ackNote"
              rows="3"
              maxlength="500"
              placeholder="e.g. Dispatched technician to inspect cooling loop"
              class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white placeholder-slate-500 focus:border-sky-500 focus:outline-none"
            />
          </div>

          <p class="text-[11px] text-slate-500">
            Acknowledging marks this alarm as seen. If the condition is still violated, it stays ACTIVE until the register recovers past its clear deadband.
          </p>

          <div class="flex items-center justify-end gap-2 pt-1">
            <button
              type="button"
              class="inline-flex h-9 items-center justify-center rounded-lg border border-slate-700 px-3.5 text-xs font-medium leading-none text-slate-300 transition hover:bg-slate-800"
              @click="ackModalOpen = false"
            >
              Cancel
            </button>
            <button
              type="submit"
              :disabled="ackSubmitting"
              class="inline-flex h-9 items-center justify-center rounded-lg bg-sky-500 px-4 text-xs font-semibold leading-none text-slate-950 transition hover:bg-sky-400 disabled:opacity-40"
            >
              {{ ackSubmitting ? 'Saving...' : 'Confirm Acknowledge' }}
            </button>
          </div>
        </form>
      </div>
    </Teleport>
  </div>
</template>
