<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  ArrowDown,
  ArrowUp,
  Bell,
  Cpu,
  Database,
  Edit3,
  LayoutGrid,
  Mail,
  Play,
  Plus,
  ShieldCheck,
  Sliders,
  Trash2,
  X,
} from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useWebviewStore } from '../../stores/webview'
import type { SiteMember } from '../../types/iam'
import type {
  AlarmCondition,
  AlarmRule,
  AlarmSeverity,
  ColorOption,
  DeviceProtocol,
  FontSizeOption,
  HistoryEntry,
  LampMode,
  LampStateConfig,
  MemberWritePolicy,
  ModbusAccessMode,
  ModbusDataType,
  ModbusFunctionCode,
  ModbusWordOrder,
  RegisterMapping,
  TelemetrySample,
  WebviewDashboard,
  WebviewDevice,
  WebviewWidget,
  WidgetType,
} from '../../types/webview'
import { clampPrecision, effectiveDisplayBounds } from '../../utils/precision'
import StatusBadge from '../../components/StatusBadge.vue'
import DeleteConfirmModal from '../../components/DeleteConfirmModal.vue'
import NumericTextInput from '../../components/NumericTextInput.vue'
import RegisterPicker from '../../components/RegisterPicker.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const route = useRoute()
const siteId = computed(() => String(route.params.siteId || ''))
const webviewStore = useWebviewStore()

type TabKey = 'devices' | 'mappings' | 'dashboards' | 'history' | 'alarms' | 'policies'
const activeTab = ref<TabKey>('devices')

const devices = ref<WebviewDevice[]>([])
const mappings = ref<RegisterMapping[]>([])
const dashboards = ref<WebviewDashboard[]>([])
const historyEntries = ref<HistoryEntry[]>([])
const alarmRules = ref<AlarmRule[]>([])
const members = ref<SiteMember[]>([])
const policies = ref<MemberWritePolicy[]>([])
const testResults = ref<Record<string, TelemetrySample>>({})
const testingId = ref<string | null>(null)

const errMsg = ref<string | null>(null)
const okMsg = ref<string | null>(null)

// --- Delete Confirm Modal State (zero window.confirm) ---
const deleteModal = ref<{
  open: boolean
  title: string
  itemName: string
  description: string
  onConfirm: (() => Promise<void>) | null
}>({
  open: false,
  title: '',
  itemName: '',
  description: '',
  onConfirm: null,
})

function askDelete(title: string, itemName: string, description: string, action: () => Promise<void>) {
  deleteModal.value = { open: true, title, itemName, description, onConfirm: action }
}

async function runDeleteConfirm() {
  const fn = deleteModal.value.onConfirm
  deleteModal.value.open = false
  if (fn) await fn()
}

// --- Device Create/Edit Modal ---
const deviceModalOpen = ref(false)
const editingDeviceId = ref<string | null>(null)
const devName = ref('')
const devHost = ref('')
const devPort = ref('502')
const devProto = ref<DeviceProtocol>('MODBUS_TCP')
const devActive = ref(true)

function openCreateDevice() {
  editingDeviceId.value = null
  devName.value = ''
  devHost.value = ''
  devPort.value = '502'
  devProto.value = 'MODBUS_TCP'
  devActive.value = true
  deviceModalOpen.value = true
}

function openEditDevice(d: WebviewDevice) {
  editingDeviceId.value = d.id
  devName.value = d.name
  devHost.value = d.host
  devPort.value = String(d.port)
  devProto.value = d.protocol
  devActive.value = d.isActive
  deviceModalOpen.value = true
}

// --- Register Create/Edit Modal (Dynamic Protocol Attributes) ---
const regModalOpen = ref(false)
const editingRegId = ref<string | null>(null)
const regDeviceId = ref('')
const regName = ref('')
const regActive = ref(true)
// MODBUS_TCP protocol attributes
const regSlave = ref('1')
const regFC = ref<ModbusFunctionCode>('READ_HOLDING')
const regAddr = ref('0')
const regDataType = ref<ModbusDataType>('UINT16')
const regWordOrder = ref<ModbusWordOrder>('BIG')
const regAccess = ref<ModbusAccessMode>('READ')
const regUnit = ref('')
const regPrecision = ref('')

const selectedRegDevice = computed(() => devices.value.find((d) => d.id === regDeviceId.value) ?? null)

function openCreateRegister() {
  editingRegId.value = null
  regDeviceId.value = devices.value[0]?.id || ''
  regName.value = ''
  regActive.value = true
  regSlave.value = '1'
  regFC.value = 'READ_HOLDING'
  regAddr.value = '0'
  regDataType.value = 'UINT16'
  regWordOrder.value = 'BIG'
  regAccess.value = 'READ'
  regUnit.value = ''
  regPrecision.value = ''
  regModalOpen.value = true
}

function openEditRegister(m: RegisterMapping) {
  editingRegId.value = m.id
  regDeviceId.value = m.deviceId
  regName.value = m.name
  regActive.value = m.isActive
  const a = m.protocolAttributes
  regSlave.value = String(a?.slaveAddress ?? 1)
  regFC.value = (a?.functionCode as ModbusFunctionCode) || 'READ_HOLDING'
  regAddr.value = String(a?.registerAddress ?? 0)
  regDataType.value = (a?.dataType as ModbusDataType) || 'UINT16'
  regWordOrder.value = (a?.wordOrder as ModbusWordOrder) || 'BIG'
  regAccess.value = (a?.accessMode as ModbusAccessMode) || 'READ'
  regUnit.value = typeof a?.unit === 'string' ? a.unit : ''
  regPrecision.value = a?.precision !== undefined && a?.precision !== null ? String(clampPrecision(a.precision)) : ''
  regModalOpen.value = true
}

// --- Dashboard Create/Edit Modal ---
const dashModalOpen = ref(false)
const editingDashId = ref<string | null>(null)
const dashName = ref('')
const dashIsDefault = ref(false)
const dashPollIntervalMs = ref('1000')
const selectedDashId = ref('')

function openCreateDashboard() {
  editingDashId.value = null
  dashName.value = ''
  dashIsDefault.value = dashboards.value.length === 0
  dashPollIntervalMs.value = '1000'
  dashModalOpen.value = true
}

function openEditDashboard(d: WebviewDashboard) {
  editingDashId.value = d.id
  dashName.value = d.name
  dashIsDefault.value = d.isDefault
  dashPollIntervalMs.value = String(d.pollIntervalMs || 1000)
  dashModalOpen.value = true
}

// --- Widget Create/Edit Modal (Card-type-specific parameters) ---
const widgetModalOpen = ref(false)
const editingWidgetId = ref<string | null>(null)
const widName = ref('')
const widType = ref<WidgetType>('display')
const widMappingId = ref('')
// Display & Number Input params
const widFontSize = ref<FontSizeOption>('lg')
const widPrefix = ref('')
const widUnit = ref('')
const widPrecision = ref('0')
const widMin = ref<string>('')
const widMax = ref<string>('')
// Lamp params (for both states)
const widLampOn = ref<LampStateConfig>({ mode: 'on', color: 'emerald', text: 'ON' })
const widLampOff = ref<LampStateConfig>({ mode: 'off', color: 'slate', text: 'OFF' })
// Switch params
const widSwitchColor = ref<ColorOption>('emerald')
const widSwitchOnLabel = ref('ON')
const widSwitchOffLabel = ref('OFF')
// Button params
const widBtnColor = ref<ColorOption>('sky')
const widBtnLabel = ref('Trigger')
const widBtnTriggerMs = ref('1000')

const widBoundMapping = computed(() => mappings.value.find((m) => m.id === widMappingId.value) ?? null)
const widEffectivePrecision = computed(() => {
  if (widPrecision.value.trim() !== '') return clampPrecision(widPrecision.value)
  return clampPrecision(widBoundMapping.value?.protocolAttributes?.precision ?? 0)
})
const widBoundTypeBounds = computed(() => {
  const dt = widBoundMapping.value?.protocolAttributes?.dataType
  return effectiveDisplayBounds(dt, widEffectivePrecision.value)
})

function openCreateWidget() {
  editingWidgetId.value = null
  widName.value = ''
  widType.value = 'display'
  widMappingId.value = mappings.value[0]?.id || ''
  widFontSize.value = 'lg'
  widPrefix.value = ''
  widUnit.value = ''
  widPrecision.value = ''
  widMin.value = ''
  widMax.value = ''
  widLampOn.value = { mode: 'on', color: 'emerald', text: 'ON' }
  widLampOff.value = { mode: 'off', color: 'slate', text: 'OFF' }
  widSwitchColor.value = 'emerald'
  widSwitchOnLabel.value = 'ON'
  widSwitchOffLabel.value = 'OFF'
  widBtnColor.value = 'sky'
  widBtnLabel.value = 'Trigger'
  widBtnTriggerMs.value = '1000'
  widgetModalOpen.value = true
}

function openEditWidget(w: WebviewWidget) {
  editingWidgetId.value = w.id
  widName.value = w.name
  widType.value = w.widgetType
  widMappingId.value = w.registerMappingId || ''
  widFontSize.value = (w.style?.fontSize as FontSizeOption) || 'lg'
  widPrefix.value = String(w.extra?.prefix ?? '')
  widUnit.value = w.extra?.unit !== undefined && w.extra?.unit !== null ? String(w.extra.unit) : ''
  widPrecision.value =
    w.extra?.precision !== undefined && w.extra?.precision !== null && w.extra?.precision !== ''
      ? String(clampPrecision(w.extra.precision))
      : ''
  widMin.value = w.extra?.min !== undefined && w.extra?.min !== null ? String(w.extra.min) : ''
  widMax.value = w.extra?.max !== undefined && w.extra?.max !== null ? String(w.extra.max) : ''

  const onCfg = (w.extra?.on as Partial<LampStateConfig> | undefined) || {}
  const offCfg = (w.extra?.off as Partial<LampStateConfig> | undefined) || {}
  widLampOn.value = {
    mode: (onCfg.mode as LampMode) || 'on',
    color: (onCfg.color as ColorOption) || 'emerald',
    text: onCfg.text ?? 'ON',
  }
  widLampOff.value = {
    mode: (offCfg.mode as LampMode) || 'off',
    color: (offCfg.color as ColorOption) || 'slate',
    text: offCfg.text ?? 'OFF',
  }

  widSwitchColor.value = (w.style?.color as ColorOption) || 'emerald'
  widSwitchOnLabel.value = String(w.extra?.onLabel ?? 'ON')
  widSwitchOffLabel.value = String(w.extra?.offLabel ?? 'OFF')

  widBtnColor.value = (w.style?.color as ColorOption) || 'sky'
  widBtnLabel.value = String(w.extra?.label ?? w.name)
  widBtnTriggerMs.value = String(Number(w.extra?.triggerTimeMs ?? 1000))
  widgetModalOpen.value = true
}

// --- History Entry Create / Edit Modal ---
const histModalOpen = ref(false)
const editingHistId = ref<string | null>(null)
const histMappingId = ref('')
const histPollSec = ref(60)
const histRetentionDays = ref(30)
const histActive = ref(true)

const POLL_INTERVAL_LABELS: Record<number, string> = {
  10: '10s',
  30: '30s',
  60: '1 min',
  300: '5 min',
  900: '15 min',
  3600: '1 hour',
}

const RETENTION_LABELS: Record<number, string> = {
  1: '1 day',
  7: '7 days',
  30: '30 days',
  90: '90 days',
  365: '1 year',
}

function formatPollInterval(sec: number): string {
  return POLL_INTERVAL_LABELS[sec] || `${sec}s`
}

function formatRetention(days: number): string {
  return RETENTION_LABELS[days] || `${days} days`
}

function openCreateHistoryEntry() {
  editingHistId.value = null
  histMappingId.value = availableHistoryMappings.value[0]?.id || ''
  histPollSec.value = 60
  histRetentionDays.value = 30
  histActive.value = true
  histModalOpen.value = true
}

function openEditHistoryEntry(he: HistoryEntry) {
  editingHistId.value = he.id
  histMappingId.value = he.registerMappingId
  histPollSec.value = he.pollIntervalSeconds
  histRetentionDays.value = he.retentionDays
  histActive.value = he.isActive
  histModalOpen.value = true
}

// --- Alarm Rule Create / Edit Modal ---
const alarmModalOpen = ref(false)
const editingAlarmId = ref<string | null>(null)
const alarmName = ref('')
const alarmMappingId = ref('')
const alarmCondition = ref<AlarmCondition>('GT')
const alarmThreshold = ref('0')
const alarmDeadband = ref('0')
const alarmHoldSec = ref('0')
const alarmPollSec = ref('10')
const alarmSeverity = ref<AlarmSeverity>('WARNING')
const alarmEmails = ref<string[]>([])
const alarmEmailDraft = ref('')
const alarmEmailErr = ref<string | null>(null)
const alarmNotifyOnClear = ref(false)
const alarmActive = ref(true)

const alarmBoundMapping = computed(() => (alarmMappingId.value ? mappingMap.value[alarmMappingId.value] ?? null : null))
const alarmNeedsDeadband = computed(() => alarmCondition.value !== 'EQ' && alarmCondition.value !== 'NEQ')

function openCreateAlarmRule() {
  editingAlarmId.value = null
  alarmName.value = ''
  alarmMappingId.value = mappings.value[0]?.id || ''
  alarmCondition.value = 'GT'
  alarmThreshold.value = '0'
  alarmDeadband.value = '0'
  alarmHoldSec.value = '0'
  alarmPollSec.value = '10'
  alarmSeverity.value = 'WARNING'
  alarmEmails.value = []
  alarmEmailDraft.value = ''
  alarmEmailErr.value = null
  alarmNotifyOnClear.value = false
  alarmActive.value = true
  alarmModalOpen.value = true
}

function openEditAlarmRule(r: AlarmRule) {
  editingAlarmId.value = r.id
  alarmName.value = r.name
  alarmMappingId.value = r.registerMappingId
  alarmCondition.value = r.condition
  alarmThreshold.value = String(r.threshold)
  alarmDeadband.value = String(r.clearDeadband)
  alarmHoldSec.value = String(r.holdSeconds)
  alarmPollSec.value = String(r.pollIntervalSeconds)
  alarmSeverity.value = r.severity
  alarmEmails.value = [...(r.notifyEmails || [])]
  alarmEmailDraft.value = ''
  alarmEmailErr.value = null
  alarmNotifyOnClear.value = r.notifyOnClear
  alarmActive.value = r.isActive
  alarmModalOpen.value = true
}

const simpleEmailRe = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

function addAlarmEmailsFromDraft() {
  alarmEmailErr.value = null
  const raw = alarmEmailDraft.value.trim()
  if (!raw) return
  const parts = raw.split(/[\s,;]+/).filter(Boolean)
  const invalid: string[] = []
  const next = [...alarmEmails.value]
  for (const part of parts) {
    const norm = part.toLowerCase()
    if (!simpleEmailRe.test(norm)) {
      invalid.push(part)
      continue
    }
    if (!next.includes(norm) && next.length < 50) {
      next.push(norm)
    }
  }
  alarmEmails.value = next
  if (invalid.length > 0) {
    alarmEmailErr.value = `Invalid email address: ${invalid.join(', ')}`
    alarmEmailDraft.value = invalid.join(', ')
  } else {
    alarmEmailDraft.value = ''
  }
}

function addMemberEmailQuick(email: string) {
  const norm = email.trim().toLowerCase()
  if (!norm || alarmEmails.value.includes(norm) || alarmEmails.value.length >= 50) return
  alarmEmails.value = [...alarmEmails.value, norm]
}

function removeAlarmEmail(email: string) {
  alarmEmails.value = alarmEmails.value.filter((e) => e !== email)
}

function alarmConditionSymbol(c: AlarmCondition): string {
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

function alarmSeverityBadgeClass(sev: AlarmSeverity): string {
  switch (sev) {
    case 'CRITICAL':
      return 'border-rose-500/40 bg-rose-500/15 text-rose-300'
    case 'WARNING':
      return 'border-amber-500/40 bg-amber-500/15 text-amber-300'
    default:
      return 'border-sky-500/40 bg-sky-500/15 text-sky-300'
  }
}

function alarmRuntimeBadgeClass(r: AlarmRule): string {
  if (!r.isActive) return 'border-slate-700 bg-slate-800/80 text-slate-400'
  switch (r.runtimeStatus) {
    case 'ACTIVE':
      return 'border-rose-500/40 bg-rose-500/20 text-rose-200'
    case 'PENDING':
      return 'border-amber-500/40 bg-amber-500/20 text-amber-200'
    default:
      return 'border-emerald-500/30 bg-emerald-500/15 text-emerald-300'
  }
}

const selectedDash = computed(() => dashboards.value.find((d) => d.id === selectedDashId.value) ?? dashboards.value[0] ?? null)
const deviceMap = computed<Record<string, WebviewDevice>>(() => {
  const m: Record<string, WebviewDevice> = {}
  for (const d of devices.value) m[d.id] = d
  return m
})
const mappingMap = computed<Record<string, RegisterMapping>>(() => {
  const m: Record<string, RegisterMapping> = {}
  for (const r of mappings.value) m[r.id] = r
  return m
})
const policyMap = computed<Record<string, boolean>>(() => {
  const m: Record<string, boolean> = {}
  for (const p of policies.value) m[p.userId] = p.canWrite
  return m
})
const siteMembersOnly = computed(() => members.value.filter((m) => m.role === 'SITE_MEMBER'))
const availableHistoryMappings = computed(() => {
  const used = new Set(historyEntries.value.map((e) => e.registerMappingId))
  return mappings.value.filter((m) => !used.has(m.id))
})

// Pagination for every list/table (25, 50, 100)
const devicesPag = usePagination(devices)
const mappingsPag = usePagination(mappings)
const dashboardsPag = usePagination(dashboards)
const widgetsPag = usePagination(() => selectedDash.value?.widgets ?? [])
const historyPag = usePagination(historyEntries)
const alarmsPag = usePagination(alarmRules)
const policiesPag = usePagination(siteMembersOnly)

watch(selectedDashId, () => {
  widgetsPag.resetPage()
})

function registerKeyLabel(m: RegisterMapping): string {
  const dev = deviceMap.value[m.deviceId]
  const a = m.protocolAttributes
  const devPart = dev ? `${dev.name} (${dev.protocol})` : 'Device'
  return `${devPart} / ${m.name} [unit=${a?.slaveAddress ?? 1} ${a?.functionCode ?? ''}@${a?.registerAddress ?? 0}]`
}

async function loadAll() {
  errMsg.value = null
  try {
    const [dRes, mRes, dbRes, hRes, aRes, pRes, memRes] = await Promise.all([
      apiRequest<{ devices: WebviewDevice[] }>(`/api/v1/webview/sites/${siteId.value}/operator/devices`),
      apiRequest<{ mappings: RegisterMapping[] }>(`/api/v1/webview/sites/${siteId.value}/operator/mappings`),
      apiRequest<{ dashboards: WebviewDashboard[] }>(`/api/v1/webview/sites/${siteId.value}/operator/dashboards`),
      apiRequest<{ entries: HistoryEntry[] }>(`/api/v1/webview/sites/${siteId.value}/operator/history-entries`),
      apiRequest<{ rules: AlarmRule[] }>(`/api/v1/webview/sites/${siteId.value}/operator/alarm-rules`),
      apiRequest<{ policies: MemberWritePolicy[] }>(`/api/v1/webview/sites/${siteId.value}/operator/member-policies`),
      apiRequest<{ members: SiteMember[] }>(`/api/v1/iam/sites/${siteId.value}/operator/members`).catch(() => ({ members: [] })),
    ])
    devices.value = dRes.devices || []
    mappings.value = mRes.mappings || []
    dashboards.value = dbRes.dashboards || []
    historyEntries.value = hRes.entries || []
    alarmRules.value = aRes.rules || []
    policies.value = pRes.policies || []
    members.value = memRes.members || []
    webviewStore.setSiteDashboards(siteId.value, dashboards.value)

    if (dashboards.value.length > 0 && !dashboards.value.some((d) => d.id === selectedDashId.value)) {
      selectedDashId.value = dashboards.value[0].id
    }
    if (!histMappingId.value && availableHistoryMappings.value.length > 0) {
      histMappingId.value = availableHistoryMappings.value[0].id
    }
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to load WebView configuration'
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  if (alarmModalOpen.value) alarmModalOpen.value = false
  else if (histModalOpen.value) histModalOpen.value = false
  else if (widgetModalOpen.value) widgetModalOpen.value = false
  else if (dashModalOpen.value) dashModalOpen.value = false
  else if (regModalOpen.value) regModalOpen.value = false
  else if (deviceModalOpen.value) deviceModalOpen.value = false
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  void loadAll()
})
onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
})
watch(siteId, loadAll)

// --- Device Actions ---
async function saveDevice() {
  errMsg.value = null
  okMsg.value = null
  try {
    if (editingDeviceId.value) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/devices/${editingDeviceId.value}`, {
        method: 'PATCH',
        body: JSON.stringify({
          name: devName.value,
          host: devHost.value,
          port: Number(devPort.value),
          protocol: devProto.value,
          isActive: devActive.value,
        }),
      })
      okMsg.value = `Updated device "${devName.value}".`
    } else {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/devices`, {
        method: 'POST',
        body: JSON.stringify({
          name: devName.value,
          host: devHost.value,
          port: Number(devPort.value),
          protocol: devProto.value,
        }),
      })
      okMsg.value = `Added device "${devName.value}".`
    }
    deviceModalOpen.value = false
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to save device'
  }
}

function confirmDeleteDevice(d: WebviewDevice) {
  askDelete('Delete Device', d.name, 'All registers belonging to this device will also be deleted.', async () => {
    errMsg.value = null
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/devices/${d.id}`, { method: 'DELETE' })
      await loadAll()
    } catch (err) {
      errMsg.value = err instanceof Error ? err.message : 'Failed to delete device'
    }
  })
}

// --- Register Actions ---
async function saveRegister() {
  errMsg.value = null
  okMsg.value = null
  const attrs: Record<string, unknown> = {
    slaveAddress: Number(regSlave.value),
    functionCode: regFC.value,
    registerAddress: Number(regAddr.value),
    dataType: regFC.value === 'READ_COIL' || regFC.value === 'READ_DISCRETE' ? 'BOOL' : regDataType.value,
    wordOrder: regWordOrder.value,
    accessMode: regFC.value === 'READ_DISCRETE' || regFC.value === 'READ_INPUT' ? 'READ' : regAccess.value,
  }
  if (regUnit.value.trim() !== '') {
    attrs.unit = regUnit.value.trim()
  }
  if (regPrecision.value.trim() !== '') {
    attrs.precision = clampPrecision(regPrecision.value)
  }
  try {
    if (editingRegId.value) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/mappings/${editingRegId.value}`, {
        method: 'PATCH',
        body: JSON.stringify({
          deviceId: regDeviceId.value,
          name: regName.value,
          protocolAttributes: attrs,
          isActive: regActive.value,
        }),
      })
      okMsg.value = `Updated register "${regName.value}".`
    } else {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/mappings`, {
        method: 'POST',
        body: JSON.stringify({
          deviceId: regDeviceId.value,
          name: regName.value,
          protocolAttributes: attrs,
        }),
      })
      okMsg.value = `Added register "${regName.value}".`
    }
    regModalOpen.value = false
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to save register'
  }
}

function confirmDeleteRegister(m: RegisterMapping) {
  askDelete('Delete Register', m.name, 'Widgets referencing this register will become unbound.', async () => {
    errMsg.value = null
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/mappings/${m.id}`, { method: 'DELETE' })
      await loadAll()
    } catch (err) {
      errMsg.value = err instanceof Error ? err.message : 'Failed to delete register'
    }
  })
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

// --- Dashboard Actions + Reorder ---
async function saveDashboard() {
  errMsg.value = null
  okMsg.value = null
  const pollMs = Math.round(Number(dashPollIntervalMs.value))
  if (!Number.isFinite(pollMs) || pollMs < 1000 || pollMs > 60000) {
    errMsg.value = 'Dashboard poll interval must be between 1000 and 60000 ms.'
    return
  }
  try {
    if (editingDashId.value) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${editingDashId.value}`, {
        method: 'PATCH',
        body: JSON.stringify({ name: dashName.value, isDefault: dashIsDefault.value, pollIntervalMs: pollMs }),
      })
      okMsg.value = `Updated dashboard "${dashName.value}".`
    } else {
      const created = await apiRequest<WebviewDashboard>(`/api/v1/webview/sites/${siteId.value}/operator/dashboards`, {
        method: 'POST',
        body: JSON.stringify({ name: dashName.value, isDefault: dashIsDefault.value, pollIntervalMs: pollMs }),
      })
      selectedDashId.value = created.id
      okMsg.value = `Created dashboard "${dashName.value}".`
    }
    dashModalOpen.value = false
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to save dashboard'
  }
}

async function moveDashboard(idx: number, delta: number) {
  const target = idx + delta
  if (target < 0 || target >= dashboards.value.length) return
  const reordered = [...dashboards.value]
  const [moved] = reordered.splice(idx, 1)
  reordered.splice(target, 0, moved)
  dashboards.value = reordered
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/reorder`, {
      method: 'POST',
      body: JSON.stringify({ dashboardIds: reordered.map((d) => d.id) }),
    })
    webviewStore.setSiteDashboards(siteId.value, reordered)
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to reorder dashboards'
    await loadAll()
  }
}

function confirmDeleteDashboard(d: WebviewDashboard) {
  askDelete('Delete Dashboard', d.name, 'All widgets on this dashboard will be permanently removed.', async () => {
    errMsg.value = null
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${d.id}`, { method: 'DELETE' })
      selectedDashId.value = ''
      await loadAll()
    } catch (err) {
      errMsg.value = err instanceof Error ? err.message : 'Failed to delete dashboard'
    }
  })
}

// --- Widget Actions + Reorder ---
async function saveWidget() {
  if (!selectedDash.value) return
  errMsg.value = null
  okMsg.value = null

  const style: Record<string, unknown> = {}
  const extra: Record<string, unknown> = {}

  if (widType.value === 'display') {
    style.fontSize = widFontSize.value
    extra.prefix = widPrefix.value
    if (widUnit.value.trim() !== '') {
      extra.unit = widUnit.value.trim()
    }
    if (widPrecision.value.trim() !== '') {
      extra.precision = clampPrecision(widPrecision.value)
    }
  } else if (widType.value === 'number_input') {
    style.fontSize = widFontSize.value
    extra.prefix = widPrefix.value
    if (widUnit.value.trim() !== '') {
      extra.unit = widUnit.value.trim()
    }
    if (widPrecision.value.trim() !== '') {
      extra.precision = clampPrecision(widPrecision.value)
    }
    if (widMin.value.trim() !== '' && Number.isFinite(Number(widMin.value))) {
      extra.min = Number(widMin.value)
    }
    if (widMax.value.trim() !== '' && Number.isFinite(Number(widMax.value))) {
      extra.max = Number(widMax.value)
    }
    if (extra.min !== undefined && extra.max !== undefined && Number(extra.min) > Number(extra.max)) {
      errMsg.value = 'Minimum value cannot exceed maximum value.'
      return
    }
  } else if (widType.value === 'lamp') {
    extra.on = { ...widLampOn.value }
    extra.off = { ...widLampOff.value }
  } else if (widType.value === 'switch') {
    style.color = widSwitchColor.value
    extra.onLabel = widSwitchOnLabel.value || 'ON'
    extra.offLabel = widSwitchOffLabel.value || 'OFF'
  } else if (widType.value === 'button') {
    style.color = widBtnColor.value
    extra.label = widBtnLabel.value || widName.value
    extra.triggerTimeMs = Math.max(100, Math.min(60000, Math.round(Number(widBtnTriggerMs.value) || 1000)))
  }

  try {
    if (editingWidgetId.value) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/widgets/${editingWidgetId.value}`, {
        method: 'PATCH',
        body: JSON.stringify({
          name: widName.value,
          widgetType: widType.value,
          registerMappingId: widMappingId.value || undefined,
          style,
          extra,
        }),
      })
      okMsg.value = `Updated widget "${widName.value}".`
    } else {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${selectedDash.value.id}/widgets`, {
        method: 'POST',
        body: JSON.stringify({
          name: widName.value,
          widgetType: widType.value,
          registerMappingId: widMappingId.value || undefined,
          sortOrder: selectedDash.value.widgets.length,
          style,
          extra,
        }),
      })
      okMsg.value = `Added widget "${widName.value}".`
    }
    widgetModalOpen.value = false
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to save widget'
  }
}

async function moveWidget(idx: number, delta: number) {
  if (!selectedDash.value) return
  const list = selectedDash.value.widgets
  const target = idx + delta
  if (target < 0 || target >= list.length) return
  const reordered = [...list]
  const [moved] = reordered.splice(idx, 1)
  reordered.splice(target, 0, moved)
  selectedDash.value.widgets = reordered
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${selectedDash.value.id}/widgets/reorder`, {
      method: 'POST',
      body: JSON.stringify({ widgetIds: reordered.map((w) => w.id) }),
    })
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to reorder widgets'
    await loadAll()
  }
}

function confirmDeleteWidget(w: WebviewWidget) {
  askDelete('Delete Widget', w.name, 'This widget will be removed from the dashboard.', async () => {
    errMsg.value = null
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/widgets/${w.id}`, { method: 'DELETE' })
      await loadAll()
    } catch (err) {
      errMsg.value = err instanceof Error ? err.message : 'Failed to delete widget'
    }
  })
}

// --- History Entry Actions ---
async function saveHistoryEntry() {
  errMsg.value = null
  okMsg.value = null
  try {
    if (editingHistId.value) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/history-entries/${editingHistId.value}`, {
        method: 'PATCH',
        body: JSON.stringify({
          pollIntervalSeconds: Number(histPollSec.value),
          retentionDays: Number(histRetentionDays.value),
          isActive: histActive.value,
        }),
      })
      okMsg.value = 'Updated history archival key.'
    } else {
      if (!histMappingId.value) return
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/history-entries`, {
        method: 'POST',
        body: JSON.stringify({
          registerMappingId: histMappingId.value,
          pollIntervalSeconds: Number(histPollSec.value),
          retentionDays: Number(histRetentionDays.value),
        }),
      })
      okMsg.value = 'Added register to history archival.'
    }
    histModalOpen.value = false
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to save history entry'
  }
}

function confirmDeleteHistoryEntry(he: HistoryEntry) {
  const m = mappingMap.value[he.registerMappingId]
  askDelete('Remove History Entry', m ? registerKeyLabel(m) : he.registerMappingId, 'Scheduled background polling for this register will stop unless a dashboard is open.', async () => {
    errMsg.value = null
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/history-entries/${he.id}`, { method: 'DELETE' })
      await loadAll()
    } catch (err) {
      errMsg.value = err instanceof Error ? err.message : 'Failed to remove history entry'
    }
  })
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

// --- Alarm Rule Actions ---
async function saveAlarmRule() {
  errMsg.value = null
  okMsg.value = null
  if (alarmEmailDraft.value.trim()) {
    addAlarmEmailsFromDraft()
    if (alarmEmailErr.value) return
  }
  const threshold = Number(alarmThreshold.value)
  if (!Number.isFinite(threshold)) {
    errMsg.value = 'Threshold must be a valid number.'
    return
  }
  const deadband = alarmNeedsDeadband.value ? Number(alarmDeadband.value || '0') : 0
  if (!Number.isFinite(deadband) || deadband < 0) {
    errMsg.value = 'Clear deadband must be 0 or greater.'
    return
  }
  const holdSec = Math.round(Number(alarmHoldSec.value || '0'))
  if (!Number.isFinite(holdSec) || holdSec < 0 || holdSec > 86400) {
    errMsg.value = 'Hold time must be between 0 and 86400 seconds.'
    return
  }
  const pollSec = Math.round(Number(alarmPollSec.value || '10'))
  if (!Number.isFinite(pollSec) || pollSec < 1 || pollSec > 3600) {
    errMsg.value = 'Poll interval must be between 1 and 3600 seconds.'
    return
  }
  if (!alarmMappingId.value) {
    errMsg.value = 'Please select a register.'
    return
  }

  const payload = {
    registerMappingId: alarmMappingId.value,
    name: alarmName.value.trim(),
    condition: alarmCondition.value,
    threshold,
    clearDeadband: deadband,
    holdSeconds: holdSec,
    pollIntervalSeconds: pollSec,
    severity: alarmSeverity.value,
    notifyEmails: alarmEmails.value,
    notifyOnClear: alarmNotifyOnClear.value,
    isActive: alarmActive.value,
  }

  try {
    if (editingAlarmId.value) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/alarm-rules/${editingAlarmId.value}`, {
        method: 'PATCH',
        body: JSON.stringify(payload),
      })
      okMsg.value = `Updated alarm rule "${payload.name}".`
    } else {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/alarm-rules`, {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      okMsg.value = `Created alarm rule "${payload.name}".`
    }
    alarmModalOpen.value = false
    await loadAll()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to save alarm rule'
  }
}

function confirmDeleteAlarmRule(r: AlarmRule) {
  askDelete(
    'Delete Alarm Rule',
    r.name,
    'Any active latched alarm for this rule will be cleared and background polling for this rule will stop.',
    async () => {
      errMsg.value = null
      try {
        await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/alarm-rules/${r.id}`, { method: 'DELETE' })
        okMsg.value = `Deleted alarm rule "${r.name}".`
        await loadAll()
      } catch (err) {
        errMsg.value = err instanceof Error ? err.message : 'Failed to delete alarm rule'
      }
    },
  )
}
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <div>
      <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">WebView Configuration</h1>
      <p class="mt-0.5 text-xs text-slate-400">
        Manage devices, protocol-defined registers, dashboards, widgets, scheduled history keys, alarm rules, and site member write policies.
      </p>
    </div>

    <!-- Tabs (horizontally scrollable strip on mobile, wrapped on sm+) -->
    <div class="no-scrollbar -mx-4 flex items-center gap-1.5 overflow-x-auto border-b border-slate-800 px-4 pb-3 sm:mx-0 sm:flex-wrap sm:gap-2 sm:px-0">
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition sm:h-8"
        :class="activeTab === 'devices' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'devices'"
      >
        <Cpu class="h-3.5 w-3.5" /> Devices ({{ devices.length }})
      </button>
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition sm:h-8"
        :class="activeTab === 'mappings' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'mappings'"
      >
        <Sliders class="h-3.5 w-3.5" /> Registers ({{ mappings.length }})
      </button>
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition sm:h-8"
        :class="activeTab === 'dashboards' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'dashboards'"
      >
        <LayoutGrid class="h-3.5 w-3.5" /> Dashboards & Widgets ({{ dashboards.length }})
      </button>
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition sm:h-8"
        :class="activeTab === 'history' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'history'"
      >
        <Database class="h-3.5 w-3.5" /> History ({{ historyEntries.length }})
      </button>
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition sm:h-8"
        :class="activeTab === 'alarms' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'alarms'"
      >
        <Bell class="h-3.5 w-3.5" /> Alarms ({{ alarmRules.length }})
      </button>
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition sm:h-8"
        :class="activeTab === 'policies' ? 'bg-amber-500/20 text-amber-200' : 'text-slate-400 hover:bg-slate-800 hover:text-white'"
        @click="activeTab = 'policies'"
      >
        <ShieldCheck class="h-3.5 w-3.5" /> Member Policies ({{ siteMembersOnly.length }})
      </button>
    </div>

    <p v-if="errMsg" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-xs text-rose-200">{{ errMsg }}</p>
    <p v-if="okMsg" class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-200">{{ okMsg }}</p>

    <!-- TAB 1: DEVICES -->
    <div v-if="activeTab === 'devices'" class="space-y-4">
      <div class="flex justify-end">
        <button
          type="button"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-amber-500 px-3.5 text-xs font-semibold text-slate-950 transition hover:bg-amber-400"
          @click="openCreateDevice"
        >
          <Plus class="h-4 w-4" /> Add Device
        </button>
      </div>

      <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
        <!-- Mobile Cards -->
        <div class="divide-y divide-slate-800/70 md:hidden">
          <div v-for="d in devicesPag.paginatedItems.value" :key="d.id" class="space-y-2.5 p-4 text-xs">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <p class="truncate text-sm font-semibold text-white">{{ d.name }}</p>
                <p class="mt-0.5 font-mono text-slate-300">{{ d.host }}:{{ d.port }}</p>
              </div>
              <StatusBadge :status="d.isActive ? 'ACTIVE' : 'DISABLED'" size="sm" />
            </div>
            <p class="font-mono text-[11px] text-slate-400">{{ d.protocol }}</p>
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-lg border border-slate-700 bg-slate-800 px-2.5 text-[11px] font-medium text-slate-200 hover:bg-slate-700"
                @click="openEditDevice(d)"
              >
                <Edit3 class="h-3 w-3" /> Edit
              </button>
              <button
                type="button"
                aria-label="Delete device"
                class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
                @click="confirmDeleteDevice(d)"
              >
                <Trash2 class="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
          <div v-if="devices.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">
            No devices configured yet.
          </div>
        </div>

        <!-- Desktop Table -->
        <div class="hidden overflow-x-auto md:block">
          <table class="w-full text-left text-xs">
            <thead class="border-b border-slate-800 text-slate-400">
              <tr>
                <th class="px-4 py-3">Name</th>
                <th class="px-4 py-3">Endpoint</th>
                <th class="px-4 py-3">Protocol</th>
                <th class="px-4 py-3">Status</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/70">
              <tr v-for="d in devicesPag.paginatedItems.value" :key="d.id" class="hover:bg-slate-800/30">
                <td class="px-4 py-3 font-medium text-white">{{ d.name }}</td>
                <td class="px-4 py-3 font-mono text-slate-300">{{ d.host }}:{{ d.port }}</td>
                <td class="px-4 py-3 font-mono text-slate-400">{{ d.protocol }}</td>
                <td class="px-4 py-3"><StatusBadge :status="d.isActive ? 'ACTIVE' : 'DISABLED'" size="sm" /></td>
                <td class="px-4 py-3 text-right">
                  <div class="inline-flex items-center justify-end gap-1.5">
                    <button
                      type="button"
                      class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 transition hover:bg-slate-700"
                      @click="openEditDevice(d)"
                    >
                      <Edit3 class="h-3 w-3" /> Edit
                    </button>
                    <button
                      type="button"
                      aria-label="Delete device"
                      class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 transition hover:bg-rose-500/20"
                      @click="confirmDeleteDevice(d)"
                    >
                      <Trash2 class="h-3.5 w-3.5" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="devices.length === 0">
                <td colspan="5" class="px-4 py-8 text-center text-slate-500">No devices configured yet.</td>
              </tr>
            </tbody>
          </table>
        </div>
        <TablePagination
          v-model:page="devicesPag.page.value"
          v-model:page-size="devicesPag.pageSize.value"
          :total-items="devicesPag.totalItems.value"
        />
      </div>
    </div>

    <!-- TAB 2: REGISTERS -->
    <div v-if="activeTab === 'mappings'" class="space-y-4">
      <div class="flex justify-end">
        <button
          type="button"
          :disabled="devices.length === 0"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-amber-500 px-3.5 text-xs font-semibold text-slate-950 transition hover:bg-amber-400 disabled:opacity-40"
          @click="openCreateRegister"
        >
          <Plus class="h-4 w-4" /> Add Register
        </button>
      </div>

      <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
        <!-- Mobile Cards -->
        <div class="divide-y divide-slate-800/70 md:hidden">
          <div v-for="m in mappingsPag.paginatedItems.value" :key="m.id" class="space-y-2.5 p-4 text-xs">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <p class="truncate text-sm font-semibold text-white">{{ m.name }}</p>
                <p class="text-[11px] text-slate-400">{{ deviceMap[m.deviceId]?.name || m.deviceId }}</p>
              </div>
              <span class="shrink-0 rounded bg-slate-800 px-2 py-0.5 font-mono text-[10px] text-slate-300">
                {{ m.protocolAttributes?.accessMode === 'READ_WRITE' ? 'R/W' : 'R' }}
              </span>
            </div>

            <div class="rounded-lg border border-slate-800/80 bg-slate-950/60 p-2.5 font-mono text-[11px] text-slate-300 space-y-1">
              <p class="break-words">
                unit {{ m.protocolAttributes?.slaveAddress ?? 1 }} |
                {{ m.protocolAttributes?.functionCode }} @ {{ m.protocolAttributes?.registerAddress }}
                [{{ m.protocolAttributes?.dataType }}/{{ m.protocolAttributes?.wordOrder || 'BIG' }}]
              </p>
              <p v-if="m.protocolAttributes?.unit || m.protocolAttributes?.precision !== undefined" class="text-sky-300">
                {{ m.protocolAttributes?.unit || '' }}{{ m.protocolAttributes?.unit && m.protocolAttributes?.precision !== undefined ? ' ' : '' }}{{ m.protocolAttributes?.precision !== undefined ? `10^${-m.protocolAttributes.precision}` : '' }}
              </p>
              <p v-if="testResults[m.id]" class="pt-0.5">
                <span v-if="testResults[m.id]?.quality === 'GOOD'" class="text-emerald-300">Test: {{ testResults[m.id].value }}</span>
                <span v-else class="break-words text-rose-300">BAD: {{ testResults[m.id].error }}</span>
              </p>
            </div>

            <div class="flex items-center gap-2">
              <button
                type="button"
                :disabled="testingId === m.id"
                class="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-lg border border-sky-500/40 bg-sky-500/15 px-2.5 text-[11px] font-medium text-sky-200 hover:bg-sky-500/25 disabled:opacity-40"
                @click="testRead(m)"
              >
                <Play class="h-3 w-3" /> {{ testingId === m.id ? 'Reading...' : 'Test' }}
              </button>
              <button
                type="button"
                class="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-lg border border-slate-700 bg-slate-800 px-2.5 text-[11px] font-medium text-slate-200 hover:bg-slate-700"
                @click="openEditRegister(m)"
              >
                <Edit3 class="h-3 w-3" /> Edit
              </button>
              <button
                type="button"
                aria-label="Delete register"
                class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
                @click="confirmDeleteRegister(m)"
              >
                <Trash2 class="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
          <div v-if="mappings.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">
            No registers configured yet.
          </div>
        </div>

        <!-- Desktop Table -->
        <div class="hidden overflow-x-auto md:block">
          <table class="w-full text-left text-xs">
            <thead class="border-b border-slate-800 text-slate-400">
              <tr>
                <th class="px-4 py-3">Name</th>
                <th class="px-4 py-3">Device</th>
                <th class="px-4 py-3">Protocol Attributes</th>
                <th class="px-4 py-3">Access</th>
                <th class="px-4 py-3">Live Test</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/70">
              <tr v-for="m in mappingsPag.paginatedItems.value" :key="m.id" class="hover:bg-slate-800/30">
                <td class="px-4 py-3 font-medium text-white">{{ m.name }}</td>
                <td class="px-4 py-3 text-slate-300">{{ deviceMap[m.deviceId]?.name || m.deviceId }}</td>
                <td class="px-4 py-3 font-mono text-slate-300">
                  unit {{ m.protocolAttributes?.slaveAddress ?? 1 }} |
                  {{ m.protocolAttributes?.functionCode }} @ {{ m.protocolAttributes?.registerAddress }}
                  [{{ m.protocolAttributes?.dataType }}/{{ m.protocolAttributes?.wordOrder || 'BIG' }}]
                  <span
                    v-if="m.protocolAttributes?.unit || m.protocolAttributes?.precision !== undefined"
                    class="ml-1.5 rounded bg-slate-800 px-1.5 py-0.5 text-[10px] text-sky-300"
                  >
                    {{ m.protocolAttributes?.unit || '' }}{{ m.protocolAttributes?.unit && m.protocolAttributes?.precision !== undefined ? ' ' : '' }}{{ m.protocolAttributes?.precision !== undefined ? `10^${-m.protocolAttributes.precision}` : '' }}
                  </span>
                </td>
                <td class="px-4 py-3 font-mono text-slate-400">
                  {{ m.protocolAttributes?.accessMode === 'READ_WRITE' ? 'R/W' : 'R' }}
                </td>
                <td class="px-4 py-3 font-mono">
                  <span v-if="testResults[m.id]?.quality === 'GOOD'" class="text-emerald-300">
                    {{ testResults[m.id].value }}
                  </span>
                  <span v-else-if="testResults[m.id]" class="text-rose-300" :title="testResults[m.id].error">
                    BAD: {{ testResults[m.id].error }}
                  </span>
                  <span v-else class="text-slate-600">-</span>
                </td>
                <td class="px-4 py-3 text-right">
                  <div class="inline-flex items-center justify-end gap-1.5">
                    <button
                      type="button"
                      :disabled="testingId === m.id"
                      class="inline-flex items-center gap-1 rounded border border-sky-500/40 bg-sky-500/15 px-2 py-1 text-[11px] text-sky-200 transition hover:bg-sky-500/25 disabled:opacity-40"
                      @click="testRead(m)"
                    >
                      <Play class="h-3 w-3" /> {{ testingId === m.id ? 'Reading...' : 'Test' }}
                    </button>
                    <button
                      type="button"
                      class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 transition hover:bg-slate-700"
                      @click="openEditRegister(m)"
                    >
                      <Edit3 class="h-3 w-3" /> Edit
                    </button>
                    <button
                      type="button"
                      aria-label="Delete register"
                      class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 transition hover:bg-rose-500/20"
                      @click="confirmDeleteRegister(m)"
                    >
                      <Trash2 class="h-3.5 w-3.5" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="mappings.length === 0">
                <td colspan="6" class="px-4 py-8 text-center text-slate-500">No registers configured yet.</td>
              </tr>
            </tbody>
          </table>
        </div>
        <TablePagination
          v-model:page="mappingsPag.page.value"
          v-model:page-size="mappingsPag.pageSize.value"
          :total-items="mappingsPag.totalItems.value"
        />
      </div>
    </div>

    <!-- TAB 3: DASHBOARDS & WIDGETS (Editable + Reorderable) -->
    <div v-if="activeTab === 'dashboards'" class="grid gap-5 lg:grid-cols-3 lg:gap-6">
      <!-- Dashboards List -->
      <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
        <div class="space-y-3 p-4">
          <div class="flex items-center justify-between gap-2">
            <h3 class="text-sm font-semibold text-white">Dashboards</h3>
            <button
              type="button"
              class="inline-flex h-8 items-center gap-1 rounded-lg bg-amber-500 px-2.5 text-xs font-semibold text-slate-950 transition hover:bg-amber-400"
              @click="openCreateDashboard"
            >
              <Plus class="h-3.5 w-3.5" /> New
            </button>
          </div>

          <div class="space-y-1.5">
            <div
              v-for="(d, idx) in dashboardsPag.paginatedItems.value"
              :key="d.id"
              class="flex items-center justify-between gap-2 rounded-lg border px-2.5 py-2 text-xs transition"
              :class="selectedDash?.id === d.id ? 'border-amber-500/50 bg-amber-500/15' : 'border-slate-800 bg-slate-950/60'"
            >
              <button type="button" class="min-w-0 flex-1 truncate py-0.5 text-left font-medium text-white" @click="selectedDashId = d.id">
                {{ d.name }}
                <span class="ml-1 text-[10px] text-slate-400">({{ d.widgets.length }})</span>
              </button>

              <div class="flex shrink-0 items-center gap-0.5">
                <button
                  type="button"
                  :disabled="dashboardsPag.startIndex.value + idx === 0"
                  title="Move up"
                  aria-label="Move dashboard up"
                  class="flex h-7 w-7 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-white disabled:opacity-30"
                  @click="moveDashboard(dashboardsPag.startIndex.value + idx, -1)"
                >
                  <ArrowUp class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  :disabled="dashboardsPag.startIndex.value + idx === dashboards.length - 1"
                  title="Move down"
                  aria-label="Move dashboard down"
                  class="flex h-7 w-7 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-white disabled:opacity-30"
                  @click="moveDashboard(dashboardsPag.startIndex.value + idx, 1)"
                >
                  <ArrowDown class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  title="Edit dashboard"
                  aria-label="Edit dashboard"
                  class="flex h-7 w-7 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-white"
                  @click="openEditDashboard(d)"
                >
                  <Edit3 class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  title="Delete dashboard"
                  aria-label="Delete dashboard"
                  class="flex h-7 w-7 items-center justify-center rounded text-rose-400 hover:bg-rose-500/15"
                  @click="confirmDeleteDashboard(d)"
                >
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
            <p v-if="dashboards.length === 0" class="py-6 text-center text-xs text-slate-500">No dashboards yet.</p>
          </div>
        </div>
        <TablePagination
          v-model:page="dashboardsPag.page.value"
          v-model:page-size="dashboardsPag.pageSize.value"
          :total-items="dashboardsPag.totalItems.value"
          compact
        />
      </section>

      <!-- Selected Dashboard Widgets -->
      <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60 lg:col-span-2">
        <div class="space-y-3 p-4">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h3 class="min-w-0 truncate text-sm font-semibold text-white">
              {{ selectedDash ? `Widgets on "${selectedDash.name}"` : 'Select a dashboard' }}
            </h3>
            <button
              v-if="selectedDash"
              type="button"
              class="inline-flex h-8 shrink-0 items-center gap-1 rounded-lg bg-amber-500 px-3 text-xs font-semibold text-slate-950 transition hover:bg-amber-400"
              @click="openCreateWidget"
            >
              <Plus class="h-3.5 w-3.5" /> Add Widget
            </button>
          </div>

          <div v-if="selectedDash" class="overflow-hidden rounded-lg border border-slate-800">
            <div class="divide-y divide-slate-800/70">
              <div
                v-for="(w, idx) in widgetsPag.paginatedItems.value"
                :key="w.id"
                class="flex flex-col gap-2.5 px-3.5 py-3 text-xs sm:flex-row sm:items-center sm:justify-between"
              >
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-1.5">
                    <span class="font-medium text-white">{{ w.name }}</span>
                    <span class="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] text-slate-300">{{ w.widgetType }}</span>
                  </div>
                  <p class="mt-0.5 truncate font-mono text-[11px] text-slate-500">
                    {{ w.registerMappingId && mappingMap[w.registerMappingId] ? mappingMap[w.registerMappingId].name : 'Unbound register' }}
                  </p>
                </div>

                <div class="flex shrink-0 items-center justify-end gap-1">
                  <button
                    type="button"
                    :disabled="widgetsPag.startIndex.value + idx === 0"
                    title="Move up"
                    aria-label="Move widget up"
                    class="flex h-7 w-7 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-white disabled:opacity-30"
                    @click="moveWidget(widgetsPag.startIndex.value + idx, -1)"
                  >
                    <ArrowUp class="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    :disabled="widgetsPag.startIndex.value + idx === selectedDash.widgets.length - 1"
                    title="Move down"
                    aria-label="Move widget down"
                    class="flex h-7 w-7 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-white disabled:opacity-30"
                    @click="moveWidget(widgetsPag.startIndex.value + idx, 1)"
                  >
                    <ArrowDown class="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    title="Edit widget"
                    class="inline-flex h-7 items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 text-[11px] text-slate-200 hover:bg-slate-700"
                    @click="openEditWidget(w)"
                  >
                    <Edit3 class="h-3 w-3" /> Edit
                  </button>
                  <button
                    type="button"
                    title="Delete widget"
                    aria-label="Delete widget"
                    class="flex h-7 w-7 items-center justify-center rounded border border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
                    @click="confirmDeleteWidget(w)"
                  >
                    <Trash2 class="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
              <div v-if="selectedDash.widgets.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">
                No widgets added to this dashboard yet.
              </div>
            </div>
            <TablePagination
              v-model:page="widgetsPag.page.value"
              v-model:page-size="widgetsPag.pageSize.value"
              :total-items="widgetsPag.totalItems.value"
            />
          </div>
        </div>
      </section>
    </div>

    <!-- TAB 4: HISTORY KEYS -->
    <div v-if="activeTab === 'history'" class="space-y-4">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h3 class="text-sm font-semibold text-white">Scheduled History Archival</h3>
          <p class="mt-0.5 text-xs text-slate-400">
            Select specific register keys to archive on a schedule. Registers without a history entry are only polled while a dashboard is open.
          </p>
        </div>
        <button
          type="button"
          :disabled="availableHistoryMappings.length === 0"
          class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 self-start rounded-lg bg-amber-500 px-3.5 text-xs font-semibold text-slate-950 transition hover:bg-amber-400 disabled:opacity-40 sm:self-auto"
          @click="openCreateHistoryEntry"
        >
          <Plus class="h-4 w-4" /> Add Key
        </button>
      </div>

      <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
        <!-- Mobile Cards -->
        <div class="divide-y divide-slate-800/70 md:hidden">
          <div v-for="he in historyPag.paginatedItems.value" :key="he.id" class="space-y-2.5 p-4 text-xs">
            <div class="flex items-start justify-between gap-2">
              <p class="min-w-0 break-words font-mono text-slate-200">
                {{ mappingMap[he.registerMappingId] ? registerKeyLabel(mappingMap[he.registerMappingId]) : he.registerMappingId }}
              </p>
              <StatusBadge :status="he.isActive ? 'ACTIVE' : 'DISABLED'" size="sm" />
            </div>
            <div class="flex items-center gap-3 font-mono text-[11px] text-slate-400">
              <span>Poll: {{ formatPollInterval(he.pollIntervalSeconds) }}</span>
              <span>|</span>
              <span>Keep: {{ formatRetention(he.retentionDays) }}</span>
            </div>
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-lg border border-slate-700 bg-slate-800 px-2.5 text-[11px] font-medium text-slate-200 hover:bg-slate-700"
                @click="openEditHistoryEntry(he)"
              >
                <Edit3 class="h-3 w-3" /> Edit
              </button>
              <button
                type="button"
                aria-label="Remove history entry"
                class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
                @click="confirmDeleteHistoryEntry(he)"
              >
                <Trash2 class="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
          <div v-if="historyEntries.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">
            No registers scheduled for history storage yet.
          </div>
        </div>

        <!-- Desktop Table -->
        <div class="hidden overflow-x-auto md:block">
          <table class="w-full text-left text-xs">
            <thead class="border-b border-slate-800 text-slate-400">
              <tr>
                <th class="px-4 py-3">Register Key</th>
                <th class="px-4 py-3">Poll Interval</th>
                <th class="px-4 py-3">Retention</th>
                <th class="px-4 py-3">Status</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/70">
              <tr v-for="he in historyPag.paginatedItems.value" :key="he.id" class="hover:bg-slate-800/30">
                <td class="px-4 py-3 font-mono text-slate-200">
                  {{ mappingMap[he.registerMappingId] ? registerKeyLabel(mappingMap[he.registerMappingId]) : he.registerMappingId }}
                </td>
                <td class="whitespace-nowrap px-4 py-3 font-mono text-slate-300">
                  {{ formatPollInterval(he.pollIntervalSeconds) }}
                </td>
                <td class="whitespace-nowrap px-4 py-3 font-mono text-slate-300">
                  {{ formatRetention(he.retentionDays) }}
                </td>
                <td class="px-4 py-3">
                  <StatusBadge :status="he.isActive ? 'ACTIVE' : 'DISABLED'" size="sm" />
                </td>
                <td class="px-4 py-3 text-right">
                  <div class="inline-flex items-center justify-end gap-1.5">
                    <button
                      type="button"
                      class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 transition hover:bg-slate-700"
                      @click="openEditHistoryEntry(he)"
                    >
                      <Edit3 class="h-3 w-3" /> Edit
                    </button>
                    <button
                      type="button"
                      aria-label="Remove history entry"
                      class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 transition hover:bg-rose-500/20"
                      @click="confirmDeleteHistoryEntry(he)"
                    >
                      <Trash2 class="h-3.5 w-3.5" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="historyEntries.length === 0">
                <td colspan="5" class="px-4 py-8 text-center text-slate-500">No registers scheduled for history storage yet.</td>
              </tr>
            </tbody>
          </table>
        </div>
        <TablePagination
          v-model:page="historyPag.page.value"
          v-model:page-size="historyPag.pageSize.value"
          :total-items="historyPag.totalItems.value"
        />
      </div>
    </div>

    <!-- TAB 5: ALARM RULES -->
    <div v-if="activeTab === 'alarms'" class="space-y-4">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h3 class="text-sm font-semibold text-white">Alarm Rules</h3>
          <p class="mt-0.5 text-xs text-slate-400">
            Latch and re-arm rules evaluated in scaled engineering units. Active rules poll their register in the background even when no dashboard is open.
          </p>
        </div>
        <button
          type="button"
          :disabled="mappings.length === 0"
          class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 self-start rounded-lg bg-amber-500 px-3.5 text-xs font-semibold text-slate-950 transition hover:bg-amber-400 disabled:opacity-40 sm:self-auto"
          @click="openCreateAlarmRule"
        >
          <Plus class="h-4 w-4" /> Add Alarm Rule
        </button>
      </div>

      <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
        <!-- Mobile Cards -->
        <div class="divide-y divide-slate-800/70 md:hidden">
          <div v-for="r in alarmsPag.paginatedItems.value" :key="r.id" class="space-y-2.5 p-4 text-xs">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <p class="truncate text-sm font-semibold text-white">{{ r.name }}</p>
                <p class="text-[11px] text-slate-400">
                  {{ r.registerName || r.registerMappingId }}
                  <span v-if="r.deviceName"> ({{ r.deviceName }})</span>
                </p>
              </div>
              <span
                class="shrink-0 rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase"
                :class="alarmSeverityBadgeClass(r.severity)"
              >
                {{ r.severity }}
              </span>
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <span
                class="inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase"
                :class="alarmRuntimeBadgeClass(r)"
              >
                {{ r.isActive ? r.runtimeStatus : 'PAUSED' }}
              </span>
              <span class="font-mono text-[11px] text-slate-200">
                {{ alarmConditionSymbol(r.condition) }} {{ r.threshold }}{{ r.unit ? ` ${r.unit}` : '' }}
              </span>
              <span v-if="r.clearDeadband > 0" class="font-mono text-[10px] text-slate-400">
                (deadband {{ r.clearDeadband }})
              </span>
            </div>

            <div class="flex items-center justify-between text-[11px] text-slate-400">
              <span class="font-mono">Hold {{ r.holdSeconds }}s | Poll {{ r.pollIntervalSeconds }}s</span>
              <span v-if="r.notifyEmails.length > 0" class="inline-flex items-center gap-1 text-sky-300">
                <Mail class="h-3.5 w-3.5" /> {{ r.notifyEmails.length }}
              </span>
            </div>

            <div class="flex items-center gap-2 pt-0.5">
              <button
                type="button"
                class="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-lg border border-slate-700 bg-slate-800 px-2.5 text-[11px] font-medium text-slate-200 hover:bg-slate-700"
                @click="openEditAlarmRule(r)"
              >
                <Edit3 class="h-3 w-3" /> Edit
              </button>
              <button
                type="button"
                aria-label="Delete alarm rule"
                class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
                @click="confirmDeleteAlarmRule(r)"
              >
                <Trash2 class="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
          <div v-if="alarmRules.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">
            No alarm rules configured for this site yet.
          </div>
        </div>

        <!-- Desktop Table -->
        <div class="hidden overflow-x-auto md:block">
          <table class="w-full text-left text-xs">
            <thead class="border-b border-slate-800 text-slate-400">
              <tr>
                <th class="px-4 py-3">Rule</th>
                <th class="px-4 py-3">Register</th>
                <th class="px-4 py-3">Condition</th>
                <th class="px-4 py-3">Hold / Poll</th>
                <th class="px-4 py-3">Severity</th>
                <th class="px-4 py-3">State</th>
                <th class="px-4 py-3">Notify</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/70">
              <tr v-for="r in alarmsPag.paginatedItems.value" :key="r.id" class="hover:bg-slate-800/30">
                <td class="px-4 py-3 font-medium text-white">{{ r.name }}</td>
                <td class="px-4 py-3 text-slate-300">
                  <p>{{ r.registerName || r.registerMappingId }}</p>
                  <p v-if="r.deviceName" class="text-[11px] text-slate-500">{{ r.deviceName }}</p>
                </td>
                <td class="whitespace-nowrap px-4 py-3 font-mono text-slate-200">
                  <span>{{ alarmConditionSymbol(r.condition) }} {{ r.threshold }}{{ r.unit ? ` ${r.unit}` : '' }}</span>
                  <span v-if="r.clearDeadband > 0" class="block text-[10px] text-slate-400">
                    deadband: {{ r.clearDeadband }}{{ r.unit ? ` ${r.unit}` : '' }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-3 font-mono text-slate-300">
                  {{ r.holdSeconds }}s / {{ r.pollIntervalSeconds }}s
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <span
                    class="inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase"
                    :class="alarmSeverityBadgeClass(r.severity)"
                  >
                    {{ r.severity }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <span
                    class="inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase"
                    :class="alarmRuntimeBadgeClass(r)"
                  >
                    {{ r.isActive ? r.runtimeStatus : 'PAUSED' }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-slate-300">
                  <span
                    v-if="r.notifyEmails.length > 0"
                    class="inline-flex items-center gap-1 text-[11px]"
                    :title="r.notifyEmails.join(', ')"
                  >
                    <Mail class="h-3.5 w-3.5 text-sky-400" />
                    {{ r.notifyEmails.length }}
                  </span>
                  <span v-else class="text-slate-500">-</span>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-right">
                  <div class="inline-flex items-center justify-end gap-1.5">
                    <button
                      type="button"
                      class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 transition hover:bg-slate-700"
                      @click="openEditAlarmRule(r)"
                    >
                      <Edit3 class="h-3 w-3" /> Edit
                    </button>
                    <button
                      type="button"
                      aria-label="Delete alarm rule"
                      class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 transition hover:bg-rose-500/20"
                      @click="confirmDeleteAlarmRule(r)"
                    >
                      <Trash2 class="h-3.5 w-3.5" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="alarmRules.length === 0">
                <td colspan="8" class="px-4 py-8 text-center text-slate-500">
                  No alarm rules configured for this site yet.
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <TablePagination
          v-model:page="alarmsPag.page.value"
          v-model:page-size="alarmsPag.pageSize.value"
          :total-items="alarmsPag.totalItems.value"
        />
      </div>
    </div>

    <!-- TAB 6: MEMBER WRITE POLICIES -->
    <div v-if="activeTab === 'policies'" class="space-y-4">
      <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
        <div class="px-4 pt-4 pb-3 sm:px-5 sm:pt-5">
          <h3 class="text-sm font-semibold text-white">Site Member Register Write Policies</h3>
          <p class="mt-0.5 text-xs text-slate-400">
            Site Operators and Platform Admins can always write to writable registers. Grant individual Site Members write access below.
          </p>
        </div>

        <!-- Mobile Cards -->
        <div class="divide-y divide-slate-800/70 md:hidden">
          <div v-for="m in policiesPag.paginatedItems.value" :key="m.userId" class="flex items-center justify-between gap-3 p-4 text-xs">
            <div class="min-w-0">
              <p class="break-all font-medium text-white">{{ m.email }}</p>
              <p class="mt-0.5 font-mono text-[10px] text-slate-400">SITE_MEMBER</p>
            </div>
            <label class="inline-flex shrink-0 items-center gap-2 text-xs text-slate-200">
              <input
                type="checkbox"
                :checked="Boolean(policyMap[m.userId])"
                @change="toggleMemberWrite(m, ($event.target as HTMLInputElement).checked)"
              />
              Write
            </label>
          </div>
          <div v-if="siteMembersOnly.length === 0" class="px-4 py-6 text-center text-xs text-slate-500">
            No Site Members assigned to this site yet.
          </div>
        </div>

        <!-- Desktop Table -->
        <div class="hidden overflow-x-auto md:block">
          <table class="w-full text-left text-xs">
            <thead class="border-b border-slate-800 text-slate-400">
              <tr>
                <th class="px-5 py-2.5">Member Email</th>
                <th class="px-4 py-2.5">Role</th>
                <th class="px-5 py-2.5 text-right">Register Write Permission</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/70">
              <tr v-for="m in policiesPag.paginatedItems.value" :key="m.userId" class="hover:bg-slate-800/30">
                <td class="px-5 py-3 font-medium text-white">{{ m.email }}</td>
                <td class="px-4 py-3 font-mono text-[11px] text-slate-400">SITE_MEMBER</td>
                <td class="px-5 py-3 text-right">
                  <label class="inline-flex items-center gap-2 text-xs text-slate-200">
                    <input
                      type="checkbox"
                      :checked="Boolean(policyMap[m.userId])"
                      @change="toggleMemberWrite(m, ($event.target as HTMLInputElement).checked)"
                    />
                    Allow register writes
                  </label>
                </td>
              </tr>
              <tr v-if="siteMembersOnly.length === 0">
                <td colspan="3" class="px-5 py-6 text-center text-xs text-slate-500">
                  No Site Members assigned to this site yet.
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <TablePagination
          v-model:page="policiesPag.page.value"
          v-model:page-size="policiesPag.pageSize.value"
          :total-items="policiesPag.totalItems.value"
        />
      </section>
    </div>

    <!-- MODAL: CREATE / EDIT DEVICE -->
    <Teleport to="body">
      <div v-if="deviceModalOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs" @click.self="deviceModalOpen = false">
        <form class="max-h-[90dvh] w-full max-w-md overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6" @submit.prevent="saveDevice">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">{{ editingDeviceId ? 'Edit Device' : 'Add Device' }}</h3>
            <button type="button" aria-label="Close modal" class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white" @click="deviceModalOpen = false"><X class="h-4 w-4" /></button>
          </div>
          <div>
            <label class="mb-1 block text-xs text-slate-400">Device Name</label>
            <input v-model="devName" required placeholder="PLC-1" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white" />
          </div>
          <div class="grid gap-3 sm:grid-cols-3">
            <div class="sm:col-span-2">
              <label class="mb-1 block text-xs text-slate-400">IP Address</label>
              <input v-model="devHost" required placeholder="172.21.0.2" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white" />
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Port</label>
              <NumericTextInput v-model="devPort" :min="1" :max="65535" required placeholder="502" />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-xs text-slate-400">Protocol</label>
            <select v-model="devProto" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
              <option value="MODBUS_TCP">Modbus TCP</option>
            </select>
          </div>
          <label v-if="editingDeviceId" class="inline-flex items-center gap-2 text-xs text-slate-300">
            <input v-model="devActive" type="checkbox" /> Active
          </label>
          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300" @click="deviceModalOpen = false">Cancel</button>
            <button type="submit" class="rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400">Save Device</button>
          </div>
        </form>
      </div>
    </Teleport>

    <!-- MODAL: CREATE / EDIT REGISTER (Dynamic Protocol Attributes) -->
    <Teleport to="body">
      <div v-if="regModalOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs" @click.self="regModalOpen = false">
        <form class="max-h-[90dvh] w-full max-w-lg overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6" @submit.prevent="saveRegister">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">{{ editingRegId ? 'Edit Register' : 'Add Register' }}</h3>
            <button type="button" aria-label="Close modal" class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white" @click="regModalOpen = false"><X class="h-4 w-4" /></button>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Device</label>
              <select v-model="regDeviceId" required class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
                <option v-for="d in devices" :key="d.id" :value="d.id">{{ d.name }} ({{ d.protocol }})</option>
              </select>
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Register Name</label>
              <input v-model="regName" required placeholder="Inlet Temp" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white" />
            </div>
          </div>

          <!-- Dynamic Protocol Section: MODBUS_TCP -->
          <div v-if="!selectedRegDevice || selectedRegDevice.protocol === 'MODBUS_TCP'" class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
            <p class="text-[11px] font-semibold uppercase tracking-wider text-amber-300">Modbus TCP Attributes</p>
            <div class="grid gap-3 sm:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Slave Address (Unit ID)</label>
                <NumericTextInput v-model="regSlave" :min="0" :max="255" required placeholder="1" input-class="bg-slate-900" />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Function Code</label>
                <select v-model="regFC" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white">
                  <option value="READ_HOLDING">FC03 Holding</option>
                  <option value="READ_INPUT">FC04 Input</option>
                  <option value="READ_COIL">FC01 Coil</option>
                  <option value="READ_DISCRETE">FC02 Discrete</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Register Address</label>
                <NumericTextInput v-model="regAddr" :min="0" :max="65535" required placeholder="0" input-class="bg-slate-900" />
              </div>
            </div>

            <div class="grid gap-3 sm:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Data Type</label>
                <select v-model="regDataType" :disabled="regFC === 'READ_COIL' || regFC === 'READ_DISCRETE'" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white disabled:opacity-50">
                  <option value="BOOL">BOOL</option>
                  <option value="UINT16">UINT16</option>
                  <option value="INT16">INT16</option>
                  <option value="UINT32">UINT32</option>
                  <option value="INT32">INT32</option>
                  <option value="FLOAT32">FLOAT32</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Word Order</label>
                <select v-model="regWordOrder" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white">
                  <option value="BIG">BIG</option>
                  <option value="LITTLE">LITTLE</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Access Permission</label>
                <select v-model="regAccess" :disabled="regFC === 'READ_DISCRETE' || regFC === 'READ_INPUT'" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white disabled:opacity-50">
                  <option value="READ">Read only (R)</option>
                  <option value="READ_WRITE">Read / Write (R/W)</option>
                </select>
              </div>
            </div>

            <div class="grid gap-3 border-t border-slate-800 pt-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Unit (optional)</label>
                <input v-model="regUnit" maxlength="32" placeholder="e.g. C, bar, %" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white" />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Precision (10^-x, optional)</label>
                <NumericTextInput v-model="regPrecision" :min="-5" :max="5" allow-negative placeholder="0" input-class="bg-slate-900" />
              </div>
            </div>
          </div>

          <label v-if="editingRegId" class="inline-flex items-center gap-2 text-xs text-slate-300">
            <input v-model="regActive" type="checkbox" /> Active
          </label>

          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300" @click="regModalOpen = false">Cancel</button>
            <button type="submit" class="rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400">Save Register</button>
          </div>
        </form>
      </div>
    </Teleport>

    <!-- MODAL: CREATE / EDIT DASHBOARD -->
    <Teleport to="body">
      <div v-if="dashModalOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs" @click.self="dashModalOpen = false">
        <form class="max-h-[90dvh] w-full max-w-sm overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6" @submit.prevent="saveDashboard">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">{{ editingDashId ? 'Edit Dashboard' : 'New Dashboard' }}</h3>
            <button type="button" aria-label="Close modal" class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white" @click="dashModalOpen = false"><X class="h-4 w-4" /></button>
          </div>
          <div>
            <label class="mb-1 block text-xs text-slate-400">Dashboard Name</label>
            <input v-model="dashName" required placeholder="Main Boiler" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-slate-400">Poll Interval (ms, 1000-60000)</label>
            <NumericTextInput v-model="dashPollIntervalMs" :min="1000" :max="60000" required placeholder="1000" />
            <p class="mt-1 text-[11px] text-slate-500">
              Applies up to 100 active registers per device; above 100, the minimum rate increases automatically to protect the device.
            </p>
          </div>
          <label class="inline-flex items-center gap-2 text-xs text-slate-300">
            <input v-model="dashIsDefault" type="checkbox" /> Set as default dashboard
          </label>
          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300" @click="dashModalOpen = false">Cancel</button>
            <button type="submit" class="rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400">Save</button>
          </div>
        </form>
      </div>
    </Teleport>

    <!-- MODAL: CREATE / EDIT WIDGET -->
    <Teleport to="body">
      <div v-if="widgetModalOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs" @click.self="widgetModalOpen = false">
        <form class="max-h-[90dvh] w-full max-w-lg overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6" @submit.prevent="saveWidget">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">{{ editingWidgetId ? 'Edit Widget' : 'Add Widget' }}</h3>
            <button type="button" aria-label="Close modal" class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white" @click="widgetModalOpen = false"><X class="h-4 w-4" /></button>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Card Title</label>
              <input v-model="widName" required placeholder="Water Temperature" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white" />
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Card Type</label>
              <select v-model="widType" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
                <option value="display">Display</option>
                <option value="lamp">Lamp</option>
                <option value="switch">Switch</option>
                <option value="button">Button</option>
                <option value="number_input">Number Input</option>
              </select>
            </div>
          </div>

          <RegisterPicker
            v-model="widMappingId"
            :devices="devices"
            :mappings="mappings"
            register-label="Bound Register"
            placeholder="Search by register name, address, or type..."
          />

          <!-- DISPLAY & NUMBER INPUT EXTRA PARAMS -->
          <div v-if="widType === 'display' || widType === 'number_input'" class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
            <p class="text-[11px] font-semibold uppercase tracking-wider text-sky-300">
              {{ widType === 'display' ? 'Display Options' : 'Number Input Options' }}
            </p>
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Font Size</label>
                <select v-model="widFontSize" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white">
                  <option value="sm">Small</option>
                  <option value="md">Medium</option>
                  <option value="lg">Large</option>
                  <option value="xl">Extra Large</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Decimal Places (10^-x)</label>
                <NumericTextInput
                  v-model="widPrecision"
                  :min="-5"
                  :max="5"
                  allow-negative
                  :placeholder="`Default: ${widBoundMapping?.protocolAttributes?.precision ?? 0}`"
                  input-class="bg-slate-900"
                />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Prefix</label>
                <input v-model="widPrefix" placeholder="e.g. ~" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white" />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Unit (Postfix)</label>
                <input
                  v-model="widUnit"
                  :placeholder="widBoundMapping?.protocolAttributes?.unit ? `Default: ${widBoundMapping.protocolAttributes.unit}` : 'e.g. C, bar, %'"
                  class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white"
                />
              </div>
            </div>
            <p class="text-[11px] text-slate-500">
              Leave Unit or Decimal Places empty to inherit from the bound register (unit: {{ widBoundMapping?.protocolAttributes?.unit || 'none' }}, precision: {{ widBoundMapping?.protocolAttributes?.precision ?? 0 }}).
            </p>

            <div v-if="widType === 'number_input'" class="space-y-2 border-t border-slate-800 pt-3">
              <div class="grid gap-3 sm:grid-cols-2">
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Min Value (optional)</label>
                  <NumericTextInput
                    v-model="widMin"
                    allow-decimal
                    allow-negative
                    :min="widBoundTypeBounds.typeMin"
                    :max="widBoundTypeBounds.typeMax"
                    :placeholder="String(widBoundTypeBounds.typeMin)"
                    input-class="bg-slate-900"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs text-slate-400">Max Value (optional)</label>
                  <NumericTextInput
                    v-model="widMax"
                    allow-decimal
                    allow-negative
                    :min="widBoundTypeBounds.typeMin"
                    :max="widBoundTypeBounds.typeMax"
                    :placeholder="String(widBoundTypeBounds.typeMax)"
                    input-class="bg-slate-900"
                  />
                </div>
              </div>
              <p class="text-[11px] text-slate-500">
                Register type limit ({{ widBoundMapping?.protocolAttributes?.dataType || 'UINT16' }}):
                {{ widBoundTypeBounds.typeMin }} to {{ widBoundTypeBounds.typeMax }}
              </p>
            </div>
          </div>

          <!-- LAMP EXTRA PARAMS (BOTH ON AND OFF STATES) -->
          <div v-else-if="widType === 'lamp'" class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
            <p class="text-[11px] font-semibold uppercase tracking-wider text-emerald-300">ON State (value != 0)</p>
            <div class="grid gap-3 sm:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Mode</label>
                <select v-model="widLampOn.mode" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white">
                  <option value="on">On</option>
                  <option value="blink">Blink</option>
                  <option value="off">Off</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Color</label>
                <select v-model="widLampOn.color" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white">
                  <option value="emerald">Green</option>
                  <option value="sky">Blue</option>
                  <option value="amber">Amber</option>
                  <option value="rose">Red</option>
                  <option value="slate">Gray</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Shown Text</label>
                <input v-model="widLampOn.text" placeholder="RUNNING" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white" />
              </div>
            </div>

            <p class="border-t border-slate-800 pt-3 text-[11px] font-semibold uppercase tracking-wider text-slate-400">OFF State (value = 0)</p>
            <div class="grid gap-3 sm:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Mode</label>
                <select v-model="widLampOff.mode" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white">
                  <option value="off">Off</option>
                  <option value="on">On</option>
                  <option value="blink">Blink</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Color</label>
                <select v-model="widLampOff.color" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white">
                  <option value="slate">Gray</option>
                  <option value="emerald">Green</option>
                  <option value="sky">Blue</option>
                  <option value="amber">Amber</option>
                  <option value="rose">Red</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Shown Text</label>
                <input v-model="widLampOff.text" placeholder="STOPPED" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white" />
              </div>
            </div>
          </div>

          <!-- SWITCH EXTRA PARAMS -->
          <div v-else-if="widType === 'switch'" class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
            <p class="text-[11px] font-semibold uppercase tracking-wider text-emerald-300">Switch Options</p>
            <div class="grid gap-3 sm:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs text-slate-400">ON State Color</label>
                <select v-model="widSwitchColor" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white">
                  <option value="emerald">Green</option>
                  <option value="sky">Blue</option>
                  <option value="amber">Amber</option>
                  <option value="rose">Red</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">ON Label</label>
                <input v-model="widSwitchOnLabel" placeholder="ON" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white" />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">OFF Label</label>
                <input v-model="widSwitchOffLabel" placeholder="OFF" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white" />
              </div>
            </div>
          </div>

          <!-- BUTTON EXTRA PARAMS -->
          <div v-else-if="widType === 'button'" class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
            <p class="text-[11px] font-semibold uppercase tracking-wider text-sky-300">Button Options</p>
            <div class="grid gap-3 sm:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Color</label>
                <select v-model="widBtnColor" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white">
                  <option value="sky">Blue</option>
                  <option value="emerald">Green</option>
                  <option value="amber">Amber</option>
                  <option value="rose">Red</option>
                  <option value="slate">Gray</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Button Label</label>
                <input v-model="widBtnLabel" placeholder="Start Pump" class="w-full rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-2 text-xs text-white" />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Trigger Time (ms)</label>
                <NumericTextInput v-model="widBtnTriggerMs" :min="100" :max="60000" required placeholder="1000" input-class="bg-slate-900" />
              </div>
            </div>
            <p class="text-[11px] text-slate-500">
              Sends 1 immediately, then automatically resets and sends 0 after the trigger time.
            </p>
          </div>

          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300" @click="widgetModalOpen = false">Cancel</button>
            <button type="submit" class="rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400">Save Widget</button>
          </div>
        </form>
      </div>
    </Teleport>

    <!-- MODAL: ADD / EDIT HISTORY KEY -->
    <Teleport to="body">
      <div v-if="histModalOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs" @click.self="histModalOpen = false">
        <form class="max-h-[90dvh] w-full max-w-lg overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6" @submit.prevent="saveHistoryEntry">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">{{ editingHistId ? 'Edit History Key' : 'Add History Key' }}</h3>
            <button type="button" aria-label="Close modal" class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white" @click="histModalOpen = false"><X class="h-4 w-4" /></button>
          </div>

          <RegisterPicker
            v-if="!editingHistId"
            v-model="histMappingId"
            :devices="devices"
            :mappings="availableHistoryMappings"
            register-label="Register Key"
            placeholder="Search register to archive..."
          />
          <div v-else class="rounded-lg border border-slate-800 bg-slate-950/60 px-3 py-2.5">
            <p class="text-[11px] text-slate-400">Register Key</p>
            <p class="mt-0.5 font-mono text-xs text-slate-200">
              {{ mappingMap[histMappingId] ? registerKeyLabel(mappingMap[histMappingId]) : histMappingId }}
            </p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Poll Interval</label>
              <select v-model.number="histPollSec" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
                <option :value="10">10s</option>
                <option :value="30">30s</option>
                <option :value="60">1 min</option>
                <option :value="300">5 min</option>
                <option :value="900">15 min</option>
                <option :value="3600">1 hour</option>
              </select>
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Retention</label>
              <select v-model.number="histRetentionDays" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white">
                <option :value="1">1 day</option>
                <option :value="7">7 days</option>
                <option :value="30">30 days</option>
                <option :value="90">90 days</option>
                <option :value="365">1 year</option>
              </select>
            </div>
          </div>

          <label v-if="editingHistId" class="inline-flex items-center gap-2 text-xs text-slate-300">
            <input v-model="histActive" type="checkbox" /> Active
          </label>

          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300" @click="histModalOpen = false">Cancel</button>
            <button
              type="submit"
              :disabled="!editingHistId && !histMappingId"
              class="rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400 disabled:opacity-40"
            >
              {{ editingHistId ? 'Save Changes' : 'Add Key' }}
            </button>
          </div>
        </form>
      </div>
    </Teleport>

    <!-- MODAL: ADD / EDIT ALARM RULE -->
    <Teleport to="body">
      <div
        v-if="alarmModalOpen"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
        @click.self="alarmModalOpen = false"
      >
        <form
          class="max-h-[90dvh] w-full max-w-2xl overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6"
          @submit.prevent="saveAlarmRule"
        >
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">{{ editingAlarmId ? 'Edit Alarm Rule' : 'Add Alarm Rule' }}</h3>
            <button
              type="button"
              aria-label="Close modal"
              class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
              @click="alarmModalOpen = false"
            >
              <X class="h-4 w-4" />
            </button>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Rule Name</label>
              <input
                v-model="alarmName"
                required
                maxlength="120"
                placeholder="e.g. Boiler High Temperature"
                class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Severity</label>
              <select
                v-model="alarmSeverity"
                class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white"
              >
                <option value="CRITICAL">Critical</option>
                <option value="WARNING">Warning</option>
                <option value="INFO">Info</option>
              </select>
            </div>
          </div>

          <div>
            <RegisterPicker
              v-model="alarmMappingId"
              :devices="devices"
              :mappings="mappings"
              register-label="Monitored Register"
              placeholder="Search register to monitor..."
            />
            <p v-if="alarmBoundMapping" class="mt-1.5 text-[11px] text-slate-400">
              Evaluated in scaled engineering units (unit:
              <span class="font-mono text-slate-200">{{ alarmBoundMapping.protocolAttributes?.unit || 'none' }}</span>,
              precision:
              <span class="font-mono text-slate-200">{{ alarmBoundMapping.protocolAttributes?.precision ?? 0 }}</span>).
            </p>
          </div>

          <div class="grid gap-3 sm:grid-cols-3">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Condition</label>
              <select
                v-model="alarmCondition"
                class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white"
              >
                <option value="GT">Greater than (&gt;)</option>
                <option value="GTE">Greater or equal (&gt;=)</option>
                <option value="LT">Less than (&lt;)</option>
                <option value="LTE">Less or equal (&lt;=)</option>
                <option value="EQ">Equal to (==)</option>
                <option value="NEQ">Not equal to (!=)</option>
              </select>
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">
                Threshold{{ alarmBoundMapping?.protocolAttributes?.unit ? ` (${alarmBoundMapping.protocolAttributes.unit})` : '' }}
              </label>
              <NumericTextInput
                v-model="alarmThreshold"
                allow-decimal
                allow-negative
                required
                placeholder="80.0"
                input-class="bg-slate-950"
              />
            </div>
            <div v-if="alarmNeedsDeadband">
              <label class="mb-1 block text-xs text-slate-400">
                Clear Deadband{{ alarmBoundMapping?.protocolAttributes?.unit ? ` (${alarmBoundMapping.protocolAttributes.unit})` : '' }}
              </label>
              <NumericTextInput
                v-model="alarmDeadband"
                allow-decimal
                :min="0"
                placeholder="0"
                input-class="bg-slate-950"
              />
            </div>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Hold Time (seconds, 0 = immediate)</label>
              <NumericTextInput
                v-model="alarmHoldSec"
                :min="0"
                :max="86400"
                placeholder="0"
                input-class="bg-slate-950"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Background Poll Interval (seconds)</label>
              <NumericTextInput
                v-model="alarmPollSec"
                :min="1"
                :max="3600"
                required
                placeholder="10"
                input-class="bg-slate-950"
              />
            </div>
          </div>

          <!-- Free-list Email Recipients -->
          <div class="space-y-2.5 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-slate-200">Email Notification Recipients (optional)</label>
              <span class="text-[11px] text-slate-500">{{ alarmEmails.length }} / 50</span>
            </div>

            <div class="flex gap-2">
              <input
                v-model="alarmEmailDraft"
                type="text"
                placeholder="Enter one or more emails (comma or space separated)"
                class="flex-1 rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white placeholder-slate-500"
                @keydown.enter.prevent="addAlarmEmailsFromDraft"
              />
              <button
                type="button"
                class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-xs font-medium text-slate-200 hover:bg-slate-700"
                @click="addAlarmEmailsFromDraft"
              >
                Add
              </button>
            </div>

            <p v-if="alarmEmailErr" class="text-[11px] text-rose-300">{{ alarmEmailErr }}</p>

            <div v-if="alarmEmails.length > 0" class="flex flex-wrap gap-1.5 pt-1">
              <span
                v-for="email in alarmEmails"
                :key="email"
                class="inline-flex items-center gap-1 rounded-full border border-sky-500/30 bg-sky-500/15 px-2.5 py-0.5 text-[11px] text-sky-200"
              >
                {{ email }}
                <button
                  type="button"
                  class="text-sky-300 hover:text-white"
                  :title="`Remove ${email}`"
                  @click="removeAlarmEmail(email)"
                >
                  <X class="h-3 w-3" />
                </button>
              </span>
            </div>
            <p v-else class="text-[11px] text-slate-500">
              No email recipients configured. Alarms will still appear in the Alarm Center.
            </p>

            <div v-if="members.length > 0" class="border-t border-slate-800 pt-2">
              <p class="mb-1.5 text-[10px] uppercase tracking-wider text-slate-500">Quick add site member</p>
              <div class="flex flex-wrap gap-1.5">
                <button
                  v-for="m in members"
                  :key="m.userId"
                  type="button"
                  :disabled="alarmEmails.includes(m.email.toLowerCase())"
                  class="rounded-md border border-slate-700 bg-slate-900 px-2 py-0.5 text-[11px] text-slate-300 hover:bg-slate-800 disabled:opacity-40"
                  @click="addMemberEmailQuick(m.email)"
                >
                  + {{ m.email }}
                </button>
              </div>
            </div>

            <label class="inline-flex items-center gap-2 pt-1 text-xs text-slate-300">
              <input v-model="alarmNotifyOnClear" type="checkbox" />
              Send recovery email when alarm clears
            </label>
          </div>

          <label v-if="editingAlarmId" class="inline-flex items-center gap-2 text-xs text-slate-300">
            <input v-model="alarmActive" type="checkbox" /> Rule Active
          </label>

          <div class="flex justify-end gap-2 pt-2">
            <button
              type="button"
              class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300 hover:bg-slate-800"
              @click="alarmModalOpen = false"
            >
              Cancel
            </button>
            <button
              type="submit"
              :disabled="!alarmMappingId || !alarmName.trim()"
              class="rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400 disabled:opacity-40"
            >
              {{ editingAlarmId ? 'Save Changes' : 'Add Alarm Rule' }}
            </button>
          </div>
        </form>
      </div>
    </Teleport>

    <!-- Custom Delete Confirmation Modal -->
    <DeleteConfirmModal
      :open="deleteModal.open"
      :title="deleteModal.title"
      :item-name="deleteModal.itemName"
      :description="deleteModal.description"
      @close="deleteModal.open = false"
      @confirm="runDeleteConfirm"
    />
  </div>
</template>
