<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Activity,
  Bell,
  LogOut,
  Monitor,
  Server,
  Settings,
  Shield,
  Sliders,
  SlidersHorizontal,
  User as UserIcon,
  LineChart,
} from 'lucide-vue-next'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/sites'
import { useAlarmStore } from '../stores/alarms'

const auth = useAuthStore()
const siteStore = useSiteStore()
const alarmStore = useAlarmStore()
const route = useRoute()
const router = useRouter()

const activeSiteId = computed(() =>
  typeof route.params.siteId === 'string' ? route.params.siteId : undefined,
)
const activeSite = computed(() =>
  activeSiteId.value ? siteStore.byId(activeSiteId.value) : undefined,
)

onMounted(() => {
  siteStore.startPolling()
  alarmStore.startPolling()
})
onUnmounted(() => {
  siteStore.stopPolling()
  alarmStore.stopPolling()
})

async function handleLogout() {
  siteStore.stopPolling()
  alarmStore.stopPolling()
  await auth.logout()
  await router.push({ name: 'login' })
}

function dotColor(status: string) {
  switch (status) {
    case 'ONLINE':
      return 'bg-emerald-400'
    case 'DEGRADED':
      return 'bg-amber-400'
    case 'OFFLINE':
      return 'bg-rose-500'
    default:
      return 'bg-slate-500'
  }
}
</script>

<template>
  <div class="flex h-screen bg-slate-950 text-slate-100">
    <!-- Operator Sidebar -->
    <aside class="flex w-64 shrink-0 flex-col border-r border-slate-800/80 bg-slate-900/70">
      <div class="flex h-14 items-center gap-2.5 border-b border-slate-800 px-4">
        <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-sky-500/20 text-sky-400">
          <Server class="h-4 w-4" />
        </div>
        <div>
          <p class="text-sm font-semibold tracking-tight text-white">WireDeck2</p>
          <p class="text-[10px] uppercase tracking-wider text-slate-400">Operations</p>
        </div>
      </div>

      <div class="flex-1 overflow-y-auto px-3 py-4 space-y-5">
        <div>
          <p class="px-2 pb-2 text-[11px] font-semibold uppercase tracking-wider text-slate-500">
            Accessible Sites
          </p>
          <div v-if="siteStore.sites.length === 0" class="px-2 text-xs text-slate-500">
            No sites assigned yet.
          </div>
          <nav class="space-y-1">
            <RouterLink
              v-for="site in siteStore.sites"
              :key="site.id"
              :to="`/sites/${site.id}/overview`"
              class="flex items-center justify-between rounded-lg px-2.5 py-2 text-sm transition"
              :class="
                activeSiteId === site.id
                  ? 'bg-sky-500/15 text-sky-300 font-medium'
                  : 'text-slate-300 hover:bg-slate-800/70 hover:text-white'
              "
            >
              <span class="truncate">{{ site.name }}</span>
              <span class="h-2 w-2 shrink-0 rounded-full" :class="dotColor(site.status)" :title="site.status" />
            </RouterLink>
          </nav>
        </div>

        <div class="border-t border-slate-800/80 pt-4">
          <RouterLink
            to="/alarms"
            class="flex items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-sm text-slate-300 hover:bg-slate-800/70 hover:text-white"
            active-class="bg-sky-500/15 text-sky-300 font-medium"
          >
            <span class="flex items-center gap-2.5">
              <Bell class="h-4 w-4" />
              Global Alarm Centre
            </span>
            <span
              v-if="alarmStore.summary.activeCount > 0"
              class="rounded-full px-1.5 py-0.5 text-[10px] font-semibold leading-none"
              :class="
                alarmStore.summary.activeCritical > 0
                  ? 'bg-rose-500 text-white'
                  : 'bg-amber-500 text-slate-950'
              "
            >
              {{ alarmStore.summary.activeCount }}
            </span>
          </RouterLink>
          <RouterLink
            to="/profile"
            class="mt-1 flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm text-slate-300 hover:bg-slate-800/70 hover:text-white"
            active-class="bg-sky-500/15 text-sky-300 font-medium"
          >
            <UserIcon class="h-4 w-4" />
            Profile & Password
          </RouterLink>
          <RouterLink
            to="/preferences"
            class="mt-1 flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm text-slate-300 hover:bg-slate-800/70 hover:text-white"
            active-class="bg-sky-500/15 text-sky-300 font-medium"
          >
            <SlidersHorizontal class="h-4 w-4" />
            Preferences
          </RouterLink>
        </div>
      </div>

      <div class="border-t border-slate-800 p-3 space-y-2">
        <RouterLink
          v-if="auth.isPlatformAdmin"
          to="/admin/sites"
          class="flex w-full items-center justify-center gap-2 rounded-lg border border-indigo-500/40 bg-indigo-500/15 px-3 py-2 text-xs font-medium text-indigo-200 hover:bg-indigo-500/25"
        >
          <Shield class="h-3.5 w-3.5" />
          Platform Admin Console
        </RouterLink>
        <div class="flex items-center justify-between px-1 pt-1">
          <span class="truncate text-xs text-slate-400" :title="auth.user?.email">{{ auth.user?.email }}</span>
          <button
            type="button"
            class="rounded p-1.5 text-slate-400 hover:bg-slate-800 hover:text-rose-300"
            title="Sign out"
            @click="handleLogout"
          >
            <LogOut class="h-4 w-4" />
          </button>
        </div>
      </div>
    </aside>

    <!-- Main Operator Area -->
    <div class="flex flex-1 flex-col overflow-hidden">
      <!-- Site Context Sub-Header -->
      <header
        v-if="activeSite"
        class="flex h-14 shrink-0 items-center justify-between border-b border-slate-800 bg-slate-900/40 px-6"
      >
        <div class="flex items-center gap-4">
          <h1 class="text-sm font-semibold text-white">{{ activeSite.name }}</h1>
          <nav class="flex items-center gap-1">
            <RouterLink
              :to="`/sites/${activeSite.id}/overview`"
              class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <Activity class="h-3.5 w-3.5" /> Overview
            </RouterLink>
            <RouterLink
              v-if="activeSite.webviewEnabled"
              :to="`/sites/${activeSite.id}/webview`"
              class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <Sliders class="h-3.5 w-3.5" /> WebView
            </RouterLink>
            <RouterLink
              v-if="activeSite.vncEnabled"
              :to="`/sites/${activeSite.id}/vnc`"
              class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <Monitor class="h-3.5 w-3.5" /> VNC
            </RouterLink>
            <RouterLink
              v-if="activeSite.webviewEnabled"
              :to="`/sites/${activeSite.id}/history`"
              class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <LineChart class="h-3.5 w-3.5" /> History
            </RouterLink>
            <RouterLink
              v-if="activeSite.webviewEnabled"
              :to="`/sites/${activeSite.id}/alarms`"
              class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <Bell class="h-3.5 w-3.5" /> Alarms
              <span
                v-if="alarmStore.siteActiveCount(activeSite.id) > 0"
                class="rounded-full bg-rose-500 px-1.5 py-0.5 text-[10px] font-semibold leading-none text-white"
              >
                {{ alarmStore.siteActiveCount(activeSite.id) }}
              </span>
            </RouterLink>
          </nav>
        </div>

        <RouterLink
          v-if="siteStore.canOperateSite(activeSite.id)"
          :to="`/sites/${activeSite.id}/manage/members`"
          class="inline-flex items-center gap-1.5 rounded-md border border-amber-500/40 bg-amber-500/15 px-3 py-1.5 text-xs font-medium text-amber-200 hover:bg-amber-500/25"
        >
          <Settings class="h-3.5 w-3.5" />
          Manage Site
        </RouterLink>
      </header>

      <main class="flex-1 overflow-y-auto p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
