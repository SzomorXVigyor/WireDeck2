<script setup lang="ts">
import { useRouter } from 'vue-router'
import {
  ArrowLeft,
  Cpu,
  LogOut,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  Server,
  SlidersHorizontal,
  User as UserIcon,
  Users,
  X,
} from 'lucide-vue-next'
import WireDeckLogo from '../components/WireDeckLogo.vue'
import { useShellNav } from '../composables/useShellNav'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const appVersion = __APP_VERSION__

const {
  desktopCollapsed,
  mobileDrawerOpen,
  toggleDesktopCollapsed,
  openMobileDrawer,
  closeMobileDrawer,
} = useShellNav()

async function handleLogout() {
  await auth.logout()
  await router.push({ name: 'login' })
}
</script>

<template>
  <div class="flex h-dvh w-full overflow-hidden bg-slate-950 text-slate-100">
    <!-- Desktop Collapsible Platform Admin Sidebar -->
    <aside
      class="hidden shrink-0 flex-col border-r border-indigo-500/25 bg-slate-900/90 transition-[width] duration-200 lg:flex"
      :class="desktopCollapsed ? 'w-[68px]' : 'w-64'"
    >
      <div
        class="flex h-14 shrink-0 items-center border-b border-indigo-500/25 bg-indigo-950/30 px-3"
        :class="desktopCollapsed ? 'justify-center' : 'justify-between gap-2'"
      >
        <div v-if="!desktopCollapsed" class="flex min-w-0 items-center gap-2.5 pl-1">
          <WireDeckLogo size="md" variant="admin" class="shrink-0" />
          <div class="min-w-0">
            <p class="truncate text-sm font-semibold text-white">WireDeck</p>
            <p class="truncate text-[10px] font-semibold uppercase tracking-wider text-indigo-300">Platform Admin</p>
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
          to="/admin/sites"
          title="Sites & Onboarding"
          class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
          :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
          active-class="bg-indigo-500/20 text-indigo-200 font-medium"
        >
          <Server class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">Sites & Onboarding</span>
        </RouterLink>
        <RouterLink
          to="/admin/users"
          title="Users & Invites"
          class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
          :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
          active-class="bg-indigo-500/20 text-indigo-200 font-medium"
        >
          <Users class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">Users & Invites</span>
        </RouterLink>
        <RouterLink
          to="/admin/runners"
          title="Tunnel Runners"
          class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
          :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
          active-class="bg-indigo-500/20 text-indigo-200 font-medium"
        >
          <Cpu class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">Tunnel Runners</span>
        </RouterLink>

        <div class="mt-4 border-t border-slate-800/80 pt-4 space-y-1">
          <RouterLink
            to="/admin/profile"
            title="Profile & Password"
            class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
            :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
            active-class="bg-indigo-500/20 text-indigo-200 font-medium"
          >
            <UserIcon class="h-4 w-4 shrink-0" />
            <span v-if="!desktopCollapsed" class="truncate">Profile & Password</span>
          </RouterLink>
          <RouterLink
            to="/admin/preferences"
            title="Preferences"
            class="flex items-center rounded-lg text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
            :class="desktopCollapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2'"
            active-class="bg-indigo-500/20 text-indigo-200 font-medium"
          >
            <SlidersHorizontal class="h-4 w-4 shrink-0" />
            <span v-if="!desktopCollapsed" class="truncate">Preferences</span>
          </RouterLink>
        </div>
      </nav>

      <div class="border-t border-slate-800 p-2.5 space-y-2">
        <RouterLink
          to="/"
          title="Exit to Operator View"
          class="flex w-full items-center justify-center gap-2 rounded-lg border border-slate-700 bg-slate-800/80 text-xs font-medium text-slate-200 transition hover:bg-slate-700"
          :class="desktopCollapsed ? 'p-2' : 'px-3 py-2'"
        >
          <ArrowLeft class="h-4 w-4 shrink-0" />
          <span v-if="!desktopCollapsed" class="truncate">Exit to Operator View</span>
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
        aria-label="Platform admin menu"
      >
        <div class="fixed inset-0 bg-black/65 backdrop-blur-xs" @click="closeMobileDrawer" />
        <aside class="safe-top safe-bottom relative z-10 flex h-full w-72 max-w-[85vw] flex-col border-r border-indigo-500/25 bg-slate-900 text-slate-100 shadow-2xl">
          <div class="flex h-14 shrink-0 items-center justify-between border-b border-indigo-500/25 bg-indigo-950/30 px-4">
            <div class="flex items-center gap-2.5">
              <WireDeckLogo size="md" variant="admin" />
              <div>
                <p class="text-sm font-semibold text-white">WireDeck</p>
                <p class="text-[10px] font-semibold uppercase tracking-wider text-indigo-300">Platform Admin</p>
              </div>
            </div>
            <button
              type="button"
              aria-label="Close navigation menu"
              class="flex h-9 w-9 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
              @click="closeMobileDrawer"
            >
              <X class="h-5 w-5" />
            </button>
          </div>

          <nav class="flex-1 overflow-y-auto p-3 space-y-1">
            <RouterLink
              to="/admin/sites"
              class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
              active-class="bg-indigo-500/20 text-indigo-200 font-medium"
            >
              <Server class="h-4 w-4" />
              Sites & Onboarding
            </RouterLink>
            <RouterLink
              to="/admin/users"
              class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
              active-class="bg-indigo-500/20 text-indigo-200 font-medium"
            >
              <Users class="h-4 w-4" />
              Users & Invites
            </RouterLink>
            <RouterLink
              to="/admin/runners"
              class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
              active-class="bg-indigo-500/20 text-indigo-200 font-medium"
            >
              <Cpu class="h-4 w-4" />
              Tunnel Runners
            </RouterLink>

            <div class="mt-4 border-t border-slate-800/80 pt-4 space-y-1">
              <RouterLink
                to="/admin/profile"
                class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
                active-class="bg-indigo-500/20 text-indigo-200 font-medium"
              >
                <UserIcon class="h-4 w-4" />
                Profile & Password
              </RouterLink>
              <RouterLink
                to="/admin/preferences"
                class="flex min-h-[42px] items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
                active-class="bg-indigo-500/20 text-indigo-200 font-medium"
              >
                <SlidersHorizontal class="h-4 w-4" />
                Preferences
              </RouterLink>
            </div>
          </nav>

          <div class="border-t border-slate-800 p-3 space-y-2.5">
            <RouterLink
              to="/"
              class="flex min-h-[40px] w-full items-center justify-center gap-2 rounded-lg border border-slate-700 bg-slate-800/80 px-3 py-2 text-xs font-medium text-slate-200 hover:bg-slate-700"
            >
              <ArrowLeft class="h-4 w-4" />
              Exit to Operator View
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

    <!-- Main Area -->
    <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <!-- Mobile Top Bar (< lg) -->
      <header class="safe-top shrink-0 border-b border-indigo-500/25 bg-indigo-950/30 lg:hidden">
        <div class="flex h-14 items-center justify-between gap-2 px-3">
          <div class="flex min-w-0 items-center gap-2">
            <button
              type="button"
              aria-label="Open admin menu"
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-200 transition hover:bg-slate-800/80"
              @click="openMobileDrawer"
            >
              <Menu class="h-5 w-5" />
            </button>
            <WireDeckLogo size="sm" variant="admin" class="shrink-0" />
            <p class="truncate text-xs font-semibold uppercase tracking-wider leading-none text-indigo-300">Platform Admin</p>
          </div>

          <RouterLink
            to="/"
            class="inline-flex h-8 shrink-0 items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900/90 px-2.5 text-xs font-medium leading-none text-slate-200 hover:bg-slate-800"
          >
            <ArrowLeft class="h-3.5 w-3.5" />
            <span>Exit</span>
          </RouterLink>
        </div>
      </header>

      <main class="safe-bottom flex-1 overflow-y-auto p-4 sm:p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
