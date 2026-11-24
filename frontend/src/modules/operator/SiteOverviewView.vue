<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Monitor, Sliders, LineChart, Bell, ShieldCheck } from 'lucide-vue-next'
import { useSiteStore } from '../../stores/sites'
import StatusBadge from '../../components/StatusBadge.vue'

const route = useRoute()
const siteStore = useSiteStore()

const siteId = computed(() => String(route.params.siteId || ''))
const site = computed(() => siteStore.byId(siteId.value))
</script>

<template>
  <div v-if="site" class="mx-auto max-w-5xl space-y-6">
    <div class="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-slate-800 bg-slate-900/60 p-5">
      <div>
        <div class="flex items-center gap-3">
          <h2 class="text-xl font-semibold text-white">{{ site.name }}</h2>
          <StatusBadge :status="site.status" />
          <span
            v-if="!site.isActive"
            class="rounded-full border border-slate-700 bg-slate-800 px-2 py-0.5 text-[10px] uppercase text-slate-400"
          >
            Inactive
          </span>
        </div>
        <p class="mt-1 font-mono text-xs text-slate-400">
          slug: {{ site.slug }} · role: {{ site.role }} · handshake: {{ site.handshakeState }}
        </p>
      </div>
      <div class="text-right text-xs text-slate-400">
        <p>Last Handshake</p>
        <p class="font-mono text-slate-200">
          {{ site.lastHandshakeAt ? new Date(site.lastHandshakeAt).toLocaleString() : 'Never' }}
        </p>
      </div>
    </div>

    <!-- Enabled Capability Cards -->
    <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <RouterLink
        v-if="site.vncEnabled"
        :to="`/sites/${site.id}/vnc`"
        class="rounded-xl border border-slate-800 bg-slate-900/50 p-4 hover:border-sky-500/40"
      >
        <Monitor class="h-5 w-5 text-emerald-400" />
        <p class="mt-3 text-sm font-semibold text-white">Browser VNC</p>
        <p class="mt-1 text-xs text-slate-400">Server-authenticated HMI access</p>
      </RouterLink>
      <RouterLink
        v-if="site.webviewEnabled"
        :to="`/sites/${site.id}/webview`"
        class="rounded-xl border border-slate-800 bg-slate-900/50 p-4 hover:border-sky-500/40"
      >
        <Sliders class="h-5 w-5 text-sky-400" />
        <p class="mt-3 text-sm font-semibold text-white">Live WebView</p>
        <p class="mt-1 text-xs text-slate-400">Modbus dashboards & controls</p>
      </RouterLink>
      <RouterLink
        v-if="site.webviewEnabled"
        :to="`/sites/${site.id}/history`"
        class="rounded-xl border border-slate-800 bg-slate-900/50 p-4 hover:border-sky-500/40"
      >
        <LineChart class="h-5 w-5 text-violet-400" />
        <p class="mt-3 text-sm font-semibold text-white">WebView History</p>
        <p class="mt-1 text-xs text-slate-400">TimescaleDB sensor trends</p>
      </RouterLink>
      <RouterLink
        v-if="site.webviewEnabled"
        :to="`/sites/${site.id}/alarms`"
        class="rounded-xl border border-slate-800 bg-slate-900/50 p-4 hover:border-sky-500/40"
      >
        <Bell class="h-5 w-5 text-amber-400" />
        <p class="mt-3 text-sm font-semibold text-white">Alarm Center</p>
        <p class="mt-1 text-xs text-slate-400">Latched alerts & acknowledgements</p>
      </RouterLink>
    </div>

    <!-- WireGuard Diagnostics (visible only when API includes wireguard metadata for admins) -->
    <div v-if="site.wireguard" class="rounded-xl border border-slate-800 bg-slate-900/60 p-5">
      <div class="mb-4 flex items-center gap-2 text-sm font-semibold text-white">
        <ShieldCheck class="h-4 w-4 text-sky-400" />
        WireGuard Tunnel Metadata (Non-Secret)
      </div>
      <dl class="grid gap-4 sm:grid-cols-2 text-xs">
        <div>
          <dt class="text-slate-500">Remote Endpoint</dt>
          <dd class="mt-1 font-mono text-slate-200">{{ site.wireguard.endpointHost }}:{{ site.wireguard.endpointPort }}</dd>
        </div>
        <div>
          <dt class="text-slate-500">Allowed LAN Subnets</dt>
          <dd class="mt-1 font-mono text-slate-200">{{ site.wireguard.lanSubnets.join(', ') }}</dd>
        </div>
        <div>
          <dt class="text-slate-500">Peer Public Key</dt>
          <dd class="mt-1 truncate font-mono text-slate-200">{{ site.wireguard.publicKey }}</dd>
        </div>
        <div>
          <dt class="text-slate-500">Platform Client Public Key</dt>
          <dd class="mt-1 truncate font-mono text-slate-200">{{ site.wireguard.clientPublicKey }}</dd>
        </div>
      </dl>
    </div>
  </div>
</template>
