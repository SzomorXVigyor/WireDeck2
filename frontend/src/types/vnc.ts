export interface VncTarget {
  id: string
  siteId: string
  name: string
  host: string
  port: number
  hasPassword: boolean
  isActive: boolean
  createdAt: string
}

export interface VncSession {
  id: string
  targetId: string
  targetName: string
  userId: string
  startedAt: string
  endedAt?: string
  bytesIn: number
  bytesOut: number
  closeReason?: string
}
