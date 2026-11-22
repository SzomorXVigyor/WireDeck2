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

export interface RunnerRecord {
  id: string
  siteId: string
  containerName: string
  containerId?: string
  image: string
  confFingerprint?: string
  desiredState: 'RUNNING' | 'STOPPED'
  observedState?: string
  status: SiteStatus
  handshakeState: string
  handshakeAgeSeconds?: number
  lastHandshakeAt?: string
  lastCheckedAt?: string
  lastError?: string
  lastReconciledAt?: string
}

export interface AuthResponse {
  accessToken: string
  expiresAt: string
  user: User
}
