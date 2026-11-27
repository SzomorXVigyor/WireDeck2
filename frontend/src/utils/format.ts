export function formatBytes(bytes?: number | null): string {
  if (bytes === undefined || bytes === null || !Number.isFinite(bytes) || bytes < 0) {
    return '-'
  }
  if (bytes < 1024) return `${Math.round(bytes)} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let val = bytes / 1024
  let idx = 0
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx++
  }
  return `${val >= 100 ? val.toFixed(0) : val >= 10 ? val.toFixed(1) : val.toFixed(2)} ${units[idx]}`
}

export function formatByteRate(bytesPerSec?: number | null): string {
  if (bytesPerSec === undefined || bytesPerSec === null || !Number.isFinite(bytesPerSec) || bytesPerSec < 0) {
    return '-'
  }
  if (bytesPerSec < 1024) return `${bytesPerSec < 10 && bytesPerSec > 0 ? bytesPerSec.toFixed(1) : Math.round(bytesPerSec)} B/s`
  const units = ['KB/s', 'MB/s', 'GB/s']
  let val = bytesPerSec / 1024
  let idx = 0
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx++
  }
  return `${val >= 100 ? val.toFixed(0) : val >= 10 ? val.toFixed(1) : val.toFixed(2)} ${units[idx]}`
}

export function formatDurationSeconds(totalSeconds?: number | null): string {
  if (totalSeconds === undefined || totalSeconds === null || !Number.isFinite(totalSeconds) || totalSeconds < 0) {
    return '-'
  }
  const s = Math.floor(totalSeconds)
  if (s < 60) return `${s}s`
  const days = Math.floor(s / 86400)
  const hours = Math.floor((s % 86400) / 3600)
  const mins = Math.floor((s % 3600) / 60)
  const secs = s % 60
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m ${secs}s`
}

export function formatElapsedSince(iso?: string | null, nowMs: number = Date.now()): string {
  if (!iso) return '-'
  const ts = Date.parse(iso)
  if (Number.isNaN(ts)) return '-'
  const diffSec = Math.max(0, Math.floor((nowMs - ts) / 1000))
  return formatDurationSeconds(diffSec)
}

export function formatDateTime(iso?: string | null): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}

export function formatHandshakeState(state?: string | null): string {
  if (!state) return 'UNSPECIFIED'
  return state.replace(/^HANDSHAKE_STATE_/, '').replace(/_/g, ' ')
}

export function shortKey(key?: string | null, head = 12, tail = 8): string {
  if (!key) return '-'
  if (key.length <= head + tail + 3) return key
  return `${key.slice(0, head)}...${key.slice(-tail)}`
}
