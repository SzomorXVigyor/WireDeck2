<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Activity,
  Bell,
  ChevronDown,
  LineChart,
  LogOut,
  Menu,
  Monitor,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
  Settings,
  Shield,
  Sliders,
  SlidersHorizontal,
  User as UserIcon,
  X,
} from 'lucide-vue-next'
import WireDeckLogo from '../components/WireDeckLogo.vue'
import { useShellNav } from '../composables/useShellNav'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/sites'
import { useAlarmStore } from '../stores/alarms'

const appVersion = __APP_VERSION__

const auth = useAuthStore()
const siteStore = useSiteStore()
const alarmStore = useAlarmStore()
const route = useRoute()
const router = useRouter()

const {
  desktopCollapsed,
  mobileDrawerOpen,
  toggleDesktopCollapsed,
  openMobileDrawer,
  closeMobileDrawer,
} = useShellNav()

const sitesSectionOpen = ref(true)
const siteFilter = ref('')
const mobileSiteNavOpen = ref(false)

const activeSiteId = computed(() =>
  typeof route.params.siteId === 'string' ? route.params.siteId : undefined,
)
const activeSite = computed(() =>
  activeSiteId.value ? siteStore.byId(activeSiteId.value) : undefined,
)

const filteredSites = computed(() => {
  const q = siteFilter.value.trim().toLowerCase()
  if (!q) return siteStore.sites
  return siteStore.sites.filter(
    (s) => s.name.toLowerCase().includes(q) || s.slug.toLowerCase().includes(q),
  )
})

const mobileHeaderTitle = computed(() => {
  if (activeSite.value) return activeSite.value.name
  switch (route.name) {
    case 'global-alarms':
      return 'Global Alarm Center'
    case 'profile':
      return 'Profile & Password'
    case 'preferences':
      return 'Preferences'
    default:
      return 'WireDeck'
  }
})

const currentSiteSection = computed(() => {
  switch (route.name) {
    case 'site-vnc':
      return { label: 'VNC', icon: Monitor }
    case 'site-webview':
    case 'site-webview-dashboard':
      return { label: 'WebView', icon: Sliders }
    case 'site-history':
      return { label: 'History', icon: LineChart }
    case 'site-alarms':
      return { label: 'Alarms', icon: Bell }
    default:
      return { label: 'Overview', icon: Activity }
  }
})

watch(
  () => route.fullPath,
  () => {
    mobileSiteNavOpen.value = false
  },
)

function onEscape(e: KeyboardEvent) {
  if (e.key === 'Escape' && mobileSiteNavOpen.value) {
    mobileSiteNavOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('keydown', onEscape)
  siteStore.startPolling()
  alarmStore.startPolling()
})
onUnmounted(() => {
  document.removeEventListener('keydown', onEscape)
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

function siteInitials(name: string): string {
  const parts = name.trim().split(/[\s_-]+/).filter(Boolean)
  if (parts.length === 0) return 'S'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return `${parts[0][0]}${parts[1][0]}`.toUpperCase()
}
</script>

<template>
  <div class="flex h-dvh w-full overflow-hidden bg-slate-950 text-slate-100">
    <!-- Desktop Collapsible Sidebar -->
    <aside
      class="hidden shrink-0 flex-col border-r border-slate-800/80 bg-slate-900/75 transition-[width] duration-200 lg:flex"
      :class="desktopCollapsed ? 'w-[68px]' : 'w-64'"
    >
      <!-- Sidebar Brand & Collapse Toggle -->
      <div
        class="flex h-14 shrink-0 items-center border-b border-slate-800 px-3"
        :class="desktopCollapsed ? 'justify-center' : 'justify-between gap-2'"
      >
        <div v-if="!desktopCollapsed" class="flex min-w-0 items-center gap-2.5 pl-1">
          <WireDeckLogo size="md" class="shrink-0" />
          <div class="min-w-0 leading-tight">
            <p class="truncate text-sm font-semibold tracking-tight text-white">WireDeck</p>
            <p class="truncate text-[10px] uppercase tracking-wider text-slate-400">Operations</p>
          </div>
        </div>
        <button
          type="button"
          class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-800 hover:text-white"
          :title="desktopCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
          :aria-label="desktopCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
          @click="toggleDesktopCollapsed"
        >
          <PanelLeftOpen v-if="desktopCollapsed" class="h-4 w-4" />
          <PanelLeftClose v-else class="h-4 w-4" />
        </button>
      </div>

      <!-- Sidebar Body -->
      <div class="flex-1 overflow-y-auto px-2.5 py-3.5 space-y-4">
        <!-- Accessible Sites Section -->
        <div>
          <button
            v-if="!desktopCollapsed"
            type="button"
            class="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] font-semibold uppercase tracking-wider text-slate-500 transition hover:text-slate-300"
            :aria-expanded="sitesSectionOpen"
            @click="sitesSectionOpen = !sitesSectionOpen"
          >
            <span>Accessible Sites ({{ siteStore.sites.length }})</span>
            <ChevronDown
              class="h-3.5 w-3.5 transition-transform duration-150"
              :class="sitesSectionOpen ? '' : '-rotate-90'"
            />
          </button>

          <div v-if="!desktopCollapsed && sitesSectionOpen && siteStore.sites.length > 6" class="relative mt-2 mb-2 px-0.5">
            <Search class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
            <input
              v-model="siteFilter"
              type="text"
              placeholder="Filter sites..."
              class="w-full rounded-lg border border-slate-800 bg-slate-950 py-1.5 pl-8 pr-2.5 text-xs text-white placeholder-slate-500 focus:border-sky-500 focus:outline-none"
            />
          </div>

          <div
            v-if="!desktopCollapsed && sitesSectionOpen && siteStore.sites.length === 0"
            class="mt-1 px-2 text-xs text-slate-500"
          >
            No sites assigned yet.
          </div>

          <nav v-if="desktopCollapsed || sitesSectionOpen" class="mt-1.5 space-y-1">
            <RouterLink
              v-for="site in desktopCollapsed ? siteStore.sites : filteredSites"
              :key="site.id"
              :to="`/sites/${site.id}/overview`"
              :title="`${site.name} (${site.status})`"
              class="flex items-center rounded-lg text-sm transition"
              :class="[
                desktopCollapsed ? 'justify-center p-2 relative' : 'justify-between gap-2 px-2.5 py-2',
                activeSiteId === site.id
                  ? 'bg-sky-500/15 text-sky-300 font-medium'
                  : 'text-slate-300 hover:bg-slate-800/70 hover:text-white',
              ]"
            >
              <template v-if="desktopCollapsed">
                <span class="flex h-7 w-7 items-center justify-center rounded-md border border-slate-700/80 bg-slate-800/80 font-mono text-[11px] font-semibold">
                  {{ siteInitials(site.name) }}
                </span>
                <span
                  class="absolute right-1.5 top-1.5 h-2 w-2 rounded-full ring-2 ring-slate-900"
                  :class="dotColor(site.status)"
                />
              </template>
              <template v-else>
                <span class="truncate">{{ site.name }}</span>
                <span
                  class="h-2 w-2 shrink-0 rounded-full"
                  :class="dotColor(site.status)"
                  :title="site.status"
                />
              </template>
            </RouterLink>
            <p
              v-if="!desktopCollapsed && siteStore.sites.length > 0 && filteredSites.length === 0"
              class="px-2 py-2 text-xs text-slate-500"
            >
              No matching sites.
            </p>
          </nav>
        </div>

        <!-- Global Workspace Links -->
        <div class="border-t border-slate-800/80 pt-3.5 space-y-1">
          <RouterLink
            to="/alarms"
            title="Global Alarm Center"
            class="relative flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800/70 hover:text-white"
            :class="desktopCollapsed ? 'justify-center p-2.5' : 'justify-between gap-2 px-2.5 py-2'"
            active-class="bg-sky-500/15 text-sky-300 font-medium"
          >
            <span class="flex min-w-0 items-center gap-2.5">
              <Bell class="h-4 w-4 shrink-0" />
              <span v-if="!desktopCollapsed" class="truncate">Global Alarm Center</span>
            </span>
            <span
              v-if="alarmStore.summary.activeCount > 0"
              class="rounded-full font-semibold leading-none tabular-nums"
              :class="[
                desktopCollapsed
                  ? 'absolute right-1 top-1 px-1 py-0.5 text-[9px]'
                  : 'px-1.5 py-0.5 text-[10px]',
                alarmStore.summary.activeCritical > 0
                  ? 'bg-rose-500 text-white'
                  : 'bg-amber-500 text-slate-950',
              ]"
            >
              {{ alarmStore.summary.activeCount }}
            </span>
          </RouterLink>

          <RouterLink
            to="/profile"
            title="Profile & Password"
            class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800/70 hover:text-white"
            :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-2.5 py-2'"
            active-class="bg-sky-500/15 text-sky-300 font-medium"
          >
            <UserIcon class="h-4 w-4 shrink-0" />
            <span v-if="!desktopCollapsed" class="truncate">Profile & Password</span>
          </RouterLink>

          <RouterLink
            to="/preferences"
            title="Preferences"
            class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800/70 hover:text-white"
            :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-2.5 py-2'"
            active-class="bg-sky-500/15 text-sky-300 font-medium"
          >
            <SlidersHorizontal class="h-4 w-4 shrink-0" />
            <span v-if="!desktopCollapsed" class="truncate">Preferences</span>
          </RouterLink>
        </div>
      </div>

      <!-- Sidebar Footer -->
      <div class="border-t border-slate-800 p-2.5 space-y-2">
        <RouterLink
          v-if="auth.isPlatformAdmin"
          to="/admin/sites"
          title="Platform Admin Console"
          class="flex w-full items-center justify-center gap-2 rounded-lg border border-indigo-500/40 bg-indigo-500/15 text-xs font-medium text-indigo-200 transition hover:bg-indigo-500/25"
          :class="desktopCollapsed ? 'h-9' : 'h-9 px-3'"
        >
          <Shield class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">Platform Admin Console</span>
        </RouterLink>

        <div
          class="flex items-center pt-1"
          :class="desktopCollapsed ? 'justify-center' : 'justify-between gap-2 px-1'"
        >
          <span
            v-if="!desktopCollapsed"
            class="truncate text-xs text-slate-400"
            :title="auth.user?.email"
          >
            {{ auth.user?.email }}
          </span>
          <button
            type="button"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-800 hover:text-rose-300"
            title="Sign out"
            aria-label="Sign out"
            @click="handleLogout"
          >
            <LogOut class="h-4 w-4" />
          </button>
        </div>

        <div
          v-if="!desktopCollapsed"
          class="flex items-center justify-between border-t border-slate-800/70 px-1 pt-2 font-mono text-[10px] text-slate-500"
        >
          <span>WireDeck</span>
          <span>{{ appVersion }}</span>
        </div>
      </div>
    </aside>

    <!-- Mobile Slide-Over Navigation Drawer -->
    <Teleport to="body">
      <div
        v-if="mobileDrawerOpen"
        class="fixed inset-0 z-50 flex lg:hidden"
        role="dialog"
        aria-modal="true"
        aria-label="Navigation menu"
      >
        <div
          class="fixed inset-0 bg-black/65 backdrop-blur-xs"
          @click="closeMobileDrawer"
        />
        <aside class="safe-top safe-bottom relative z-10 flex h-full w-72 max-w-[85vw] flex-col border-r border-slate-800 bg-slate-900 text-slate-100 shadow-2xl">
          <div class="flex h-14 shrink-0 items-center justify-between border-b border-slate-800 px-4">
            <div class="flex items-center gap-2.5">
              <WireDeckLogo size="md" />
              <div class="leading-tight">
                <p class="text-sm font-semibold tracking-tight text-white">WireDeck</p>
                <p class="text-[10px] uppercase tracking-wider text-slate-400">Operations</p>
              </div>
            </div>
            <button
              type="button"
              aria-label="Close navigation menu"
              class="flex h-9 w-9 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-800 hover:text-white"
              @click="closeMobileDrawer"
            >
              <X class="h-5 w-5" />
            </button>
          </div>

          <div class="flex-1 overflow-y-auto px-3 py-4 space-y-5">
            <div>
              <button
                type="button"
                class="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] font-semibold uppercase tracking-wider text-slate-500"
                :aria-expanded="sitesSectionOpen"
                @click="sitesSectionOpen = !sitesSectionOpen"
              >
                <span>Accessible Sites ({{ siteStore.sites.length }})</span>
                <ChevronDown
                  class="h-3.5 w-3.5 transition-transform duration-150"
                  :class="sitesSectionOpen ? '' : '-rotate-90'"
                />
              </button>

              <div v-if="sitesSectionOpen && siteStore.sites.length > 6" class="relative mt-2 mb-2">
                <Search class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
                <input
                  v-model="siteFilter"
                  type="text"
                  placeholder="Filter sites..."
                  class="w-full rounded-lg border border-slate-700 bg-slate-950 py-2 pl-8 pr-3 text-xs text-white placeholder-slate-500 focus:border-sky-500 focus:outline-none"
                />
              </div>

              <div v-if="sitesSectionOpen && siteStore.sites.length === 0" class="mt-2 px-2 text-xs text-slate-500">
                No sites assigned yet.
              </div>

              <nav v-if="sitesSectionOpen" class="mt-2 space-y-1">
                <RouterLink
                  v-for="site in filteredSites"
                  :key="site.id"
                  :to="`/sites/${site.id}/overview`"
                  class="flex h-10 items-center justify-between gap-2 rounded-lg px-3 text-sm transition"
                  :class="
                    activeSiteId === site.id
                      ? 'bg-sky-500/15 text-sky-300 font-medium'
                      : 'text-slate-300 hover:bg-slate-800/70 hover:text-white'
                  "
                >
                  <span class="truncate">{{ site.name }}</span>
                  <span class="h-2.5 w-2.5 shrink-0 rounded-full" :class="dotColor(site.status)" :title="site.status" />
                </RouterLink>
              </nav>
            </div>

            <div class="border-t border-slate-800/80 pt-4 space-y-1">
              <RouterLink
                to="/alarms"
                class="flex h-10 items-center justify-between gap-2 rounded-lg px-3 text-sm text-slate-300 hover:bg-slate-800/70 hover:text-white"
                active-class="bg-sky-500/15 text-sky-300 font-medium"
              >
                <span class="flex items-center gap-2.5">
                  <Bell class="h-4 w-4" />
                  Global Alarm Center
                </span>
                <span
                  v-if="alarmStore.summary.activeCount > 0"
                  class="rounded-full px-2 py-0.5 text-[11px] font-semibold leading-none tabular-nums"
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
                class="flex h-10 items-center gap-2.5 rounded-lg px-3 text-sm text-slate-300 hover:bg-slate-800/70 hover:text-white"
                active-class="bg-sky-500/15 text-sky-300 font-medium"
              >
                <UserIcon class="h-4 w-4" />
                Profile & Password
              </RouterLink>
              <RouterLink
                to="/preferences"
                class="flex h-10 items-center gap-2.5 rounded-lg px-3 text-sm text-slate-300 hover:bg-slate-800/70 hover:text-white"
                active-class="bg-sky-500/15 text-sky-300 font-medium"
              >
                <SlidersHorizontal class="h-4 w-4" />
                Preferences
              </RouterLink>
            </div>
          </div>

          <div class="border-t border-slate-800 p-3 space-y-2.5">
            <RouterLink
              v-if="auth.isPlatformAdmin"
              to="/admin/sites"
              class="flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-indigo-500/40 bg-indigo-500/15 px-3 text-xs font-medium text-indigo-200 hover:bg-indigo-500/25"
            >
              <Shield class="h-4 w-4" />
              Platform Admin Console
            </RouterLink>
            <div class="flex items-center justify-between gap-2 px-1">
              <span class="truncate text-xs text-slate-400" :title="auth.user?.email">{{ auth.user?.email }}</span>
              <button
                type="button"
                class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-rose-300"
                title="Sign out"
                aria-label="Sign out"
                @click="handleLogout"
              >
                <LogOut class="h-4 w-4" />
              </button>
            </div>
            <div class="flex items-center justify-between border-t border-slate-800/70 px-1 pt-2 font-mono text-[10px] text-slate-500">
              <span>WireDeck</span>
              <span>{{ appVersion }}</span>
            </div>
          </div>
        </aside>
      </div>
    </Teleport>

    <!-- Main Operator Area -->
    <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <!-- Mobile Top Bar (< lg) -->
      <header class="safe-top shrink-0 border-b border-slate-800 bg-slate-900/90 lg:hidden">
        <div class="flex h-14 items-center justify-between gap-2 px-3">
          <div class="flex min-w-0 items-center gap-2">
            <button
              type="button"
              aria-label="Open navigation menu"
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-300 transition hover:bg-slate-800 hover:text-white"
              @click="openMobileDrawer"
            >
              <Menu class="h-5 w-5" />
            </button>
            <div class="flex min-w-0 items-center gap-2">
              <span
                v-if="activeSite"
                class="h-2.5 w-2.5 shrink-0 rounded-full"
                :class="dotColor(activeSite.status)"
                :title="activeSite.status"
              />
              <WireDeckLogo v-else size="sm" class="shrink-0" />
              <span class="truncate text-sm font-semibold leading-none text-white">{{ mobileHeaderTitle }}</span>
            </div>
          </div>

          <div class="flex shrink-0 items-center gap-1.5">
            <RouterLink
              v-if="activeSite && siteStore.canOperateSite(activeSite.id)"
              :to="`/sites/${activeSite.id}/manage/tunnel`"
              class="inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/15 px-2.5 text-xs font-medium leading-none text-amber-200 hover:bg-amber-500/25"
            >
              <Settings class="h-3.5 w-3.5" />
              <span class="hidden sm:inline">Manage Site</span>
              <span class="sm:hidden">Manage</span>
            </RouterLink>
            <RouterLink
              v-else-if="alarmStore.summary.activeCount > 0 && route.name !== 'global-alarms'"
              to="/alarms"
              aria-label="Active alarms"
              class="inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border border-rose-500/40 bg-rose-500/15 px-2.5 text-xs font-semibold leading-none text-rose-200"
            >
              <Bell class="h-3.5 w-3.5" />
              <span class="tabular-nums">{{ alarmStore.summary.activeCount }}</span>
            </RouterLink>
          </div>
        </div>
      </header>

      <!-- Mobile Collapsible Site Section Menu (< lg) -->
      <div
        v-if="activeSite"
        class="shrink-0 border-b border-slate-800 bg-slate-900/50 lg:hidden"
      >
        <button
          type="button"
          class="flex h-11 w-full items-center justify-between gap-2 px-4 text-xs font-medium text-slate-200 transition hover:bg-slate-800/40"
          :aria-expanded="mobileSiteNavOpen"
          @click="mobileSiteNavOpen = !mobileSiteNavOpen"
        >
          <span class="flex items-center gap-2 leading-none">
            <component :is="currentSiteSection.icon" class="h-4 w-4 text-sky-400" />
            <span>{{ currentSiteSection.label }}</span>
            <span
              v-if="activeSite.webviewEnabled && alarmStore.siteActiveCount(activeSite.id) > 0 && route.name !== 'site-alarms'"
              class="rounded-full bg-rose-500 px-1.5 py-0.5 text-[10px] font-semibold leading-none tabular-nums text-white"
            >
              {{ alarmStore.siteActiveCount(activeSite.id) }}
            </span>
          </span>
          <ChevronDown
            class="h-4 w-4 text-slate-400 transition-transform duration-150"
            :class="mobileSiteNavOpen ? 'rotate-180' : ''"
          />
        </button>

        <nav
          v-if="mobileSiteNavOpen"
          aria-label="Site sections"
          class="space-y-1 border-t border-slate-800/80 px-3 py-2"
        >
          <RouterLink
            :to="`/sites/${activeSite.id}/overview`"
            class="flex h-10 items-center gap-2.5 rounded-lg px-3 text-xs font-medium text-slate-300 transition hover:bg-slate-800 hover:text-white"
            active-class="bg-slate-800 text-white"
          >
            <Activity class="h-4 w-4" /> Overview
          </RouterLink>
          <RouterLink
            v-if="activeSite.vncEnabled"
            :to="`/sites/${activeSite.id}/vnc`"
            class="flex h-10 items-center gap-2.5 rounded-lg px-3 text-xs font-medium text-slate-300 transition hover:bg-slate-800 hover:text-white"
            active-class="bg-slate-800 text-white"
          >
            <Monitor class="h-4 w-4" /> VNC
          </RouterLink>
          <RouterLink
            v-if="activeSite.webviewEnabled"
            :to="`/sites/${activeSite.id}/webview`"
            class="flex h-10 items-center gap-2.5 rounded-lg px-3 text-xs font-medium text-slate-300 transition hover:bg-slate-800 hover:text-white"
            active-class="bg-slate-800 text-white"
          >
            <Sliders class="h-4 w-4" /> WebView
          </RouterLink>
          <RouterLink
            v-if="activeSite.webviewEnabled"
            :to="`/sites/${activeSite.id}/history`"
            class="flex h-10 items-center gap-2.5 rounded-lg px-3 text-xs font-medium text-slate-300 transition hover:bg-slate-800 hover:text-white"
            active-class="bg-slate-800 text-white"
          >
            <LineChart class="h-4 w-4" /> History
          </RouterLink>
          <RouterLink
            v-if="activeSite.webviewEnabled"
            :to="`/sites/${activeSite.id}/alarms`"
            class="flex h-10 items-center justify-between gap-2 rounded-lg px-3 text-xs font-medium text-slate-300 transition hover:bg-slate-800 hover:text-white"
            active-class="bg-slate-800 text-white"
          >
            <span class="flex items-center gap-2.5">
              <Bell class="h-4 w-4" /> Alarms
            </span>
            <span
              v-if="alarmStore.siteActiveCount(activeSite.id) > 0"
              class="rounded-full bg-rose-500 px-1.5 py-0.5 text-[10px] font-semibold leading-none tabular-nums text-white"
            >
              {{ alarmStore.siteActiveCount(activeSite.id) }}
            </span>
          </RouterLink>
        </nav>
      </div>

      <!-- Desktop Site Context Sub-Header (lg+) -->
      <header
        v-if="activeSite"
        class="hidden h-14 shrink-0 items-center justify-between gap-4 border-b border-slate-800 bg-slate-900/40 px-6 lg:flex"
      >
        <div class="flex min-w-0 items-center gap-4">
          <div class="flex min-w-0 items-center gap-2">
            <span class="h-2 w-2 shrink-0 rounded-full" :class="dotColor(activeSite.status)" :title="activeSite.status" />
            <h1 class="truncate text-sm font-semibold leading-none text-white">{{ activeSite.name }}</h1>
          </div>

          <nav aria-label="Site sections" class="flex items-center gap-1">
            <RouterLink
              :to="`/sites/${activeSite.id}/overview`"
              class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium leading-none text-slate-400 transition hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <Activity class="h-3.5 w-3.5" /> Overview
            </RouterLink>
            <RouterLink
              v-if="activeSite.vncEnabled"
              :to="`/sites/${activeSite.id}/vnc`"
              class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium leading-none text-slate-400 transition hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <Monitor class="h-3.5 w-3.5" /> VNC
            </RouterLink>
            <RouterLink
              v-if="activeSite.webviewEnabled"
              :to="`/sites/${activeSite.id}/webview`"
              class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium leading-none text-slate-400 transition hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <Sliders class="h-3.5 w-3.5" /> WebView
            </RouterLink>
            <RouterLink
              v-if="activeSite.webviewEnabled"
              :to="`/sites/${activeSite.id}/history`"
              class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium leading-none text-slate-400 transition hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <LineChart class="h-3.5 w-3.5" /> History
            </RouterLink>
            <RouterLink
              v-if="activeSite.webviewEnabled"
              :to="`/sites/${activeSite.id}/alarms`"
              class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium leading-none text-slate-400 transition hover:bg-slate-800 hover:text-slate-100"
              active-class="bg-slate-800 text-white"
            >
              <Bell class="h-3.5 w-3.5" /> Alarms
              <span
                v-if="alarmStore.siteActiveCount(activeSite.id) > 0"
                class="rounded-full bg-rose-500 px-1.5 py-0.5 text-[10px] font-semibold leading-none tabular-nums text-white"
              >
                {{ alarmStore.siteActiveCount(activeSite.id) }}
              </span>
            </RouterLink>
          </nav>
        </div>

        <RouterLink
          v-if="siteStore.canOperateSite(activeSite.id)"
          :to="`/sites/${activeSite.id}/manage/tunnel`"
          class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/15 px-3 text-xs font-medium leading-none text-amber-200 transition hover:bg-amber-500/25"
        >
          <Settings class="h-3.5 w-3.5" />
          Manage Site
        </RouterLink>
      </header>

      <main class="safe-bottom flex-1 overflow-y-auto p-4 sm:p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
