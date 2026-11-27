export type UserStatus = 'INVITED' | 'ACTIVE' | 'DISABLED'
export type SiteRole = 'SITE_OPERATOR' | 'SITE_MEMBER'
export type SiteStatus = 'ONLINE' | 'DEGRADED' | 'OFFLINE' | 'UNKNOWN'

export interface User {
  id: string
  email: string
  status: UserStatus
  isPlatformAdmin: boolean
  createdAt: string
}

export interface WireguardMeta {
  publicKey: string
  clientPublicKey: string
  endpointHost: string
  endpointPort: number
  lanSubnets: string[]
  confFingerprint: string
  mtu?: number
  persistentKeepalive: number
}

export interface Site {
  id: string
  name: string
  slug: string
  role?: SiteRole
  vncEnabled: boolean
  webviewEnabled: boolean
  isActive: boolean
  status: SiteStatus
  handshakeState: string
  lastHandshakeAt?: string
  wireguard?: WireguardMeta
  createdAt: string
}

export interface SiteMember {
  userId: string
  email: string
  status: UserStatus
  role: SiteRole
  createdAt: string
}

export interface RunnerPeerStats {
  publicKey: string
  rxBytes: number
  txBytes: number
}

export interface RunnerStats {
  interface?: string
  rxBytes: number
  txBytes: number
  rxBytesPerSec: number
  txBytesPerSec: number
  uptimeSeconds: number
  activeStreams: number
  maxStreams: number
  dialErrorsTotal: number
  streamsRejectedTotal: number
  peers?: RunnerPeerStats[]
  collectedAt: string
}

export interface RunnerRuntime {
  siteId: string
  containerId?: string
  observedState?: string
  configured: boolean
  status: SiteStatus
  handshakeState: string
  handshakeAgeSeconds?: number
  lastHandshakeAt?: string
  lastCheckedAt?: string
  lastReconciledAt?: string
  lastError?: string
  stats?: RunnerStats
}

export interface RunnerRecord {
  id: string
  siteId: string
  containerName: string
  containerId?: string
  image: string
  confFingerprint?: string
  desiredState: 'RUNNING' | 'STOPPED'
  observedState?: string
  configured?: boolean
  status: SiteStatus
  handshakeState: string
  handshakeAgeSeconds?: number
  lastHandshakeAt?: string
  lastCheckedAt?: string
  lastError?: string
  lastReconciledAt?: string
  stats?: RunnerStats
}

export interface RunnerDetailResponse {
  runner: RunnerRecord
  site?: Site
}

export interface DeviceRuntimeState {
  deviceId: string
  name: string
  healthy: boolean
  failures: number
  nextRetryAt?: string
  inFlight: boolean
  lastPolledAt?: string
}

export interface WebviewActiveSession {
  clientId: string
  userId: string
  userEmail?: string
  connectedAt: string
  subscribedCount: number
  pollIntervalMs: number
}

export interface SiteCacheMetrics {
  webviewSampleCount: number
  goodSamples: number
  staleSamples: number
  badSamples: number
  sampleCacheTtlSeconds: number
  authzCacheEntries: number
}

export interface WebviewRuntime {
  siteId: string
  activeWsClients: number
  subscribedMappings: number
  sessions?: WebviewActiveSession[]
  devices?: DeviceRuntimeState[]
  updatedAt: string
}

export interface VncActiveSession {
  sessionId: string
  siteId: string
  targetId: string
  targetName: string
  userId: string
  userEmail?: string
  startedAt: string
  lastActivityAt: string
  bytesIn: number
  bytesOut: number
  updatedAt: string
}

export interface AlarmPendingRecord {
  ruleId: string
  siteId: string
  sinceAt: string
  lastValue: number
}

export interface SiteRuntimeMetricsResponse {
  site: Site
  webview?: WebviewRuntime
  cache: SiteCacheMetrics
  vncActiveSessions: VncActiveSession[]
  pendingAlarms: AlarmPendingRecord[]
  collectedAt: string
}

export interface AuthResponse {
  accessToken: string
  expiresAt: string
  user: User
}
