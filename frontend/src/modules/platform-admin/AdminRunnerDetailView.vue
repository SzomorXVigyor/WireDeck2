<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  Activity,
  AlertTriangle,
  ArrowDownLeft,
  ArrowLeft,
  ArrowUpRight,
  Check,
  Copy,
  Pause,
  Play,
  RefreshCw,
  RotateCw,
  Server,
  ShieldCheck,
} from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import type { RunnerDetailResponse, RunnerRecord, Site } from '../../types/iam'
import {
  formatByteRate,
  formatBytes,
  formatDateTime,
  formatDurationSeconds,
  formatHandshakeState,
  shortKey,
} from '../../utils/format'
import StatusBadge from '../../components/StatusBadge.vue'
import ConfirmActionModal from '../../components/ConfirmActionModal.vue'

const route = useRoute()
const toast = useToastStore()

const siteId = computed(() => String(route.params.siteId || ''))
const runner = ref<RunnerRecord | null>(null)
const site = ref<Site | null>(null)
const loading = ref(true)
const refreshing = ref(false)
const restarting = ref(false)
const showRestartConfirm = ref(false)
const loadError = ref<string | null>(null)
const livePolling = ref(true)
const copiedKey = ref<string | null>(null)

let pollTimer: number | null = null

async function loadRunner(silent = false) {
  if (!siteId.value) return
  if (silent) {
    refreshing.value = true
  } else {
    loading.value = true
  }
  try {
    const res = await apiRequest<RunnerDetailResponse>(`/api/v1/iam/admin/runners/${siteId.value}`)
    runner.value = res.runner
    site.value = res.site ?? null
    loadError.value = null
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Failed to load runner telemetry'
    loadError.value = msg
    if (!silent) {
      toast.error(err, 'Failed to load runner telemetry')
    }
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

function startPolling() {
  stopPolling()
  if (!livePolling.value) return
  pollTimer = window.setInterval(() => {
    if (!document.hidden && !refreshing.value) {
      void loadRunner(true)
    }
  }, 5000)
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
    void loadRunner(true)
    startPolling()
  } else {
    stopPolling()
  }
}

async function restartRunner() {
  if (!siteId.value || restarting.value) return
  restarting.value = true
  try {
    await apiRequest(`/api/v1/iam/admin/runners/${siteId.value}/restart`, { method: 'POST' })
    showRestartConfirm.value = false
    toast.success('Runner restart triggered.')
    await loadRunner(true)
  } catch (err) {
    toast.error(err, 'Failed to restart runner')
  } finally {
    restarting.value = false
  }
}

async function copyText(val: string) {
  try {
    await navigator.clipboard.writeText(val)
    copiedKey.value = val
    window.setTimeout(() => {
      if (copiedKey.value === val) copiedKey.value = null
    }, 1500)
  } catch {
    toast.error('Clipboard access unavailable.')
  }
}

const stats = computed(() => runner.value?.stats ?? null)

const streamUtilizationPct = computed(() => {
  if (!stats.value || stats.value.maxStreams <= 0) return 0
  return Math.min(100, Math.round((stats.value.activeStreams / stats.value.maxStreams) * 100))
})

const streamBarClass = computed(() => {
  if (streamUtilizationPct.value >= 85) return 'bg-rose-400'
  if (streamUtilizationPct.value >= 60) return 'bg-amber-400'
  return 'bg-sky-400'
})

const totalErrors = computed(() => {
  if (!stats.value) return 0
  return stats.value.dialErrorsTotal + stats.value.streamsRejectedTotal
})

watch(siteId, () => {
  void loadRunner(false)
})

onMounted(() => {
  void loadRunner(false)
  startPolling()
})

onUnmounted(stopPolling)
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <!-- Top navigation & action bar -->
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div class="min-w-0 space-y-1.5">
        <div class="flex flex-wrap items-center gap-2 text-xs text-slate-400">
          <RouterLink
            to="/admin/runners"
            class="inline-flex items-center gap-1 text-slate-400 transition hover:text-white"
          >
            <ArrowLeft class="h-3.5 w-3.5" />
            Tunnel Runners
          </RouterLink>
          <span class="text-slate-600">/</span>
          <span class="truncate font-mono text-slate-300">{{ site?.slug || siteId }}</span>
        </div>

        <div class="flex flex-wrap items-center gap-2.5">
          <h1 class="truncate text-lg font-semibold tracking-tight text-white sm:text-xl">
            {{ site?.name ? `${site.name} Tunnel` : 'Site Runner Tunnel' }}
          </h1>
          <StatusBadge v-if="runner" :status="runner.status" size="sm" />
          <span
            v-if="runner"
            class="rounded-full border border-slate-700 bg-slate-800 px-2 py-0.5 font-mono text-[10px] uppercase text-slate-300"
          >
            {{ runner.desiredState }} / {{ runner.observedState || 'unknown' }}
          </span>
        </div>

        <p v-if="runner" class="truncate font-mono text-xs text-slate-400">
          {{ runner.containerName }}
          <span v-if="stats?.interface" class="text-slate-500">({{ stats.interface }})</span>
        </p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <RouterLink
          :to="`/admin/sites/${siteId}`"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 text-xs font-medium text-slate-200 transition hover:bg-slate-800"
        >
          <Server class="h-3.5 w-3.5 text-indigo-400" />
          Site Metrics
        </RouterLink>
        <button
          type="button"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg border px-3 text-xs font-medium transition"
          :class="
            livePolling
              ? 'border-emerald-500/40 bg-emerald-500/15 text-emerald-300 hover:bg-emerald-500/20'
              : 'border-slate-700 bg-slate-900 text-slate-300 hover:bg-slate-800'
          "
          :title="livePolling ? 'Pause 5s live refresh' : 'Resume 5s live refresh'"
          @click="toggleLive"
        >
          <Pause v-if="livePolling" class="h-3.5 w-3.5" />
          <Play v-else class="h-3.5 w-3.5" />
          {{ livePolling ? 'Live 5s' : 'Paused' }}
        </button>
        <button
          type="button"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 text-xs font-medium text-slate-200 transition hover:bg-slate-800"
          :disabled="refreshing"
          @click="loadRunner(true)"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': refreshing }" />
          Refresh
        </button>
        <button
          type="button"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-indigo-500/40 bg-indigo-500/15 px-3 text-xs font-medium text-indigo-200 transition hover:bg-indigo-500/25 disabled:opacity-50"
          :disabled="restarting || !runner"
          @click="showRestartConfirm = true"
        >
          <RotateCw class="h-3.5 w-3.5" :class="{ 'animate-spin': restarting }" />
          Restart Runner
        </button>
      </div>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading && !runner" class="space-y-4">
      <div class="h-28 animate-pulse rounded-xl border border-slate-800 bg-slate-900/60" />
      <div class="h-44 animate-pulse rounded-xl border border-slate-800 bg-slate-900/60" />
    </div>

    <!-- Fatal load error -->
    <div
      v-else-if="loadError && !runner"
      class="flex flex-col items-start justify-between gap-3 rounded-xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs text-rose-200 sm:flex-row sm:items-center"
    >
      <div class="flex items-start gap-2.5">
        <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-rose-400" />
        <div>
          <p class="font-semibold text-rose-100">Unable to load tunnel runner telemetry</p>
          <p class="mt-0.5 text-rose-300">{{ loadError }}</p>
        </div>
      </div>
      <button
        type="button"
        class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-rose-500/40 bg-rose-500/20 px-3 text-xs font-medium text-rose-100 hover:bg-rose-500/30"
        @click="loadRunner(false)"
      >
        Retry
      </button>
    </div>

    <template v-else-if="runner">
      <!-- Runner error alert -->
      <div
        v-if="runner.lastError"
        class="flex flex-col items-start justify-between gap-3 rounded-xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs sm:flex-row sm:items-center"
      >
        <div class="flex items-start gap-2.5">
          <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-rose-400" />
          <div class="min-w-0">
            <p class="font-semibold text-rose-200">Last Runner Error</p>
            <p class="mt-0.5 break-words font-mono text-[11px] text-rose-300">{{ runner.lastError }}</p>
          </div>
        </div>
        <button
          type="button"
          class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-rose-500/40 bg-rose-500/20 px-3 text-xs font-medium text-rose-100 hover:bg-rose-500/30"
          :disabled="restarting"
          @click="showRestartConfirm = true"
        >
          <RotateCw class="h-3.5 w-3.5" :class="{ 'animate-spin': restarting }" />
          Restart Runner
        </button>
      </div>

      <!-- Unified Live Tunnel Telemetry Strip -->
      <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
        <div class="grid divide-y divide-slate-800/80 sm:grid-cols-2 sm:divide-y-0 lg:grid-cols-6 lg:divide-x">
          <!-- RX Throughput -->
          <div class="p-4">
            <div class="flex items-center gap-1.5 text-[11px] text-slate-400">
              <ArrowDownLeft class="h-3.5 w-3.5 text-emerald-400" />
              <span>Inbound (RX)</span>
            </div>
            <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
              {{ stats ? formatByteRate(stats.rxBytesPerSec) : '-' }}
            </p>
            <p class="mt-0.5 font-mono text-[11px] tabular-nums text-slate-500">
              Total {{ stats ? formatBytes(stats.rxBytes) : '-' }}
            </p>
          </div>

          <!-- TX Throughput -->
          <div class="border-slate-800/80 p-4 sm:border-l lg:border-l-0">
            <div class="flex items-center gap-1.5 text-[11px] text-slate-400">
              <ArrowUpRight class="h-3.5 w-3.5 text-sky-400" />
              <span>Outbound (TX)</span>
            </div>
            <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
              {{ stats ? formatByteRate(stats.txBytesPerSec) : '-' }}
            </p>
            <p class="mt-0.5 font-mono text-[11px] tabular-nums text-slate-500">
              Total {{ stats ? formatBytes(stats.txBytes) : '-' }}
            </p>
          </div>

          <!-- Active Streams -->
          <div class="border-slate-800/80 p-4 sm:border-t lg:border-t-0">
            <div class="flex items-center justify-between text-[11px] text-slate-400">
              <span>Active Streams</span>
              <span v-if="stats" class="font-mono tabular-nums text-slate-500">{{ streamUtilizationPct }}%</span>
            </div>
            <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
              <template v-if="stats">
                {{ stats.activeStreams }}
                <span class="text-xs font-normal text-slate-500">/ {{ stats.maxStreams }}</span>
              </template>
              <template v-else>-</template>
            </p>
            <div class="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-slate-800">
              <div
                class="h-full rounded-full transition-[width] duration-200"
                :class="streamBarClass"
                :style="{ width: `${streamUtilizationPct}%` }"
              />
            </div>
          </div>

          <!-- Relay Uptime -->
          <div class="border-slate-800/80 p-4 sm:border-l sm:border-t lg:border-l-0 lg:border-t-0">
            <p class="text-[11px] text-slate-400">Relay Uptime</p>
            <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
              {{ stats ? formatDurationSeconds(stats.uptimeSeconds) : '-' }}
            </p>
            <p class="mt-0.5 font-mono text-[11px] text-slate-500">
              gRPC {{ runner.configured ? 'configured' : 'awaiting config' }}
            </p>
          </div>

          <!-- Handshake Age -->
          <div class="border-slate-800/80 p-4 sm:border-t lg:border-t-0">
            <p class="text-[11px] text-slate-400">Handshake Age</p>
            <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
              {{ runner.handshakeAgeSeconds !== undefined ? formatDurationSeconds(runner.handshakeAgeSeconds) : '-' }}
            </p>
            <p class="mt-0.5 truncate font-mono text-[11px] text-slate-500" :title="runner.handshakeState">
              {{ formatHandshakeState(runner.handshakeState) }}
            </p>
          </div>

          <!-- Dial Errors & Rejected Streams -->
          <div class="border-slate-800/80 p-4 sm:border-l sm:border-t lg:border-l-0 lg:border-t-0">
            <p class="text-[11px] text-slate-400">Dial Errors / Rejected</p>
            <p
              class="mt-1.5 font-mono text-lg font-semibold tabular-nums"
              :class="totalErrors > 0 ? 'text-amber-300' : 'text-white'"
            >
              <template v-if="stats">
                {{ stats.dialErrorsTotal }}
                <span class="text-xs font-normal text-slate-500">/ {{ stats.streamsRejectedTotal }}</span>
              </template>
              <template v-else>-</template>
            </p>
            <p class="mt-0.5 font-mono text-[11px] text-slate-500">
              {{ stats ? 'since relay start' : 'stats unavailable' }}
            </p>
          </div>
        </div>
      </section>

      <!-- WireGuard Peer Transfer Breakdown -->
      <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800 px-4 py-3 sm:px-5">
          <div class="flex items-center gap-2 text-sm font-semibold text-white">
            <Activity class="h-4 w-4 text-sky-400" />
            WireGuard Peer Transfer
          </div>
          <span v-if="stats?.collectedAt" class="font-mono text-[11px] tabular-nums text-slate-500">
            Sampled {{ formatDateTime(stats.collectedAt) }}
          </span>
        </div>

        <div v-if="!stats" class="flex flex-col items-start justify-between gap-3 p-4 text-xs text-slate-400 sm:flex-row sm:items-center sm:p-5">
          <div class="space-y-1">
            <p class="font-medium text-slate-200">Live WireRelay transfer stats are not available for this runner.</p>
            <p class="text-slate-400">
              This happens when the runner is stopped, unreachable, or still running an older WireRelay image without the
              <code class="rounded bg-slate-800 px-1 py-0.5 font-mono text-[11px] text-slate-200">GetStats</code> RPC.
            </p>
          </div>
          <button
            v-if="runner.desiredState === 'RUNNING'"
            type="button"
            class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-800 px-3 text-xs font-medium text-slate-200 hover:bg-slate-700"
            :disabled="restarting"
            @click="showRestartConfirm = true"
          >
            <RotateCw class="h-3.5 w-3.5" :class="{ 'animate-spin': restarting }" />
            Recreate / Restart
          </button>
        </div>

        <div v-else-if="!stats.peers || stats.peers.length === 0" class="px-4 py-6 text-center text-xs text-slate-500">
          No WireGuard peers reported on interface {{ stats.interface || 'wg0' }}.
        </div>

        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead class="border-b border-slate-800 text-slate-400">
              <tr>
                <th class="px-4 py-2.5 sm:px-5">Peer Public Key</th>
                <th class="px-4 py-2.5 text-right">RX Total</th>
                <th class="px-4 py-2.5 text-right">TX Total</th>
                <th class="px-4 py-2.5 text-right sm:px-5">Combined</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/70">
              <tr v-for="peer in stats.peers" :key="peer.publicKey" class="hover:bg-slate-800/30">
                <td class="px-4 py-2.5 sm:px-5">
                  <div class="flex items-center gap-2">
                    <span class="font-mono text-slate-200" :title="peer.publicKey">{{ peer.publicKey }}</span>
                    <button
                      type="button"
                      class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-slate-400 transition hover:bg-slate-800 hover:text-white"
                      :title="copiedKey === peer.publicKey ? 'Copied' : 'Copy public key'"
                      @click="copyText(peer.publicKey)"
                    >
                      <Check v-if="copiedKey === peer.publicKey" class="h-3.5 w-3.5 text-emerald-400" />
                      <Copy v-else class="h-3.5 w-3.5" />
                    </button>
                  </div>
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-emerald-300">
                  {{ formatBytes(peer.rxBytes) }}
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-sky-300">
                  {{ formatBytes(peer.txBytes) }}
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-200 sm:px-5">
                  {{ formatBytes(peer.rxBytes + peer.txBytes) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- Container Runtime & WireGuard Metadata -->
      <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
        <div class="mb-4 flex items-center gap-2 text-sm font-semibold text-white">
          <ShieldCheck class="h-4 w-4 text-indigo-400" />
          Container & Tunnel Configuration
        </div>
        <dl class="grid gap-x-6 gap-y-4 text-xs sm:grid-cols-2 lg:grid-cols-4">
          <div class="min-w-0">
            <dt class="text-slate-500">Container Name</dt>
            <dd class="mt-1 break-all font-mono text-slate-200">{{ runner.containerName }}</dd>
          </div>
          <div class="min-w-0">
            <dt class="text-slate-500">Container ID</dt>
            <dd class="mt-1 truncate font-mono text-slate-200" :title="runner.containerId">
              {{ runner.containerId ? shortKey(runner.containerId, 12, 0) : '-' }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-slate-500">WireRelay Image</dt>
            <dd class="mt-1 break-all font-mono text-slate-200">{{ runner.image }}</dd>
          </div>
          <div class="min-w-0">
            <dt class="text-slate-500">Config Fingerprint</dt>
            <dd class="mt-1 truncate font-mono text-slate-200" :title="runner.confFingerprint">
              {{ shortKey(runner.confFingerprint, 16, 8) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-slate-500">Last Handshake</dt>
            <dd class="mt-1 font-mono tabular-nums text-slate-200">{{ formatDateTime(runner.lastHandshakeAt) }}</dd>
          </div>
          <div class="min-w-0">
            <dt class="text-slate-500">Last Health Check</dt>
            <dd class="mt-1 font-mono tabular-nums text-slate-200">{{ formatDateTime(runner.lastCheckedAt) }}</dd>
          </div>
          <div class="min-w-0">
            <dt class="text-slate-500">Last Reconciled</dt>
            <dd class="mt-1 font-mono tabular-nums text-slate-200">{{ formatDateTime(runner.lastReconciledAt) }}</dd>
          </div>
          <div v-if="site?.wireguard" class="min-w-0">
            <dt class="text-slate-500">Remote Endpoint</dt>
            <dd class="mt-1 break-all font-mono text-slate-200">
              {{ site.wireguard.endpointHost }}:{{ site.wireguard.endpointPort }}
            </dd>
          </div>
          <div v-if="site?.wireguard" class="min-w-0 sm:col-span-2">
            <dt class="text-slate-500">Allowed LAN Subnets</dt>
            <dd class="mt-1 break-words font-mono text-slate-200">{{ site.wireguard.lanSubnets.join(', ') }}</dd>
          </div>
          <div v-if="site?.wireguard" class="min-w-0">
            <dt class="text-slate-500">MTU / Keepalive</dt>
            <dd class="mt-1 font-mono tabular-nums text-slate-200">
              {{ site.wireguard.mtu || 1420 }} / {{ site.wireguard.persistentKeepalive }}s
            </dd>
          </div>
          <div v-if="site?.wireguard" class="min-w-0">
            <dt class="text-slate-500">Platform Client Key</dt>
            <dd class="mt-1 truncate font-mono text-slate-200" :title="site.wireguard.clientPublicKey">
              {{ shortKey(site.wireguard.clientPublicKey, 14, 6) }}
            </dd>
          </div>
        </dl>
      </section>
    </template>

    <ConfirmActionModal
      :open="showRestartConfirm"
      title="Restart Site Runner"
      :item-name="runner?.containerName || site?.name"
      description="Drains all active WebView and VNC sessions on this site, stops the WireRelay container, and restarts it with the site's WireGuard config."
      confirm-label="Restart Runner"
      tone="warning"
      :busy="restarting"
      @close="showRestartConfirm = false"
      @confirm="restartRunner"
    />
  </div>
</template>
