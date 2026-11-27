<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ChevronRight, Minus, Plus, X } from 'lucide-vue-next'
import { ApiError, apiRequest } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { firstError, validateCidrList, validateRequiredText, validateSlug } from '../../utils/validation'
import type { Site } from '../../types/iam'
import StatusBadge from '../../components/StatusBadge.vue'
import WireguardConfEditor from '../../components/WireguardConfEditor.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

type ServiceKey = 'webviewEnabled' | 'vncEnabled'

interface PlatformServiceDef {
  key: ServiceKey
  name: string
  description: string
  badgeClass: string
}

// Dynamic registry of attachable site services - add future platform services here.
const PLATFORM_SERVICES: PlatformServiceDef[] = [
  {
    key: 'vncEnabled',
    name: 'WebVNC',
    description: 'Browser remote VNC access with server-side authentication',
    badgeClass: 'bg-emerald-500/15 text-emerald-300',
  },
  {
    key: 'webviewEnabled',
    name: 'WebView',
    description: 'Modbus dashboards, controls, TimescaleDB history, and alarms',
    badgeClass: 'bg-sky-500/15 text-sky-300',
  },
]

const toast = useToastStore()
const sites = ref<Site[]>([])
const sitesPag = usePagination(sites)
const loading = ref(false)

// Create Site modal state
const showCreate = ref(false)
const newName = ref('')
const newSlug = ref('')
const newConf = ref('')
const newOverrideSubnets = ref('')
const needsNarrowing = ref(false)
const newServices = ref<Record<ServiceKey, boolean>>({
  webviewEnabled: true,
  vncEnabled: true,
})
const newActivate = ref(true)
const createWarnings = ref<string[]>([])
const creating = ref(false)
const createSubmitted = ref(false)
const createTouched = ref<Record<string, boolean>>({})

const createErrors = computed(() => ({
  name: validateRequiredText(newName.value, 'Site name', { min: 2, max: 100 }),
  slug: newSlug.value.trim() ? validateSlug(newSlug.value, 'Slug') : null,
  conf: newConf.value.trim() ? null : 'WireGuard configuration is required.',
  subnets:
    needsNarrowing.value && !newOverrideSubnets.value.trim()
      ? 'Enter the site LAN subnets to narrow 0.0.0.0/0.'
      : validateCidrList(newOverrideSubnets.value, 'LAN Subnets'),
}))

function showCreateErr(field: 'name' | 'slug' | 'conf' | 'subnets'): string | null {
  return createSubmitted.value || createTouched.value[field] ? createErrors.value[field] : null
}

function openCreateModal() {
  createSubmitted.value = false
  createTouched.value = {}
  createWarnings.value = []
  needsNarrowing.value = false
  showCreate.value = true
}

async function loadSites() {
  loading.value = true
  try {
    const sRes = await apiRequest<{ sites: Site[] }>('/api/v1/iam/admin/sites')
    sites.value = sRes.sites || []
  } catch (err) {
    toast.error(err, 'Failed to load sites')
  } finally {
    loading.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && showCreate.value) {
    showCreate.value = false
  }
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  void loadSites()
})
onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
})

function toggleNewService(key: ServiceKey) {
  newServices.value = {
    ...newServices.value,
    [key]: !newServices.value[key],
  }
}

async function createSite() {
  createSubmitted.value = true
  const vErr = firstError(createErrors.value)
  if (vErr) {
    toast.error(vErr, 'Validation Error')
    return
  }
  createWarnings.value = []
  creating.value = true
  try {
    const overrides = newOverrideSubnets.value
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    const res = await apiRequest<{ site: Site; warnings?: string[] }>('/api/v1/iam/admin/sites', {
      method: 'POST',
      body: JSON.stringify({
        name: newName.value.trim(),
        slug: newSlug.value.trim() || undefined,
        wireguardConf: newConf.value,
        ...(overrides.length ? { overrideAllowedIPs: overrides } : {}),
        vncEnabled: newServices.value.vncEnabled,
        webviewEnabled: newServices.value.webviewEnabled,
        activate: newActivate.value,
      }),
    })
    createWarnings.value = res.warnings || []
    showCreate.value = false
    newName.value = ''
    newSlug.value = ''
    newConf.value = ''
    newOverrideSubnets.value = ''
    needsNarrowing.value = false
    toast.success(`Created site "${res.site.name}".`)
    await loadSites()
  } catch (err) {
    if (err instanceof ApiError && err.isCatchAllAllowedIPs()) {
      needsNarrowing.value = true
      toast.warning(
        'Config uses 0.0.0.0/0. Enter the site LAN subnets to narrow AllowedIPs and submit again.',
        'Subnet Narrowing Required',
      )
    } else {
      toast.error(err, 'Failed to create site')
    }
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">Sites & Onboarding</h1>
        <p class="mt-0.5 text-xs text-slate-400">
          Onboard sites or open any site to manage metrics, services, WireGuard configs, members, and lifecycle settings.
        </p>
      </div>
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 self-start rounded-lg bg-indigo-500 px-3.5 text-xs font-semibold text-white transition hover:bg-indigo-400 sm:self-auto"
        @click="openCreateModal"
      >
        <Plus class="h-4 w-4" />
        Onboard Site
      </button>
    </div>

    <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <!-- Mobile Cards -->
      <div class="divide-y divide-slate-800/70 md:hidden">
        <div v-for="s in sitesPag.paginatedItems.value" :key="s.id" class="space-y-3 p-4 text-xs">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <RouterLink
                :to="`/admin/sites/${s.id}`"
                class="block truncate text-sm font-semibold text-white transition hover:text-indigo-300"
              >
                {{ s.name }}
              </RouterLink>
              <p class="truncate font-mono text-[11px] text-slate-500">{{ s.slug }}</p>
            </div>
            <StatusBadge :status="s.status" size="sm" />
          </div>

          <div class="space-y-1 rounded-lg border border-slate-800/80 bg-slate-950/60 p-2.5 font-mono text-[11px] text-slate-300">
            <p v-if="s.wireguard" class="break-all">{{ s.wireguard.endpointHost }}:{{ s.wireguard.endpointPort }}</p>
            <p v-if="s.wireguard" class="break-words text-slate-500">{{ s.wireguard.lanSubnets.join(', ') }}</p>
            <p class="text-[10px] text-slate-500">{{ s.isActive ? 'ACTIVE' : 'STOPPED' }} | {{ s.handshakeState }}</p>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
              <template v-for="svc in PLATFORM_SERVICES" :key="svc.key">
                <span v-if="s[svc.key]" class="rounded px-2 py-0.5 font-medium" :class="svc.badgeClass">
                  {{ svc.name }}
                </span>
              </template>
              <span v-if="!PLATFORM_SERVICES.some((svc) => s[svc.key])" class="text-slate-500">No attached services</span>
            </div>
            <RouterLink
              :to="`/admin/sites/${s.id}`"
              class="inline-flex h-8 items-center justify-center gap-1 rounded-lg border border-indigo-500/40 bg-indigo-500/15 px-3 text-[11px] font-medium text-indigo-200 transition hover:bg-indigo-500/25"
            >
              Manage
              <ChevronRight class="h-3.5 w-3.5" />
            </RouterLink>
          </div>
        </div>
        <div v-if="!loading && sites.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">
          No sites onboarded yet.
        </div>
      </div>

      <!-- Desktop Table -->
      <div class="hidden overflow-x-auto md:block">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-4 py-3">Site</th>
              <th class="px-4 py-3">Endpoint & Subnets</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3">Attached Services</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="s in sitesPag.paginatedItems.value" :key="s.id" class="hover:bg-slate-800/30">
              <td class="px-4 py-3">
                <RouterLink
                  :to="`/admin/sites/${s.id}`"
                  class="font-medium text-white transition hover:text-indigo-300"
                >
                  {{ s.name }}
                </RouterLink>
                <p class="font-mono text-[11px] text-slate-500">{{ s.slug }}</p>
              </td>
              <td class="px-4 py-3 font-mono text-[11px] text-slate-300">
                <p v-if="s.wireguard">{{ s.wireguard.endpointHost }}:{{ s.wireguard.endpointPort }}</p>
                <p v-if="s.wireguard" class="text-slate-500">{{ s.wireguard.lanSubnets.join(', ') }}</p>
              </td>
              <td class="space-y-1 px-4 py-3">
                <StatusBadge :status="s.status" size="sm" />
                <p class="font-mono text-[10px] text-slate-500">{{ s.isActive ? 'ACTIVE' : 'STOPPED' }} | {{ s.handshakeState }}</p>
              </td>
              <td class="px-4 py-3 text-[11px]">
                <div class="flex flex-wrap gap-1.5">
                  <template v-for="svc in PLATFORM_SERVICES" :key="svc.key">
                    <span v-if="s[svc.key]" class="rounded px-1.5 py-0.5 font-medium" :class="svc.badgeClass">
                      {{ svc.name }}
                    </span>
                  </template>
                  <span v-if="!PLATFORM_SERVICES.some((svc) => s[svc.key])" class="text-slate-500">None</span>
                </div>
              </td>
              <td class="px-4 py-3 text-right">
                <RouterLink
                  :to="`/admin/sites/${s.id}`"
                  class="inline-flex items-center gap-1 rounded-lg border border-indigo-500/40 bg-indigo-500/15 px-3 py-1.5 text-[11px] font-medium text-indigo-200 transition hover:bg-indigo-500/25"
                >
                  Manage
                  <ChevronRight class="h-3.5 w-3.5" />
                </RouterLink>
              </td>
            </tr>
            <tr v-if="!loading && sites.length === 0">
              <td colspan="5" class="px-4 py-8 text-center text-slate-500">No sites onboarded yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <TablePagination
        v-model:page="sitesPag.page.value"
        v-model:page-size="sitesPag.pageSize.value"
        :total-items="sitesPag.totalItems.value"
      />
    </div>

    <!-- Create Site Modal -->
    <Teleport to="body">
      <div
        v-if="showCreate"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
        @click.self="showCreate = false"
      >
        <div class="max-h-[90dvh] w-full max-w-xl overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl sm:p-6">
          <div class="flex items-center justify-between gap-2">
            <h2 class="text-base font-semibold text-white">Onboard New Site</h2>
            <button
              type="button"
              aria-label="Close modal"
              class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
              @click="showCreate = false"
            >
              <X class="h-4 w-4" />
            </button>
          </div>
          <form novalidate class="mt-4 space-y-4" @submit.prevent="createSite">
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-xs text-slate-400">Site Name</label>
                <input
                  v-model="newName"
                  placeholder="Plant Alpha"
                  class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-sm text-white focus:outline-none"
                  :class="showCreateErr('name') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-indigo-500'"
                  @blur="createTouched.name = true"
                />
                <p v-if="showCreateErr('name')" class="mt-1 text-[11px] text-rose-400">{{ showCreateErr('name') }}</p>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Slug (optional)</label>
                <input
                  v-model="newSlug"
                  placeholder="auto-generated"
                  class="w-full rounded-lg border bg-slate-950 px-3 py-2 font-mono text-xs text-white focus:outline-none"
                  :class="showCreateErr('slug') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-indigo-500'"
                  @blur="createTouched.slug = true"
                />
                <p v-if="showCreateErr('slug')" class="mt-1 text-[11px] text-rose-400">{{ showCreateErr('slug') }}</p>
              </div>
            </div>

            <WireguardConfEditor
              v-model="newConf"
              v-model:override-subnets="newOverrideSubnets"
              :needs-narrowing="needsNarrowing"
              :warnings="createWarnings"
            />
            <p v-if="showCreateErr('conf')" class="text-[11px] text-rose-400">{{ showCreateErr('conf') }}</p>
            <p v-if="showCreateErr('subnets')" class="text-[11px] text-rose-400">{{ showCreateErr('subnets') }}</p>

            <!-- Dynamic Service List -->
            <div class="space-y-2">
              <p class="text-xs font-medium uppercase tracking-wider text-slate-400">Site Services</p>
              <div class="divide-y divide-slate-800 rounded-xl border border-slate-800 bg-slate-950/70">
                <div
                  v-for="svc in PLATFORM_SERVICES"
                  :key="svc.key"
                  class="flex items-center justify-between gap-3 px-3.5 py-2.5"
                >
                  <div class="min-w-0">
                    <p class="text-xs font-medium text-white">{{ svc.name }}</p>
                    <p class="text-[11px] text-slate-400">{{ svc.description }}</p>
                  </div>
                  <button
                    type="button"
                    class="inline-flex shrink-0 items-center gap-1 rounded-md border px-2.5 py-1.5 text-[11px] font-medium transition"
                    :class="
                      newServices[svc.key]
                        ? 'border-rose-500/40 bg-rose-500/10 text-rose-200 hover:bg-rose-500/20'
                        : 'border-emerald-500/40 bg-emerald-500/10 text-emerald-200 hover:bg-emerald-500/20'
                    "
                    @click="toggleNewService(svc.key)"
                  >
                    <Minus v-if="newServices[svc.key]" class="h-3 w-3" />
                    <Plus v-else class="h-3 w-3" />
                    {{ newServices[svc.key] ? 'Remove' : 'Add' }}
                  </button>
                </div>
              </div>
            </div>

            <label class="inline-flex items-center gap-2 text-xs text-slate-200">
              <input v-model="newActivate" type="checkbox" />
              Activate runner immediately
            </label>

            <div class="flex items-center justify-end gap-2 pt-2">
              <button
                type="button"
                class="inline-flex h-9 items-center justify-center rounded-lg border border-slate-700 px-3.5 text-xs font-medium leading-none text-slate-300 hover:bg-slate-800"
                @click="showCreate = false"
              >
                Cancel
              </button>
              <button
                type="submit"
                :disabled="creating"
                class="inline-flex h-9 items-center justify-center rounded-lg bg-indigo-500 px-4 text-xs font-semibold leading-none text-white hover:bg-indigo-400 disabled:opacity-50"
              >
                {{ creating ? 'Validating...' : 'Create Site' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </Teleport>
  </div>
</template>
