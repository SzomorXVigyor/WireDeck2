<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RefreshCw, RotateCw } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import type { RunnerRecord } from '../../types/iam'
import StatusBadge from '../../components/StatusBadge.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const runners = ref<RunnerRecord[]>([])
const runnersPag = usePagination(runners)
const loading = ref(false)
const error = ref<string | null>(null)

async function loadRunners() {
  loading.value = true
  error.value = null
  try {
    const res = await apiRequest<{ runners: RunnerRecord[] }>('/api/v1/iam/admin/runners')
    runners.value = res.runners || []
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Failed to fetch runners'
  } finally {
    loading.value = false
  }
}

onMounted(loadRunners)

async function restartRunner(siteId: string) {
  await apiRequest(`/api/v1/iam/admin/runners/${siteId}/restart`, { method: 'POST' })
  await loadRunners()
}
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">Tunnel Runners</h1>
        <p class="mt-0.5 text-xs text-slate-400">Live WireRelay container state and WireGuard handshake telemetry.</p>
      </div>
      <button
        type="button"
        class="hidden h-9 shrink-0 items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3.5 text-xs font-medium leading-none text-slate-200 transition hover:bg-slate-800 sm:inline-flex"
        @click="loadRunners"
      >
        <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
        Refresh
      </button>
    </div>

    <p v-if="error" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-xs text-rose-200">{{ error }}</p>

    <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <!-- Mobile Cards -->
      <div class="divide-y divide-slate-800/70 md:hidden">
        <div v-for="r in runnersPag.paginatedItems.value" :key="r.id" class="space-y-3 p-4 text-xs">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <p class="break-all font-mono text-xs font-semibold text-white">{{ r.containerName }}</p>
              <p class="mt-0.5 break-all font-mono text-[10px] text-slate-500">{{ r.image }}</p>
            </div>
            <StatusBadge :status="r.status" size="sm" />
          </div>
          <div class="grid grid-cols-2 gap-2 rounded-lg border border-slate-800/80 bg-slate-950/60 p-2.5 font-mono text-[11px]">
            <div>
              <p class="text-[10px] text-slate-500">State</p>
              <p class="text-slate-200">{{ r.desiredState }} / {{ r.observedState || 'unknown' }}</p>
            </div>
            <div>
              <p class="text-[10px] text-slate-500">Handshake</p>
              <p class="text-slate-200">
                {{ r.handshakeState }}
                <span v-if="r.handshakeAgeSeconds !== undefined">({{ r.handshakeAgeSeconds }}s)</span>
              </p>
            </div>
          </div>
          <p v-if="r.lastError" class="break-words rounded-lg border border-rose-500/30 bg-rose-500/10 px-2.5 py-1.5 text-[11px] text-rose-300">
            {{ r.lastError }}
          </p>
          <button
            type="button"
            class="inline-flex h-8 w-full items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-800 px-3 text-[11px] font-medium text-slate-200 hover:bg-slate-700"
            @click="restartRunner(r.siteId)"
          >
            <RotateCw class="h-3.5 w-3.5" /> Restart Runner
          </button>
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
              <th class="px-4 py-3">Container</th>
              <th class="px-4 py-3">Desired / Observed</th>
              <th class="px-4 py-3">Tunnel Health</th>
              <th class="px-4 py-3">Handshake Age</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="r in runnersPag.paginatedItems.value" :key="r.id" class="hover:bg-slate-800/30">
              <td class="px-4 py-3">
                <p class="font-mono text-slate-200">{{ r.containerName }}</p>
                <p class="font-mono text-[10px] text-slate-500">{{ r.image }}</p>
              </td>
              <td class="px-4 py-3 font-mono text-[11px] text-slate-300">
                {{ r.desiredState }} / {{ r.observedState || 'unknown' }}
              </td>
              <td class="space-y-1 px-4 py-3">
                <StatusBadge :status="r.status" size="sm" />
                <p class="font-mono text-[10px] text-slate-500">{{ r.handshakeState }}</p>
                <p v-if="r.lastError" class="max-w-xs truncate text-[10px] text-rose-400" :title="r.lastError">{{ r.lastError }}</p>
              </td>
              <td class="whitespace-nowrap px-4 py-3 font-mono tabular-nums text-slate-300">
                {{ r.handshakeAgeSeconds !== undefined ? `${r.handshakeAgeSeconds}s` : '-' }}
              </td>
              <td class="px-4 py-3 text-right">
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2.5 py-1 text-[11px] text-slate-200 transition hover:bg-slate-700"
                  @click="restartRunner(r.siteId)"
                >
                  <RotateCw class="h-3 w-3" /> Restart
                </button>
              </td>
            </tr>
            <tr v-if="!loading && runners.length === 0">
              <td colspan="5" class="px-4 py-8 text-center text-slate-500">No runners provisioned yet.</td>
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
  </div>
</template>
