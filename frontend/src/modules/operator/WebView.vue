<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { LayoutGrid, Pencil, RefreshCw, Wifi, WifiOff } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useSiteStore } from '../../stores/sites'
import { useToastStore } from '../../stores/toast'
import { useWebviewStore } from '../../stores/webview'
import type { RegisterMapping, WebviewDashboard, WebviewWorkspace } from '../../types/webview'
import DashboardTileGrid from '../../components/webview/DashboardTileGrid.vue'

const route = useRoute()
const router = useRouter()
const siteId = computed(() => String(route.params.siteId || ''))
const routeDashboardId = computed(() => (typeof route.params.dashboardId === 'string' ? route.params.dashboardId : ''))
const siteStore = useSiteStore()
const toast = useToastStore()
const webviewStore = useWebviewStore()

const workspace = ref<WebviewWorkspace | null>(null)
const loading = ref(false)

const canOperate = computed(() => siteStore.canOperateSite(siteId.value))

const mappingsById = computed<Record<string, RegisterMapping>>(() => {
  const out: Record<string, RegisterMapping> = {}
  for (const m of workspace.value?.mappings || []) {
    out[m.id] = m
  }
  return out
})

const activeDashboard = computed<WebviewDashboard | null>(() => {
  const list = workspace.value?.dashboards || []
  if (list.length === 0) return null
  if (routeDashboardId.value) {
    const found = list.find((d) => d.id === routeDashboardId.value)
    if (found) return found
  }
  return list.find((d) => d.isDefault) ?? list[0]
})

const editDashboardRoute = computed(() => {
  const query: Record<string, string> = { tab: 'dashboards' }
  if (activeDashboard.value?.id) {
    query.dashboard = activeDashboard.value.id
  }
  return {
    path: `/sites/${siteId.value}/manage/webview`,
    query,
  }
})

function syncSubscriptions() {
  const dash = activeDashboard.value
  if (!dash || !siteId.value) {
    webviewStore.disconnect()
    return
  }
  const ids = dash.widgets
    .map((w) => w.registerMappingId || '')
    .filter((id) => Boolean(id && mappingsById.value[id]))
  webviewStore.connect(siteId.value, ids, dash.pollIntervalMs || 1000)
}

async function loadWorkspace() {
  if (!siteId.value) return
  loading.value = true
  try {
    const data = await apiRequest<WebviewWorkspace>(`/api/v1/webview/sites/${siteId.value}/workspace`)
    workspace.value = data
    webviewStore.setSiteDashboards(siteId.value, data.dashboards || [])
    if (data.dashboards.length > 0 && !routeDashboardId.value) {
      const target = data.dashboards.find((d) => d.isDefault) ?? data.dashboards[0]
      await router.replace(`/sites/${siteId.value}/webview/${target.id}`)
    }
    syncSubscriptions()
  } catch (err) {
    toast.error(err, 'Failed to load WebView')
  } finally {
    loading.value = false
  }
}

// On WebView, only surface errors (no noisy success toasts on routine control writes).
async function handleWrite(mappingId: string, value: number, pulseMs?: number) {
  try {
    await webviewStore.writeValue(siteId.value, mappingId, value, pulseMs)
  } catch (err) {
    const name = mappingsById.value[mappingId]?.name
    toast.error(err, name ? `Write to ${name} failed` : 'Write command failed')
  }
}

onMounted(loadWorkspace)
watch(siteId, () => {
  void loadWorkspace()
})
watch(routeDashboardId, syncSubscriptions)
onUnmounted(() => {
  webviewStore.disconnect()
})
</script>

<template>
  <div class="w-full space-y-5">
    <!-- Horizontal Dashboard Selector Bar (above the title) -->
    <nav
      v-if="workspace && workspace.dashboards.length > 0"
      aria-label="Site dashboards"
      class="no-scrollbar -mx-4 flex items-center gap-2 overflow-x-auto border-b border-slate-800/80 px-4 pb-3 sm:mx-0 sm:px-0"
    >
      <RouterLink
        v-for="d in workspace.dashboards"
        :key="d.id"
        :to="`/sites/${siteId}/webview/${d.id}`"
        class="inline-flex min-h-[38px] shrink-0 items-center gap-2 rounded-lg border px-3.5 py-2 text-xs font-medium transition"
        :class="
          activeDashboard?.id === d.id
            ? 'border-sky-500/60 bg-sky-500/15 text-sky-200'
            : 'border-slate-800 bg-slate-900/70 text-slate-300 hover:border-slate-700 hover:bg-slate-800/80 hover:text-white'
        "
      >
        <LayoutGrid class="h-3.5 w-3.5" />
        <span>{{ d.name }}</span>
      </RouterLink>
    </nav>

    <!-- Dashboard Title & Live Connection Header -->
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <h2 class="truncate text-lg font-semibold tracking-tight text-white sm:text-xl">
        {{ activeDashboard?.name || 'WebView' }}
      </h2>

      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span
          class="inline-flex h-8 items-center gap-1.5 rounded-full border px-2.5 text-[11px] font-medium"
          :class="
            webviewStore.connectionState === 'connected'
              ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300'
              : webviewStore.connectionState === 'connecting'
                ? 'border-amber-500/30 bg-amber-500/10 text-amber-300'
                : 'border-slate-700 bg-slate-900 text-slate-400'
          "
          :title="webviewStore.connectionError || webviewStore.connectionState"
        >
          <Wifi v-if="webviewStore.connectionState === 'connected'" class="h-3.5 w-3.5" />
          <WifiOff v-else class="h-3.5 w-3.5" />
          {{ webviewStore.connectionState }}
        </span>

        <RouterLink
          v-if="canOperate"
          :to="editDashboardRoute"
          class="inline-flex h-9 items-center justify-center gap-1.5 rounded-lg border border-amber-500/45 bg-amber-500/15 px-3 text-xs font-medium leading-none text-amber-200 transition hover:bg-amber-500/25"
        >
          <Pencil class="h-3.5 w-3.5" />
          Edit
        </RouterLink>

        <button
          type="button"
          class="hidden h-9 items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 text-xs font-medium leading-none text-slate-200 transition hover:bg-slate-800 sm:inline-flex"
          @click="loadWorkspace"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
          Refresh
        </button>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-if="!loading && (!workspace || workspace.dashboards.length === 0)"
      class="flex flex-col items-center justify-center rounded-xl border border-dashed border-slate-800 bg-slate-900/40 px-6 py-16 text-center"
    >
      <LayoutGrid class="h-9 w-9 text-slate-700" />
      <p class="mt-3 text-sm font-medium text-slate-300">No WebView dashboards configured</p>
      <p class="mt-1 max-w-md text-xs text-slate-500">
        Ask a Site Operator to add devices, registers, and dashboards under Manage Site -> WebView.
      </p>
      <RouterLink
        v-if="canOperate"
        :to="editDashboardRoute"
        class="mt-4 inline-flex h-9 items-center gap-1.5 rounded-lg bg-amber-500 px-4 text-xs font-semibold text-slate-950 transition hover:bg-amber-400"
      >
        <Pencil class="h-3.5 w-3.5" />
        Configure Dashboards
      </RouterLink>
    </div>

    <!-- Active Dashboard Tile Grid -->
    <DashboardTileGrid
      v-else-if="activeDashboard"
      :widgets="activeDashboard.widgets"
      :layout="activeDashboard.layout"
      :mappings-by-id="mappingsById"
      :samples="webviewStore.samples"
      :can-write="Boolean(workspace?.canWrite)"
      :editing="false"
      @write="(mid, val, pulse) => handleWrite(mid, val, pulse)"
    />
  </div>
</template>
