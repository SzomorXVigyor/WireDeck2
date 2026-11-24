import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiRequest } from '../services/api'
import type { AlarmSummary, GlobalAlarmSummaryResponse } from '../types/webview'

const emptySummary: AlarmSummary = {
  activeCount: 0,
  activeCritical: 0,
  activeWarning: 0,
  activeInfo: 0,
  unacknowledgedCount: 0,
}

export const useAlarmStore = defineStore('alarms', () => {
  const summary = ref<AlarmSummary>({ ...emptySummary })
  const bySite = ref<Record<string, number>>({})
  let pollTimer: number | null = null

  async function fetchSummary() {
    try {
      const res = await apiRequest<GlobalAlarmSummaryResponse>('/api/v1/webview/alarms/summary')
      summary.value = res.summary || { ...emptySummary }
      bySite.value = res.bySite || {}
    } catch {
      // Ignore transient network errors during background badge polling
    }
  }

  function startPolling() {
    if (pollTimer !== null) return
    void fetchSummary()
    pollTimer = window.setInterval(() => {
      if (!document.hidden) {
        void fetchSummary()
      }
    }, 15000)
  }

  function stopPolling() {
    if (pollTimer !== null) {
      window.clearInterval(pollTimer)
      pollTimer = null
    }
    summary.value = { ...emptySummary }
    bySite.value = {}
  }

  function siteActiveCount(siteId: string): number {
    return bySite.value[siteId] || 0
  }

  return {
    summary,
    bySite,
    fetchSummary,
    startPolling,
    stopPolling,
    siteActiveCount,
  }
})
