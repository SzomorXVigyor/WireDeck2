<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowLeft,
  LogOut,
  Menu,
  Monitor,
  PanelLeftClose,
  PanelLeftOpen,
  ShieldCheck,
  Sliders,
  Users,
  X,
} from 'lucide-vue-next'
import WireDeckLogo from '../components/WireDeckLogo.vue'
import { useShellNav } from '../composables/useShellNav'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/sites'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const siteStore = useSiteStore()
const appVersion = __APP_VERSION__

const {
  desktopCollapsed,
  mobileDrawerOpen,
  toggleDesktopCollapsed,
  openMobileDrawer,
  closeMobileDrawer,
} = useShellNav()

const siteId = computed(() => String(route.params.siteId || ''))
const site = computed(() => siteStore.byId(siteId.value))

onMounted(() => {
  if (siteStore.sites.length === 0) {
    void siteStore.fetchSites()
  }
})

async function handleLogout() {
  await auth.logout()
  await router.push({ name: 'login' })
}
</script>

<template>
  <div class="flex h-dvh w-full overflow-hidden bg-slate-950 text-slate-100">
    <!-- Desktop Collapsible Site Operator Sidebar -->
    <aside
      class="hidden shrink-0 flex-col border-r border-amber-500/25 bg-slate-900/80 transition-[width] duration-200 lg:flex"
      :class="desktopCollapsed ? 'w-[68px]' : 'w-64'"
    >
      <div
        class="flex h-14 shrink-0 items-center border-b border-amber-500/25 bg-amber-950/30 px-3"
        :class="desktopCollapsed ? 'justify-center' : 'justify-between gap-2'"
      >
        <div v-if="!desktopCollapsed" class="flex min-w-0 items-center gap-2.5 pl-1">
          <WireDeckLogo size="md" variant="operator" class="shrink-0" />
          <div class="min-w-0">
            <p class="truncate text-[10px] font-semibold uppercase tracking-wider text-amber-300">Site Operator</p>
            <p class="truncate text-sm font-semibold text-white">{{ site?.name ?? siteId }}</p>
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

      <nav class="flex-1 overflow-y-auto p-2.5 space-y-1">
        <RouterLink
          :to="`/sites/${siteId}/manage/tunnel`"
          title="Tunnel Details"
          class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
          :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
          active-class="bg-amber-500/15 text-amber-200 font-medium"
        >
          <ShieldCheck class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">Tunnel Details</span>
        </RouterLink>
        <RouterLink
          :to="`/sites/${siteId}/manage/members`"
          title="Site Members & Invites"
          class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
          :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
          active-class="bg-amber-500/15 text-amber-200 font-medium"
        >
          <Users class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">Site Members & Invites</span>
        </RouterLink>
        <RouterLink
          v-if="site?.vncEnabled"
          :to="`/sites/${siteId}/manage/vnc-targets`"
          title="VNC Edge Points"
          class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
          :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
          active-class="bg-amber-500/15 text-amber-200 font-medium"
        >
          <Monitor class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">VNC Edge Points</span>
        </RouterLink>
        <RouterLink
          v-if="site?.webviewEnabled"
          :to="`/sites/${siteId}/manage/webview`"
          title="WebView & History"
          class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
          :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
          active-class="bg-amber-500/15 text-amber-200 font-medium"
        >
          <Sliders class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">WebView & History</span>
        </RouterLink>
      </nav>

      <div class="border-t border-slate-800 p-2.5 space-y-2">
        <RouterLink
          :to="`/sites/${siteId}/overview`"
          title="Back to Live View"
          class="flex w-full items-center justify-center gap-2 rounded-lg border border-slate-700 bg-slate-800/80 text-xs font-medium text-slate-200 transition hover:bg-slate-700"
          :class="desktopCollapsed ? 'p-2' : 'px-3 py-2'"
        >
          <ArrowLeft class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">Back to Live View</span>
        </RouterLink>

        <div
          class="flex items-center pt-1"
          :class="desktopCollapsed ? 'justify-center' : 'justify-between gap-2 px-1'"
        >
          <span v-if="!desktopCollapsed" class="truncate text-xs text-slate-400" :title="auth.user?.email">
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

    <!-- Mobile Slide-Over Drawer -->
    <Teleport to="body">
      <div
        v-if="mobileDrawerOpen"
        class="fixed inset-0 z-50 flex lg:hidden"
        role="dialog"
        aria-modal="true"
        aria-label="Site operator menu"
      >
        <div class="fixed inset-0 bg-black/65 backdrop-blur-xs" @click="closeMobileDrawer" />
        <aside class="safe-top safe-bottom relative z-10 flex h-full w-72 max-w-[85vw] flex-col border-r border-amber-500/25 bg-slate-900 text-slate-100 shadow-2xl">
          <div class="flex h-14 shrink-0 items-center justify-between border-b border-amber-500/25 bg-amber-950/30 px-4">
            <div class="flex min-w-0 items-center gap-2.5">
              <WireDeckLogo size="md" variant="operator" class="shrink-0" />
              <div class="min-w-0">
                <p class="truncate text-[10px] font-semibold uppercase tracking-wider text-amber-300">Site Operator</p>
                <p class="truncate text-sm font-semibold text-white">{{ site?.name ?? siteId }}</p>
              </div>
            </div>
            <button
              type="button"
              aria-label="Close navigation menu"
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
              @click="closeMobileDrawer"
            >
              <X class="h-5 w-5" />
            </button>
          </div>

          <nav class="flex-1 overflow-y-auto p-3 space-y-1">
            <RouterLink
              :to="`/sites/${siteId}/manage/tunnel`"
              class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
              active-class="bg-amber-500/15 text-amber-200 font-medium"
            >
              <ShieldCheck class="h-4 w-4" />
              Tunnel Details
            </RouterLink>
            <RouterLink
              :to="`/sites/${siteId}/manage/members`"
              class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
              active-class="bg-amber-500/15 text-amber-200 font-medium"
            >
              <Users class="h-4 w-4" />
              Site Members & Invites
            </RouterLink>
            <RouterLink
              v-if="site?.vncEnabled"
              :to="`/sites/${siteId}/manage/vnc-targets`"
              class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
              active-class="bg-amber-500/15 text-amber-200 font-medium"
            >
              <Monitor class="h-4 w-4" />
              VNC Edge Points
            </RouterLink>
            <RouterLink
              v-if="site?.webviewEnabled"
              :to="`/sites/${siteId}/manage/webview`"
              class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
              active-class="bg-amber-500/15 text-amber-200 font-medium"
            >
              <Sliders class="h-4 w-4" />
              WebView & History
            </RouterLink>
          </nav>

          <div class="border-t border-slate-800 p-3 space-y-2.5">
            <RouterLink
              :to="`/sites/${siteId}/overview`"
              class="flex min-h-[40px] w-full items-center justify-center gap-2 rounded-lg border border-slate-700 bg-slate-800/80 px-3 py-2 text-xs font-medium text-slate-200 hover:bg-slate-700"
            >
              <ArrowLeft class="h-4 w-4" />
              Back to Live View
            </RouterLink>
            <div class="flex items-center justify-between gap-2 px-1">
              <span class="truncate text-xs text-slate-400">{{ auth.user?.email }}</span>
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

    <!-- Main Workspace Area -->
    <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <!-- Top Context Header (Mobile + Desktop bar for unambiguous Site Operator workspace banner) -->
      <header class="safe-top shrink-0 border-b border-amber-500/30 bg-amber-950/30">
        <div class="flex h-14 items-center justify-between gap-3 px-3 lg:px-6">
          <div class="flex min-w-0 items-center gap-2.5">
            <button
              type="button"
              aria-label="Open site operator menu"
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-200 transition hover:bg-slate-800/80 lg:hidden"
              @click="openMobileDrawer"
            >
              <Menu class="h-5 w-5" />
            </button>
            <WireDeckLogo size="sm" variant="operator" class="shrink-0 lg:hidden" />
            <div class="min-w-0 leading-tight">
              <p class="truncate text-[10px] font-semibold uppercase tracking-wider text-amber-300">Site Operator Workspace</p>
              <p class="truncate text-sm font-semibold text-white">{{ site?.name ?? siteId }}</p>
            </div>
          </div>

          <RouterLink
            :to="`/sites/${siteId}/overview`"
            class="inline-flex h-8 shrink-0 items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 text-xs font-medium leading-none text-slate-200 transition hover:bg-slate-800"
          >
            <ArrowLeft class="h-3.5 w-3.5" />
            <span class="hidden sm:inline">Back to Live View</span>
            <span class="sm:hidden">Live</span>
          </RouterLink>
        </div>
      </header>

      <main class="safe-bottom flex-1 overflow-y-auto p-4 sm:p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
