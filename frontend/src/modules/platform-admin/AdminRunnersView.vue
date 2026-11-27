<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Activity, Pause, Play, RefreshCw, RotateCw } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import type { RunnerRecord, Site } from '../../types/iam'
import { formatByteRate, formatDurationSeconds, formatHandshakeState } from '../../utils/format'
import StatusBadge from '../../components/StatusBadge.vue'
import ConfirmActionModal from '../../components/ConfirmActionModal.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const toast = useToastStore()
const runners = ref<RunnerRecord[]>([])
const sitesById = ref<Record<string, Site>>({})
const runnersPag = usePagination(runners)
const loading = ref(false)
const livePolling = ref(true)
const runnerToRestart = ref<RunnerRecord | null>(null)
const restarting = ref(false)

let pollTimer: number | null = null

async function loadRunners(silent = false) {
  if (!silent) loading.value = true
  try {
    const [rRes, sRes] = await Promise.all([
      apiRequest<{ runners: RunnerRecord[] }>('/api/v1/iam/admin/runners'),
      apiRequest<{ sites: Site[] }>('/api/v1/iam/admin/sites'),
    ])
    runners.value = rRes.runners || []
    const map: Record<string, Site> = {}
    for (const s of sRes.sites || []) {
      map[s.id] = s
    }
    sitesById.value = map
  } catch (err) {
    if (!silent) {
      toast.error(err, 'Failed to fetch runners')
    }
  } finally {
    loading.value = false
  }
}

function startPolling() {
  stopPolling()
  if (!livePolling.value) return
  pollTimer = window.setInterval(() => {
    if (!document.hidden) {
      void loadRunners(true)
    }
  }, 10000)
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function toggleLive() {
  livePolling.value = !livePolling.value
  if (livePolling.value) {
    void loadRunners(true)
    startPolling()
  } else {
    stopPolling()
  }
}

onMounted(() => {
  void loadRunners(false)
  startPolling()
})

onUnmounted(stopPolling)

const siteName = computed(() => (siteId: string) => sitesById.value[siteId]?.name || siteId.slice(0, 8))

async function confirmRestartRunner() {
  if (!runnerToRestart.value || restarting.value) return
  const target = runnerToRestart.value
  restarting.value = true
  try {
    await apiRequest(`/api/v1/iam/admin/runners/${target.siteId}/restart`, { method: 'POST' })
    runnerToRestart.value = null
    toast.success('Runner restart triggered.')
    await loadRunners(true)
  } catch (err) {
    toast.error(err, 'Failed to restart runner')
  } finally {
    restarting.value = false
  }
}
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">Tunnel Runners</h1>
        <p class="mt-0.5 text-xs text-slate-400">
          Live WireRelay container state, throughput, stream load, and WireGuard handshake telemetry.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 rounded-lg border px-3 text-xs font-medium leading-none transition"
          :class="
            livePolling
              ? 'border-emerald-500/40 bg-emerald-500/15 text-emerald-300 hover:bg-emerald-500/20'
              : 'border-slate-700 bg-slate-900 text-slate-300 hover:bg-slate-800'
          "
          @click="toggleLive"
        >
          <Pause v-if="livePolling" class="h-3.5 w-3.5" />
          <Play v-else class="h-3.5 w-3.5" />
          {{ livePolling ? 'Live 10s' : 'Paused' }}
        </button>
        <button
          type="button"
          class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3.5 text-xs font-medium leading-none text-slate-200 transition hover:bg-slate-800"
          @click="loadRunners(false)"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
          Refresh
        </button>
      </div>
    </div>

    <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <!-- Mobile Cards -->
      <div class="divide-y divide-slate-800/70 md:hidden">
        <div v-for="r in runnersPag.paginatedItems.value" :key="r.id" class="space-y-3 p-4 text-xs">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <RouterLink
                :to="`/admin/runners/${r.siteId}`"
                class="block truncate text-sm font-semibold text-white transition hover:text-indigo-300"
              >
                {{ siteName(r.siteId) }}
              </RouterLink>
              <p class="mt-0.5 break-all font-mono text-[10px] text-slate-400">{{ r.containerName }}</p>
              <p class="mt-0.5 break-all font-mono text-[10px] text-slate-500">{{ r.image }}</p>
            </div>
            <StatusBadge :status="r.status" size="sm" />
          </div>
          <div class="grid grid-cols-2 gap-2 rounded-lg border border-slate-800/80 bg-slate-950/60 p-2.5 font-mono text-[11px]">
            <div>
              <p class="text-[10px] text-slate-500">Traffic (RX / TX)</p>
              <p class="tabular-nums text-slate-200">
                {{ r.stats ? `${formatByteRate(r.stats.rxBytesPerSec)} / ${formatByteRate(r.stats.txBytesPerSec)}` : '-' }}
              </p>
            </div>
            <div>
              <p class="text-[10px] text-slate-500">Streams</p>
              <p class="tabular-nums text-slate-200">
                {{ r.stats ? `${r.stats.activeStreams} / ${r.stats.maxStreams}` : '-' }}
              </p>
            </div>
            <div>
              <p class="text-[10px] text-slate-500">State</p>
              <p class="text-slate-200">{{ r.desiredState }} / {{ r.observedState || 'unknown' }}</p>
            </div>
            <div>
              <p class="text-[10px] text-slate-500">Handshake</p>
              <p class="tabular-nums text-slate-200">
                {{ formatHandshakeState(r.handshakeState) }}
                <span v-if="r.handshakeAgeSeconds !== undefined">({{ formatDurationSeconds(r.handshakeAgeSeconds) }})</span>
              </p>
            </div>
          </div>
          <p v-if="r.lastError" class="break-words rounded-lg border border-rose-500/30 bg-rose-500/10 px-2.5 py-1.5 text-[11px] text-rose-300">
            {{ r.lastError }}
          </p>
          <div class="flex items-center gap-2">
            <RouterLink
              :to="`/admin/runners/${r.siteId}`"
              class="inline-flex h-8 flex-1 items-center justify-center gap-1.5 rounded-lg border border-indigo-500/40 bg-indigo-500/15 px-3 text-[11px] font-medium text-indigo-200 hover:bg-indigo-500/25"
            >
              <Activity class="h-3.5 w-3.5" /> Tunnel Metrics
            </RouterLink>
            <button
              type="button"
              class="inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-800 px-3 text-[11px] font-medium text-slate-200 hover:bg-slate-700"
              @click="runnerToRestart = r"
            >
              <RotateCw class="h-3.5 w-3.5" /> Restart
            </button>
          </div>
        </div>
        <div v-if="!loading && runners.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">
          No runners provisioned yet.
        </div>
      </div>

      <!-- Desktop Table -->
      <div class="hidden overflow-x-auto md:block">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-4 py-3">Site & Container</th>
              <th class="px-4 py-3">Tunnel Health</th>
              <th class="px-4 py-3 text-right">RX / TX Rate</th>
              <th class="px-4 py-3 text-right">Streams</th>
              <th class="px-4 py-3 text-right">Handshake Age</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="r in runnersPag.paginatedItems.value" :key="r.id" class="hover:bg-slate-800/30">
              <td class="px-4 py-3">
                <RouterLink
                  :to="`/admin/runners/${r.siteId}`"
                  class="font-medium text-white transition hover:text-indigo-300"
                >
                  {{ siteName(r.siteId) }}
                </RouterLink>
                <p class="font-mono text-[11px] text-slate-400">{{ r.containerName }}</p>
                <p class="font-mono text-[10px] text-slate-500">
                  {{ r.image }} | {{ r.desiredState }} / {{ r.observedState || 'unknown' }}
                </p>
              </td>
              <td class="space-y-1 px-4 py-3">
                <StatusBadge :status="r.status" size="sm" />
                <p class="font-mono text-[10px] text-slate-500">{{ formatHandshakeState(r.handshakeState) }}</p>
                <p v-if="r.lastError" class="max-w-xs truncate text-[10px] text-rose-400" :title="r.lastError">{{ r.lastError }}</p>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-right font-mono tabular-nums">
                <template v-if="r.stats">
                  <p class="text-emerald-300">{{ formatByteRate(r.stats.rxBytesPerSec) }}</p>
                  <p class="text-[11px] text-sky-300">{{ formatByteRate(r.stats.txBytesPerSec) }}</p>
                </template>
                <span v-else class="text-slate-500">-</span>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-right font-mono tabular-nums text-slate-300">
                <template v-if="r.stats">
                  {{ r.stats.activeStreams }} <span class="text-slate-500">/ {{ r.stats.maxStreams }}</span>
                </template>
                <span v-else class="text-slate-500">-</span>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-right font-mono tabular-nums text-slate-300">
                {{ r.handshakeAgeSeconds !== undefined ? formatDurationSeconds(r.handshakeAgeSeconds) : '-' }}
              </td>
              <td class="px-4 py-3 text-right">
                <div class="inline-flex items-center justify-end gap-1.5">
                  <RouterLink
                    :to="`/admin/runners/${r.siteId}`"
                    class="inline-flex items-center gap-1 rounded border border-indigo-500/40 bg-indigo-500/15 px-2.5 py-1 text-[11px] text-indigo-200 transition hover:bg-indigo-500/25"
                  >
                    <Activity class="h-3 w-3" /> Tunnel
                  </RouterLink>
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2.5 py-1 text-[11px] text-slate-200 transition hover:bg-slate-700"
                    @click="runnerToRestart = r"
                  >
                    <RotateCw class="h-3 w-3" /> Restart
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="!loading && runners.length === 0">
              <td colspan="6" class="px-4 py-8 text-center text-slate-500">No runners provisioned yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <TablePagination
        v-model:page="runnersPag.page.value"
        v-model:page-size="runnersPag.pageSize.value"
        :total-items="runnersPag.totalItems.value"
      />
    </div>

    <ConfirmActionModal
      :open="runnerToRestart !== null"
      title="Restart Site Runner"
      :item-name="runnerToRestart ? `${siteName(runnerToRestart.siteId)} (${runnerToRestart.containerName})` : ''"
      description="Drains all active WebView and VNC sessions on this site, stops the WireRelay container, and restarts it with the site's WireGuard config."
      confirm-label="Restart Runner"
      tone="warning"
      :busy="restarting"
      @close="runnerToRestart = null"
      @confirm="confirmRestartRunner"
    />
  </div>
</template>
