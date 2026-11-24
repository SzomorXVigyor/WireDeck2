<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, Monitor, Sliders, Users } from 'lucide-vue-next'
import WireDeckLogo from '../components/WireDeckLogo.vue'
import { useSiteStore } from '../stores/sites'

const route = useRoute()
const siteStore = useSiteStore()

const siteId = computed(() => String(route.params.siteId || ''))
const site = computed(() => siteStore.byId(siteId.value))

onMounted(() => {
  if (siteStore.sites.length === 0) {
    void siteStore.fetchSites()
  }
})
</script>

<template>
  <div class="flex h-screen flex-col bg-slate-950 text-slate-100">
    <!-- Explicit Site Operator Management Banner -->
    <header class="flex h-14 shrink-0 items-center justify-between border-b border-amber-500/30 bg-amber-950/30 px-6">
      <div class="flex items-center gap-3">
        <WireDeckLogo size="md" variant="operator" />
        <div>
          <p class="text-xs font-semibold uppercase tracking-wider text-amber-300">Site Operator Workspace</p>
          <p class="text-sm font-semibold text-white">{{ site?.name ?? siteId }}</p>
        </div>
      </div>

      <RouterLink
        :to="`/sites/${siteId}/overview`"
        class="inline-flex items-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-slate-800"
      >
        <ArrowLeft class="h-3.5 w-3.5" />
        Back to Live View
      </RouterLink>
    </header>

    <div class="flex flex-1 overflow-hidden">
      <aside class="w-60 shrink-0 border-r border-slate-800 bg-slate-900/50 p-3">
        <nav class="space-y-1">
          <RouterLink
            :to="`/sites/${siteId}/manage/members`"
            class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800"
            active-class="bg-amber-500/15 text-amber-200 font-medium"
          >
            <Users class="h-4 w-4" />
            Site Members & Invites
          </RouterLink>
          <RouterLink
            v-if="site?.vncEnabled"
            :to="`/sites/${siteId}/manage/vnc-targets`"
            class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800"
            active-class="bg-amber-500/15 text-amber-200 font-medium"
          >
            <Monitor class="h-4 w-4" />
            VNC Edge Points
          </RouterLink>
          <RouterLink
            v-if="site?.webviewEnabled"
            :to="`/sites/${siteId}/manage/webview`"
            class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800"
            active-class="bg-amber-500/15 text-amber-200 font-medium"
          >
            <Sliders class="h-4 w-4" />
            WebView & History
          </RouterLink>
        </nav>
      </aside>

      <main class="flex-1 overflow-y-auto p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
