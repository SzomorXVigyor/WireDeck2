<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowRight, Bell, LineChart, Monitor, Sliders } from 'lucide-vue-next'
import { useAlarmStore } from '../../stores/alarms'
import { useSiteStore } from '../../stores/sites'
import StatusBadge from '../../components/StatusBadge.vue'

const route = useRoute()
const siteStore = useSiteStore()
const alarmStore = useAlarmStore()

const siteId = computed(() => String(route.params.siteId || ''))
const site = computed(() => siteStore.byId(siteId.value))
const activeAlarms = computed(() => (site.value ? alarmStore.siteActiveCount(site.value.id) : 0))
</script>

<template>
  <div v-if="site" class="w-full space-y-5 sm:space-y-6">
    <!-- Site Header Banner -->
    <section class="flex flex-col justify-between gap-3 rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:flex-row sm:items-center sm:p-5">
      <div class="flex min-w-0 flex-wrap items-center gap-2.5">
        <h2 class="truncate text-lg font-semibold tracking-tight text-white sm:text-xl">{{ site.name }}</h2>
        <span
          v-if="!site.isActive"
          class="rounded-full border border-slate-700 bg-slate-800 px-2 py-0.5 text-[10px] font-medium uppercase text-slate-400"
        >
          Inactive
        </span>
      </div>

      <div class="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t border-slate-800/80 pt-3 sm:flex-col sm:items-end sm:gap-1.5 sm:border-t-0 sm:pt-0 sm:text-right">
        <StatusBadge :status="site.status" />
        <div class="text-xs text-slate-400">
          <span class="text-slate-500">Last Handshake: </span>
          <span class="font-mono tabular-nums text-slate-200">
            {{ site.lastHandshakeAt ? new Date(site.lastHandshakeAt).toLocaleString() : 'Never' }}
          </span>
        </div>
      </div>
    </section>

    <!-- Enabled Capability Cards (VNC before WebView) -->
    <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2 lg:grid-cols-4 sm:gap-4">
      <RouterLink
        v-if="site.vncEnabled"
        :to="`/sites/${site.id}/vnc`"
        class="group flex flex-col justify-between rounded-xl border border-slate-800 bg-slate-900/60 p-4 transition hover:border-emerald-500/40 hover:bg-slate-900"
      >
        <div class="flex items-center justify-between">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-400">
            <Monitor class="h-4.5 w-4.5" />
          </div>
          <ArrowRight class="h-4 w-4 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-emerald-400" />
        </div>
        <div class="mt-3.5">
          <p class="text-sm font-semibold text-white">Browser VNC</p>
          <p class="mt-1 text-xs text-slate-400">Server-authenticated HMI access</p>
        </div>
      </RouterLink>

      <RouterLink
        v-if="site.webviewEnabled"
        :to="`/sites/${site.id}/webview`"
        class="group flex flex-col justify-between rounded-xl border border-slate-800 bg-slate-900/60 p-4 transition hover:border-sky-500/40 hover:bg-slate-900"
      >
        <div class="flex items-center justify-between">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-sky-500/15 text-sky-400">
            <Sliders class="h-4.5 w-4.5" />
          </div>
          <ArrowRight class="h-4 w-4 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-sky-400" />
        </div>
        <div class="mt-3.5">
          <p class="text-sm font-semibold text-white">Live WebView</p>
          <p class="mt-1 text-xs text-slate-400">Modbus dashboards & controls</p>
        </div>
      </RouterLink>

      <RouterLink
        v-if="site.webviewEnabled"
        :to="`/sites/${site.id}/history`"
        class="group flex flex-col justify-between rounded-xl border border-slate-800 bg-slate-900/60 p-4 transition hover:border-violet-500/40 hover:bg-slate-900"
      >
        <div class="flex items-center justify-between">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-violet-500/15 text-violet-400">
            <LineChart class="h-4.5 w-4.5" />
          </div>
          <ArrowRight class="h-4 w-4 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-violet-400" />
        </div>
        <div class="mt-3.5">
          <p class="text-sm font-semibold text-white">WebView History</p>
          <p class="mt-1 text-xs text-slate-400">TimescaleDB sensor trends</p>
        </div>
      </RouterLink>

      <RouterLink
        v-if="site.webviewEnabled"
        :to="`/sites/${site.id}/alarms`"
        class="group flex flex-col justify-between rounded-xl border border-slate-800 bg-slate-900/60 p-4 transition hover:border-amber-500/40 hover:bg-slate-900"
      >
        <div class="flex items-center justify-between">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-amber-500/15 text-amber-400">
            <Bell class="h-4.5 w-4.5" />
          </div>
          <div class="flex items-center gap-2">
            <span
              v-if="activeAlarms > 0"
              class="rounded-full bg-rose-500 px-2 py-0.5 text-[10px] font-semibold tabular-nums text-white"
            >
              {{ activeAlarms }} active
            </span>
            <ArrowRight class="h-4 w-4 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-amber-400" />
          </div>
        </div>
        <div class="mt-3.5">
          <p class="text-sm font-semibold text-white">Alarm Center</p>
          <p class="mt-1 text-xs text-slate-400">Latched alerts & acknowledgements</p>
        </div>
      </RouterLink>
    </div>
  </div>
</template>
