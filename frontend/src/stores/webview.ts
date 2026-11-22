import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'
import { apiRequest } from '../services/api'
import type { LivePoint, TelemetrySample } from '../types/webview'

const MAX_RING_POINTS = 900 // D18: 900-pt client-side live ring buffer

export const useWebviewStore = defineStore('webview', () => {
  const connectionState = ref<'disconnected' | 'connecting' | 'connected' | 'error'>('disconnected')
  const connectionError = ref('')
  const samples = shallowRef<Record<string, TelemetrySample>>({})
  const ringBuffers = shallowRef<Record<string, LivePoint[]>>({})

  let activeSiteId = ''
  let subscribedIds: string[] = []
  let ws: WebSocket | null = null
  let pingTimer: number | null = null
  let reconnectTimer: number | null = null
  let intentionalClose = false
  let visibilityBound = false

  function handleVisibilityChange() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return
    if (document.hidden) {
      ws.send(JSON.stringify({ type: 'unsubscribe' }))
    } else if (subscribedIds.length > 0) {
      ws.send(JSON.stringify({ type: 'subscribe', mappingIds: subscribedIds }))
    }
  }

  function applySamples(list: TelemetrySample[]) {
    if (!list || list.length === 0) return
    const nextSamples = { ...samples.value }
    const nextRings = { ...ringBuffers.value }
    let ringDirty = false

    for (const s of list) {
      nextSamples[s.mappingId] = s
      if (s.quality === 'GOOD' && s.value !== null && Number.isFinite(s.value)) {
        const prev = nextRings[s.mappingId] ? [...nextRings[s.mappingId]] : []
        prev.push({ ts: Date.parse(s.timestamp) || Date.now(), value: s.value })
        if (prev.length > MAX_RING_POINTS) {
          prev.splice(0, prev.length - MAX_RING_POINTS)
        }
        nextRings[s.mappingId] = prev
        ringDirty = true
      }
    }

    samples.value = nextSamples
    if (ringDirty) {
      ringBuffers.value = nextRings
    }
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
          sock.send(JSON.stringify({ type: 'subscribe', mappingIds: subscribedIds }))
        }
        pingTimer = window.setInterval(() => {
          if (sock.readyState === WebSocket.OPEN) {
            sock.send(JSON.stringify({ type: 'ping' }))
          }
        }, 15000)
      }

      sock.onmessage = (evt) => {
        try {
          const msg = JSON.parse(String(evt.data)) as { type?: string; samples?: TelemetrySample[] }
          if ((msg.type === 'snapshot' || msg.type === 'value_update') && Array.isArray(msg.samples)) {
            applySamples(msg.samples)
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

  function connect(siteId: string, mappingIds: string[]) {
    const uniqueIds = Array.from(new Set(mappingIds.filter(Boolean)))
    if (activeSiteId !== siteId) {
      disconnect()
      samples.value = {}
      ringBuffers.value = {}
      intentionalClose = false
      activeSiteId = siteId
      subscribedIds = uniqueIds
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
        ws.send(JSON.stringify({ type: 'subscribe', mappingIds: uniqueIds }))
      }
    }
  }

  async function writeValue(siteId: string, mappingId: string, value: number) {
    await apiRequest(`/api/v1/webview/sites/${siteId}/write`, {
      method: 'POST',
      body: JSON.stringify({ mappingId, value }),
    })
  }

  return {
    connectionState,
    connectionError,
    samples,
    ringBuffers,
    connect,
    disconnect,
    writeValue,
  }
})
