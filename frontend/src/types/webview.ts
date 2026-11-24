export type DeviceProtocol = 'MODBUS_TCP'

export interface WebviewDevice {
  id: string
  siteId: string
  name: string
  host: string
  port: number
  protocol: DeviceProtocol
  isActive: boolean
  createdAt: string
}

export type ModbusFunctionCode = 'READ_COIL' | 'READ_DISCRETE' | 'READ_HOLDING' | 'READ_INPUT'
export type ModbusDataType = 'BOOL' | 'UINT16' | 'INT16' | 'UINT32' | 'INT32' | 'FLOAT32'
export type ModbusWordOrder = 'BIG' | 'LITTLE'
export type ModbusAccessMode = 'READ' | 'READ_WRITE'

export interface ModbusTCPAttributes {
  slaveAddress: number
  functionCode: ModbusFunctionCode
  registerAddress: number
  dataType: ModbusDataType
  wordOrder: ModbusWordOrder
  accessMode: ModbusAccessMode
  unit?: string
  precision?: number
}

export interface RegisterMapping {
  id: string
  deviceId: string
  siteId: string
  name: string
  protocolAttributes: ModbusTCPAttributes & Record<string, unknown>
  isActive: boolean
}

export interface HistoryEntry {
  id: string
  siteId: string
  registerMappingId: string
  pollIntervalSeconds: number
  retentionDays: number
  isActive: boolean
  createdAt: string
}

export type WidgetType = 'display' | 'lamp' | 'switch' | 'button' | 'number_input'
export type FontSizeOption = 'sm' | 'md' | 'lg' | 'xl'
export type LampMode = 'off' | 'blink' | 'on'
export type ColorOption = 'emerald' | 'sky' | 'rose' | 'amber' | 'slate'

export interface LampStateConfig {
  mode: LampMode
  color: ColorOption
  text: string
}

export interface WebviewWidget {
  id: string
  dashboardId: string
  registerMappingId?: string
  name: string
  widgetType: WidgetType
  sortOrder: number
  style: Record<string, unknown>
  extra: Record<string, unknown>
}

export interface WebviewDashboard {
  id: string
  siteId: string
  name: string
  layout: Record<string, unknown>
  isDefault: boolean
  sortOrder: number
  pollIntervalMs: number
  widgets: WebviewWidget[]
}

export interface WebviewWorkspace {
  canWrite: boolean
  dashboards: WebviewDashboard[]
  mappings: RegisterMapping[]
  devices: WebviewDevice[]
}

export interface MemberWritePolicy {
  userId: string
  canWrite: boolean
  updatedAt: string
}

export type TelemetryQuality = 'GOOD' | 'STALE' | 'BAD'

export interface TelemetrySample {
  siteId: string
  mappingId: string
  value: number | null
  quality: TelemetryQuality
  error?: string
  timestamp: string
}

export interface HistoryKeyInfo {
  entryId: string
  registerMappingId: string
  deviceId: string
  deviceName: string
  protocol: string
  registerName: string
  dataType: string
  unit: string
  precision: number
  pollIntervalSeconds: number
  retentionDays: number
  isActive: boolean
}

export interface HistoryPoint {
  time: string
  value: number | null
  min?: number | null
  max?: number | null
  count: number
}

export interface HistorySeries {
  registerMappingId: string
  deviceName: string
  registerName: string
  unit: string
  precision: number
  points: HistoryPoint[]
}

export interface HistoryQueryResponse {
  resolution: string
  from: string
  to: string
  series: HistorySeries[]
}

export type AlarmCondition = 'GT' | 'GTE' | 'LT' | 'LTE' | 'EQ' | 'NEQ'
export type AlarmSeverity = 'INFO' | 'WARNING' | 'CRITICAL'
export type AlarmState = 'ACTIVE' | 'CLEARED'
export type AlarmRuntimeStatus = 'NORMAL' | 'PENDING' | 'ACTIVE'

export interface AlarmRule {
  id: string
  siteId: string
  registerMappingId: string
  registerName: string
  deviceName: string
  unit: string
  precision: number
  name: string
  condition: AlarmCondition
  threshold: number
  clearDeadband: number
  holdSeconds: number
  pollIntervalSeconds: number
  severity: AlarmSeverity
  notifyEmails: string[]
  notifyOnClear: boolean
  isActive: boolean
  runtimeStatus: AlarmRuntimeStatus
  createdAt: string
  updatedAt: string
}

export interface AlarmEvent {
  id: string
  siteId: string
  siteName: string
  ruleId?: string
  registerMappingId?: string
  ruleName: string
  registerName: string
  condition: AlarmCondition
  threshold: number
  unit: string
  severity: AlarmSeverity
  state: AlarmState
  isStale: boolean
  triggeredAt: string
  triggeredValue: number
  clearedAt?: string
  clearedValue?: number
  acknowledgedAt?: string
  acknowledgedByUserId?: string
  acknowledgedByEmail?: string
  ackNote?: string
  notifiedAt?: string
  notifyFailed: boolean
  notifyError?: string
}

export interface AlarmSummary {
  activeCount: number
  activeCritical: number
  activeWarning: number
  activeInfo: number
  unacknowledgedCount: number
}

export interface AlarmEventsResponse {
  events: AlarmEvent[]
  summary: AlarmSummary
}

export interface GlobalAlarmSummaryResponse {
  summary: AlarmSummary
  bySite: Record<string, number>
}
