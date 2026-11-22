<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { LayoutGrid, RefreshCw, Wifi, WifiOff } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useWebviewStore } from '../../stores/webview'
import type { RegisterMapping, WebviewDashboard, WebviewWorkspace } from '../../types/webview'
import WidgetLamp from '../../components/widgets/WidgetLamp.vue'
import WidgetDisplay from '../../components/widgets/WidgetDisplay.vue'
import WidgetSwitch from '../../components/widgets/WidgetSwitch.vue'
import WidgetButton from '../../components/widgets/WidgetButton.vue'
import WidgetNumberInput from '../../components/widgets/WidgetNumberInput.vue'

const route = useRoute()
const router = useRouter()
const siteId = computed(() => String(route.params.siteId || ''))
const routeDashboardId = computed(() => (typeof route.params.dashboardId === 'string' ? route.params.dashboardId : ''))
const webviewStore = useWebviewStore()

const workspace = ref<WebviewWorkspace | null>(null)
const loading = ref(false)
const errMsg = ref<string | null>(null)
const writeNotice = ref<{ ok: boolean; text: string } | null>(null)

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
  errMsg.value = null
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
    errMsg.value = err instanceof Error ? err.message : 'Failed to load WebView workspace'
  } finally {
    loading.value = false
  }
}

async function handleWrite(mappingId: string, value: number, pulseMs?: number) {
  writeNotice.value = null
  try {
    await webviewStore.writeValue(siteId.value, mappingId, value, pulseMs)
    const name = mappingsById.value[mappingId]?.name || 'register'
    writeNotice.value = {
      ok: true,
      text: pulseMs ? `Triggered ${name} for ${pulseMs} ms.` : `Wrote value to ${name}.`,
    }
  } catch (err) {
    writeNotice.value = {
      ok: false,
      text: err instanceof Error ? err.message : 'Write command failed',
    }
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
  <div class="space-y-5">
    <!-- Horizontal Dashboard Selector Bar (above the title) -->
    <nav
      v-if="workspace && workspace.dashboards.length > 0"
      aria-label="Site dashboards"
      class="flex items-center gap-2 overflow-x-auto border-b border-slate-800/80 pb-3"
    >
      <RouterLink
        v-for="d in workspace.dashboards"
        :key="d.id"
        :to="`/sites/${siteId}/webview/${d.id}`"
        class="inline-flex shrink-0 items-center gap-2 rounded-lg border px-3.5 py-2 text-xs font-medium transition"
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
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-lg font-semibold text-white">
        {{ activeDashboard?.name || 'WebView' }}
      </h2>

      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span
          class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px]"
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

        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-1.5 text-xs text-slate-200 hover:bg-slate-800"
          @click="loadWorkspace"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
          Refresh
        </button>
      </div>
    </div>

    <p v-if="errMsg" class="rounded-lg border border-rose-500/40 bg-rose-500/10 px-4 py-2.5 text-xs text-rose-200">
      {{ errMsg }}
    </p>

    <p
      v-if="writeNotice"
      class="rounded-lg border px-4 py-2 text-xs"
      :class="
        writeNotice.ok
          ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200'
          : 'border-rose-500/40 bg-rose-500/10 text-rose-200'
      "
    >
      {{ writeNotice.text }}
    </p>

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
    </div>

    <!-- Active Dashboard Widget Grid -->
    <div
      v-else-if="activeDashboard"
      class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
    >
      <template v-for="w in activeDashboard.widgets" :key="w.id">
        <WidgetDisplay
          v-if="w.widgetType === 'display'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
        />
        <WidgetLamp
          v-else-if="w.widgetType === 'lamp'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
        />
        <WidgetSwitch
          v-else-if="w.widgetType === 'switch'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
          :can-write="Boolean(workspace?.canWrite)"
          @write="(mid, val) => handleWrite(mid, val)"
        />
        <WidgetButton
          v-else-if="w.widgetType === 'button'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
          :can-write="Boolean(workspace?.canWrite)"
          @write="(mid, val, pulse) => handleWrite(mid, val, pulse)"
        />
        <WidgetNumberInput
          v-else-if="w.widgetType === 'number_input'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
          :can-write="Boolean(workspace?.canWrite)"
          @write="(mid, val) => handleWrite(mid, val)"
        />
      </template>

      <div
        v-if="activeDashboard.widgets.length === 0"
        class="col-span-full rounded-xl border border-dashed border-slate-800 bg-slate-900/30 p-10 text-center text-xs text-slate-500"
      >
        This dashboard has no widgets yet.
      </div>
    </div>
  </div>
</template>
