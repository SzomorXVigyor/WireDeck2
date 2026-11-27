<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  Check,
  Copy,
  Cpu,
  Database,
  ExternalLink,
  Gauge,
  Layers,
  Minus,
  Monitor,
  Pause,
  Play,
  Plus,
  Power,
  Radio,
  RefreshCw,
  Settings,
  ShieldCheck,
  Trash2,
  Users,
} from 'lucide-vue-next'
import { ApiError, apiRequest } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { validateCidrList, validateRequiredText } from '../../utils/validation'
import type { Site, SiteMember, SiteRole, SiteRuntimeMetricsResponse, User } from '../../types/iam'
import type { AlarmSummary } from '../../types/webview'
import {
  formatBytes,
  formatDateTime,
  formatDurationSeconds,
  formatElapsedSince,
  shortKey,
} from '../../utils/format'
import StatusBadge from '../../components/StatusBadge.vue'
import ConfirmActionModal from '../../components/ConfirmActionModal.vue'
import CustomSelect, { type SelectOption } from '../../components/CustomSelect.vue'
import WireguardConfEditor from '../../components/WireguardConfEditor.vue'
import DeleteConfirmModal from '../../components/DeleteConfirmModal.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

type SiteTab = 'metrics' | 'services' | 'wireguard' | 'members' | 'settings'
type ServiceKey = 'webviewEnabled' | 'vncEnabled'

interface PlatformServiceDef {
  key: ServiceKey
  name: string
  description: string
  badgeClass: string
}

const PLATFORM_SERVICES: PlatformServiceDef[] = [
  {
    key: 'vncEnabled',
    name: 'WebVNC',
    description: 'Browser remote VNC access with server-side authentication',
    badgeClass: 'bg-emerald-500/15 text-emerald-300',
  },
  {
    key: 'webviewEnabled',
    name: 'WebView',
    description: 'Modbus dashboards, controls, TimescaleDB history, and alarms',
    badgeClass: 'bg-sky-500/15 text-sky-300',
  },
]

const SITE_ROLE_OPTIONS: SelectOption<SiteRole>[] = [
  { value: 'SITE_MEMBER', label: 'SITE_MEMBER' },
  { value: 'SITE_OPERATOR', label: 'SITE_OPERATOR' },
]

const TABS: { id: SiteTab; label: string; icon: typeof Activity }[] = [
  { id: 'metrics', label: 'Metrics', icon: Activity },
  { id: 'services', label: 'Services', icon: Layers },
  { id: 'wireguard', label: 'WireGuard', icon: ShieldCheck },
  { id: 'members', label: 'Members', icon: Users },
  { id: 'settings', label: 'Settings', icon: Settings },
]

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const siteId = computed(() => String(route.params.siteId || ''))

function parseTab(val: unknown): SiteTab {
  const s = typeof val === 'string' ? val : ''
  if (s === 'services' || s === 'wireguard' || s === 'members' || s === 'settings') return s
  return 'metrics'
}

const activeTab = ref<SiteTab>(parseTab(route.query.tab))

function setTab(tab: SiteTab) {
  activeTab.value = tab
  void router.replace({ query: { ...route.query, tab } })
  if (tab === 'members' && !membersLoaded.value) {
    void loadMembersAndUsers()
  }
  if (tab === 'metrics') {
    void loadMetrics(true)
    startPolling()
  } else {
    stopPolling()
  }
}

watch(
  () => route.query.tab,
  (q) => {
    const next = parseTab(q)
    if (next !== activeTab.value) {
      activeTab.value = next
      if (next === 'members' && !membersLoaded.value) {
        void loadMembersAndUsers()
      }
    }
  },
)

// Core site + live runtime metrics state
const siteFull = ref<Site | null>(null)
const data = ref<SiteRuntimeMetricsResponse | null>(null)
const alarmSummary = ref<AlarmSummary>({
  activeCount: 0,
  activeCritical: 0,
  activeWarning: 0,
  activeInfo: 0,
  unacknowledgedCount: 0,
})
const loading = ref(true)
const refreshing = ref(false)
const loadError = ref<string | null>(null)
const livePolling = ref(true)
const nowMs = ref(Date.now())
const copiedKey = ref<string | null>(null)

// Services & Settings edit state
const editName = ref('')
const editNameSubmitted = ref(false)
const editNameTouched = ref(false)
const savingSettings = ref(false)
const togglingActive = ref(false)
const showDeleteModal = ref(false)

interface PendingConfirmAction {
  title: string
  itemName: string
  description: string
  confirmLabel: string
  tone: 'warning' | 'danger' | 'success' | 'primary'
  run: () => Promise<void>
}
const pendingConfirm = ref<PendingConfirmAction | null>(null)
const confirmingAction = ref(false)

async function executePendingConfirm() {
  if (!pendingConfirm.value || confirmingAction.value) return
  const action = pendingConfirm.value
  confirmingAction.value = true
  try {
    await action.run()
    pendingConfirm.value = null
  } finally {
    confirmingAction.value = false
  }
}

const editNameErr = computed(() =>
  editNameSubmitted.value || editNameTouched.value
    ? validateRequiredText(editName.value, 'Site name', { min: 2, max: 100 })
    : null,
)

// WireGuard rotation state
const wgConf = ref('')
const wgOverrideSubnets = ref('')
const wgNeedsNarrowing = ref(false)
const wgWarnings = ref<string[]>([])
const rotatingWg = ref(false)

// Members state
const membersLoaded = ref(false)
const loadingMembers = ref(false)
const siteMembers = ref<SiteMember[]>([])
const siteMembersPag = usePagination(siteMembers)
const users = ref<User[]>([])
const assignUserId = ref('')
const assignRole = ref<SiteRole>('SITE_MEMBER')
const savingMember = ref(false)

const userSelectOptions = computed<SelectOption<string>[]>(() =>
  users.value.map((u) => ({ value: u.id, label: u.email })),
)

let pollTimer: number | null = null
let clockTimer: number | null = null

const site = computed<Site | null>(() => {
  if (!data.value && !siteFull.value) return null
  if (siteFull.value && data.value?.site) {
    return {
      ...siteFull.value,
      ...data.value.site,
      wireguard: siteFull.value.wireguard ?? data.value.site.wireguard,
    }
  }
  return siteFull.value ?? data.value?.site ?? null
})

async function loadSiteRecord() {
  if (!siteId.value) return
  try {
    const s = await apiRequest<Site>(`/api/v1/iam/sites/${siteId.value}`)
    siteFull.value = s
    if (!editNameTouched.value) {
      editName.value = s.name
    }
  } catch (err) {
    if (!data.value) {
      loadError.value = err instanceof Error ? err.message : 'Failed to load site'
    }
  }
}

async function loadMetrics(silent = false) {
  if (!siteId.value) return
  if (silent) {
    refreshing.value = true
  } else {
    loading.value = true
  }
  try {
    const res = await apiRequest<SiteRuntimeMetricsResponse>(`/api/v1/iam/admin/sites/${siteId.value}/metrics`)
    data.value = res
    nowMs.value = Date.now()
    loadError.value = null
    if (!editName.value && res.site?.name) {
      editName.value = res.site.name
    }

    if (res.site.webviewEnabled) {
      try {
        const aRes = await apiRequest<{ summary: AlarmSummary }>(
          `/api/v1/webview/sites/${siteId.value}/alarms?state=ACTIVE&limit=1`,
        )
        alarmSummary.value = aRes.summary || {
          activeCount: 0,
          activeCritical: 0,
          activeWarning: 0,
          activeInfo: 0,
          unacknowledgedCount: 0,
        }
      } catch {
        // Keep last summary on transient error
      }
    } else {
      alarmSummary.value = {
        activeCount: 0,
        activeCritical: 0,
        activeWarning: 0,
        activeInfo: 0,
        unacknowledgedCount: 0,
      }
    }
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Failed to load site metrics'
    loadError.value = msg
    if (!silent) {
      toast.error(err, 'Failed to load site metrics')
    }
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function loadMembersAndUsers() {
  if (!siteId.value) return
  loadingMembers.value = true
  try {
    const [mRes, uRes] = await Promise.all([
      apiRequest<{ members: SiteMember[] }>(`/api/v1/iam/admin/sites/${siteId.value}/members`),
      apiRequest<{ users: User[] }>('/api/v1/iam/admin/users'),
    ])
    siteMembers.value = mRes.members || []
    users.value = uRes.users || []
    membersLoaded.value = true
  } catch (err) {
    toast.error(err, 'Failed to load site members')
  } finally {
    loadingMembers.value = false
  }
}

async function loadAll() {
  await Promise.all([loadSiteRecord(), loadMetrics(false)])
  if (activeTab.value === 'members') {
    await loadMembersAndUsers()
  }
}

function startPolling() {
  stopPolling()
  if (!livePolling.value || activeTab.value !== 'metrics') return
  pollTimer = window.setInterval(() => {
    if (!document.hidden && !refreshing.value && activeTab.value === 'metrics') {
      void loadMetrics(true)
    }
  }, 5000)
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function toggleLive() {
  livePolling.value = !livePolling.value
  if (livePolling.value) {
    void loadMetrics(true)
    startPolling()
  } else {
    stopPolling()
  }
}

function requestToggleService(svc: PlatformServiceDef) {
  if (!site.value || savingSettings.value) return
  const enabling = !site.value[svc.key]
  pendingConfirm.value = {
    title: enabling ? `Enable ${svc.name}` : `Disable ${svc.name}`,
    itemName: `${site.value.name} (${svc.name})`,
    description: enabling
      ? `Enable ${svc.name} on this site so authorized members can access it.`
      : `Disable ${svc.name} on this site. Active ${svc.name} sessions will be drained immediately.`,
    confirmLabel: enabling ? `Enable ${svc.name}` : `Disable ${svc.name}`,
    tone: enabling ? 'success' : 'warning',
    run: () => toggleService(svc.key),
  }
}

async function toggleService(key: ServiceKey) {
  if (!site.value || savingSettings.value) return
  savingSettings.value = true
  try {
    const nextPayload = {
      name: site.value.name,
      vncEnabled: key === 'vncEnabled' ? !site.value.vncEnabled : site.value.vncEnabled,
      webviewEnabled: key === 'webviewEnabled' ? !site.value.webviewEnabled : site.value.webviewEnabled,
    }
    const updated = await apiRequest<Site>(`/api/v1/iam/admin/sites/${site.value.id}`, {
      method: 'PATCH',
      body: JSON.stringify(nextPayload),
    })
    siteFull.value = updated
    if (data.value) {
      data.value = { ...data.value, site: { ...data.value.site, ...updated } }
    }
    toast.success(`Updated services for "${updated.name}".`)
  } catch (err) {
    toast.error(err, 'Failed to update site services')
  } finally {
    savingSettings.value = false
  }
}

async function saveSiteName() {
  if (!site.value || savingSettings.value) return
  editNameSubmitted.value = true
  const nameErr = validateRequiredText(editName.value, 'Site name', { min: 2, max: 100 })
  if (nameErr) {
    toast.error(nameErr, 'Validation Error')
    return
  }
  savingSettings.value = true
  try {
    const updated = await apiRequest<Site>(`/api/v1/iam/admin/sites/${site.value.id}`, {
      method: 'PATCH',
      body: JSON.stringify({
        name: editName.value.trim(),
        vncEnabled: site.value.vncEnabled,
        webviewEnabled: site.value.webviewEnabled,
      }),
    })
    siteFull.value = updated
    editName.value = updated.name
    editNameTouched.value = false
    if (data.value) {
      data.value = { ...data.value, site: { ...data.value.site, ...updated } }
    }
    toast.success(`Renamed site to "${updated.name}".`)
  } catch (err) {
    toast.error(err, 'Failed to rename site')
  } finally {
    savingSettings.value = false
  }
}

async function rotateSiteConf() {
  if (!site.value || !wgConf.value.trim() || rotatingWg.value) return
  const subnetErr =
    wgNeedsNarrowing.value && !wgOverrideSubnets.value.trim()
      ? 'Enter the site LAN subnets to narrow 0.0.0.0/0.'
      : validateCidrList(wgOverrideSubnets.value, 'LAN Subnets')
  if (subnetErr) {
    toast.error(subnetErr, 'Validation Error')
    return
  }
  rotatingWg.value = true
  wgWarnings.value = []
  try {
    const overrides = wgOverrideSubnets.value
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    const res = await apiRequest<{ site: Site; warnings?: string[] }>(
      `/api/v1/iam/admin/sites/${site.value.id}/conf`,
      {
        method: 'POST',
        body: JSON.stringify({
          wireguardConf: wgConf.value,
          ...(overrides.length ? { overrideAllowedIPs: overrides } : {}),
        }),
      },
    )
    siteFull.value = res.site
    wgWarnings.value = res.warnings || []
    wgConf.value = ''
    wgOverrideSubnets.value = ''
    wgNeedsNarrowing.value = false
    toast.success('WireGuard config rotated and runner updated.')
    await loadMetrics(true)
  } catch (err) {
    if (err instanceof ApiError && err.isCatchAllAllowedIPs()) {
      wgNeedsNarrowing.value = true
      toast.warning(
        'Config uses 0.0.0.0/0. Enter the site LAN subnets and submit again.',
        'Subnet Narrowing Required',
      )
    } else {
      toast.error(err, 'Config rotation failed')
    }
  } finally {
    rotatingWg.value = false
  }
}

async function addOrUpdateMember() {
  if (!site.value || savingMember.value) return
  if (!assignUserId.value) {
    toast.error('Please select a user to assign.', 'Validation Error')
    return
  }
  savingMember.value = true
  try {
    await apiRequest(`/api/v1/iam/admin/sites/${site.value.id}/members/${assignUserId.value}`, {
      method: 'PUT',
      body: JSON.stringify({ role: assignRole.value }),
    })
    toast.success('Updated site member role.')
    await loadMembersAndUsers()
  } catch (err) {
    toast.error(err, 'Failed to assign member')
  } finally {
    savingMember.value = false
  }
}

async function removeMember(userId: string) {
  if (!site.value) return
  try {
    await apiRequest(`/api/v1/iam/admin/sites/${site.value.id}/members/${userId}`, {
      method: 'DELETE',
    })
    toast.success('Removed site member.')
    await loadMembersAndUsers()
  } catch (err) {
    toast.error(err, 'Failed to remove member')
  }
}

function requestToggleSiteActive() {
  if (!site.value || togglingActive.value) return
  const activating = !site.value.isActive
  pendingConfirm.value = {
    title: activating ? 'Activate Site Runner' : 'Deactivate Site Runner',
    itemName: `${site.value.name} (${site.value.slug})`,
    description: activating
      ? 'Starts the WireRelay tunnel container and delivers its WireGuard configuration over gRPC.'
      : 'Stops the WireRelay tunnel container and immediately disconnects all active WebView and VNC sessions on this site.',
    confirmLabel: activating ? 'Activate Site' : 'Deactivate Site',
    tone: activating ? 'success' : 'warning',
    run: toggleSiteActive,
  }
}

async function toggleSiteActive() {
  if (!site.value || togglingActive.value) return
  const cur = site.value
  togglingActive.value = true
  try {
    const action = cur.isActive ? 'deactivate' : 'activate'
    const updated = await apiRequest<Site>(`/api/v1/iam/admin/sites/${cur.id}/${action}`, { method: 'POST' })
    siteFull.value = updated
    if (data.value) {
      data.value = { ...data.value, site: { ...data.value.site, ...updated } }
    }
    toast.success(`${cur.isActive ? 'Deactivated' : 'Activated'} site "${cur.name}".`)
    await loadMetrics(true)
  } catch (err) {
    toast.error(err, 'Failed to update site state')
  } finally {
    togglingActive.value = false
  }
}

async function confirmDeleteSite() {
  const cur = site.value
  showDeleteModal.value = false
  if (!cur) return
  try {
    await apiRequest(`/api/v1/iam/admin/sites/${cur.id}`, { method: 'DELETE' })
    toast.success(`Deleted site "${cur.name}".`)
    await router.push('/admin/sites')
  } catch (err) {
    toast.error(err, 'Failed to delete site')
  }
}

async function copyText(val: string) {
  try {
    await navigator.clipboard.writeText(val)
    copiedKey.value = val
    window.setTimeout(() => {
      if (copiedKey.value === val) copiedKey.value = null
    }, 1500)
  } catch {
    toast.error('Clipboard access unavailable.')
  }
}

const webview = computed(() => data.value?.webview ?? null)
const webviewSessions = computed(() => webview.value?.sessions ?? [])
const devices = computed(() => webview.value?.devices ?? [])
const healthyDeviceCount = computed(() => devices.value.filter((d) => d.healthy).length)
const failingDeviceCount = computed(() => devices.value.filter((d) => !d.healthy).length)
const vncSessions = computed(() => data.value?.vncActiveSessions ?? [])
const vncCombinedBytes = computed(() =>
  vncSessions.value.reduce((acc, s) => acc + (s.bytesIn || 0) + (s.bytesOut || 0), 0),
)
const totalSessions = computed(() => (webview.value?.activeWsClients ?? 0) + vncSessions.value.length)
const cache = computed(() => data.value?.cache ?? null)
const pendingAlarmsCount = computed(() => data.value?.pendingAlarms?.length ?? 0)

watch(siteId, () => {
  siteFull.value = null
  data.value = null
  membersLoaded.value = false
  siteMembers.value = []
  editName.value = ''
  editNameTouched.value = false
  editNameSubmitted.value = false
  wgConf.value = ''
  wgOverrideSubnets.value = ''
  wgNeedsNarrowing.value = false
  wgWarnings.value = []
  void loadAll()
})

onMounted(() => {
  void loadAll()
  startPolling()
  clockTimer = window.setInterval(() => {
    nowMs.value = Date.now()
  }, 1000)
})

onUnmounted(() => {
  stopPolling()
  if (clockTimer !== null) {
    window.clearInterval(clockTimer)
    clockTimer = null
  }
})
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <!-- Top navigation & action bar -->
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div class="min-w-0 space-y-1.5">
        <div class="flex flex-wrap items-center gap-2 text-xs text-slate-400">
          <RouterLink
            to="/admin/sites"
            class="inline-flex items-center gap-1 text-slate-400 transition hover:text-white"
          >
            <ArrowLeft class="h-3.5 w-3.5" />
            Sites & Onboarding
          </RouterLink>
          <span class="text-slate-600">/</span>
          <span class="truncate font-mono text-slate-300">{{ site?.slug || siteId }}</span>
        </div>

        <div class="flex flex-wrap items-center gap-2.5">
          <h1 class="truncate text-lg font-semibold tracking-tight text-white sm:text-xl">
            {{ site?.name || 'Site Workspace' }}
          </h1>
          <StatusBadge v-if="site" :status="site.status" size="sm" />
          <span
            v-if="site"
            class="rounded-full border border-slate-700 bg-slate-800 px-2 py-0.5 font-mono text-[10px] uppercase text-slate-300"
          >
            {{ site.isActive ? 'ACTIVE' : 'STOPPED' }}
          </span>
          <span
            v-if="site?.webviewEnabled"
            class="rounded bg-sky-500/15 px-1.5 py-0.5 text-[11px] font-medium text-sky-300"
          >
            WebView
          </span>
          <span
            v-if="site?.vncEnabled"
            class="rounded bg-emerald-500/15 px-1.5 py-0.5 text-[11px] font-medium text-emerald-300"
          >
            WebVNC
          </span>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <RouterLink
          :to="`/admin/runners/${siteId}`"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-indigo-500/40 bg-indigo-500/15 px-3 text-xs font-medium text-indigo-200 transition hover:bg-indigo-500/25"
        >
          <Cpu class="h-3.5 w-3.5" />
          Tunnel Metrics
        </RouterLink>
        <RouterLink
          v-if="site"
          :to="`/sites/${site.id}/overview`"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 text-xs font-medium text-slate-200 transition hover:bg-slate-800"
        >
          <ExternalLink class="h-3.5 w-3.5" />
          Operator View
        </RouterLink>
        <template v-if="activeTab === 'metrics'">
          <button
            type="button"
            class="inline-flex h-9 items-center gap-1.5 rounded-lg border px-3 text-xs font-medium transition"
            :class="
              livePolling
                ? 'border-emerald-500/40 bg-emerald-500/15 text-emerald-300 hover:bg-emerald-500/20'
                : 'border-slate-700 bg-slate-900 text-slate-300 hover:bg-slate-800'
            "
            @click="toggleLive"
          >
            <Pause v-if="livePolling" class="h-3.5 w-3.5" />
            <Play v-else class="h-3.5 w-3.5" />
            {{ livePolling ? 'Live 5s' : 'Paused' }}
          </button>
          <button
            type="button"
            class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 text-xs font-medium text-slate-200 transition hover:bg-slate-800"
            :disabled="refreshing"
            @click="loadMetrics(true)"
          >
            <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': refreshing }" />
            Refresh
          </button>
        </template>
      </div>
    </div>

    <!-- Common Site Sub-Page Tabs -->
    <div class="no-scrollbar overflow-x-auto border-b border-slate-800">
      <div role="tablist" aria-label="Site management sections" class="flex min-w-max items-center gap-1">
        <button
          v-for="tab in TABS"
          :key="tab.id"
          type="button"
          role="tab"
          :aria-selected="activeTab === tab.id"
          class="inline-flex items-center gap-1.5 border-b-2 px-3.5 py-2.5 text-xs font-medium transition"
          :class="
            activeTab === tab.id
              ? 'border-indigo-400 text-white'
              : 'border-transparent text-slate-400 hover:border-slate-700 hover:text-slate-200'
          "
          @click="setTab(tab.id)"
        >
          <component :is="tab.icon" class="h-3.5 w-3.5" />
          {{ tab.label }}
        </button>
      </div>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading && !site" class="space-y-4">
      <div class="h-24 animate-pulse rounded-xl border border-slate-800 bg-slate-900/60" />
      <div class="h-44 animate-pulse rounded-xl border border-slate-800 bg-slate-900/60" />
    </div>

    <!-- Load error -->
    <div
      v-else-if="loadError && !site"
      class="flex flex-col items-start justify-between gap-3 rounded-xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs text-rose-200 sm:flex-row sm:items-center"
    >
      <div class="flex items-start gap-2.5">
        <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-rose-400" />
        <div>
          <p class="font-semibold text-rose-100">Unable to load site</p>
          <p class="mt-0.5 text-rose-300">{{ loadError }}</p>
        </div>
      </div>
      <button
        type="button"
        class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-rose-500/40 bg-rose-500/20 px-3 text-xs font-medium text-rose-100 hover:bg-rose-500/30"
        @click="loadAll"
      >
        Retry
      </button>
    </div>

    <template v-else-if="site">
      <!-- =================================================================== -->
      <!-- TAB 1: METRICS (Application-Level Telemetry)                        -->
      <!-- =================================================================== -->
      <div v-if="activeTab === 'metrics'" class="space-y-5 sm:space-y-6">
        <!-- Application-Level Summary Strip -->
        <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="grid divide-y divide-slate-800/80 sm:grid-cols-2 sm:divide-y-0 lg:grid-cols-5 lg:divide-x">
            <div class="p-4">
              <p class="text-[11px] text-slate-400">Active Sessions</p>
              <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
                {{ totalSessions }}
              </p>
              <p class="mt-0.5 font-mono text-[11px] tabular-nums text-slate-500">
                {{ webview?.activeWsClients ?? 0 }} WebView | {{ vncSessions.length }} VNC
              </p>
            </div>

            <div class="border-slate-800/80 p-4 sm:border-l lg:border-l-0">
              <p class="text-[11px] text-slate-400">Valkey Cache</p>
              <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
                {{ cache ? cache.webviewSampleCount : 0 }}
                <span class="text-xs font-normal text-slate-500">samples</span>
              </p>
              <p class="mt-0.5 font-mono text-[11px] tabular-nums text-slate-500">
                {{ cache ? cache.authzCacheEntries : 0 }} authz entries
              </p>
            </div>

            <div class="border-slate-800/80 p-4 sm:border-t lg:border-t-0">
              <p class="text-[11px] text-slate-400">WebView Devices</p>
              <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
                {{ healthyDeviceCount }}
                <span class="text-xs font-normal text-slate-500">/ {{ devices.length }}</span>
              </p>
              <p
                class="mt-0.5 font-mono text-[11px]"
                :class="failingDeviceCount > 0 ? 'text-amber-300' : 'text-slate-500'"
              >
                {{ failingDeviceCount > 0 ? `${failingDeviceCount} backing off` : 'all healthy' }}
              </p>
            </div>

            <div class="border-slate-800/80 p-4 sm:border-l sm:border-t lg:border-l-0 lg:border-t-0">
              <p class="text-[11px] text-slate-400">VNC Sessions</p>
              <p class="mt-1.5 font-mono text-lg font-semibold tabular-nums text-white">
                {{ vncSessions.length }}
              </p>
              <p class="mt-0.5 font-mono text-[11px] tabular-nums text-slate-500">
                {{ vncSessions.length > 0 ? formatBytes(vncCombinedBytes) : 'no live traffic' }}
              </p>
            </div>

            <div class="border-slate-800/80 p-4 sm:col-span-2 sm:border-t lg:col-span-1 lg:border-t-0">
              <p class="text-[11px] text-slate-400">Alarms</p>
              <p
                class="mt-1.5 font-mono text-lg font-semibold tabular-nums"
                :class="alarmSummary.activeCount > 0 ? 'text-rose-300' : pendingAlarmsCount > 0 ? 'text-amber-300' : 'text-white'"
              >
                {{ alarmSummary.activeCount }}
                <span class="text-xs font-normal text-slate-500">active</span>
              </p>
              <p class="mt-0.5 font-mono text-[11px] tabular-nums text-slate-500">
                <template v-if="alarmSummary.activeCritical > 0 || alarmSummary.activeWarning > 0">
                  {{ alarmSummary.activeCritical }} crit | {{ alarmSummary.activeWarning }} warn | {{ pendingAlarmsCount }} hold
                </template>
                <template v-else>
                  {{ pendingAlarmsCount }} pending hold | {{ alarmSummary.unacknowledgedCount }} unacked
                </template>
              </p>
            </div>
          </div>
        </section>

        <!-- Aggregated Valkey Cache State -->
        <section v-if="cache" class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800 px-4 py-3 sm:px-5">
            <div class="flex items-center gap-2 text-sm font-semibold text-white">
              <Database class="h-4 w-4 text-indigo-400" />
              Valkey Cache State
            </div>
            <span v-if="data?.collectedAt" class="font-mono text-[11px] tabular-nums text-slate-500">
              Snapshot {{ formatDateTime(data.collectedAt) }}
            </span>
          </div>
          <dl class="grid divide-y divide-slate-800/70 text-xs sm:grid-cols-2 sm:divide-y-0 lg:grid-cols-4 lg:divide-x">
            <div class="p-4">
              <dt class="text-slate-500">Warm Register Samples</dt>
              <dd class="mt-1 font-mono text-base font-semibold tabular-nums text-white">
                {{ cache.webviewSampleCount }}
              </dd>
            </div>
            <div class="border-slate-800/70 p-4 sm:border-l lg:border-l-0">
              <dt class="text-slate-500">Sample Quality Breakdown</dt>
              <dd class="mt-1.5 flex flex-wrap items-center gap-1.5 font-mono text-[11px] tabular-nums">
                <span class="rounded bg-emerald-500/15 px-1.5 py-0.5 text-emerald-300">GOOD {{ cache.goodSamples }}</span>
                <span class="rounded bg-amber-500/15 px-1.5 py-0.5 text-amber-300">STALE {{ cache.staleSamples }}</span>
                <span class="rounded bg-rose-500/15 px-1.5 py-0.5 text-rose-300">BAD {{ cache.badSamples }}</span>
              </dd>
            </div>
            <div class="border-slate-800/70 p-4 sm:border-t lg:border-t-0">
              <dt class="text-slate-500">Sample Cache TTL</dt>
              <dd class="mt-1 font-mono text-base font-semibold tabular-nums text-slate-100">
                {{ cache.sampleCacheTtlSeconds > 0 ? formatDurationSeconds(cache.sampleCacheTtlSeconds) : 'Empty' }}
              </dd>
            </div>
            <div class="border-slate-800/70 p-4 sm:border-l sm:border-t lg:border-l-0 lg:border-t-0">
              <dt class="text-slate-500">Cached Authz Entries</dt>
              <dd class="mt-1 font-mono text-base font-semibold tabular-nums text-slate-100">
                {{ cache.authzCacheEntries }}
              </dd>
            </div>
          </dl>
        </section>

        <!-- Live WebView Sessions -->
        <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800 px-4 py-3 sm:px-5">
            <div class="flex items-center gap-2 text-sm font-semibold text-white">
              <Radio class="h-4 w-4 text-sky-400" />
              WebView Live Sessions
            </div>
            <span class="font-mono text-[11px] tabular-nums text-slate-500">
              {{ webview?.activeWsClients ?? 0 }} clients | {{ webview?.subscribedMappings ?? 0 }} subscribed registers
            </span>
          </div>

          <div v-if="!site.webviewEnabled && webviewSessions.length === 0" class="px-4 py-5 text-center text-xs text-slate-500">
            WebView service is not enabled on this site.
          </div>

          <div v-else-if="webviewSessions.length === 0" class="px-4 py-5 text-center text-xs text-slate-500">
            No browser WebView WebSocket clients connected right now.
          </div>

          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead class="border-b border-slate-800 text-slate-400">
                <tr>
                  <th class="px-4 py-2.5 sm:px-5">User</th>
                  <th class="px-4 py-2.5">Client ID</th>
                  <th class="px-4 py-2.5 text-right">Subscribed Regs</th>
                  <th class="px-4 py-2.5 text-right">Poll Interval</th>
                  <th class="px-4 py-2.5 text-right sm:px-5">Connected</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-800/70">
                <tr v-for="ws in webviewSessions" :key="ws.clientId" class="hover:bg-slate-800/30">
                  <td class="px-4 py-2.5 font-mono text-slate-100 sm:px-5">
                    {{ ws.userEmail || shortKey(ws.userId, 8, 4) }}
                  </td>
                  <td class="px-4 py-2.5 font-mono text-slate-400" :title="ws.clientId">
                    {{ shortKey(ws.clientId, 8, 4) }}
                  </td>
                  <td class="px-4 py-2.5 text-right font-mono tabular-nums text-slate-200">
                    {{ ws.subscribedCount }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-300">
                    {{ ws.pollIntervalMs > 0 ? `${ws.pollIntervalMs} ms` : '-' }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-300 sm:px-5">
                    {{ formatElapsedSince(ws.connectedAt, nowMs) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <!-- Live VNC Sessions -->
        <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800 px-4 py-3 sm:px-5">
            <div class="flex items-center gap-2 text-sm font-semibold text-white">
              <Monitor class="h-4 w-4 text-emerald-400" />
              Live VNC Sessions
            </div>
            <span class="font-mono text-[11px] tabular-nums text-slate-500">
              {{ vncSessions.length }} active
            </span>
          </div>

          <div v-if="!site.vncEnabled && vncSessions.length === 0" class="px-4 py-5 text-center text-xs text-slate-500">
            WebVNC service is not enabled on this site.
          </div>

          <div v-else-if="vncSessions.length === 0" class="px-4 py-5 text-center text-xs text-slate-500">
            No active VNC sessions on this site right now.
          </div>

          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead class="border-b border-slate-800 text-slate-400">
                <tr>
                  <th class="px-4 py-2.5 sm:px-5">Target</th>
                  <th class="px-4 py-2.5">User</th>
                  <th class="px-4 py-2.5 text-right">Duration</th>
                  <th class="px-4 py-2.5 text-right">Browser -> Target</th>
                  <th class="px-4 py-2.5 text-right">Target -> Browser</th>
                  <th class="px-4 py-2.5 text-right sm:px-5">Last Activity</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-800/70">
                <tr v-for="sess in vncSessions" :key="sess.sessionId" class="hover:bg-slate-800/30">
                  <td class="px-4 py-2.5 sm:px-5">
                    <p class="font-medium text-slate-100">{{ sess.targetName }}</p>
                    <p class="font-mono text-[10px] text-slate-500">{{ shortKey(sess.sessionId, 8, 4) }}</p>
                  </td>
                  <td class="px-4 py-2.5 font-mono text-slate-200">
                    {{ sess.userEmail || shortKey(sess.userId, 8, 4) }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-200">
                    {{ formatElapsedSince(sess.startedAt, nowMs) }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-sky-300">
                    {{ formatBytes(sess.bytesIn) }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-emerald-300">
                    {{ formatBytes(sess.bytesOut) }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-400 sm:px-5">
                    {{ formatElapsedSince(sess.lastActivityAt, nowMs) }} ago
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <!-- WebView Devices -->
        <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800 px-4 py-3 sm:px-5">
            <div class="flex items-center gap-2 text-sm font-semibold text-white">
              <Gauge class="h-4 w-4 text-sky-400" />
              WebView Devices
            </div>
            <span v-if="webview?.updatedAt" class="font-mono text-[11px] tabular-nums text-slate-500">
              Updated {{ formatDateTime(webview.updatedAt) }}
            </span>
          </div>

          <div v-if="!site.webviewEnabled && devices.length === 0" class="px-4 py-5 text-center text-xs text-slate-500">
            WebView service is not enabled on this site.
          </div>

          <div v-else-if="devices.length === 0" class="px-4 py-5 text-center text-xs text-slate-500">
            No active Modbus devices reported in Valkey runtime state yet.
          </div>

          <div v-else>
            <div class="divide-y divide-slate-800/70 md:hidden">
              <div v-for="dev in devices" :key="dev.deviceId" class="space-y-2 p-4 text-xs">
                <div class="flex items-center justify-between gap-2">
                  <p class="font-medium text-white">{{ dev.name }}</p>
                  <span
                    class="rounded-full px-2 py-0.5 font-mono text-[10px] font-medium"
                    :class="
                      dev.healthy
                        ? 'bg-emerald-500/15 text-emerald-300'
                        : 'bg-amber-500/15 text-amber-300'
                    "
                  >
                    {{ dev.healthy ? 'HEALTHY' : `BACKOFF (${dev.failures})` }}
                  </span>
                </div>
                <div class="grid grid-cols-2 gap-2 font-mono text-[11px] text-slate-400">
                  <div>
                    <span class="text-slate-500">Poll: </span>
                    <span class="text-slate-200">{{ dev.inFlight ? 'in-flight' : 'idle' }}</span>
                  </div>
                  <div>
                    <span class="text-slate-500">Last: </span>
                    <span class="text-slate-200">
                      {{ dev.lastPolledAt ? formatElapsedSince(dev.lastPolledAt, nowMs) + ' ago' : '-' }}
                    </span>
                  </div>
                </div>
              </div>
            </div>

            <div class="hidden overflow-x-auto md:block">
              <table class="w-full text-left text-xs">
                <thead class="border-b border-slate-800 text-slate-400">
                  <tr>
                    <th class="px-4 py-2.5 sm:px-5">Device</th>
                    <th class="px-4 py-2.5">Health</th>
                    <th class="px-4 py-2.5">Poll Activity</th>
                    <th class="px-4 py-2.5 text-right">Consecutive Failures</th>
                    <th class="px-4 py-2.5 text-right">Last Polled</th>
                    <th class="px-4 py-2.5 text-right sm:px-5">Next Retry</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-800/70">
                  <tr v-for="dev in devices" :key="dev.deviceId" class="hover:bg-slate-800/30">
                    <td class="px-4 py-2.5 sm:px-5">
                      <p class="font-medium text-slate-100">{{ dev.name }}</p>
                      <p class="font-mono text-[10px] text-slate-500">{{ dev.deviceId }}</p>
                    </td>
                    <td class="px-4 py-2.5">
                      <span
                        class="inline-flex rounded-full px-2 py-0.5 font-mono text-[10px] font-medium"
                        :class="
                          dev.healthy
                            ? 'bg-emerald-500/15 text-emerald-300'
                            : 'bg-amber-500/15 text-amber-300'
                        "
                      >
                        {{ dev.healthy ? 'HEALTHY' : 'BACKING OFF' }}
                      </span>
                    </td>
                    <td class="px-4 py-2.5 font-mono text-[11px] text-slate-300">
                      {{ dev.inFlight ? 'Polling' : 'Idle' }}
                    </td>
                    <td
                      class="px-4 py-2.5 text-right font-mono tabular-nums"
                      :class="dev.failures > 0 ? 'text-amber-300' : 'text-slate-400'"
                    >
                      {{ dev.failures }}
                    </td>
                    <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-300">
                      {{ dev.lastPolledAt ? formatDateTime(dev.lastPolledAt) : '-' }}
                    </td>
                    <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-400 sm:px-5">
                      {{ dev.nextRetryAt ? formatDateTime(dev.nextRetryAt) : '-' }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>
      </div>

      <!-- =================================================================== -->
      <!-- TAB 2: SERVICES                                                     -->
      <!-- =================================================================== -->
      <div v-else-if="activeTab === 'services'" class="space-y-4">
        <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="border-b border-slate-800 px-4 py-3 sm:px-5">
            <h2 class="text-sm font-semibold text-white">Attached Site Services</h2>
            <p class="mt-0.5 text-xs text-slate-400">
              Enable or disable platform services for this site. Disabling a service immediately drains its live sessions.
            </p>
          </div>
          <div class="divide-y divide-slate-800/70">
            <div
              v-for="svc in PLATFORM_SERVICES"
              :key="svc.key"
              class="flex flex-col gap-3 px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-5"
            >
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <p class="text-sm font-medium text-white">{{ svc.name }}</p>
                  <span
                    class="rounded-full px-2 py-0.5 font-mono text-[10px] font-medium uppercase"
                    :class="site[svc.key] ? 'bg-emerald-500/15 text-emerald-300' : 'bg-slate-800 text-slate-400'"
                  >
                    {{ site[svc.key] ? 'Enabled' : 'Disabled' }}
                  </span>
                </div>
                <p class="mt-1 text-xs text-slate-400">{{ svc.description }}</p>
              </div>
              <button
                type="button"
                :disabled="savingSettings"
                class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 self-start rounded-lg border px-3.5 text-xs font-medium transition disabled:opacity-50 sm:self-auto"
                :class="
                  site[svc.key]
                    ? 'border-rose-500/40 bg-rose-500/10 text-rose-200 hover:bg-rose-500/20'
                    : 'border-emerald-500/40 bg-emerald-500/10 text-emerald-200 hover:bg-emerald-500/20'
                "
                @click="requestToggleService(svc)"
              >
                <Minus v-if="site[svc.key]" class="h-3.5 w-3.5" />
                <Plus v-else class="h-3.5 w-3.5" />
                {{ site[svc.key] ? 'Disable Service' : 'Enable Service' }}
              </button>
            </div>
          </div>
        </section>
      </div>

      <!-- =================================================================== -->
      <!-- TAB 3: WIREGUARD                                                    -->
      <!-- =================================================================== -->
      <div v-else-if="activeTab === 'wireguard'" class="space-y-5">
        <section v-if="site.wireguard" class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
          <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 class="text-sm font-semibold text-white">Active WireGuard Metadata (Non-Secret)</h2>
              <p class="mt-0.5 text-xs text-slate-400">
                Private keys are encrypted at rest and delivered to the runner strictly in RAM over gRPC.
              </p>
            </div>
            <RouterLink
              :to="`/admin/runners/${site.id}`"
              class="inline-flex h-8 items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-800 px-3 text-xs font-medium text-slate-200 transition hover:bg-slate-700"
            >
              <Cpu class="h-3.5 w-3.5 text-indigo-400" />
              Inspect Tunnel Traffic
            </RouterLink>
          </div>
          <dl class="grid gap-x-6 gap-y-4 text-xs sm:grid-cols-2 lg:grid-cols-3">
            <div class="min-w-0">
              <dt class="text-slate-500">Remote Endpoint</dt>
              <dd class="mt-1 break-all font-mono text-slate-200">
                {{ site.wireguard.endpointHost }}:{{ site.wireguard.endpointPort }}
              </dd>
            </div>
            <div class="min-w-0">
              <dt class="text-slate-500">Allowed LAN Subnets</dt>
              <dd class="mt-1 break-words font-mono text-slate-200">
                {{ site.wireguard.lanSubnets.join(', ') }}
              </dd>
            </div>
            <div class="min-w-0">
              <dt class="text-slate-500">MTU / Persistent Keepalive</dt>
              <dd class="mt-1 font-mono tabular-nums text-slate-200">
                {{ site.wireguard.mtu || 1420 }} / {{ site.wireguard.persistentKeepalive }}s
              </dd>
            </div>
            <div class="min-w-0">
              <dt class="text-slate-500">Peer Public Key</dt>
              <dd class="mt-1 flex items-center gap-1.5 font-mono text-slate-200">
                <span class="truncate" :title="site.wireguard.publicKey">{{ site.wireguard.publicKey }}</span>
                <button
                  type="button"
                  class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-white"
                  @click="copyText(site.wireguard.publicKey)"
                >
                  <Check v-if="copiedKey === site.wireguard.publicKey" class="h-3.5 w-3.5 text-emerald-400" />
                  <Copy v-else class="h-3.5 w-3.5" />
                </button>
              </dd>
            </div>
            <div class="min-w-0">
              <dt class="text-slate-500">Platform Client Public Key</dt>
              <dd class="mt-1 flex items-center gap-1.5 font-mono text-slate-200">
                <span class="truncate" :title="site.wireguard.clientPublicKey">{{ site.wireguard.clientPublicKey }}</span>
                <button
                  type="button"
                  class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-white"
                  @click="copyText(site.wireguard.clientPublicKey)"
                >
                  <Check v-if="copiedKey === site.wireguard.clientPublicKey" class="h-3.5 w-3.5 text-emerald-400" />
                  <Copy v-else class="h-3.5 w-3.5" />
                </button>
              </dd>
            </div>
            <div class="min-w-0">
              <dt class="text-slate-500">Config Fingerprint</dt>
              <dd class="mt-1 truncate font-mono text-slate-200" :title="site.wireguard.confFingerprint">
                {{ shortKey(site.wireguard.confFingerprint, 16, 8) }}
              </dd>
            </div>
          </dl>
        </section>

        <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
          <h2 class="text-sm font-semibold text-white">Rotate WireGuard Client Config</h2>
          <p class="mt-0.5 text-xs text-slate-400">
            Upload or paste a replacement WireGuard client config. Active sessions are drained before rotating the runner.
          </p>
          <form class="mt-4 space-y-3" @submit.prevent="rotateSiteConf">
            <WireguardConfEditor
              v-model="wgConf"
              v-model:override-subnets="wgOverrideSubnets"
              :needs-narrowing="wgNeedsNarrowing"
              :warnings="wgWarnings"
            />
            <button
              type="submit"
              :disabled="rotatingWg || !wgConf.trim()"
              class="inline-flex h-9 items-center justify-center rounded-lg bg-sky-500 px-4 text-xs font-semibold text-slate-950 transition hover:bg-sky-400 disabled:opacity-50"
            >
              {{ rotatingWg ? 'Validating & Rotating...' : 'Validate & Rotate WireGuard Config' }}
            </button>
          </form>
        </section>
      </div>

      <!-- =================================================================== -->
      <!-- TAB 4: MEMBERS                                                      -->
      <!-- =================================================================== -->
      <div v-else-if="activeTab === 'members'" class="space-y-5">
        <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
          <h2 class="text-sm font-semibold text-white">Assign Site Member</h2>
          <p class="mt-0.5 text-xs text-slate-400">
            Grant an existing platform user access to this site as a Site Member or Site Operator.
          </p>
          <form class="mt-4 flex flex-col gap-2.5 sm:flex-row sm:items-center" @submit.prevent="addOrUpdateMember">
            <CustomSelect
              v-model="assignUserId"
              :options="userSelectOptions"
              placeholder="Select user..."
              aria-label="Select user"
              class="w-full min-w-0 sm:flex-1"
              trigger-class="h-10"
            />
            <div class="flex items-center gap-2">
              <CustomSelect
                v-model="assignRole"
                :options="SITE_ROLE_OPTIONS"
                aria-label="Site role"
                class="flex-1 sm:w-44 sm:flex-initial"
                trigger-class="h-10"
              />
              <button
                type="submit"
                :disabled="savingMember"
                class="inline-flex h-10 shrink-0 items-center justify-center rounded-lg bg-indigo-500 px-4 text-xs font-semibold leading-none text-white transition hover:bg-indigo-400 disabled:opacity-50"
              >
                Assign
              </button>
            </div>
          </form>
        </section>

        <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
          <div class="flex items-center justify-between border-b border-slate-800 px-4 py-3 sm:px-5">
            <h2 class="text-sm font-semibold text-white">Assigned Members</h2>
            <span class="font-mono text-[11px] tabular-nums text-slate-500">{{ siteMembers.length }} total</span>
          </div>

          <div v-if="loadingMembers && !membersLoaded" class="px-4 py-6 text-center text-xs text-slate-500">
            Loading members...
          </div>

          <div v-else-if="siteMembers.length === 0" class="px-4 py-6 text-center text-xs text-slate-500">
            No members explicitly assigned to this site. Platform Admins still retain operator access.
          </div>

          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead class="border-b border-slate-800 text-slate-400">
                <tr>
                  <th class="px-4 py-2.5 sm:px-5">Email</th>
                  <th class="px-4 py-2.5">Role</th>
                  <th class="px-4 py-2.5">Account Status</th>
                  <th class="px-4 py-2.5 text-right">Assigned</th>
                  <th class="px-4 py-2.5 text-right sm:px-5">Actions</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-800/70">
                <tr v-for="m in siteMembersPag.paginatedItems.value" :key="m.userId" class="hover:bg-slate-800/30">
                  <td class="px-4 py-2.5 font-mono text-slate-100 sm:px-5">{{ m.email }}</td>
                  <td class="px-4 py-2.5">
                    <span
                      class="inline-flex rounded px-1.5 py-0.5 font-mono text-[10px] font-medium"
                      :class="
                        m.role === 'SITE_OPERATOR'
                          ? 'bg-indigo-500/15 text-indigo-300'
                          : 'bg-slate-800 text-slate-300'
                      "
                    >
                      {{ m.role }}
                    </span>
                  </td>
                  <td class="px-4 py-2.5 font-mono text-[11px] text-slate-400">{{ m.status }}</td>
                  <td class="whitespace-nowrap px-4 py-2.5 text-right font-mono tabular-nums text-slate-400">
                    {{ formatDateTime(m.createdAt) }}
                  </td>
                  <td class="px-4 py-2.5 text-right sm:px-5">
                    <button
                      type="button"
                      class="text-xs font-medium text-rose-400 transition hover:text-rose-300 hover:underline"
                      @click="removeMember(m.userId)"
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <TablePagination
            v-model:page="siteMembersPag.page.value"
            v-model:page-size="siteMembersPag.pageSize.value"
            :total-items="siteMembersPag.totalItems.value"
          />
        </section>
      </div>

      <!-- =================================================================== -->
      <!-- TAB 5: SETTINGS & LIFECYCLE                                         -->
      <!-- =================================================================== -->
      <div v-else-if="activeTab === 'settings'" class="space-y-5">
        <!-- General Info & Rename -->
        <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5 space-y-4">
          <div>
            <h2 class="text-sm font-semibold text-white">General Site Settings</h2>
            <p class="mt-0.5 text-xs text-slate-400">Update the site display name or inspect immutable identifiers.</p>
          </div>

          <form novalidate class="flex flex-col gap-2.5 sm:flex-row sm:items-end" @submit.prevent="saveSiteName">
            <div class="w-full sm:max-w-md sm:flex-1">
              <label class="mb-1 block text-xs text-slate-400">Site Name</label>
              <input
                v-model="editName"
                class="h-10 w-full rounded-lg border bg-slate-950 px-3 text-sm text-white focus:outline-none"
                :class="editNameErr ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-indigo-500'"
                @blur="editNameTouched = true"
              />
              <p v-if="editNameErr" class="mt-1 text-[11px] text-rose-400">{{ editNameErr }}</p>
            </div>
            <button
              type="submit"
              :disabled="savingSettings"
              class="inline-flex h-10 shrink-0 items-center justify-center rounded-lg bg-indigo-500 px-4 text-xs font-semibold leading-none text-white transition hover:bg-indigo-400 disabled:opacity-50"
            >
              Save Name
            </button>
          </form>

          <dl class="grid gap-4 border-t border-slate-800 pt-4 text-xs sm:grid-cols-3">
            <div class="min-w-0">
              <dt class="text-slate-500">Site ID</dt>
              <dd class="mt-1 break-all font-mono text-slate-200">{{ site.id }}</dd>
            </div>
            <div class="min-w-0">
              <dt class="text-slate-500">Slug</dt>
              <dd class="mt-1 font-mono text-slate-200">{{ site.slug }}</dd>
            </div>
            <div class="min-w-0">
              <dt class="text-slate-500">Created At</dt>
              <dd class="mt-1 font-mono tabular-nums text-slate-200">{{ formatDateTime(site.createdAt) }}</dd>
            </div>
          </dl>
        </section>

        <!-- Runner Activation / Deactivation -->
        <section class="flex flex-col gap-3 rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5">
          <div class="min-w-0">
            <h2 class="text-sm font-semibold text-white">
              {{ site.isActive ? 'Deactivate Site Runner' : 'Activate Site Runner' }}
            </h2>
            <p class="mt-0.5 text-xs text-slate-400">
              {{
                site.isActive
                  ? 'Stops the WireRelay container and drains active WebView and VNC sessions while keeping all site configuration intact.'
                  : 'Starts the WireRelay container and pushes its WireGuard config over gRPC.'
              }}
            </p>
          </div>
          <button
            type="button"
            :disabled="togglingActive"
            class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 self-start rounded-lg border px-3.5 text-xs font-medium transition disabled:opacity-50 sm:self-auto"
            :class="
              site.isActive
                ? 'border-amber-500/40 bg-amber-500/15 text-amber-200 hover:bg-amber-500/25'
                : 'border-emerald-500/40 bg-emerald-500/15 text-emerald-200 hover:bg-emerald-500/25'
            "
            @click="requestToggleSiteActive"
          >
            <Power class="h-3.5 w-3.5" />
            {{ site.isActive ? 'Deactivate Site' : 'Activate Site' }}
          </button>
        </section>

        <!-- Danger Zone: Delete Site -->
        <section class="flex flex-col gap-3 rounded-xl border border-rose-500/30 bg-rose-500/5 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5">
          <div class="min-w-0">
            <h2 class="text-sm font-semibold text-rose-200">Delete Site</h2>
            <p class="mt-0.5 text-xs text-slate-400">
              Permanently removes this site, its runner container, VNC targets, WebView dashboards, devices, and alarm rules.
            </p>
          </div>
          <button
            type="button"
            class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 self-start rounded-lg border border-rose-500/40 bg-rose-500/15 px-3.5 text-xs font-semibold text-rose-200 transition hover:bg-rose-500/25 sm:self-auto"
            @click="showDeleteModal = true"
          >
            <Trash2 class="h-3.5 w-3.5" />
            Delete Site
          </button>
        </section>
      </div>
    </template>

    <ConfirmActionModal
      :open="pendingConfirm !== null"
      :title="pendingConfirm?.title || ''"
      :item-name="pendingConfirm?.itemName"
      :description="pendingConfirm?.description || ''"
      :confirm-label="pendingConfirm?.confirmLabel"
      :tone="pendingConfirm?.tone"
      :busy="confirmingAction"
      @close="pendingConfirm = null"
      @confirm="executePendingConfirm"
    />

    <DeleteConfirmModal
      :open="showDeleteModal"
      title="Delete Site"
      :item-name="site?.name || ''"
      description="This permanently removes the site, its runner container, VNC targets, WebView dashboards, and history."
      confirm-label="Delete Site"
      :require-confirm-text="site?.slug || ''"
      confirm-input-label="Type site slug to confirm"
      @close="showDeleteModal = false"
      @confirm="confirmDeleteSite"
    />
  </div>
</template>
