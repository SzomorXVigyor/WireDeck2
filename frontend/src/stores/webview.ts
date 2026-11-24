import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'
import { apiRequest } from '../services/api'
import type { AlarmEvent, TelemetrySample, WebviewDashboard, WebviewWorkspace } from '../types/webview'
import { useAlarmStore } from './alarms'

export const useWebviewStore = defineStore('webview', () => {
  const connectionState = ref<'disconnected' | 'connecting' | 'connected' | 'error'>('disconnected')
  const connectionError = ref('')
  const samples = shallowRef<Record<string, TelemetrySample>>({})
  const siteDashboards = ref<Record<string, WebviewDashboard[]>>({})
  const alarmListeners = new Set<(ev: AlarmEvent) => void>()

  function subscribeAlarmEvents(cb: (ev: AlarmEvent) => void): () => void {
    alarmListeners.add(cb)
    return () => {
      alarmListeners.delete(cb)
    }
  }

  let activeSiteId = ''
  let subscribedIds: string[] = []
  let activePollIntervalMs = 1000
  let ws: WebSocket | null = null
  let pingTimer: number | null = null
  let reconnectTimer: number | null = null
  let intentionalClose = false
  let visibilityBound = false

  async function fetchSiteDashboards(siteId: string): Promise<WebviewDashboard[]> {
    if (!siteId) return []
    try {
      const wsData = await apiRequest<WebviewWorkspace>(`/api/v1/webview/sites/${siteId}/workspace`)
      const list = wsData.dashboards || []
      siteDashboards.value = { ...siteDashboards.value, [siteId]: list }
      return list
    } catch {
      return siteDashboards.value[siteId] || []
    }
  }

  function setSiteDashboards(siteId: string, list: WebviewDashboard[]) {
    siteDashboards.value = { ...siteDashboards.value, [siteId]: list }
  }

  function handleVisibilityChange() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return
    if (document.hidden) {
      ws.send(JSON.stringify({ type: 'unsubscribe' }))
    } else if (subscribedIds.length > 0) {
      ws.send(JSON.stringify({ type: 'subscribe', mappingIds: subscribedIds, pollIntervalMs: activePollIntervalMs }))
    }
  }

  function applySamples(list: TelemetrySample[]) {
    if (!list || list.length === 0) return
    const nextSamples = { ...samples.value }
    for (const s of list) {
      nextSamples[s.mappingId] = s
    }
    samples.value = nextSamples
  }

  function clearTimers() {
    if (pingTimer !== null) {
      window.clearInterval(pingTimer)
      pingTimer = null
    }
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function disconnect() {
    intentionalClose = true
    clearTimers()
    if (visibilityBound) {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      visibilityBound = false
    }
    if (ws) {
      try {
        ws.close()
      } catch {
        // ignore teardown error
      }
      ws = null
    }
    activeSiteId = ''
    subscribedIds = []
    connectionState.value = 'disconnected'
    connectionError.value = ''
  }

  async function openSocket(siteId: string) {
    clearTimers()
    if (ws) {
      try {
        ws.close()
      } catch {
        // ignore
      }
      ws = null
    }

    connectionState.value = 'connecting'
    connectionError.value = ''
    try {
      const ticket = await apiRequest<{ ticket: string; expiresAt: string }>(
        `/api/v1/webview/sites/${siteId}/ws-ticket`,
        { method: 'POST' },
      )
      if (intentionalClose || activeSiteId !== siteId) return

      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${proto}//${window.location.host}/api/v1/webview/stream?ticket=${encodeURIComponent(ticket.ticket)}`
      const sock = new WebSocket(url)
      ws = sock

      sock.onopen = () => {
        if (ws !== sock) return
        connectionState.value = 'connected'
        connectionError.value = ''
        if (!document.hidden && subscribedIds.length > 0) {
          sock.send(JSON.stringify({ type: 'subscribe', mappingIds: subscribedIds, pollIntervalMs: activePollIntervalMs }))
        }
        pingTimer = window.setInterval(() => {
          if (sock.readyState === WebSocket.OPEN) {
            sock.send(JSON.stringify({ type: 'ping' }))
          }
        }, 15000)
      }

      sock.onmessage = (evt) => {
        try {
          const msg = JSON.parse(String(evt.data)) as {
            type?: string
            samples?: TelemetrySample[]
            alarm?: AlarmEvent
          }
          if ((msg.type === 'snapshot' || msg.type === 'value_update') && Array.isArray(msg.samples)) {
            applySamples(msg.samples)
          } else if (msg.type === 'alarm_event' && msg.alarm) {
            for (const cb of alarmListeners) {
              cb(msg.alarm)
            }
            void useAlarmStore().fetchSummary()
          }
        } catch {
          // ignore malformed frames
        }
      }

      sock.onerror = () => {
        connectionError.value = 'Telemetry WebSocket error'
      }

      sock.onclose = () => {
        if (ws === sock) {
          ws = null
        }
        clearTimers()
        if (!intentionalClose && activeSiteId === siteId) {
          connectionState.value = 'error'
          reconnectTimer = window.setTimeout(() => {
            if (!intentionalClose && activeSiteId === siteId) {
              void openSocket(siteId)
            }
          }, 3000)
        }
      }
    } catch (err) {
      connectionState.value = 'error'
      connectionError.value = err instanceof Error ? err.message : 'Failed to connect telemetry stream'
      if (!intentionalClose && activeSiteId === siteId) {
        reconnectTimer = window.setTimeout(() => {
          if (!intentionalClose && activeSiteId === siteId) {
            void openSocket(siteId)
          }
        }, 4000)
      }
    }
  }

  function connect(siteId: string, mappingIds: string[], pollIntervalMs = 1000) {
    const uniqueIds = Array.from(new Set(mappingIds.filter(Boolean)))
    const clampedPoll = Math.max(1000, Math.min(60000, Math.round(pollIntervalMs || 1000)))
    activePollIntervalMs = clampedPoll

    if (activeSiteId !== siteId) {
      disconnect()
      samples.value = {}
      intentionalClose = false
      activeSiteId = siteId
      subscribedIds = uniqueIds
      activePollIntervalMs = clampedPoll
      if (!visibilityBound) {
        document.addEventListener('visibilitychange', handleVisibilityChange)
        visibilityBound = true
      }
      void openSocket(siteId)
      return
    }

    subscribedIds = uniqueIds
    if (ws && ws.readyState === WebSocket.OPEN && !document.hidden) {
      ws.send(JSON.stringify({ type: 'unsubscribe' }))
      if (uniqueIds.length > 0) {
        ws.send(JSON.stringify({ type: 'subscribe', mappingIds: uniqueIds, pollIntervalMs: clampedPoll }))
      }
    }
  }

  async function writeValue(siteId: string, mappingId: string, value: number, pulseMs?: number) {
    await apiRequest(`/api/v1/webview/sites/${siteId}/write`, {
      method: 'POST',
      body: JSON.stringify({
        mappingId,
        value,
        ...(pulseMs && pulseMs > 0 ? { pulseMs } : {}),
      }),
    })
  }

  return {
    connectionState,
    connectionError,
    samples,
    siteDashboards,
    fetchSiteDashboards,
    setSiteDashboards,
    subscribeAlarmEvents,
    connect,
    disconnect,
    writeValue,
  }
})
