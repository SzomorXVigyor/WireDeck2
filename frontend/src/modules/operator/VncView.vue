<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import RFB from '@novnc/novnc'
import { KeyRound, Maximize2, Minimize2, Monitor, Plug, PowerOff, RefreshCw, Terminal } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import type { VncTarget } from '../../types/vnc'

type QualityPresetKey = 'high' | 'balanced' | 'low'

const QUALITY_PRESETS: Record<QualityPresetKey, { label: string; quality: number; compression: number }> = {
  high: { label: 'High (Q9 / C1)', quality: 9, compression: 1 },
  balanced: { label: 'Balanced (Q6 / C2)', quality: 6, compression: 2 },
  low: { label: 'Low bandwidth (Q2 / C6)', quality: 2, compression: 6 },
}

const route = useRoute()
const siteId = computed(() => String(route.params.siteId || ''))

const targets = ref<VncTarget[]>([])
const selectedId = ref<string>('')
const selectedTarget = computed(() => targets.value.find((t) => t.id === selectedId.value) ?? null)
const loadingTargets = ref(false)
const status = ref<'idle' | 'connecting' | 'connected' | 'disconnected' | 'error'>('idle')
const statusMsg = ref<string>('')
const scaleViewport = ref(true)
const qualityPreset = ref<QualityPresetKey>('balanced')
const isFullscreen = ref(false)

// Connection-time VNC credential prompt (used only when target has no saved server-side password)
const authPromptVisible = ref(false)
const authNeedUsername = ref(false)
const authUsername = ref('')
const authPassword = ref('')
const authPasswordInput = ref<HTMLInputElement | null>(null)

const screenHost = ref<HTMLElement | null>(null)
const viewerShell = ref<HTMLElement | null>(null)
let rfb: RFB | null = null
let lastSecurityError = ''

async function loadTargets() {
  disconnect()
  loadingTargets.value = true
  statusMsg.value = ''
  try {
    const res = await apiRequest<{ targets: VncTarget[] }>(`/api/v1/vnc/sites/${siteId.value}/targets`)
    targets.value = res.targets || []
    selectedId.value = targets.value[0]?.id ?? ''
  } catch (err) {
    status.value = 'error'
    statusMsg.value = err instanceof Error ? err.message : 'Failed to load VNC targets'
  } finally {
    loadingTargets.value = false
  }
}

function handleFullscreenChange() {
  isFullscreen.value = Boolean(document.fullscreenElement)
}

onMounted(() => {
  void loadTargets()
  document.addEventListener('fullscreenchange', handleFullscreenChange)
})
watch(siteId, loadTargets)
onUnmounted(() => {
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  if (document.fullscreenElement) {
    void document.exitFullscreen()
  }
  disconnect()
})

watch(scaleViewport, (val) => {
  if (rfb) {
    rfb.scaleViewport = val
  }
})

watch(qualityPreset, (key) => {
  if (rfb) {
    const preset = QUALITY_PRESETS[key]
    rfb.qualityLevel = preset.quality
    rfb.compressionLevel = preset.compression
  }
})

function clearAuthPrompt() {
  authPromptVisible.value = false
  authNeedUsername.value = false
  authUsername.value = ''
  authPassword.value = ''
}

function disconnect() {
  clearAuthPrompt()
  if (rfb) {
    try {
      rfb.disconnect()
    } catch {
      // ignore disconnect errors during teardown
    }
    rfb = null
  }
  if (status.value === 'connected' || status.value === 'connecting') {
    status.value = 'disconnected'
  }
}

async function connect() {
  if (!selectedId.value) return
  disconnect()
  lastSecurityError = ''
  status.value = 'connecting'
  statusMsg.value = 'Requesting single-use session ticket...'
  try {
    const ticketRes = await apiRequest<{ ticket: string; expiresAt: string }>(
      `/api/v1/vnc/sites/${siteId.value}/ws-ticket`,
      {
        method: 'POST',
        body: JSON.stringify({ targetId: selectedId.value }),
      },
    )

    await nextTick()
    if (!screenHost.value) {
      throw new Error('VNC display container not mounted')
    }
    screenHost.value.innerHTML = ''

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${proto}//${window.location.host}/api/v1/vnc/stream?ticket=${encodeURIComponent(ticketRes.ticket)}`

    const preset = QUALITY_PRESETS[qualityPreset.value]
    const hasSavedPassword = Boolean(selectedTarget.value?.hasPassword)

    rfb = new RFB(screenHost.value, wsUrl, { wsProtocols: ['binary'] })
    rfb.scaleViewport = scaleViewport.value
    rfb.resizeSession = false
    rfb.showDotCursor = true
    rfb.background = '#020617'
    rfb.qualityLevel = preset.quality
    rfb.compressionLevel = preset.compression

    rfb.addEventListener('connect', () => {
      clearAuthPrompt()
      status.value = 'connected'
      statusMsg.value = hasSavedPassword
        ? 'Connected (server-side authenticated)'
        : 'Connected (interactive session)'
      rfb?.focus()
    })

    rfb.addEventListener('credentialsrequired', (e) => {
      const types = Array.isArray(e.detail?.types) ? (e.detail.types as string[]) : ['password']
      authNeedUsername.value = types.includes('username')
      authUsername.value = ''
      authPassword.value = ''
      authPromptVisible.value = true
      statusMsg.value = 'VNC host requires credentials for this session.'
      void nextTick(() => authPasswordInput.value?.focus())
    })

    rfb.addEventListener('securityfailure', (e) => {
      clearAuthPrompt()
      const reason = String(e.detail?.reason || 'Authentication failed')
      lastSecurityError = reason
      status.value = 'error'
      statusMsg.value = reason
    })

    rfb.addEventListener('disconnect', (e) => {
      clearAuthPrompt()
      const clean = Boolean(e.detail?.clean)
      if (lastSecurityError) {
        status.value = 'error'
        statusMsg.value = lastSecurityError
      } else {
        status.value = clean ? 'disconnected' : 'error'
        statusMsg.value = clean ? 'Session closed.' : 'Disconnected unexpectedly from VNC host.'
      }
      rfb = null
    })
  } catch (err) {
    status.value = 'error'
    statusMsg.value = err instanceof Error ? err.message : 'Failed to start VNC session'
  }
}

function submitCredentials() {
  if (!rfb) return
  const creds: { password?: string; username?: string } = { password: authPassword.value }
  if (authNeedUsername.value) {
    creds.username = authUsername.value
  }
  rfb.sendCredentials(creds)
  authPromptVisible.value = false
  authPassword.value = ''
  statusMsg.value = 'Authenticating with VNC host...'
}

function cancelCredentials() {
  disconnect()
  status.value = 'disconnected'
  statusMsg.value = 'Connection cancelled.'
}

function sendCtrlAltDel() {
  rfb?.sendCtrlAltDel()
}

function toggleFullscreen() {
  if (document.fullscreenElement) {
    void document.exitFullscreen()
  } else {
    // Fullscreen documentElement (not a child div) so noVNC's fallback cursor canvas
    // attached to document.body remains inside the fullscreen top layer.
    void document.documentElement.requestFullscreen()
  }
}
</script>

<template>
  <div
    ref="viewerShell"
    class="flex flex-col gap-4"
    :class="isFullscreen ? 'fixed inset-0 z-40 h-screen w-screen bg-slate-950 p-3' : 'h-full'"
  >
    <!-- Control Toolbar -->
    <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-800 bg-slate-900/70 px-4 py-3">
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex items-center gap-2 text-sm font-semibold text-white">
          <Monitor class="h-4 w-4 text-emerald-400" />
          WebVNC
        </div>

        <select
          v-model="selectedId"
          :disabled="loadingTargets || status === 'connected'"
          class="min-w-[220px] rounded-lg border border-slate-700 bg-slate-950 px-3 py-1.5 text-xs text-white"
        >
          <option v-if="targets.length === 0" value="">No active VNC edge points</option>
          <option v-for="t in targets" :key="t.id" :value="t.id">
            {{ t.name }} ({{ t.host }}:{{ t.port }}){{ t.hasPassword ? '' : ' [prompt password]' }}
          </option>
        </select>

        <button
          v-if="status !== 'connected' && status !== 'connecting'"
          type="button"
          :disabled="!selectedId"
          class="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500 px-3 py-1.5 text-xs font-semibold text-slate-950 hover:bg-emerald-400 disabled:opacity-50"
          @click="connect"
        >
          <Plug class="h-3.5 w-3.5" />
          Connect
        </button>

        <button
          v-else
          type="button"
          class="inline-flex items-center gap-1.5 rounded-lg bg-rose-500 px-3 py-1.5 text-xs font-semibold text-white hover:bg-rose-400"
          @click="disconnect"
        >
          <PowerOff class="h-3.5 w-3.5" />
          Disconnect
        </button>

        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-lg border border-slate-700 bg-slate-800 px-2.5 py-1.5 text-xs text-slate-200 hover:bg-slate-700"
          title="Reload edge points"
          @click="loadTargets"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loadingTargets }" />
        </button>
      </div>

      <div class="flex flex-wrap items-center gap-2 text-xs">
        <select
          v-model="qualityPreset"
          title="VNC image quality and compression preset"
          class="rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-xs text-slate-200"
        >
          <option v-for="(preset, key) in QUALITY_PRESETS" :key="key" :value="key">
            {{ preset.label }}
          </option>
        </select>

        <label class="inline-flex items-center gap-1.5 text-slate-300">
          <input v-model="scaleViewport" type="checkbox" />
          Fit to screen
        </label>
        <button
          type="button"
          :disabled="status !== 'connected'"
          class="inline-flex items-center gap-1 rounded-lg border border-slate-700 bg-slate-800 px-2.5 py-1.5 text-slate-200 hover:bg-slate-700 disabled:opacity-40"
          @click="sendCtrlAltDel"
        >
          <Terminal class="h-3.5 w-3.5" />
          Ctrl+Alt+Del
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-lg border border-slate-700 bg-slate-800 px-2.5 py-1.5 text-slate-200 hover:bg-slate-700"
          @click="toggleFullscreen"
        >
          <Minimize2 v-if="isFullscreen" class="h-3.5 w-3.5" />
          <Maximize2 v-else class="h-3.5 w-3.5" />
          {{ isFullscreen ? 'Exit Fullscreen' : 'Fullscreen' }}
        </button>
      </div>
    </div>

    <!-- Status Notice -->
    <div
      v-if="statusMsg"
      class="rounded-lg border px-3.5 py-2 text-xs"
      :class="
        status === 'error'
          ? 'border-rose-500/40 bg-rose-500/10 text-rose-200'
          : status === 'connected'
            ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200'
            : 'border-slate-700 bg-slate-900/70 text-slate-300'
      "
    >
      {{ statusMsg }}
    </div>

    <!-- RFB Canvas Stage -->
    <div class="relative flex-1 min-h-[480px] overflow-hidden rounded-xl border border-slate-800 bg-slate-950">
      <div
        v-if="status !== 'connected' && status !== 'connecting'"
        class="absolute inset-0 flex flex-col items-center justify-center p-6 text-center text-slate-500"
      >
        <Monitor class="h-10 w-10 text-slate-700" />
        <p class="mt-3 text-sm font-medium text-slate-300">No active VNC session</p>
        <p class="mt-1 max-w-md text-xs text-slate-500">
          Select a VNC edge point above and click Connect. Targets with saved passwords authenticate server-side; targets without a saved password prompt here at connect time.
        </p>
      </div>

      <!-- Connection-time VNC Password Modal -->
      <div
        v-if="authPromptVisible"
        class="absolute inset-0 z-20 flex items-center justify-center bg-slate-950/85 p-4 backdrop-blur-xs"
      >
        <form
          class="w-full max-w-sm rounded-xl border border-slate-700 bg-slate-900 p-5 shadow-2xl"
          @submit.prevent="submitCredentials"
        >
          <div class="flex items-center gap-2 text-sm font-semibold text-white">
            <KeyRound class="h-4 w-4 text-emerald-400" />
            VNC Password Required
          </div>
          <p class="mt-1 text-xs text-slate-400">
            Enter the password for {{ selectedTarget?.name || 'this VNC host' }}. It is used only for this session and is not saved.
          </p>

          <div class="mt-4 space-y-3">
            <div v-if="authNeedUsername">
              <label class="block text-xs font-medium text-slate-300">Username</label>
              <input
                v-model="authUsername"
                type="text"
                autocomplete="username"
                required
                class="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white focus:border-emerald-500 focus:outline-none"
              />
            </div>

            <div>
              <label class="block text-xs font-medium text-slate-300">VNC Password</label>
              <input
                ref="authPasswordInput"
                v-model="authPassword"
                type="password"
                autocomplete="current-password"
                required
                class="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white focus:border-emerald-500 focus:outline-none"
              />
            </div>
          </div>

          <div class="mt-5 flex justify-end gap-2">
            <button
              type="button"
              class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-1.5 text-xs text-slate-200 hover:bg-slate-700"
              @click="cancelCredentials"
            >
              Cancel
            </button>
            <button
              type="submit"
              class="rounded-lg bg-emerald-500 px-3.5 py-1.5 text-xs font-semibold text-slate-950 hover:bg-emerald-400"
            >
              Authenticate
            </button>
          </div>
        </form>
      </div>

      <div ref="screenHost" class="h-full w-full" />
    </div>
  </div>
</template>
