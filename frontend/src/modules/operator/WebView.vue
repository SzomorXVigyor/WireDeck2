<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { LayoutGrid, RefreshCw, ShieldCheck, ShieldOff, Wifi, WifiOff } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useWebviewStore } from '../../stores/webview'
import type { RegisterMapping, WebviewDashboard, WebviewWorkspace } from '../../types/webview'
import WidgetLamp from '../../components/widgets/WidgetLamp.vue'
import WidgetDisplay from '../../components/widgets/WidgetDisplay.vue'
import WidgetSwitch from '../../components/widgets/WidgetSwitch.vue'
import WidgetButton from '../../components/widgets/WidgetButton.vue'
import WidgetNumberInput from '../../components/widgets/WidgetNumberInput.vue'
import WidgetChart from '../../components/widgets/WidgetChart.vue'

const route = useRoute()
const siteId = computed(() => String(route.params.siteId || ''))
const webviewStore = useWebviewStore()

const workspace = ref<WebviewWorkspace | null>(null)
const activeDashboardId = ref<string>('')
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
  return list.find((d) => d.id === activeDashboardId.value) ?? list[0] ?? null
})

function syncSubscriptions() {
  const dash = activeDashboard.value
  if (!dash || !siteId.value) {
    webviewStore.disconnect()
    return
  }
  const ids = dash.widgets
    .map((w) => w.registerMappingId || '')
    .filter((id) => {
      if (!id) return false
      const m = mappingsById.value[id]
      return m && m.accessMode !== 'WRITE'
    })
  webviewStore.connect(siteId.value, ids)
}

async function loadWorkspace() {
  if (!siteId.value) return
  loading.value = true
  errMsg.value = null
  try {
    const data = await apiRequest<WebviewWorkspace>(`/api/v1/webview/sites/${siteId.value}/workspace`)
    workspace.value = data
    if (data.dashboards.length > 0) {
      const stillExists = data.dashboards.some((d) => d.id === activeDashboardId.value)
      if (!stillExists) {
        const def = data.dashboards.find((d) => d.isDefault) ?? data.dashboards[0]
        activeDashboardId.value = def.id
      }
    } else {
      activeDashboardId.value = ''
    }
    syncSubscriptions()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to load WebView workspace'
  } finally {
    loading.value = false
  }
}

async function handleWrite(mappingId: string, value: number) {
  writeNotice.value = null
  try {
    await webviewStore.writeValue(siteId.value, mappingId, value)
    const name = mappingsById.value[mappingId]?.name || 'register'
    writeNotice.value = { ok: true, text: `Wrote ${value} to ${name}.` }
  } catch (err) {
    writeNotice.value = {
      ok: false,
      text: err instanceof Error ? err.message : 'Write command failed',
    }
  }
}

onMounted(loadWorkspace)
watch(siteId, () => {
  activeDashboardId.value = ''
  void loadWorkspace()
})
watch(activeDashboardId, syncSubscriptions)
onUnmounted(() => {
  webviewStore.disconnect()
})
</script>

<template>
  <div class="space-y-5">
    <!-- Top Header & Live Telemetry Status -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="d in workspace?.dashboards || []"
          :key="d.id"
          type="button"
          class="rounded-lg border px-3 py-1.5 text-xs font-medium transition"
          :class="
            activeDashboard?.id === d.id
              ? 'border-sky-500/50 bg-sky-500/15 text-sky-200'
              : 'border-slate-800 bg-slate-900/70 text-slate-400 hover:border-slate-700 hover:text-white'
          "
          @click="activeDashboardId = d.id"
        >
          {{ d.name }}
        </button>
      </div>

      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span
          v-if="workspace"
          class="inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[11px]"
          :class="
            workspace.canWrite
              ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300'
              : 'border-slate-700 bg-slate-900 text-slate-400'
          "
        >
          <ShieldCheck v-if="workspace.canWrite" class="h-3.5 w-3.5" />
          <ShieldOff v-else class="h-3.5 w-3.5" />
          {{ workspace.canWrite ? 'Control enabled' : 'Read-only' }}
        </span>

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
        Ask a Site Operator to add Modbus devices, register mappings, and dashboard widgets under Manage Site -> WebView.
      </p>
    </div>

    <!-- Active Dashboard Widget Grid -->
    <div
      v-else-if="activeDashboard"
      class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
    >
      <template v-for="w in activeDashboard.widgets" :key="w.id">
        <WidgetLamp
          v-if="w.widgetType === 'lamp'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
        />
        <WidgetDisplay
          v-else-if="w.widgetType === 'display'"
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
          @write="handleWrite"
        />
        <WidgetButton
          v-else-if="w.widgetType === 'button'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
          :can-write="Boolean(workspace?.canWrite)"
          @write="handleWrite"
        />
        <WidgetNumberInput
          v-else-if="w.widgetType === 'number_input'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
          :can-write="Boolean(workspace?.canWrite)"
          @write="handleWrite"
        />
        <WidgetChart
          v-else-if="w.widgetType === 'chart'"
          :widget="w"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :sample="w.registerMappingId ? webviewStore.samples[w.registerMappingId] : undefined"
          :points="w.registerMappingId ? webviewStore.ringBuffers[w.registerMappingId] || [] : []"
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
