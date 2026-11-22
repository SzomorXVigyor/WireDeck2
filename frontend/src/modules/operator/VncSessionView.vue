<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import RFB from '@novnc/novnc'
import {
  Eye,
  EyeOff,
  KeyRound,
  Maximize2,
  Minimize2,
  Monitor,
  Plug,
  PowerOff,
  Scan,
  SlidersHorizontal,
  Terminal,
} from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import type { VncTarget } from '../../types/vnc'

type QualityPresetKey = 'high' | 'balanced' | 'low' | 'performance'

const QUALITY_PRESETS: Record<QualityPresetKey, { label: string; quality: number; compression: number }> = {
  high: { label: 'High', quality: 9, compression: 1 },
  balanced: { label: 'Balanced', quality: 6, compression: 2 },
  low: { label: 'Low bandwidth', quality: 2, compression: 6 },
  performance: { label: 'Performance', quality: 0, compression: 0 },
}

const route = useRoute()
const siteId = computed(() => String(route.params.siteId || ''))
const targetId = computed(() => String(route.params.targetId || ''))

const target = ref<VncTarget | null>(null)
const status = ref<'idle' | 'connecting' | 'connected' | 'disconnected' | 'error'>('idle')
const statusMsg = ref<string>('Loading endpoint...')
const scaleViewport = ref(true)
const viewOnly = ref(false)
const qualityPreset = ref<QualityPresetKey>('balanced')
const isFullscreen = ref(false)
const mobileMenuOpen = ref(false)
const mobileMenuRef = ref<HTMLElement | null>(null)

// Connection-time VNC credential prompt (used only when target has no saved server-side password)
const authPromptVisible = ref(false)
const authNeedUsername = ref(false)
const authUsername = ref('')
const authPassword = ref('')
const authPasswordInput = ref<HTMLInputElement | null>(null)

const screenHost = ref<HTMLElement | null>(null)
let rfb: RFB | null = null
let lastSecurityError = ''

const statusDotClass = computed(() => {
  switch (status.value) {
    case 'connected':
      return 'bg-emerald-400'
    case 'connecting':
      return 'bg-amber-400 animate-pulse'
    case 'error':
      return 'bg-rose-500'
    default:
      return 'bg-slate-500'
  }
})

watch(scaleViewport, (val) => {
  if (rfb) {
    rfb.scaleViewport = val
  }
})

watch(viewOnly, (val) => {
  if (rfb) {
    rfb.viewOnly = val
  }
})

watch(qualityPreset, (key) => {
  if (rfb) {
    const preset = QUALITY_PRESETS[key]
    rfb.qualityLevel = preset.quality
    rfb.compressionLevel = preset.compression
  }
})

function handleFullscreenChange() {
  isFullscreen.value = Boolean(document.fullscreenElement)
}

function handleDocumentClick(e: MouseEvent) {
  if (!mobileMenuOpen.value) return
  if (mobileMenuRef.value && !mobileMenuRef.value.contains(e.target as Node)) {
    mobileMenuOpen.value = false
  }
}

function handleDocumentKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && mobileMenuOpen.value) {
    mobileMenuOpen.value = false
  }
}

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
    statusMsg.value = 'Disconnected.'
  }
}

async function initAndConnect() {
  status.value = 'connecting'
  statusMsg.value = 'Loading VNC endpoint...'
  try {
    const res = await apiRequest<{ targets: VncTarget[] }>(`/api/v1/vnc/sites/${siteId.value}/targets`)
    const found = (res.targets || []).find((t) => t.id === targetId.value) ?? null
    if (!found) {
      status.value = 'error'
      statusMsg.value = 'VNC endpoint not found or disabled on this site.'
      return
    }
    target.value = found
    document.title = `${found.name} - WebVNC`
    await connect()
  } catch (err) {
    status.value = 'error'
    statusMsg.value = err instanceof Error ? err.message : 'Failed to load VNC endpoint'
  }
}

async function connect() {
  if (!targetId.value) return
  disconnect()
  lastSecurityError = ''
  status.value = 'connecting'
  statusMsg.value = 'Requesting single-use session ticket...'
  try {
    const ticketRes = await apiRequest<{ ticket: string; expiresAt: string }>(
      `/api/v1/vnc/sites/${siteId.value}/ws-ticket`,
      {
        method: 'POST',
        body: JSON.stringify({ targetId: targetId.value }),
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
    const hasSavedPassword = Boolean(target.value?.hasPassword)

    rfb = new RFB(screenHost.value, wsUrl, { wsProtocols: ['binary'] })
    rfb.scaleViewport = scaleViewport.value
    rfb.resizeSession = false
    rfb.viewOnly = viewOnly.value
    rfb.showDotCursor = true
    rfb.background = '#020617'
    rfb.qualityLevel = preset.quality
    rfb.compressionLevel = preset.compression

    rfb.addEventListener('connect', () => {
      clearAuthPrompt()
      status.value = 'connected'
      statusMsg.value = hasSavedPassword ? 'Connected (server-side auth)' : 'Connected'
      rfb?.focus()
    })

    rfb.addEventListener('credentialsrequired', (e) => {
      const types = Array.isArray(e.detail?.types) ? (e.detail.types as string[]) : ['password']
      authNeedUsername.value = types.includes('username')
      authUsername.value = ''
      authPassword.value = ''
      authPromptVisible.value = true
      statusMsg.value = 'Password required'
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
  statusMsg.value = 'Authenticating...'
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
    // Fullscreen documentElement so noVNC's document.body fallback cursor canvas stays visible.
    void document.documentElement.requestFullscreen()
  }
}

onMounted(() => {
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  document.addEventListener('click', handleDocumentClick)
  document.addEventListener('keydown', handleDocumentKeydown)
  void initAndConnect()
})

onUnmounted(() => {
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  document.removeEventListener('click', handleDocumentClick)
  document.removeEventListener('keydown', handleDocumentKeydown)
  if (document.fullscreenElement) {
    void document.exitFullscreen()
  }
  disconnect()
})
</script>

<template>
  <div class="flex h-screen w-screen flex-col overflow-hidden bg-slate-950 text-slate-100">
    <!-- Dedicated Session Control Bar -->
    <header class="flex h-12 shrink-0 items-center justify-between gap-3 border-b border-slate-800 bg-slate-900/90 px-4">
      <div class="flex items-center gap-3 min-w-0">
        <Monitor class="h-4 w-4 shrink-0 text-emerald-400" />
        <span class="h-2 w-2 shrink-0 rounded-full" :class="statusDotClass" :title="status" />
        <div class="flex items-baseline gap-2 min-w-0">
          <span class="truncate text-sm font-semibold text-white">
            {{ target?.name || 'WebVNC Session' }}
          </span>
          <span v-if="target" class="hidden sm:inline font-mono text-xs text-slate-400">
            {{ target.host }}:{{ target.port }}
          </span>
        </div>
        <span
          v-if="statusMsg"
          class="hidden md:inline truncate rounded-md px-2 py-0.5 text-[11px]"
          :class="
            status === 'error'
              ? 'bg-rose-500/15 text-rose-200'
              : status === 'connected'
                ? 'bg-emerald-500/15 text-emerald-200'
                : 'bg-slate-800 text-slate-300'
          "
        >
          {{ statusMsg }}
        </span>
      </div>

      <!-- Quality Selector & Extra Controls -->
      <div class="flex items-center gap-2 text-xs">
        <!-- Desktop inline controls -->
        <div class="hidden lg:flex items-center gap-2">
          <select
            v-model="qualityPreset"
            aria-label="VNC quality preset"
            title="Image quality and compression level"
            class="rounded-md border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-xs text-slate-200"
          >
            <option v-for="(preset, key) in QUALITY_PRESETS" :key="key" :value="key">
              {{ preset.label }}
            </option>
          </select>

          <button
            type="button"
            class="inline-flex items-center gap-1 rounded-md border px-2.5 py-1.5 text-xs transition"
            :class="
              scaleViewport
                ? 'border-emerald-500/40 bg-emerald-500/15 text-emerald-200'
                : 'border-slate-700 bg-slate-800 text-slate-300 hover:bg-slate-700'
            "
            title="Toggle fit to window vs 1:1 native resolution"
            @click="scaleViewport = !scaleViewport"
          >
            <Scan class="h-3.5 w-3.5" />
            {{ scaleViewport ? 'Fit' : '1:1' }}
          </button>

          <button
            type="button"
            class="inline-flex items-center gap-1 rounded-md border px-2.5 py-1.5 text-xs transition"
            :class="
              viewOnly
                ? 'border-amber-500/40 bg-amber-500/15 text-amber-200'
                : 'border-slate-700 bg-slate-800 text-slate-300 hover:bg-slate-700'
            "
            title="Toggle read-only view mode (blocks mouse and keyboard input)"
            @click="viewOnly = !viewOnly"
          >
            <Eye v-if="viewOnly" class="h-3.5 w-3.5" />
            <EyeOff v-else class="h-3.5 w-3.5" />
            {{ viewOnly ? 'View only' : 'Interactive' }}
          </button>

          <button
            type="button"
            :disabled="status !== 'connected' || viewOnly"
            class="inline-flex items-center gap-1 rounded-md border border-slate-700 bg-slate-800 px-2.5 py-1.5 text-slate-200 hover:bg-slate-700 disabled:opacity-40"
            title="Send Ctrl+Alt+Del to remote host"
            @click="sendCtrlAltDel"
          >
            <Terminal class="h-3.5 w-3.5" />
            Ctrl+Alt+Del
          </button>

          <button
            type="button"
            class="inline-flex items-center gap-1 rounded-md border border-slate-700 bg-slate-800 px-2.5 py-1.5 text-slate-200 hover:bg-slate-700"
            @click="toggleFullscreen"
          >
            <Minimize2 v-if="isFullscreen" class="h-3.5 w-3.5" />
            <Maximize2 v-else class="h-3.5 w-3.5" />
            {{ isFullscreen ? 'Exit' : 'Fullscreen' }}
          </button>
        </div>

        <!-- Compact mobile / tablet options menu -->
        <div ref="mobileMenuRef" class="relative lg:hidden">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border border-slate-700 bg-slate-800 px-2.5 py-1.5 text-xs text-slate-200 hover:bg-slate-700"
            aria-haspopup="true"
            :aria-expanded="mobileMenuOpen"
            @click.stop="mobileMenuOpen = !mobileMenuOpen"
          >
            <SlidersHorizontal class="h-3.5 w-3.5" />
            Options
          </button>

          <div
            v-if="mobileMenuOpen"
            class="absolute right-0 top-full z-30 mt-1.5 w-56 rounded-xl border border-slate-700 bg-slate-900 p-2.5 shadow-2xl space-y-2"
          >
            <div>
              <label class="block px-1 pb-1 text-[11px] font-medium text-slate-400">Quality</label>
              <select
                v-model="qualityPreset"
                aria-label="VNC quality preset"
                class="w-full rounded-md border border-slate-700 bg-slate-950 px-2.5 py-1.5 text-xs text-slate-200"
              >
                <option v-for="(preset, key) in QUALITY_PRESETS" :key="key" :value="key">
                  {{ preset.label }}
                </option>
              </select>
            </div>

            <div class="border-t border-slate-800 pt-2 space-y-1">
              <button
                type="button"
                class="flex w-full items-center justify-between rounded-md px-2.5 py-2 text-left text-xs text-slate-200 hover:bg-slate-800"
                @click="scaleViewport = !scaleViewport"
              >
                <span class="inline-flex items-center gap-2">
                  <Scan class="h-3.5 w-3.5 text-slate-400" />
                  Scaling
                </span>
                <span class="font-medium text-emerald-300">{{ scaleViewport ? 'Fit' : '1:1' }}</span>
              </button>

              <button
                type="button"
                class="flex w-full items-center justify-between rounded-md px-2.5 py-2 text-left text-xs text-slate-200 hover:bg-slate-800"
                @click="viewOnly = !viewOnly"
              >
                <span class="inline-flex items-center gap-2">
                  <Eye v-if="viewOnly" class="h-3.5 w-3.5 text-amber-400" />
                  <EyeOff v-else class="h-3.5 w-3.5 text-slate-400" />
                  Input mode
                </span>
                <span class="font-medium" :class="viewOnly ? 'text-amber-300' : 'text-slate-300'">
                  {{ viewOnly ? 'View only' : 'Interactive' }}
                </span>
              </button>

              <button
                type="button"
                :disabled="status !== 'connected' || viewOnly"
                class="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-xs text-slate-200 hover:bg-slate-800 disabled:opacity-40"
                @click="sendCtrlAltDel(); mobileMenuOpen = false"
              >
                <Terminal class="h-3.5 w-3.5 text-slate-400" />
                Send Ctrl+Alt+Del
              </button>

              <button
                type="button"
                class="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-xs text-slate-200 hover:bg-slate-800"
                @click="toggleFullscreen(); mobileMenuOpen = false"
              >
                <Minimize2 v-if="isFullscreen" class="h-3.5 w-3.5 text-slate-400" />
                <Maximize2 v-else class="h-3.5 w-3.5 text-slate-400" />
                {{ isFullscreen ? 'Exit Fullscreen' : 'Fullscreen' }}
              </button>
            </div>
          </div>
        </div>

        <button
          v-if="status === 'connected' || status === 'connecting'"
          type="button"
          class="inline-flex items-center gap-1 rounded-md bg-rose-500 px-2.5 py-1.5 font-semibold text-white hover:bg-rose-400"
          @click="disconnect"
        >
          <PowerOff class="h-3.5 w-3.5" />
          <span class="hidden sm:inline">Disconnect</span>
        </button>

        <button
          v-else
          type="button"
          class="inline-flex items-center gap-1 rounded-md bg-emerald-500 px-2.5 py-1.5 font-semibold text-slate-950 hover:bg-emerald-400"
          @click="connect"
        >
          <Plug class="h-3.5 w-3.5" />
          <span class="hidden sm:inline">Reconnect</span>
        </button>
      </div>
    </header>

    <!-- Full-Viewport RFB Canvas Stage -->
    <div class="relative flex-1 overflow-hidden bg-slate-950">
      <div
        v-if="status === 'disconnected' || status === 'error'"
        class="absolute inset-0 z-10 flex flex-col items-center justify-center bg-slate-950/90 p-6 text-center"
      >
        <Monitor class="h-10 w-10" :class="status === 'error' ? 'text-rose-400' : 'text-slate-600'" />
        <p class="mt-3 text-sm font-semibold text-white">
          {{ status === 'error' ? 'Connection Failed' : 'Session Disconnected' }}
        </p>
        <p class="mt-1 max-w-md text-xs text-slate-400">{{ statusMsg }}</p>
        <button
          type="button"
          class="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-emerald-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-emerald-400"
          @click="connect"
        >
          <Plug class="h-3.5 w-3.5" />
          Reconnect
        </button>
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
            Enter the password for {{ target?.name || 'this VNC host' }}. It is used only for this session and is not saved.
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
