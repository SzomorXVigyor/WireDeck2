<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ArrowLeft, Cpu, LogOut, Server, Shield, Users } from 'lucide-vue-next'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function handleLogout() {
  await auth.logout()
  await router.push({ name: 'login' })
}
</script>

<template>
  <div class="flex h-screen bg-slate-950 text-slate-100">
    <!-- Dedicated Platform Admin Console Sidebar -->
    <aside class="flex w-64 shrink-0 flex-col border-r border-indigo-500/25 bg-slate-900/90">
      <div class="flex h-14 items-center gap-2.5 border-b border-indigo-500/25 bg-indigo-950/30 px-4">
        <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/25 text-indigo-300">
          <Shield class="h-4 w-4" />
        </div>
        <div>
          <p class="text-sm font-semibold text-white">WireDeck2</p>
          <p class="text-[10px] font-semibold uppercase tracking-wider text-indigo-300">Platform Admin</p>
        </div>
      </div>

      <nav class="flex-1 p-3 space-y-1">
        <RouterLink
          to="/admin/sites"
          class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
          active-class="bg-indigo-500/20 text-indigo-200 font-medium"
        >
          <Server class="h-4 w-4" />
          Sites & Onboarding
        </RouterLink>
        <RouterLink
          to="/admin/users"
          class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
          active-class="bg-indigo-500/20 text-indigo-200 font-medium"
        >
          <Users class="h-4 w-4" />
          Users & Invites
        </RouterLink>
        <RouterLink
          to="/admin/runners"
          class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800 hover:text-white"
          active-class="bg-indigo-500/20 text-indigo-200 font-medium"
        >
          <Cpu class="h-4 w-4" />
          Tunnel Runners
        </RouterLink>
      </nav>

      <div class="border-t border-slate-800 p-3 space-y-2">
        <RouterLink
          to="/"
          class="flex w-full items-center justify-center gap-2 rounded-lg border border-slate-700 bg-slate-800/80 px-3 py-2 text-xs font-medium text-slate-200 hover:bg-slate-700"
        >
          <ArrowLeft class="h-3.5 w-3.5" />
          Exit to Operator View
        </RouterLink>
        <div class="flex items-center justify-between px-1 pt-1">
          <span class="truncate text-xs text-slate-400">{{ auth.user?.email }}</span>
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

    <main class="flex-1 overflow-y-auto p-6">
      <RouterView />
    </main>
  </div>
</template>
