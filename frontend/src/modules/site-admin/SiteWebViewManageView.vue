<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  Bell,
  Cpu,
  Database,
  Edit3,
  ExternalLink,
  GripVertical,
  LayoutGrid,
  Mail,
  Play,
  Plus,
  ShieldCheck,
  Sliders,
  SlidersHorizontal,
  Star,
  Trash2,
  X,
} from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { useWebviewStore } from '../../stores/webview'
import type { SiteMember } from '../../types/iam'
import type {
  AlarmCondition,
  AlarmRule,
  AlarmSeverity,
  DeviceProtocol,
  HistoryEntry,
  MemberWritePolicy,
  ModbusAccessMode,
  ModbusDataType,
  ModbusFunctionCode,
  ModbusWordOrder,
  RegisterMapping,
  TelemetrySample,
  TileDensity,
  TileSize,
  WebviewDashboard,
  WebviewDevice,
  WebviewWidget,
} from '../../types/webview'
import { clampPrecision } from '../../utils/precision'
import { resolveDashboardLayout } from '../../utils/tiles'
import {
  firstError,
  validateIntegerRange,
  validateIpAddress,
  validateNumberRange,
  validateOptionalText,
  validatePort,
  validateRequiredText,
} from '../../utils/validation'
import CustomSelect, { type SelectOption } from '../../components/CustomSelect.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import DeleteConfirmModal from '../../components/DeleteConfirmModal.vue'
import NumericTextInput from '../../components/NumericTextInput.vue'
import RegisterPicker from '../../components/RegisterPicker.vue'
import TablePagination from '../../components/TablePagination.vue'
import DashboardTileGrid from '../../components/webview/DashboardTileGrid.vue'
import DashboardOptionsModal, { type DashboardOptionsPayload } from '../../components/webview/DashboardOptionsModal.vue'
import WidgetEditModal, { type WidgetDraftPayload } from '../../components/webview/WidgetEditModal.vue'
import { usePagination } from '../../composables/usePagination'

const route = useRoute()
const toast = useToastStore()
const siteId = computed(() => String(route.params.siteId || ''))
const webviewStore = useWebviewStore()

const PROTOCOL_OPTIONS: SelectOption<DeviceProtocol>[] = [
  { value: 'MODBUS_TCP', label: 'Modbus TCP' },
]

const FC_OPTIONS: SelectOption<ModbusFunctionCode>[] = [
  { value: 'READ_HOLDING', label: 'FC03 Holding' },
  { value: 'READ_INPUT', label: 'FC04 Input' },
  { value: 'READ_COIL', label: 'FC01 Coil' },
  { value: 'READ_DISCRETE', label: 'FC02 Discrete' },
]

const DATA_TYPE_OPTIONS: SelectOption<ModbusDataType>[] = [
  { value: 'BOOL', label: 'BOOL' },
  { value: 'UINT16', label: 'UINT16' },
  { value: 'INT16', label: 'INT16' },
  { value: 'UINT32', label: 'UINT32' },
  { value: 'INT32', label: 'INT32' },
  { value: 'FLOAT32', label: 'FLOAT32' },
]

const WORD_ORDER_OPTIONS: SelectOption<ModbusWordOrder>[] = [
  { value: 'BIG', label: 'BIG' },
  { value: 'LITTLE', label: 'LITTLE' },
]

const ACCESS_OPTIONS: SelectOption<ModbusAccessMode>[] = [
  { value: 'READ', label: 'Read only (R)' },
  { value: 'READ_WRITE', label: 'Read / Write (R/W)' },
]

const HIST_POLL_OPTIONS: SelectOption<number>[] = [
  { value: 10, label: '10s' },
  { value: 30, label: '30s' },
  { value: 60, label: '1 min' },
  { value: 300, label: '5 min' },
  { value: 900, label: '15 min' },
  { value: 3600, label: '1 hour' },
]

const HIST_RETENTION_OPTIONS: SelectOption<number>[] = [
  { value: 1, label: '1 day' },
  { value: 7, label: '7 days' },
  { value: 30, label: '30 days' },
  { value: 90, label: '90 days' },
  { value: 365, label: '1 year' },
]

const ALARM_SEVERITY_OPTIONS: SelectOption<AlarmSeverity>[] = [
  { value: 'CRITICAL', label: 'Critical' },
  { value: 'WARNING', label: 'Warning' },
  { value: 'INFO', label: 'Info' },
]

const ALARM_CONDITION_OPTIONS: SelectOption<AlarmCondition>[] = [
  { value: 'GT', label: 'Greater than (>)' },
  { value: 'GTE', label: 'Greater or equal (>=)' },
  { value: 'LT', label: 'Less than (<)' },
  { value: 'LTE', label: 'Less or equal (<=)' },
  { value: 'EQ', label: 'Equal to (==)' },
  { value: 'NEQ', label: 'Not equal to (!=)' },
]

type TabKey = 'devices' | 'mappings' | 'dashboards' | 'history' | 'alarms' | 'policies'
const VALID_TABS: TabKey[] = ['devices', 'mappings', 'dashboards', 'history', 'alarms', 'policies']

function resolveRouteTab(): TabKey {
  const q = typeof route.query.tab === 'string' ? route.query.tab : ''
  return (VALID_TABS as string[]).includes(q) ? (q as TabKey) : 'devices'
}

const activeTab = ref<TabKey>(resolveRouteTab())

const devices = ref<WebviewDevice[]>([])
const mappings = ref<RegisterMapping[]>([])
const dashboards = ref<WebviewDashboard[]>([])
const historyEntries = ref<HistoryEntry[]>([])
const alarmRules = ref<AlarmRule[]>([])
const members = ref<SiteMember[]>([])
const policies = ref<MemberWritePolicy[]>([])
const testResults = ref<Record<string, TelemetrySample>>({})
const testingId = ref<string | null>(null)

const deviceSelectOptions = computed<SelectOption<string>[]>(() =>
  devices.value.map((d) => ({ value: d.id, label: `${d.name} (${d.protocol})` })),
)

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
const devSubmitted = ref(false)
const devTouched = ref<Record<string, boolean>>({})

const devErrors = computed(() => ({
  name: validateRequiredText(devName.value, 'Device name', { min: 1, max: 100 }),
  host: validateIpAddress(devHost.value, 'IP address'),
  port: validatePort(devPort.value, 'Port'),
}))

function showDevErr(field: 'name' | 'host' | 'port'): string | null {
  return devSubmitted.value || devTouched.value[field] ? devErrors.value[field] : null
}

function openCreateDevice() {
  editingDeviceId.value = null
  devName.value = ''
  devHost.value = ''
  devPort.value = '502'
  devProto.value = 'MODBUS_TCP'
  devActive.value = true
  devSubmitted.value = false
  devTouched.value = {}
  deviceModalOpen.value = true
}

function openEditDevice(d: WebviewDevice) {
  editingDeviceId.value = d.id
  devName.value = d.name
  devHost.value = d.host
  devPort.value = String(d.port)
  devProto.value = d.protocol
  devActive.value = d.isActive
  devSubmitted.value = false
  devTouched.value = {}
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
const regSubmitted = ref(false)
const regTouched = ref<Record<string, boolean>>({})

const selectedRegDevice = computed(() => devices.value.find((d) => d.id === regDeviceId.value) ?? null)

const regErrors = computed(() => ({
  deviceId: regDeviceId.value ? null : 'Device is required.',
  name: validateRequiredText(regName.value, 'Register name', { min: 1, max: 100 }),
  slave: validateIntegerRange(regSlave.value, 'Slave address', 0, 255, { required: true }),
  addr: validateIntegerRange(regAddr.value, 'Register address', 0, 65535, { required: true }),
  unit: validateOptionalText(regUnit.value, 'Unit', 32),
  precision: validateIntegerRange(regPrecision.value, 'Precision', -5, 5, { required: false }),
}))

function showRegErr(field: 'deviceId' | 'name' | 'slave' | 'addr' | 'unit' | 'precision'): string | null {
  return regSubmitted.value || regTouched.value[field] ? regErrors.value[field] : null
}

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
  regSubmitted.value = false
  regTouched.value = {}
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
  regSubmitted.value = false
  regTouched.value = {}
  regModalOpen.value = true
}

// --- Dashboard Create/Edit Modal ---
const dashModalOpen = ref(false)
const editingDash = ref<WebviewDashboard | null>(null)
const selectedDashId = ref('')

function openCreateDashboard() {
  editingDash.value = null
  dashModalOpen.value = true
}

function openEditDashboard(d: WebviewDashboard) {
  editingDash.value = d
  dashModalOpen.value = true
}

// --- Widget Create/Edit Modal (WYSIWYG Tile Preview) ---
const widgetModalOpen = ref(false)
const editingWidget = ref<WebviewWidget | null>(null)

function openCreateWidget() {
  editingWidget.value = null
  widgetModalOpen.value = true
}

function openEditWidget(w: WebviewWidget) {
  editingWidget.value = w
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
const alarmSubmitted = ref(false)
const alarmTouched = ref<Record<string, boolean>>({})

const alarmBoundMapping = computed(() => (alarmMappingId.value ? mappingMap.value[alarmMappingId.value] ?? null : null))
const alarmNeedsDeadband = computed(() => alarmCondition.value !== 'EQ' && alarmCondition.value !== 'NEQ')

const alarmErrors = computed(() => ({
  name: validateRequiredText(alarmName.value, 'Rule name', { min: 1, max: 120 }),
  mappingId: alarmMappingId.value ? null : 'Please select a monitored register.',
  threshold: validateNumberRange(alarmThreshold.value, 'Threshold', undefined, undefined, { required: true }),
  deadband: alarmNeedsDeadband.value
    ? validateNumberRange(alarmDeadband.value || '0', 'Clear deadband', 0, undefined, { required: false })
    : null,
  holdSec: validateIntegerRange(alarmHoldSec.value || '0', 'Hold time', 0, 86400, { required: false }),
  pollSec: validateIntegerRange(alarmPollSec.value || '10', 'Poll interval', 1, 3600, { required: true }),
}))

function showAlarmErr(field: 'name' | 'mappingId' | 'threshold' | 'deadband' | 'holdSec' | 'pollSec'): string | null {
  return alarmSubmitted.value || alarmTouched.value[field] ? alarmErrors.value[field] : null
}

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
  alarmSubmitted.value = false
  alarmTouched.value = {}
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
  alarmSubmitted.value = false
  alarmTouched.value = {}
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

// Pagination for list/table tabs (25, 50, 100)
const devicesPag = usePagination(devices)
const mappingsPag = usePagination(mappings)
const historyPag = usePagination(historyEntries)
const alarmsPag = usePagination(alarmRules)
const policiesPag = usePagination(siteMembersOnly)

const selectedDashLayout = computed(() => resolveDashboardLayout(selectedDash.value?.layout))

// --- Drag-to-Reorder Dashboard Tabs ---
const draggedDashId = ref<string | null>(null)
const dragDashList = ref<WebviewDashboard[] | null>(null)
const orderedDashboards = computed(() => dragDashList.value ?? dashboards.value)

function onDashDragStart(e: DragEvent, dashId: string) {
  draggedDashId.value = dashId
  dragDashList.value = [...dashboards.value]
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', dashId)
  }
}

function onDashDragOver(targetId: string) {
  if (!draggedDashId.value || draggedDashId.value === targetId || !dragDashList.value) return
  const fromIdx = dragDashList.value.findIndex((d) => d.id === draggedDashId.value)
  const toIdx = dragDashList.value.findIndex((d) => d.id === targetId)
  if (fromIdx === -1 || toIdx === -1) return
  const next = [...dragDashList.value]
  const [moved] = next.splice(fromIdx, 1)
  if (!moved) return
  next.splice(toIdx, 0, moved)
  dragDashList.value = next
}

async function finishDashDrag() {
  const finalOrder = dragDashList.value
  draggedDashId.value = null
  dragDashList.value = null
  if (!finalOrder) return
  const changed =
    finalOrder.length === dashboards.value.length &&
    finalOrder.some((d, i) => d.id !== dashboards.value[i]?.id)
  if (!changed) return
  dashboards.value = finalOrder
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/reorder`, {
      method: 'POST',
      body: JSON.stringify({ dashboardIds: finalOrder.map((d) => d.id) }),
    })
    webviewStore.setSiteDashboards(siteId.value, finalOrder)
  } catch (err) {
    toast.error(err, 'Failed to reorder dashboards')
    await loadAll()
  }
}

function registerKeyLabel(m: RegisterMapping): string {
  const dev = deviceMap.value[m.deviceId]
  const a = m.protocolAttributes
  const devPart = dev ? `${dev.name} (${dev.protocol})` : 'Device'
  return `${devPart} / ${m.name} [unit=${a?.slaveAddress ?? 1} ${a?.functionCode ?? ''}@${a?.registerAddress ?? 0}]`
}

async function loadAll() {
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

    const queryDashId = typeof route.query.dashboard === 'string' ? route.query.dashboard : ''
    if (!selectedDashId.value && queryDashId && dashboards.value.some((d) => d.id === queryDashId)) {
      selectedDashId.value = queryDashId
    } else if (dashboards.value.length > 0 && !dashboards.value.some((d) => d.id === selectedDashId.value)) {
      selectedDashId.value = dashboards.value[0].id
    }
    if (!histMappingId.value && availableHistoryMappings.value.length > 0) {
      histMappingId.value = availableHistoryMappings.value[0].id
    }
  } catch (err) {
    toast.error(err, 'Failed to load WebView configuration')
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
watch(
  () => [route.query.tab, route.query.dashboard] as const,
  ([tabParam, dashParam]) => {
    if (typeof tabParam === 'string' && (VALID_TABS as string[]).includes(tabParam)) {
      activeTab.value = tabParam as TabKey
    }
    if (typeof dashParam === 'string' && dashParam && dashboards.value.some((d) => d.id === dashParam)) {
      selectedDashId.value = dashParam
    }
  },
)

// --- Device Actions ---
async function saveDevice() {
  devSubmitted.value = true
  const vErr = firstError(devErrors.value)
  if (vErr) {
    toast.error(vErr, 'Validation Error')
    return
  }
  try {
    if (editingDeviceId.value) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/devices/${editingDeviceId.value}`, {
        method: 'PATCH',
        body: JSON.stringify({
          name: devName.value.trim(),
          host: devHost.value.trim(),
          port: Number(devPort.value),
          protocol: devProto.value,
          isActive: devActive.value,
        }),
      })
      toast.success(`Updated device "${devName.value.trim()}".`)
    } else {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/devices`, {
        method: 'POST',
        body: JSON.stringify({
          name: devName.value.trim(),
          host: devHost.value.trim(),
          port: Number(devPort.value),
          protocol: devProto.value,
        }),
      })
      toast.success(`Added device "${devName.value.trim()}".`)
    }
    deviceModalOpen.value = false
    await loadAll()
  } catch (err) {
    toast.error(err, 'Failed to save device')
  }
}

function confirmDeleteDevice(d: WebviewDevice) {
  askDelete('Delete Device', d.name, 'All registers belonging to this device will also be deleted.', async () => {
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/devices/${d.id}`, { method: 'DELETE' })
      toast.success(`Deleted device "${d.name}".`)
      await loadAll()
    } catch (err) {
      toast.error(err, 'Failed to delete device')
    }
  })
}

// --- Register Actions ---
async function saveRegister() {
  regSubmitted.value = true
  const vErr = firstError(regErrors.value)
  if (vErr) {
    toast.error(vErr, 'Validation Error')
    return
  }
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
          name: regName.value.trim(),
          protocolAttributes: attrs,
          isActive: regActive.value,
        }),
      })
      toast.success(`Updated register "${regName.value.trim()}".`)
    } else {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/mappings`, {
        method: 'POST',
        body: JSON.stringify({
          deviceId: regDeviceId.value,
          name: regName.value.trim(),
          protocolAttributes: attrs,
        }),
      })
      toast.success(`Added register "${regName.value.trim()}".`)
    }
    regModalOpen.value = false
    await loadAll()
  } catch (err) {
    toast.error(err, 'Failed to save register')
  }
}

function confirmDeleteRegister(m: RegisterMapping) {
  askDelete('Delete Register', m.name, 'Widgets referencing this register will become unbound.', async () => {
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/mappings/${m.id}`, { method: 'DELETE' })
      toast.success(`Deleted register "${m.name}".`)
      await loadAll()
    } catch (err) {
      toast.error(err, 'Failed to delete register')
    }
  })
}

async function testRead(m: RegisterMapping) {
  testingId.value = m.id
  try {
    const sample = await apiRequest<TelemetrySample>(
      `/api/v1/webview/sites/${siteId.value}/operator/mappings/${m.id}/test-read`,
      { method: 'POST' },
    )
    testResults.value = { ...testResults.value, [m.id]: sample }
    if (sample.quality !== 'GOOD' && sample.error) {
      toast.warning(`${m.name}: ${sample.error}`, 'Test Read Failed')
    }
  } catch (err) {
    toast.error(err, 'Test read failed')
  } finally {
    testingId.value = null
  }
}

// --- Dashboard Actions + Reorder + Layout ---
async function saveDashboard(payload: DashboardOptionsPayload) {
  try {
    if (editingDash.value) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${editingDash.value.id}`, {
        method: 'PATCH',
        body: JSON.stringify({
          name: payload.name,
          isDefault: payload.isDefault,
          pollIntervalMs: payload.pollIntervalMs,
          layout: payload.layout,
        }),
      })
      toast.success(`Updated dashboard "${payload.name}".`)
    } else {
      const created = await apiRequest<WebviewDashboard>(`/api/v1/webview/sites/${siteId.value}/operator/dashboards`, {
        method: 'POST',
        body: JSON.stringify({
          name: payload.name,
          isDefault: payload.isDefault,
          pollIntervalMs: payload.pollIntervalMs,
          layout: payload.layout,
        }),
      })
      selectedDashId.value = created.id
      toast.success(`Created dashboard "${payload.name}".`)
    }
    dashModalOpen.value = false
    editingDash.value = null
    await loadAll()
  } catch (err) {
    toast.error(err, 'Failed to save dashboard')
  }
}

async function updateSelectedDashLayout(patch: { density?: TileDensity }) {
  const dash = selectedDash.value
  if (!dash) return
  const nextLayout = { ...resolveDashboardLayout(dash.layout), ...patch }
  dash.layout = nextLayout
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${dash.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ layout: nextLayout }),
    })
    webviewStore.setSiteDashboards(siteId.value, dashboards.value)
  } catch (err) {
    toast.error(err, 'Failed to update dashboard layout')
    await loadAll()
  }
}

function confirmDeleteDashboard(d: WebviewDashboard) {
  askDelete('Delete Dashboard', d.name, 'All tiles on this dashboard will be permanently removed.', async () => {
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${d.id}`, { method: 'DELETE' })
      selectedDashId.value = ''
      toast.success(`Deleted dashboard "${d.name}".`)
      await loadAll()
    } catch (err) {
      toast.error(err, 'Failed to delete dashboard')
    }
  })
}

// --- Widget Actions + Live Canvas Resize / Reorder ---
async function saveWidget(payload: WidgetDraftPayload) {
  if (!selectedDash.value) return
  try {
    if (payload.id) {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/widgets/${payload.id}`, {
        method: 'PATCH',
        body: JSON.stringify({
          name: payload.name,
          widgetType: payload.widgetType,
          registerMappingId: payload.registerMappingId || undefined,
          style: payload.style,
          extra: payload.extra,
        }),
      })
      toast.success(`Updated tile "${payload.name}".`)
    } else {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${selectedDash.value.id}/widgets`, {
        method: 'POST',
        body: JSON.stringify({
          name: payload.name,
          widgetType: payload.widgetType,
          registerMappingId: payload.registerMappingId || undefined,
          sortOrder: selectedDash.value.widgets.length,
          style: payload.style,
          extra: payload.extra,
        }),
      })
      toast.success(`Added tile "${payload.name}".`)
    }
    widgetModalOpen.value = false
    editingWidget.value = null
    await loadAll()
  } catch (err) {
    toast.error(err, 'Failed to save tile')
  }
}

async function resizeWidgetOnCanvas(w: WebviewWidget, nextSize: TileSize) {
  if (!selectedDash.value) return
  const nextStyle = { ...(w.style || {}), tileSize: nextSize }
  w.style = nextStyle
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/widgets/${w.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ style: nextStyle }),
    })
  } catch (err) {
    toast.error(err, 'Failed to resize tile')
    await loadAll()
  }
}

async function reorderWidgetsOnCanvas(orderedWidgets: WebviewWidget[]) {
  if (!selectedDash.value) return
  selectedDash.value.widgets = orderedWidgets.map((w, idx) => ({ ...w, sortOrder: idx }))
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/dashboards/${selectedDash.value.id}/widgets/reorder`, {
      method: 'POST',
      body: JSON.stringify({ widgetIds: orderedWidgets.map((w) => w.id) }),
    })
  } catch (err) {
    toast.error(err, 'Failed to reorder tiles')
    await loadAll()
  }
}

function confirmDeleteWidget(w: WebviewWidget) {
  widgetModalOpen.value = false
  askDelete('Remove Tile', w.name, 'This tile will be removed from the dashboard.', async () => {
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/widgets/${w.id}`, { method: 'DELETE' })
      toast.success(`Removed tile "${w.name}".`)
      await loadAll()
    } catch (err) {
      toast.error(err, 'Failed to remove tile')
    }
  })
}

// --- History Entry Actions ---
async function saveHistoryEntry() {
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
      toast.success('Updated history archival key.')
    } else {
      if (!histMappingId.value) {
        toast.error('Please select a register key.', 'Validation Error')
        return
      }
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/history-entries`, {
        method: 'POST',
        body: JSON.stringify({
          registerMappingId: histMappingId.value,
          pollIntervalSeconds: Number(histPollSec.value),
          retentionDays: Number(histRetentionDays.value),
        }),
      })
      toast.success('Added register to history archival.')
    }
    histModalOpen.value = false
    await loadAll()
  } catch (err) {
    toast.error(err, 'Failed to save history entry')
  }
}

function confirmDeleteHistoryEntry(he: HistoryEntry) {
  const m = mappingMap.value[he.registerMappingId]
  askDelete('Remove History Entry', m ? registerKeyLabel(m) : he.registerMappingId, 'Scheduled background polling for this register will stop unless a dashboard is open.', async () => {
    try {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/history-entries/${he.id}`, { method: 'DELETE' })
      toast.success('Removed history archival key.')
      await loadAll()
    } catch (err) {
      toast.error(err, 'Failed to remove history entry')
    }
  })
}

async function toggleMemberWrite(member: SiteMember, canWrite: boolean) {
  try {
    await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/member-policies`, {
      method: 'PUT',
      body: JSON.stringify({ userId: member.userId, canWrite }),
    })
    toast.success(`Updated write permission for ${member.email}.`)
    await loadAll()
  } catch (err) {
    toast.error(err, 'Failed to update member write policy')
  }
}

// --- Alarm Rule Actions ---
async function saveAlarmRule() {
  alarmSubmitted.value = true
  if (alarmEmailDraft.value.trim()) {
    addAlarmEmailsFromDraft()
    if (alarmEmailErr.value) {
      toast.error(alarmEmailErr.value, 'Validation Error')
      return
    }
  }
  const vErr = firstError(alarmErrors.value)
  if (vErr) {
    toast.error(vErr, 'Validation Error')
    return
  }
  const deadband = alarmNeedsDeadband.value ? Number(alarmDeadband.value || '0') : 0

  const payload = {
    registerMappingId: alarmMappingId.value,
    name: alarmName.value.trim(),
    condition: alarmCondition.value,
    threshold: Number(alarmThreshold.value),
    clearDeadband: deadband,
    holdSeconds: Math.round(Number(alarmHoldSec.value || '0')),
    pollIntervalSeconds: Math.round(Number(alarmPollSec.value || '10')),
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
      toast.success(`Updated alarm rule "${payload.name}".`)
    } else {
      await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/alarm-rules`, {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      toast.success(`Created alarm rule "${payload.name}".`)
    }
    alarmModalOpen.value = false
    await loadAll()
  } catch (err) {
    toast.error(err, 'Failed to save alarm rule')
  }
}

function confirmDeleteAlarmRule(r: AlarmRule) {
  askDelete(
    'Delete Alarm Rule',
    r.name,
    'Any active latched alarm for this rule will be cleared and background polling for this rule will stop.',
    async () => {
      try {
        await apiRequest(`/api/v1/webview/sites/${siteId.value}/operator/alarm-rules/${r.id}`, { method: 'DELETE' })
        toast.success(`Deleted alarm rule "${r.name}".`)
        await loadAll()
      } catch (err) {
        toast.error(err, 'Failed to delete alarm rule')
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

    <!-- TAB 3: DASHBOARDS & WIDGETS (Full-Width WYSIWYG Tile Studio) -->
    <div v-if="activeTab === 'dashboards'" class="space-y-4">
      <!-- Dashboard Selector Bar (Full-width strip, drag-to-reorder pages) -->
      <div class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-slate-800 bg-slate-900/60 p-3">
        <div
          class="no-scrollbar flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto"
          :class="draggedDashId ? 'select-none' : ''"
        >
          <button
            v-for="d in orderedDashboards"
            :key="d.id"
            type="button"
            draggable="true"
            title="Click to select - Drag to reorder pages"
            class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border px-2.5 text-xs font-medium transition"
            :class="[
              selectedDash?.id === d.id
                ? 'border-amber-500/60 bg-amber-500/15 text-amber-200'
                : 'border-slate-800 bg-slate-950/60 text-slate-300 hover:border-slate-700 hover:text-white',
              draggedDashId === d.id ? 'opacity-40 cursor-grabbing' : 'cursor-grab',
            ]"
            @dragstart="onDashDragStart($event, d.id)"
            @dragover.prevent="onDashDragOver(d.id)"
            @drop.prevent="finishDashDrag"
            @dragend="finishDashDrag"
            @click="selectedDashId = d.id"
          >
            <GripVertical class="h-3.5 w-3.5 shrink-0 opacity-45" />
            <Star v-if="d.isDefault" class="h-3 w-3 shrink-0 text-amber-400" title="Default dashboard" />
            <LayoutGrid v-else class="h-3.5 w-3.5 shrink-0 opacity-70" />
            <span>{{ d.name }}</span>
            <span class="rounded bg-slate-900/80 px-1.5 py-0.5 font-mono text-[10px] text-slate-400">
              {{ d.widgets.length }}
            </span>
          </button>

          <p v-if="dashboards.length === 0" class="px-2 text-xs text-slate-500">
            No dashboards created yet.
          </p>
        </div>

        <button
          type="button"
          class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg bg-amber-500 px-3 text-xs font-semibold text-slate-950 transition hover:bg-amber-400"
          @click="openCreateDashboard"
        >
          <Plus class="h-3.5 w-3.5" /> New Dashboard
        </button>
      </div>

      <!-- Empty Dashboards State -->
      <div
        v-if="!selectedDash"
        class="flex flex-col items-center justify-center rounded-xl border border-dashed border-slate-800 bg-slate-900/40 px-6 py-16 text-center"
      >
        <LayoutGrid class="h-9 w-9 text-slate-700" />
        <p class="mt-3 text-sm font-medium text-slate-300">Create your first dashboard</p>
        <p class="mt-1 max-w-md text-xs text-slate-500">
          Build live telemetry and control tile boards with drag-and-drop arrangement and modular tile sizes.
        </p>
        <button
          type="button"
          class="mt-4 inline-flex h-9 items-center gap-1.5 rounded-lg bg-amber-500 px-4 text-xs font-semibold text-slate-950 transition hover:bg-amber-400"
          @click="openCreateDashboard"
        >
          <Plus class="h-4 w-4" /> New Dashboard
        </button>
      </div>

      <!-- Selected Dashboard WYSIWYG Studio (100% Full Horizontal Width) -->
      <div v-else class="space-y-4">
        <!-- Studio Header & Live Layout Controls -->
        <div class="flex flex-col gap-3 rounded-xl border border-slate-800 bg-slate-900/60 p-3.5 sm:p-4">
          <div class="flex flex-wrap items-center justify-between gap-2.5">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <h3 class="truncate text-base font-semibold tracking-tight text-white">
                {{ selectedDash.name }}
              </h3>
              <span class="rounded bg-slate-800 px-2 py-0.5 font-mono text-[10px] text-slate-400">
                {{ selectedDash.pollIntervalMs || 1000 }} ms
              </span>
              <span
                v-if="selectedDash.isDefault"
                class="rounded-full border border-amber-500/35 bg-amber-500/15 px-2 py-0.5 text-[10px] font-semibold uppercase text-amber-200"
              >
                Default
              </span>
            </div>

            <div class="flex flex-wrap items-center gap-1.5">
              <button
                type="button"
                class="inline-flex h-8 items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-2.5 text-xs font-medium text-slate-200 transition hover:bg-slate-800"
                @click="openEditDashboard(selectedDash)"
              >
                <SlidersHorizontal class="h-3.5 w-3.5" /> Settings
              </button>
              <RouterLink
                :to="`/sites/${siteId}/webview/${selectedDash.id}`"
                class="inline-flex h-8 items-center gap-1.5 rounded-lg border border-sky-500/40 bg-sky-500/15 px-2.5 text-xs font-medium text-sky-200 transition hover:bg-sky-500/25"
              >
                <ExternalLink class="h-3.5 w-3.5" /> Live View
              </RouterLink>
              <button
                type="button"
                title="Delete dashboard"
                aria-label="Delete dashboard"
                class="flex h-8 w-8 items-center justify-center rounded-lg border border-rose-500/30 bg-rose-500/10 text-rose-300 transition hover:bg-rose-500/20"
                @click="confirmDeleteDashboard(selectedDash)"
              >
                <Trash2 class="h-3.5 w-3.5" />
              </button>
            </div>
          </div>

          <!-- Scale Quick Bar + Add Tile CTA -->
          <div class="flex flex-wrap items-center justify-between gap-3 border-t border-slate-800/80 pt-3 text-xs">
            <div class="flex items-center gap-1.5">
              <span class="text-[11px] font-medium text-slate-400">Scale:</span>
              <div class="inline-flex rounded-lg border border-slate-700/80 bg-slate-950/80 p-0.5 text-[11px]">
                <button
                  v-for="opt in [
                    { value: 'compact', label: 'Compact' },
                    { value: 'comfortable', label: 'Balanced' },
                    { value: 'spacious', label: 'Spacious' },
                  ] as const"
                  :key="opt.value"
                  type="button"
                  class="rounded-md px-2 py-1 font-medium transition"
                  :class="
                    selectedDashLayout.density === opt.value
                      ? 'bg-amber-500 text-slate-950'
                      : 'text-slate-300 hover:text-white'
                  "
                  @click="updateSelectedDashLayout({ density: opt.value })"
                >
                  {{ opt.label }}
                </button>
              </div>
            </div>

            <button
              type="button"
              class="inline-flex h-8 shrink-0 items-center gap-1 rounded-lg bg-amber-500 px-3 text-xs font-semibold text-slate-950 transition hover:bg-amber-400"
              @click="openCreateWidget"
            >
              <Plus class="h-3.5 w-3.5" /> Add Tile
            </button>
          </div>
        </div>

        <!-- Live End-Result Tile Canvas (Exact same grid & tiles as WebView) -->
        <DashboardTileGrid
          :widgets="selectedDash.widgets"
          :layout="selectedDash.layout"
          :mappings-by-id="mappingMap"
          :can-write="true"
          :editing="true"
          @add="openCreateWidget"
          @edit="openEditWidget"
          @delete="confirmDeleteWidget"
          @resize="resizeWidgetOnCanvas"
          @reorder="reorderWidgetsOnCanvas"
        />
      </div>
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
        <form novalidate class="max-h-[90dvh] w-full max-w-md overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6" @submit.prevent="saveDevice">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">{{ editingDeviceId ? 'Edit Device' : 'Add Device' }}</h3>
            <button type="button" aria-label="Close modal" class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white" @click="deviceModalOpen = false"><X class="h-4 w-4" /></button>
          </div>
          <div>
            <label class="mb-1 block text-xs text-slate-400">Device Name</label>
            <input
              v-model="devName"
              placeholder="PLC-1"
              class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-xs text-white focus:outline-none"
              :class="showDevErr('name') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
              @blur="devTouched.name = true"
            />
            <p v-if="showDevErr('name')" class="mt-1 text-[11px] text-rose-400">{{ showDevErr('name') }}</p>
          </div>
          <div class="grid gap-3 sm:grid-cols-3">
            <div class="sm:col-span-2">
              <label class="mb-1 block text-xs text-slate-400">IP Address</label>
              <input
                v-model="devHost"
                placeholder="172.21.0.2"
                class="w-full rounded-lg border bg-slate-950 px-3 py-2 font-mono text-xs text-white focus:outline-none"
                :class="showDevErr('host') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
                @blur="devTouched.host = true"
              />
              <p v-if="showDevErr('host')" class="mt-1 text-[11px] text-rose-400">{{ showDevErr('host') }}</p>
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Port</label>
              <NumericTextInput
                v-model="devPort"
                :min="1"
                :max="65535"
                required
                placeholder="502"
                :external-error="showDevErr('port')"
                @blur="devTouched.port = true"
              />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-xs text-slate-400">Protocol</label>
            <CustomSelect
              v-model="devProto"
              :options="PROTOCOL_OPTIONS"
              aria-label="Device protocol"
            />
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
        <form novalidate class="max-h-[90dvh] w-full max-w-lg overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6" @submit.prevent="saveRegister">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-white">{{ editingRegId ? 'Edit Register' : 'Add Register' }}</h3>
            <button type="button" aria-label="Close modal" class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white" @click="regModalOpen = false"><X class="h-4 w-4" /></button>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Device</label>
              <CustomSelect
                v-model="regDeviceId"
                :options="deviceSelectOptions"
                :invalid="Boolean(showRegErr('deviceId'))"
                aria-label="Register device"
              />
              <p v-if="showRegErr('deviceId')" class="mt-1 text-[11px] text-rose-400">{{ showRegErr('deviceId') }}</p>
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Register Name</label>
              <input
                v-model="regName"
                placeholder="Inlet Temp"
                class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-xs text-white focus:outline-none"
                :class="showRegErr('name') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
                @blur="regTouched.name = true"
              />
              <p v-if="showRegErr('name')" class="mt-1 text-[11px] text-rose-400">{{ showRegErr('name') }}</p>
            </div>
          </div>

          <!-- Dynamic Protocol Section: MODBUS_TCP -->
          <div v-if="!selectedRegDevice || selectedRegDevice.protocol === 'MODBUS_TCP'" class="space-y-3 rounded-xl border border-slate-800 bg-slate-950/60 p-4">
            <p class="text-[11px] font-semibold uppercase tracking-wider text-amber-300">Modbus TCP Attributes</p>
            <div class="grid gap-3 sm:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Slave Address (Unit ID)</label>
                <NumericTextInput
                  v-model="regSlave"
                  :min="0"
                  :max="255"
                  required
                  placeholder="1"
                  input-class="bg-slate-900"
                  :external-error="showRegErr('slave')"
                  @blur="regTouched.slave = true"
                />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Function Code</label>
                <CustomSelect
                  v-model="regFC"
                  :options="FC_OPTIONS"
                  aria-label="Modbus function code"
                  trigger-class="bg-slate-900"
                />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Register Address</label>
                <NumericTextInput
                  v-model="regAddr"
                  :min="0"
                  :max="65535"
                  required
                  placeholder="0"
                  input-class="bg-slate-900"
                  :external-error="showRegErr('addr')"
                  @blur="regTouched.addr = true"
                />
              </div>
            </div>

            <div class="grid gap-3 sm:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Data Type</label>
                <CustomSelect
                  v-model="regDataType"
                  :options="DATA_TYPE_OPTIONS"
                  :disabled="regFC === 'READ_COIL' || regFC === 'READ_DISCRETE'"
                  aria-label="Modbus data type"
                  trigger-class="bg-slate-900"
                />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Word Order</label>
                <CustomSelect
                  v-model="regWordOrder"
                  :options="WORD_ORDER_OPTIONS"
                  aria-label="Modbus word order"
                  trigger-class="bg-slate-900"
                />
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Access Permission</label>
                <CustomSelect
                  v-model="regAccess"
                  :options="ACCESS_OPTIONS"
                  :disabled="regFC === 'READ_DISCRETE' || regFC === 'READ_INPUT'"
                  aria-label="Modbus access permission"
                  trigger-class="bg-slate-900"
                />
              </div>
            </div>

            <div class="grid gap-3 border-t border-slate-800 pt-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Unit (optional)</label>
                <input
                  v-model="regUnit"
                  maxlength="32"
                  placeholder="e.g. C, bar, %"
                  class="w-full rounded-lg border bg-slate-900 px-3 py-2 text-xs text-white focus:outline-none"
                  :class="showRegErr('unit') ? 'border-rose-500' : 'border-slate-700 focus:border-amber-500'"
                  @blur="regTouched.unit = true"
                />
                <p v-if="showRegErr('unit')" class="mt-1 text-[11px] text-rose-400">{{ showRegErr('unit') }}</p>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Precision (10^-x, optional)</label>
                <NumericTextInput
                  v-model="regPrecision"
                  :min="-5"
                  :max="5"
                  allow-negative
                  placeholder="0"
                  input-class="bg-slate-900"
                  :external-error="showRegErr('precision')"
                  @blur="regTouched.precision = true"
                />
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

    <!-- MODAL: CREATE / EDIT DASHBOARD (Shared WYSIWYG Options) -->
    <DashboardOptionsModal
      :open="dashModalOpen"
      :dashboard="editingDash"
      :default-if-new="dashboards.length === 0"
      @close="dashModalOpen = false"
      @save="saveDashboard"
    />

    <!-- MODAL: CREATE / EDIT WIDGET TILE (Shared Live Preview Editor) -->
    <WidgetEditModal
      :open="widgetModalOpen"
      :widget="editingWidget"
      :devices="devices"
      :mappings="mappings"
      @close="widgetModalOpen = false"
      @save="saveWidget"
      @delete="confirmDeleteWidget"
    />

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
              <CustomSelect
                v-model="histPollSec"
                :options="HIST_POLL_OPTIONS"
                aria-label="History poll interval"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Retention</label>
              <CustomSelect
                v-model="histRetentionDays"
                :options="HIST_RETENTION_OPTIONS"
                aria-label="History retention"
              />
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
          novalidate
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
                maxlength="120"
                placeholder="e.g. Boiler High Temperature"
                class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-xs text-white focus:outline-none"
                :class="showAlarmErr('name') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
                @blur="alarmTouched.name = true"
              />
              <p v-if="showAlarmErr('name')" class="mt-1 text-[11px] text-rose-400">{{ showAlarmErr('name') }}</p>
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Severity</label>
              <CustomSelect
                v-model="alarmSeverity"
                :options="ALARM_SEVERITY_OPTIONS"
                aria-label="Alarm severity"
              />
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
            <p v-if="showAlarmErr('mappingId')" class="mt-1 text-[11px] text-rose-400">{{ showAlarmErr('mappingId') }}</p>
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
              <CustomSelect
                v-model="alarmCondition"
                :options="ALARM_CONDITION_OPTIONS"
                aria-label="Alarm condition"
              />
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
                :external-error="showAlarmErr('threshold')"
                @blur="alarmTouched.threshold = true"
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
                :external-error="showAlarmErr('deadband')"
                @blur="alarmTouched.deadband = true"
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
                :external-error="showAlarmErr('holdSec')"
                @blur="alarmTouched.holdSec = true"
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
                :external-error="showAlarmErr('pollSec')"
                @blur="alarmTouched.pollSec = true"
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
              class="rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400"
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
