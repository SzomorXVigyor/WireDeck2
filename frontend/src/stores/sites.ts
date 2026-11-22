import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiRequest } from '../services/api'
import type { Site } from '../types/iam'
import { useAuthStore } from './auth'

export const useSiteStore = defineStore('sites', () => {
  const sites = ref<Site[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  let pollTimer: number | null = null

  async function fetchSites() {
    loading.value = true
    error.value = null
    try {
      const res = await apiRequest<{ sites: Site[] }>('/api/v1/iam/sites')
      sites.value = res.sites || []
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load sites'
    } finally {
      loading.value = false
    }
  }

  function startPolling() {
    stopPolling()
    void fetchSites()
    pollTimer = window.setInterval(() => {
      void fetchSites()
    }, 30_000)
  }

  function stopPolling() {
    if (pollTimer !== null) {
      window.clearInterval(pollTimer)
      pollTimer = null
    }
  }

  function byId(siteId: string): Site | undefined {
    return sites.value.find((s) => s.id === siteId)
  }

  // Site Operator (or Platform Admin) can manage site members, VNC edge points, and WebView config.
  function canOperateSite(siteId: string): boolean {
    const auth = useAuthStore()
    if (auth.isPlatformAdmin) return true
    return byId(siteId)?.role === 'SITE_OPERATOR'
  }

  return {
    sites,
    loading,
    error,
    fetchSites,
    startPolling,
    stopPolling,
    byId,
    canOperateSite,
  }
})
