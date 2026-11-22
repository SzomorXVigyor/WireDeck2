<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ExternalLink, KeyRound, Lock, Monitor, RefreshCw } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import type { VncTarget } from '../../types/vnc'

const route = useRoute()
const siteId = computed(() => String(route.params.siteId || ''))

const targets = ref<VncTarget[]>([])
const loading = ref(false)
const error = ref('')

async function loadTargets() {
  loading.value = true
  error.value = ''
  try {
    const res = await apiRequest<{ targets: VncTarget[] }>(`/api/v1/vnc/sites/${siteId.value}/targets`)
    targets.value = res.targets || []
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Failed to load VNC endpoints'
  } finally {
    loading.value = false
  }
}

onMounted(loadTargets)
watch(siteId, loadTargets)

function sessionHref(targetId: string) {
  return `/sites/${encodeURIComponent(siteId.value)}/vnc/${encodeURIComponent(targetId)}`
}
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold text-white">WebVNC Endpoints</h2>
        <p class="mt-0.5 text-xs text-slate-400">
          Click any endpoint to open its remote desktop console in a new browser tab.
        </p>
      </div>

      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-slate-800"
        @click="loadTargets"
      >
        <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
        Refresh
      </button>
    </div>

    <div
      v-if="error"
      class="rounded-lg border border-rose-500/40 bg-rose-500/10 px-4 py-2.5 text-xs text-rose-200"
    >
      {{ error }}
    </div>

    <div
      v-if="!loading && targets.length === 0"
      class="flex flex-col items-center justify-center rounded-xl border border-dashed border-slate-800 bg-slate-900/40 px-6 py-14 text-center"
    >
      <Monitor class="h-9 w-9 text-slate-700" />
      <p class="mt-3 text-sm font-medium text-slate-300">No active VNC endpoints</p>
      <p class="mt-1 max-w-md text-xs text-slate-500">
        Ask a Site Operator to add VNC edge points under Manage Site -> VNC Edge Points.
      </p>
    </div>

    <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <a
        v-for="t in targets"
        :key="t.id"
        :href="sessionHref(t.id)"
        target="_blank"
        rel="noopener noreferrer"
        class="group flex flex-col justify-between rounded-xl border border-slate-800 bg-slate-900/70 p-4 transition hover:border-emerald-500/50 hover:bg-slate-900"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-3 min-w-0">
            <div
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-400 transition group-hover:bg-emerald-500/25"
            >
              <Monitor class="h-5 w-5" />
            </div>
            <div class="min-w-0">
              <h3 class="truncate text-sm font-semibold text-white group-hover:text-emerald-300">
                {{ t.name }}
              </h3>
              <p class="mt-0.5 truncate font-mono text-xs text-slate-400">
                {{ t.host }}:{{ t.port }}
              </p>
            </div>
          </div>

          <ExternalLink
            class="h-4 w-4 shrink-0 text-slate-500 transition group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-emerald-400"
          />
        </div>

        <div class="mt-4 flex items-center justify-between border-t border-slate-800/80 pt-3 text-[11px]">
          <span
            v-if="t.hasPassword"
            class="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 font-medium text-emerald-300"
          >
            <Lock class="h-3 w-3" />
            Server-side auth
          </span>
          <span
            v-else
            class="inline-flex items-center gap-1 rounded-full bg-slate-800 px-2 py-0.5 font-medium text-slate-300"
          >
            <KeyRound class="h-3 w-3" />
            Prompt on connect
          </span>

          <span class="font-medium text-emerald-400 opacity-80 group-hover:opacity-100">
            Open console
          </span>
        </div>
      </a>
    </div>
  </div>
</template>
