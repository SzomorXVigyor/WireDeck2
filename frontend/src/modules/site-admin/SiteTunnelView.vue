<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Activity, ShieldCheck } from 'lucide-vue-next'
import { useSiteStore } from '../../stores/sites'
import StatusBadge from '../../components/StatusBadge.vue'

const route = useRoute()
const siteStore = useSiteStore()

const siteId = computed(() => String(route.params.siteId || ''))
const site = computed(() => siteStore.byId(siteId.value))

onMounted(() => {
  void siteStore.fetchSites()
})
</script>

<template>
  <div v-if="site" class="w-full space-y-5 sm:space-y-6">
    <div>
      <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">Tunnel Details</h1>
      <p class="mt-0.5 text-xs text-slate-400">
        Site connectivity status, handshake telemetry, and tunnel metadata.
      </p>
    </div>

    <!-- Tunnel Status & Handshake Telemetry -->
    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
      <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2 text-sm font-semibold text-white">
          <Activity class="h-4 w-4 text-amber-400" />
          Tunnel Status
        </div>
        <div class="flex items-center gap-2">
          <StatusBadge :status="site.status" size="sm" />
          <span
            class="rounded-full border border-slate-700 bg-slate-800 px-2 py-0.5 font-mono text-[10px] uppercase text-slate-300"
          >
            {{ site.isActive ? 'ACTIVE' : 'INACTIVE' }}
          </span>
        </div>
      </div>

      <dl class="grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-4">
        <div class="min-w-0">
          <dt class="text-slate-500">Site Slug</dt>
          <dd class="mt-1 break-all font-mono text-slate-200">{{ site.slug }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-slate-500">Site Role</dt>
          <dd class="mt-1 font-mono text-slate-200">{{ site.role }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-slate-500">Handshake State</dt>
          <dd class="mt-1 font-mono text-slate-200">{{ site.handshakeState }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-slate-500">Last Handshake</dt>
          <dd class="mt-1 font-mono tabular-nums text-slate-200">
            {{ site.lastHandshakeAt ? new Date(site.lastHandshakeAt).toLocaleString() : 'Never' }}
          </dd>
        </div>
      </dl>
    </section>

    <!-- WireGuard Metadata (Non-Secret, populated by API for Platform Admins) -->
    <section v-if="site.wireguard" class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
      <div class="mb-4 flex items-center gap-2 text-sm font-semibold text-white">
        <ShieldCheck class="h-4 w-4 text-sky-400" />
        WireGuard Tunnel Metadata (Non-Secret)
      </div>
      <dl class="grid gap-4 text-xs sm:grid-cols-2">
        <div class="min-w-0">
          <dt class="text-slate-500">Remote Endpoint</dt>
          <dd class="mt-1 break-all font-mono text-slate-200">
            {{ site.wireguard.endpointHost }}:{{ site.wireguard.endpointPort }}
          </dd>
        </div>
        <div class="min-w-0">
          <dt class="text-slate-500">Allowed LAN Subnets</dt>
          <dd class="mt-1 break-words font-mono text-slate-200">{{ site.wireguard.lanSubnets.join(', ') }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-slate-500">Peer Public Key</dt>
          <dd class="mt-1 break-all font-mono text-slate-200" :title="site.wireguard.publicKey">
            {{ site.wireguard.publicKey }}
          </dd>
        </div>
        <div class="min-w-0">
          <dt class="text-slate-500">Platform Client Public Key</dt>
          <dd class="mt-1 break-all font-mono text-slate-200" :title="site.wireguard.clientPublicKey">
            {{ site.wireguard.clientPublicKey }}
          </dd>
        </div>
      </dl>
    </section>
  </div>
</template>
