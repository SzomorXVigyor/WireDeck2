export interface WebviewDevice {
  id: string
  siteId: string
  name: string
  host: string
  port: number
  protocol: 'MODBUS_TCP' | 'MODBUS_UDP'
  unitId: number
  maxConcurrentRequests: number
  requestTimeoutMs: number
  isActive: boolean
  createdAt: string
}

export type FunctionCode = 'READ_COIL' | 'READ_DISCRETE' | 'READ_HOLDING' | 'READ_INPUT'
export type DataType = 'BOOL' | 'UINT16' | 'INT16' | 'UINT32' | 'INT32' | 'FLOAT32'
export type AccessMode = 'READ' | 'WRITE' | 'READ_WRITE'
export type WriteFunctionCode = '' | 'WRITE_COIL' | 'WRITE_REGISTER' | 'WRITE_MULTIPLE_REGISTERS'
export type WidgetType = 'lamp' | 'display' | 'switch' | 'button' | 'number_input' | 'chart'

export interface RegisterMapping {
  id: string
  deviceId: string
  siteId: string
  name: string
  functionCode: FunctionCode
  registerAddress: number
  registerCount: number
  dataType: DataType
  wordOrder: 'BIG' | 'LITTLE'
  byteOrder: 'BIG' | 'LITTLE'
  scale: number
  offset: number
  unit: string
  accessMode: AccessMode
  writeFunctionCode: WriteFunctionCode
  interactivePollIntervalMs: number
  archivalPollIntervalMs: number
  deadband: number
  maxIntervalSeconds: number
  isActive: boolean
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
  widgets: WebviewWidget[]
}

export interface WebviewWorkspace {
  historyEnabled: boolean
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

export interface LivePoint {
  ts: number
  value: number
}
