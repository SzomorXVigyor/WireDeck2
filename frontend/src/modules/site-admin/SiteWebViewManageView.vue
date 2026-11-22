<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Cpu, Database, LayoutGrid, Play, Plus, Sliders, Trash2 } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import type { SiteMember } from '../../types/iam'
import type {
  AccessMode,
  DataType,
  FunctionCode,
  MemberWritePolicy,
  RegisterMapping,
  TelemetrySample,
  WebviewDashboard,
  WebviewDevice,
  WidgetType,
  WriteFunctionCode,
} from '../../types/webview'
import StatusBadge from '../../components/StatusBadge.vue'

const route = useRoute()
const siteId = computed(() => String(route.params.siteId || ''))

type TabKey = 'devices' | 'mappings' | 'dashboards' | 'settings'
const activeTab = ref<TabKey>('devices')

const devices = ref<WebviewDevice[]>([])
const mappings = ref<RegisterMapping[]>([])
const dashboards = ref<WebviewDashboard[]>([])
const members = ref<SiteMember[]>([])
const policies = ref<MemberWritePolicy[]>([])
const historyEnabled = ref(false)
const testResults = ref<Record<string, TelemetrySample>>({})
const testingId = ref<string | null>(null)

const errMsg = ref<string | null>(null)
const okMsg = ref<string | null>(null)

// Device form
const devName = ref('')
const devHost = ref('')
const devPort = ref(502)
const devProto = ref<'MODBUS_TCP' | 'MODBUS_UDP'>('MODBUS_TCP')
const devUnit = ref(1)
const devTimeout = ref(2000)

// Register mapping form
const mapDeviceId = ref('')
const mapName = ref('')
const mapFC = ref<FunctionCode>('READ_HOLDING')
const mapAddr = ref(0)
const mapType = ref<DataType>('UINT16')
const mapWordOrder = ref<'BIG' | 'LITTLE'>('BIG')
const mapByteOrder = ref<'BIG' | 'LITTLE'>('BIG')
const mapScale = ref(1)
const mapOffset = ref(0)
const mapUnit = ref('')
const mapAccess = ref<AccessMode>('READ')
const mapWriteFC = ref<WriteFunctionCode>('')
const mapIntMs = ref(1000)
const mapArchMs = ref(60000)
const mapDeadband = ref(0)
const mapMaxSec = ref(300)

// Dashboard & Widget forms
const newDashName = ref('')
const selectedDashId = ref('')
const widName = ref('')
const widType = ref<WidgetType>('display')
const widMappingId = ref('')
const widPressValue = ref(1)
const widButtonLabel = ref('Trigger')
const widOnLabel = ref('RUNNING')
const widOffLabel = ref('STOPPED')

const selectedDash = computed(() => dashboards.value.find((d) => d.id === selectedDashId.value) ?? dashboards.value[0] ?? null)
const deviceNameMap = computed<Record<string, string>>(() => {
  const m: Record<string, string> = {}
  for (const d of devices.value) m[d.id] = d.name
  return m
})
const policyMap = computed<Record<string, boolean>>(() => {
  const m: Record<string, boolean> = {}
  for (const p of policies.value) m[p.userId] = p.canWrite
  return m
})
const siteMembersOnly = computed(() => members.value.filter((m) => m.role === 'SITE_MEMBER'))

async function loadAll() {
  errMsg.value = null
  try {
    const [dRes, mRes, dbRes, stRes, pRes, memRes] = await Promise.all([
      apiRequest<{ devices: WebviewDevice[] }>(`/api/v1/webview/sites/${siteId.value}/operator/devices`),
      apiRequest<{ mappings: RegisterMapping[] }>(`/api/v1/webview/sites/${siteId.value}/operator/mappings`),
      apiRequest<{ dashboards: WebviewDashboard[] }>(`/api/v1/webview/sites/${siteId.value}/operator/dashboards`),
      apiRequest<{ historyEnabled: boolean }>(`/api/v1/webview/sites/${siteId.value}/operator/settings`),
      apiRequest<{ policies: MemberWritePolicy[] }>(`/api/v1/webview/sites/${siteId.value}/operator/member-policies`),
      apiRequest<{ members: SiteMember[] }>(`/api/v1/iam/sites/${siteId.value}/operator/members`).catch(() => ({ members: [] })),
    ])
    devices.value = dRes.devices || []
    mappings.value = mRes.mappings || []
    dashboards.value = dbRes.dashboards || []
    historyEnabled.value = Boolean(stRes.historyEnabled)
    policies.value = pRes.policies || []
    members.value = memRes.members || []

    if (!mapDeviceId.value && devices.value.length > 0) {
      mapDeviceId.value = devices.value[0].id
    }
    if (!selectedDashId.value && dashboards.value.length > 0) {
      selectedDashId.value = dashboards.value[0].id
    }
    if (!widMappingId.value && mappings.value.length > 0) {
      widMappingId.value = mappings.value[0].id
    }
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to load WebView configuration'
  }
}

onMounted(loadAll)
watch(siteId, loadAll)

async function addDevice() {
  errMsg.value = null
  okMsg.value = null
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/devices`, {
      method: 'POST',
      body: JSON.stringify({
        name: devName.value,
        host: devHost.value,
        port: Number(devPort.value),
        protocol: devProto.value,
        unitId: Number(devUnit.value),
        requestTimeoutMs: Number(devTimeout.value),
      }),
    })
    okMsg.value = `Added Modbus device "${devName.value}".`
    devName.value = ''
    devHost.value = ''
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to add device'
  }
}

async function deleteDevice(d: WebviewDevice) {
  if (!window.confirm(`Delete device "${d.name}" and all its register mappings?`)) return
  errMsg.value = null
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/devices/${d.id}`, { method: 'DELETE' })
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to delete device'
  }
}

async function addMapping() {
  errMsg.value = null
  okMsg.value = null
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/mappings`, {
      method: 'POST',
      body: JSON.stringify({
        deviceId: mapDeviceId.value,
        name: mapName.value,
        functionCode: mapFC.value,
        registerAddress: Number(mapAddr.value),
        dataType: mapType.value,
        wordOrder: mapWordOrder.value,
        byteOrder: mapByteOrder.value,
        scale: Number(mapScale.value) || 1,
        offset: Number(mapOffset.value) || 0,
        unit: mapUnit.value,
        accessMode: mapAccess.value,
        writeFunctionCode: mapAccess.value === 'READ' ? '' : mapWriteFC.value,
        interactivePollIntervalMs: Number(mapIntMs.value),
        archivalPollIntervalMs: Number(mapArchMs.value),
        deadband: Number(mapDeadband.value),
        maxIntervalSeconds: Number(mapMaxSec.value),
      }),
    })
    okMsg.value = `Added register mapping "${mapName.value}".`
    mapName.value = ''
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to add register mapping'
  }
}

async function deleteMapping(m: RegisterMapping) {
  if (!window.confirm(`Delete register mapping "${m.name}"?`)) return
  errMsg.value = null
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/mappings/${m.id}`, { method: 'DELETE' })
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to delete mapping'
  }
}

async function testRead(m: RegisterMapping) {
  testingId.value = m.id
  errMsg.value = null
  try {
    const sample = await apiRequest<TelemetrySample>(
      `/api/v1/webview/sites/${siteId.value}/operator/mappings/${m.id}/test-read`,
      { method: 'POST' },
    )
    testResults.value = { ...testResults.value, [m.id]: sample }
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Test read failed'
  } finally {
    testingId.value = null
  }
}

async function addDashboard() {
  errMsg.value = null
  okMsg.value = null
  try {
    const created = await apiRequest<WebviewDashboard>(`/api/v1/webview/sites/${siteId.value}/operator/dashboards`, {
      method: 'POST',
      body: JSON.stringify({ name: newDashName.value, isDefault: dashboards.value.length === 0 }),
    })
    okMsg.value = `Created dashboard "${newDashName.value}".`
    newDashName.value = ''
    selectedDashId.value = created.id
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to create dashboard'
  }
}

async function deleteDashboard(d: WebviewDashboard) {
  if (!window.confirm(`Delete dashboard "${d.name}" and its widgets?`)) return
  errMsg.value = null
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${d.id}`, { method: 'DELETE' })
    selectedDashId.value = ''
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to delete dashboard'
  }
}

async function addWidget() {
  if (!selectedDash.value) return
  errMsg.value = null
  okMsg.value = null
  const extra: Record<string, unknown> = {}
  if (widType.value === 'button') {
    extra.pressValue = Number(widPressValue.value)
    extra.buttonLabel = widButtonLabel.value || 'Trigger'
  } else if (widType.value === 'lamp') {
    extra.onLabel = widOnLabel.value || 'ACTIVE'
    extra.offLabel = widOffLabel.value || 'INACTIVE'
  }
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${selectedDash.value.id}/widgets`, {
      method: 'POST',
      body: JSON.stringify({
        name: widName.value,
        widgetType: widType.value,
        registerMappingId: widMappingId.value || undefined,
        sortOrder: selectedDash.value.widgets.length,
        extra,
      }),
    })
    okMsg.value = `Added widget "${widName.value}".`
    widName.value = ''
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to add widget'
  }
}

async function deleteWidget(widgetId: string) {
  errMsg.value = null
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/widgets/${widgetId}`, { method: 'DELETE' })
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to delete widget'
  }
}

async function saveHistorySetting() {
  errMsg.value = null
  okMsg.value = null
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/settings`, {
      method: 'PATCH',
      body: JSON.stringify({ historyEnabled: historyEnabled.value }),
    })
    okMsg.value = historyEnabled.value
      ? 'WebView history archival enabled for this site.'
      : 'WebView history archival disabled for this site.'
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to update WebView settings'
  }
}

async function toggleMemberWrite(member: SiteMember, canWrite: boolean) {
  errMsg.value = null
  okMsg.value = null
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/member-policies`, {
      method: 'PUT',
      body: JSON.stringify({ userId: member.userId, canWrite }),
    })
    okMsg.value = `Updated write permission for ${member.email}.`
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to update member write policy'
  }
}
</script>

<template>
  <div class="mx-auto max-w-6xl space-y-6">
    <div>
      <h2 class="text-lg font-semibold text-white">WebView Configuration</h2>
      <p class="text-xs text-slate-400">
        Manage Modbus devices, register mappings, dashboards, widgets, WebView history archival, and site member write permissions.
      </p>
    </div>

    <!-- Sub-navigation Tabs -->
    <div class="flex flex-wrap gap-2 border-b border-slate-800 pb-3">
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium"
        :class="activeTab === 'devices' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'devices'"
      >
        <Cpu class="h-3.5 w-3.5" /> Devices ({{ devices.length }})
      </button>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium"
        :class="activeTab === 'mappings' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'mappings'"
      >
        <Sliders class="h-3.5 w-3.5" /> Registers ({{ mappings.length }})
      </button>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium"
        :class="activeTab === 'dashboards' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'dashboards'"
      >
        <LayoutGrid class="h-3.5 w-3.5" /> Dashboards & Widgets ({{ dashboards.length }})
      </button>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium"
        :class="activeTab === 'settings' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'settings'"
      >
        <Database class="h-3.5 w-3.5" /> History & Member Policies
      </button>
    </div>

    <p v-if="errMsg" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-xs text-rose-200">{{ errMsg }}</p>
    <p v-if="okMsg" class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-200">{{ okMsg }}</p>

    <!-- TAB 1: DEVICES -->
    <div v-if="activeTab === 'devices'" class="space-y-5">
      <form
        class="grid gap-3 rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:grid-cols-6"
        @submit.prevent="addDevice"
      >
        <input v-model="devName" required placeholder="Device name (e.g. PLC-1)" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white sm:col-span-2" />
        <input v-model="devHost" required placeholder="Host IP (in site LAN)" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white" />
        <input v-model.number="devPort" type="number" min="1" max="65535" required placeholder="502" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white" />
        <select v-model="devProto" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
          <option value="MODBUS_TCP">Modbus TCP</option>
          <option value="MODBUS_UDP">Modbus UDP</option>
        </select>
        <div class="flex gap-2">
          <input v-model.number="devUnit" type="number" min="0" max="255" title="Unit ID" class="w-16 rounded-lg border border-slate-700 bg-slate-950 px-2 py-2 font-mono text-xs text-white" />
          <button type="submit" class="inline-flex flex-1 items-center justify-center gap-1 rounded-lg bg-amber-500 px-3 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400">
            <Plus class="h-3.5 w-3.5" /> Add
          </button>
        </div>
      </form>

      <div class="overflow-x-auto rounded-xl border border-slate-800 bg-slate-900/60">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-4 py-3">Name</th>
              <th class="px-4 py-3">Endpoint</th>
              <th class="px-4 py-3">Protocol / Unit</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="d in devices" :key="d.id">
              <td class="px-4 py-3 font-medium text-white">{{ d.name }}</td>
              <td class="px-4 py-3 font-mono text-slate-300">{{ d.host }}:{{ d.port }}</td>
              <td class="px-4 py-3 font-mono text-slate-400">{{ d.protocol }} / unit {{ d.unitId }}</td>
              <td class="px-4 py-3"><StatusBadge :status="d.isActive ? 'ACTIVE' : 'DISABLED'" size="sm" /></td>
              <td class="px-4 py-3 text-right">
                <button type="button" class="rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 hover:bg-rose-500/20" @click="deleteDevice(d)">
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
              </td>
            </tr>
            <tr v-if="devices.length === 0">
              <td colspan="5" class="px-4 py-8 text-center text-slate-500">No Modbus devices configured yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- TAB 2: REGISTER MAPPINGS -->
    <div v-if="activeTab === 'mappings'" class="space-y-5">
      <form
        class="grid gap-3 rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:grid-cols-6"
        @submit.prevent="addMapping"
      >
        <select v-model="mapDeviceId" required class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white sm:col-span-2">
          <option value="" disabled>Select device...</option>
          <option v-for="d in devices" :key="d.id" :value="d.id">{{ d.name }} ({{ d.host }})</option>
        </select>
        <input v-model="mapName" required placeholder="Register name (e.g. Tank Temp)" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white sm:col-span-2" />
        <select v-model="mapFC" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
          <option value="READ_HOLDING">FC03 Holding</option>
          <option value="READ_INPUT">FC04 Input</option>
          <option value="READ_COIL">FC01 Coil</option>
          <option value="READ_DISCRETE">FC02 Discrete</option>
        </select>
        <input v-model.number="mapAddr" type="number" min="0" max="65535" required placeholder="Addr" title="Register address" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white" />

        <select v-model="mapType" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
          <option value="BOOL">BOOL</option>
          <option value="UINT16">UINT16</option>
          <option value="INT16">INT16</option>
          <option value="UINT32">UINT32</option>
          <option value="INT32">INT32</option>
          <option value="FLOAT32">FLOAT32</option>
        </select>
        <select v-model="mapWordOrder" title="Word order" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
          <option value="BIG">Word: BIG</option>
          <option value="LITTLE">Word: LITTLE</option>
        </select>
        <input v-model.number="mapScale" type="number" step="any" placeholder="Scale (1)" title="Scale factor" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white" />
        <input v-model="mapUnit" placeholder="Unit (C, bar)" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white" />
        <select v-model="mapAccess" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
          <option value="READ">READ</option>
          <option value="READ_WRITE">READ_WRITE</option>
          <option value="WRITE">WRITE</option>
        </select>
        <button type="submit" :disabled="!mapDeviceId" class="inline-flex items-center justify-center gap-1 rounded-lg bg-amber-500 px-3 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400 disabled:opacity-40">
          <Plus class="h-3.5 w-3.5" /> Add Register
        </button>
      </form>

      <div class="overflow-x-auto rounded-xl border border-slate-800 bg-slate-900/60">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-4 py-3">Name</th>
              <th class="px-4 py-3">Device</th>
              <th class="px-4 py-3">FC / Addr / Type</th>
              <th class="px-4 py-3">Access</th>
              <th class="px-4 py-3">Live Test</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="m in mappings" :key="m.id">
              <td class="px-4 py-3 font-medium text-white">{{ m.name }} <span v-if="m.unit" class="text-slate-400">({{ m.unit }})</span></td>
              <td class="px-4 py-3 text-slate-300">{{ deviceNameMap[m.deviceId] || m.deviceId }}</td>
              <td class="px-4 py-3 font-mono text-slate-300">{{ m.functionCode }} @ {{ m.registerAddress }} [{{ m.dataType }}]</td>
              <td class="px-4 py-3 font-mono text-slate-400">{{ m.accessMode }}</td>
              <td class="px-4 py-3 font-mono">
                <span v-if="testResults[m.id]?.quality === 'GOOD'" class="text-emerald-300">
                  {{ testResults[m.id].value }} {{ m.unit }}
                </span>
                <span v-else-if="testResults[m.id]" class="text-rose-300" :title="testResults[m.id].error">
                  BAD: {{ testResults[m.id].error }}
                </span>
                <span v-else class="text-slate-600">-</span>
              </td>
              <td class="px-4 py-3 text-right space-x-1.5">
                <button
                  v-if="m.accessMode !== 'WRITE'"
                  type="button"
                  :disabled="testingId === m.id"
                  class="inline-flex items-center gap-1 rounded border border-sky-500/40 bg-sky-500/15 px-2 py-1 text-[11px] text-sky-200 hover:bg-sky-500/25 disabled:opacity-40"
                  @click="testRead(m)"
                >
                  <Play class="h-3 w-3" /> {{ testingId === m.id ? 'Reading...' : 'Test Read' }}
                </button>
                <button type="button" class="rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 hover:bg-rose-500/20" @click="deleteMapping(m)">
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
              </td>
            </tr>
            <tr v-if="mappings.length === 0">
              <td colspan="6" class="px-4 py-8 text-center text-slate-500">No register mappings configured yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- TAB 3: DASHBOARDS & WIDGETS -->
    <div v-if="activeTab === 'dashboards'" class="space-y-5">
      <form class="flex flex-wrap gap-2 rounded-xl border border-slate-800 bg-slate-900/60 p-4" @submit.prevent="addDashboard">
        <input v-model="newDashName" required placeholder="New dashboard name (e.g. Main Boiler)" class="min-w-[240px] flex-1 rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white" />
        <button type="submit" class="inline-flex items-center gap-1 rounded-lg bg-amber-500 px-3.5 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400">
          <Plus class="h-3.5 w-3.5" /> Create Dashboard
        </button>
      </form>

      <div v-if="dashboards.length > 0" class="flex flex-wrap items-center gap-2">
        <div v-for="d in dashboards" :key="d.id" class="inline-flex items-center rounded-lg border" :class="selectedDash?.id === d.id ? 'border-amber-500/50 bg-amber-500/15' : 'border-slate-800 bg-slate-900'">
          <button type="button" class="px-3 py-1.5 text-xs font-medium text-white" @click="selectedDashId = d.id">
            {{ d.name }} ({{ d.widgets.length }})
          </button>
          <button type="button" class="px-2 py-1.5 text-rose-400 hover:text-rose-300" title="Delete dashboard" @click="deleteDashboard(d)">
            <Trash2 class="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      <div v-if="selectedDash" class="space-y-4 rounded-xl border border-slate-800 bg-slate-900/60 p-4">
        <h3 class="text-sm font-semibold text-white">Widgets on "{{ selectedDash.name }}"</h3>
        <form class="grid gap-3 sm:grid-cols-5" @submit.prevent="addWidget">
          <input v-model="widName" required placeholder="Widget title" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white" />
          <select v-model="widType" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
            <option value="display">Display (Value)</option>
            <option value="lamp">Lamp (Status)</option>
            <option value="switch">Switch (Toggle)</option>
            <option value="button">Button (Momentary)</option>
            <option value="number_input">Number Input (Setpoint)</option>
            <option value="chart">Live Chart</option>
          </select>
          <select v-model="widMappingId" class="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white sm:col-span-2">
            <option value="">Select register mapping...</option>
            <option v-for="m in mappings" :key="m.id" :value="m.id">{{ m.name }} ({{ m.accessMode }})</option>
          </select>
          <button type="submit" class="inline-flex items-center justify-center gap-1 rounded-lg bg-amber-500 px-3 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400">
            <Plus class="h-3.5 w-3.5" /> Add Widget
          </button>
        </form>

        <div class="divide-y divide-slate-800/70 rounded-lg border border-slate-800">
          <div v-for="w in selectedDash.widgets" :key="w.id" class="flex items-center justify-between px-3 py-2.5 text-xs">
            <div>
              <span class="font-medium text-white">{{ w.name }}</span>
              <span class="ml-2 rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] text-slate-300">{{ w.widgetType }}</span>
            </div>
            <button type="button" class="rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 hover:bg-rose-500/20" @click="deleteWidget(w.id)">
              <Trash2 class="h-3.5 w-3.5" />
            </button>
          </div>
          <div v-if="selectedDash.widgets.length === 0" class="px-3 py-6 text-center text-xs text-slate-500">
            No widgets added to this dashboard yet.
          </div>
        </div>
      </div>
    </div>

    <!-- TAB 4: HISTORY & MEMBER WRITE POLICIES -->
    <div v-if="activeTab === 'settings'" class="space-y-5">
      <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h3 class="text-sm font-semibold text-white">WebView History Storage</h3>
            <p class="mt-0.5 text-xs text-slate-400">
              Enable archival polling and TimescaleDB history storage for active registers on this site.
            </p>
          </div>
          <label class="inline-flex items-center gap-2 text-xs font-medium text-slate-200">
            <input v-model="historyEnabled" type="checkbox" @change="saveHistorySetting" />
            {{ historyEnabled ? 'Enabled' : 'Disabled' }}
          </label>
        </div>
      </section>

      <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
        <div>
          <h3 class="text-sm font-semibold text-white">Site Member Register Write Policies</h3>
          <p class="mt-0.5 text-xs text-slate-400">
            Site Operators and Platform Admins can always write to writable registers. Grant individual Site Members write permission below.
          </p>
        </div>

        <div class="divide-y divide-slate-800/70 rounded-lg border border-slate-800">
          <div v-for="m in siteMembersOnly" :key="m.userId" class="flex items-center justify-between px-4 py-3 text-xs">
            <div>
              <p class="font-medium text-white">{{ m.email }}</p>
              <p class="text-[11px] text-slate-500">SITE_MEMBER</p>
            </div>
            <label class="inline-flex items-center gap-2 text-xs text-slate-200">
              <input
                type="checkbox"
                :checked="Boolean(policyMap[m.userId])"
                @change="toggleMemberWrite(m, ($event.target as HTMLInputElement).checked)"
              />
              Allow register writes
            </label>
          </div>
          <div v-if="siteMembersOnly.length === 0" class="px-4 py-6 text-center text-xs text-slate-500">
            No Site Members assigned to this site yet. Invite members under Manage Site -> Members.
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
