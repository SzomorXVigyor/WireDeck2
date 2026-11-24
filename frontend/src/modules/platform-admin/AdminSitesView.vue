<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Minus, Plus, Power, Settings, Trash2, Users } from 'lucide-vue-next'
import { ApiError, apiRequest } from '../../services/api'
import type { Site, SiteMember, SiteRole, User } from '../../types/iam'
import StatusBadge from '../../components/StatusBadge.vue'
import WireguardConfEditor from '../../components/WireguardConfEditor.vue'
import DeleteConfirmModal from '../../components/DeleteConfirmModal.vue'
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

const sites = ref<Site[]>([])
const sitesPag = usePagination(sites)
const users = ref<User[]>([])
const loading = ref(false)
const errBanner = ref<string | null>(null)
const siteToDelete = ref<Site | null>(null)

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
const createErr = ref<string | null>(null)
const createWarnings = ref<string[]>([])
const creating = ref(false)

// Edit Site Services & WireGuard Config modal (Platform Admin ONLY)
const editSite = ref<Site | null>(null)
const editName = ref('')
const editServices = ref<Record<ServiceKey, boolean>>({
  webviewEnabled: false,
  vncEnabled: false,
})
const editConf = ref('')
const editOverrideSubnets = ref('')
const editNeedsNarrowing = ref(false)
const editWarnings = ref<string[]>([])
const editErr = ref<string | null>(null)
const editOk = ref<string | null>(null)
const savingEdit = ref(false)

// Membership modal state
const memberSite = ref<Site | null>(null)
const siteMembers = ref<SiteMember[]>([])
const siteMembersPag = usePagination(siteMembers)
const assignUserId = ref('')
const assignRole = ref<SiteRole>('SITE_MEMBER')

async function loadSites() {
  loading.value = true
  try {
    const [sRes, uRes] = await Promise.all([
      apiRequest<{ sites: Site[] }>('/api/v1/iam/admin/sites'),
      apiRequest<{ users: User[] }>('/api/v1/iam/admin/users'),
    ])
    sites.value = sRes.sites || []
    users.value = uRes.users || []
  } catch (err) {
    errBanner.value = err instanceof Error ? err.message : 'Failed to load sites'
  } finally {
    loading.value = false
  }
}

onMounted(loadSites)

function toggleNewService(key: ServiceKey) {
  newServices.value = {
    ...newServices.value,
    [key]: !newServices.value[key],
  }
}

async function toggleEditService(key: ServiceKey) {
  if (!editSite.value) return
  const nextVal = !editServices.value[key]
  editServices.value = {
    ...editServices.value,
    [key]: nextVal,
  }
  await saveSiteFeatures()
}

async function createSite() {
  createErr.value = null
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
        name: newName.value,
        slug: newSlug.value || undefined,
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
    await loadSites()
  } catch (err) {
    if (err instanceof ApiError && err.isCatchAllAllowedIPs()) {
      needsNarrowing.value = true
      createErr.value = 'Enter the site LAN subnets above to narrow 0.0.0.0/0 and click Create Site again.'
    } else {
      createErr.value = err instanceof Error ? err.message : 'Failed to create site'
    }
  } finally {
    creating.value = false
  }
}

function openEditSite(site: Site) {
  editSite.value = site
  editName.value = site.name
  editServices.value = {
    webviewEnabled: site.webviewEnabled,
    vncEnabled: site.vncEnabled,
  }
  editConf.value = ''
  editOverrideSubnets.value = ''
  editNeedsNarrowing.value = false
  editWarnings.value = []
  editErr.value = null
  editOk.value = null
}

async function saveSiteFeatures() {
  if (!editSite.value) return
  editErr.value = null
  editOk.value = null
  savingEdit.value = true
  try {
    const updated = await apiRequest<Site>(`/api/v1/iam/admin/sites/${editSite.value.id}`, {
      method: 'PATCH',
      body: JSON.stringify({
        name: editName.value,
        vncEnabled: editServices.value.vncEnabled,
        webviewEnabled: editServices.value.webviewEnabled,
      }),
    })
    editSite.value = updated
    editOk.value = 'Site services and settings updated.'
    await loadSites()
  } catch (err) {
    editErr.value = err instanceof Error ? err.message : 'Failed to update site'
  } finally {
    savingEdit.value = false
  }
}

async function rotateSiteConf() {
  if (!editSite.value || !editConf.value.trim()) return
  editErr.value = null
  editOk.value = null
  editWarnings.value = []
  savingEdit.value = true
  try {
    const overrides = editOverrideSubnets.value
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    const res = await apiRequest<{ site: Site; warnings?: string[] }>(
      `/api/v1/iam/admin/sites/${editSite.value.id}/conf`,
      {
        method: 'POST',
        body: JSON.stringify({
          wireguardConf: editConf.value,
          ...(overrides.length ? { overrideAllowedIPs: overrides } : {}),
        }),
      },
    )
    editConf.value = ''
    editOverrideSubnets.value = ''
    editNeedsNarrowing.value = false
    editWarnings.value = res.warnings || []
    editOk.value = 'WireGuard config rotated & encrypted.'
    await loadSites()
  } catch (err) {
    if (err instanceof ApiError && err.isCatchAllAllowedIPs()) {
      editNeedsNarrowing.value = true
      editErr.value = 'Enter the site LAN subnets above and submit again.'
    } else {
      editErr.value = err instanceof Error ? err.message : 'Config rotation failed'
    }
  } finally {
    savingEdit.value = false
  }
}

async function toggleActive(site: Site) {
  errBanner.value = null
  try {
    const action = site.isActive ? 'deactivate' : 'activate'
    await apiRequest(`/api/v1/iam/admin/sites/${site.id}/${action}`, { method: 'POST' })
    await loadSites()
  } catch (err) {
    errBanner.value = err instanceof Error ? err.message : 'Toggle failed'
  }
}

function deleteSite(site: Site) {
  siteToDelete.value = site
}

async function confirmDeleteSite() {
  const site = siteToDelete.value
  siteToDelete.value = null
  if (!site) return
  errBanner.value = null
  try {
    await apiRequest(`/api/v1/iam/admin/sites/${site.id}`, { method: 'DELETE' })
    await loadSites()
  } catch (err) {
    errBanner.value = err instanceof Error ? err.message : 'Delete failed'
  }
}

async function openMembers(site: Site) {
  memberSite.value = site
  siteMembersPag.resetPage()
  const res = await apiRequest<{ members: SiteMember[] }>(`/api/v1/iam/admin/sites/${site.id}/members`)
  siteMembers.value = res.members || []
}

async function addOrUpdateMember() {
  if (!memberSite.value || !assignUserId.value) return
  await apiRequest(`/api/v1/iam/admin/sites/${memberSite.value.id}/members/${assignUserId.value}`, {
    method: 'PUT',
    body: JSON.stringify({ role: assignRole.value }),
  })
  await openMembers(memberSite.value)
}

async function removeMember(userId: string) {
  if (!memberSite.value) return
  await apiRequest(`/api/v1/iam/admin/sites/${memberSite.value.id}/members/${userId}`, {
    method: 'DELETE',
  })
  await openMembers(memberSite.value)
}
</script>

<template>
  <div class="mx-auto max-w-6xl space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold text-white">Sites, Services & WireGuard</h1>
        <p class="text-xs text-slate-400">
          Platform Admin exclusive: onboard sites, add or remove site services, manage WireGuard configs, and assign Site Operators.
        </p>
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg bg-indigo-500 px-3.5 py-2 text-xs font-semibold text-white hover:bg-indigo-400"
        @click="showCreate = true"
      >
        <Plus class="h-4 w-4" />
        Onboard Site
      </button>
    </div>

    <p v-if="errBanner" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-xs text-rose-200">{{ errBanner }}</p>

    <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <div class="overflow-x-auto">
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
                <p class="font-medium text-white">{{ s.name }}</p>
                <p class="font-mono text-[11px] text-slate-500">{{ s.slug }}</p>
              </td>
              <td class="px-4 py-3 font-mono text-[11px] text-slate-300">
                <p v-if="s.wireguard">{{ s.wireguard.endpointHost }}:{{ s.wireguard.endpointPort }}</p>
                <p v-if="s.wireguard" class="text-slate-500">{{ s.wireguard.lanSubnets.join(', ') }}</p>
              </td>
              <td class="px-4 py-3 space-y-1">
                <StatusBadge :status="s.status" size="sm" />
                <p class="font-mono text-[10px] text-slate-500">{{ s.isActive ? 'ACTIVE' : 'STOPPED' }} · {{ s.handshakeState }}</p>
              </td>
              <td class="px-4 py-3 text-[11px]">
                <template v-for="svc in PLATFORM_SERVICES" :key="svc.key">
                  <span v-if="s[svc.key]" class="mr-1.5 rounded px-1.5 py-0.5" :class="svc.badgeClass">
                    {{ svc.name }}
                  </span>
                </template>
                <span v-if="!PLATFORM_SERVICES.some((svc) => s[svc.key])" class="text-slate-500">None</span>
              </td>
              <td class="px-4 py-3 text-right space-x-1.5">
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded border border-indigo-500/40 bg-indigo-500/15 px-2 py-1 text-[11px] text-indigo-200 hover:bg-indigo-500/25"
                  @click="openEditSite(s)"
                >
                  <Settings class="h-3 w-3" /> Services / WG
                </button>
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 hover:bg-slate-700"
                  @click="openMembers(s)"
                >
                  <Users class="h-3 w-3" /> Members
                </button>
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 hover:bg-slate-700"
                  @click="toggleActive(s)"
                >
                  <Power class="h-3 w-3" /> {{ s.isActive ? 'Deactivate' : 'Activate' }}
                </button>
                <button
                  type="button"
                  class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 hover:bg-rose-500/20"
                  title="Delete site"
                  @click="deleteSite(s)"
                >
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
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
    <div v-if="showCreate" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div class="max-h-[90vh] w-full max-w-xl overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-6 shadow-2xl">
        <h2 class="text-lg font-semibold text-white">Onboard New Site</h2>
        <form class="mt-4 space-y-4" @submit.prevent="createSite">
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Site Name</label>
              <input v-model="newName" required class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white" />
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Slug (optional)</label>
              <input v-model="newSlug" placeholder="auto-generated" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white" />
            </div>
          </div>

          <WireguardConfEditor
            v-model="newConf"
            v-model:override-subnets="newOverrideSubnets"
            :needs-narrowing="needsNarrowing"
            :warnings="createWarnings"
          />

          <!-- Dynamic Service List -->
          <div class="space-y-2">
            <p class="text-xs font-medium uppercase tracking-wider text-slate-400">Site Services</p>
            <div class="divide-y divide-slate-800 rounded-xl border border-slate-800 bg-slate-950/70">
              <div
                v-for="svc in PLATFORM_SERVICES"
                :key="svc.key"
                class="flex items-center justify-between gap-3 px-3.5 py-2.5"
              >
                <div>
                  <p class="text-xs font-medium text-white">{{ svc.name }}</p>
                  <p class="text-[11px] text-slate-400">{{ svc.description }}</p>
                </div>
                <button
                  type="button"
                  class="inline-flex shrink-0 items-center gap-1 rounded-md border px-2.5 py-1 text-[11px] font-medium transition"
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

          <p v-if="createErr" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-2.5 text-xs text-rose-200">{{ createErr }}</p>

          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300" @click="showCreate = false">Cancel</button>
            <button type="submit" :disabled="creating" class="rounded-lg bg-indigo-500 px-4 py-2 text-xs font-semibold text-white hover:bg-indigo-400 disabled:opacity-50">
              {{ creating ? 'Validating...' : 'Create Site' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Configure Site Services & WireGuard Config Modal (Platform Admin ONLY) -->
    <div v-if="editSite" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div class="max-h-[90vh] w-full max-w-xl overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-6 shadow-2xl space-y-5">
        <div class="flex items-center justify-between">
          <h2 class="text-base font-semibold text-white">Configure Site - {{ editSite.name }}</h2>
          <button type="button" class="text-xs text-slate-400 hover:text-white" @click="editSite = null">Close</button>
        </div>

        <form class="flex items-end gap-2 border-b border-slate-800 pb-4" @submit.prevent="saveSiteFeatures">
          <div class="flex-1">
            <label class="mb-1 block text-xs text-slate-400">Site Name</label>
            <input v-model="editName" required class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white" />
          </div>
          <button type="submit" :disabled="savingEdit" class="rounded-lg bg-indigo-500 px-3.5 py-2 text-xs font-semibold text-white hover:bg-indigo-400">
            Rename
          </button>
        </form>

        <!-- Dynamic Site Services Add/Remove List -->
        <div class="space-y-2 border-b border-slate-800 pb-5">
          <p class="text-xs font-medium uppercase tracking-wider text-slate-400">Attached Services</p>
          <div class="divide-y divide-slate-800 rounded-xl border border-slate-800 bg-slate-950/70">
            <div
              v-for="svc in PLATFORM_SERVICES"
              :key="svc.key"
              class="flex items-center justify-between gap-3 px-3.5 py-2.5"
            >
              <div>
                <div class="flex items-center gap-2">
                  <p class="text-xs font-medium text-white">{{ svc.name }}</p>
                  <span
                    class="rounded-full px-2 py-0.5 text-[10px] font-medium uppercase"
                    :class="editServices[svc.key] ? 'bg-emerald-500/15 text-emerald-300' : 'bg-slate-800 text-slate-400'"
                  >
                    {{ editServices[svc.key] ? 'Enabled' : 'Disabled' }}
                  </span>
                </div>
                <p class="mt-0.5 text-[11px] text-slate-400">{{ svc.description }}</p>
              </div>
              <button
                type="button"
                :disabled="savingEdit"
                class="inline-flex shrink-0 items-center gap-1 rounded-md border px-2.5 py-1 text-[11px] font-medium transition disabled:opacity-50"
                :class="
                  editServices[svc.key]
                    ? 'border-rose-500/40 bg-rose-500/10 text-rose-200 hover:bg-rose-500/20'
                    : 'border-emerald-500/40 bg-emerald-500/10 text-emerald-200 hover:bg-emerald-500/20'
                "
                @click="toggleEditService(svc.key)"
              >
                <Minus v-if="editServices[svc.key]" class="h-3 w-3" />
                <Plus v-else class="h-3 w-3" />
                {{ editServices[svc.key] ? 'Remove' : 'Add' }}
              </button>
            </div>
          </div>
        </div>

        <form class="space-y-3" @submit.prevent="rotateSiteConf">
          <h3 class="text-sm font-semibold text-white">Rotate WireGuard Client Config</h3>
          <WireguardConfEditor
            v-model="editConf"
            v-model:override-subnets="editOverrideSubnets"
            :needs-narrowing="editNeedsNarrowing"
            :warnings="editWarnings"
          />
          <button type="submit" :disabled="savingEdit || !editConf.trim()" class="rounded-lg bg-sky-500 px-3.5 py-2 text-xs font-semibold text-slate-950 hover:bg-sky-400 disabled:opacity-50">
            Validate & Rotate WireGuard Config
          </button>
        </form>

        <p v-if="editErr" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-2.5 text-xs text-rose-200">{{ editErr }}</p>
        <p v-if="editOk" class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-2.5 text-xs text-emerald-200">{{ editOk }}</p>
      </div>
    </div>

    <!-- Site Membership Modal -->
    <div v-if="memberSite" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div class="w-full max-w-lg rounded-2xl border border-slate-700 bg-slate-900 p-6 shadow-2xl space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-base font-semibold text-white">Members - {{ memberSite.name }}</h2>
          <button type="button" class="text-xs text-slate-400 hover:text-white" @click="memberSite = null">Close</button>
        </div>

        <form class="flex gap-2" @submit.prevent="addOrUpdateMember">
          <select v-model="assignUserId" required class="flex-1 rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-2 text-xs text-white">
            <option value="" disabled>Select user...</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{ u.email }}</option>
          </select>
          <select v-model="assignRole" class="rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-2 text-xs text-white">
            <option value="SITE_MEMBER">SITE_MEMBER</option>
            <option value="SITE_OPERATOR">SITE_OPERATOR</option>
          </select>
          <button type="submit" class="rounded-lg bg-indigo-500 px-3 py-2 text-xs font-medium text-white">Assign</button>
        </form>

        <div class="overflow-hidden rounded-lg border border-slate-800">
          <div class="divide-y divide-slate-800 px-3 text-xs">
            <div v-for="m in siteMembersPag.paginatedItems.value" :key="m.userId" class="flex items-center justify-between py-2">
              <div>
                <p class="font-mono text-slate-200">{{ m.email }}</p>
                <p class="text-[11px] text-slate-400">{{ m.role }}</p>
              </div>
              <button type="button" class="text-rose-400 hover:underline" @click="removeMember(m.userId)">Remove</button>
            </div>
            <p v-if="siteMembers.length === 0" class="py-3 text-center text-slate-500">No members assigned.</p>
          </div>
          <TablePagination
            v-model:page="siteMembersPag.page.value"
            v-model:page-size="siteMembersPag.pageSize.value"
            :total-items="siteMembersPag.totalItems.value"
            compact
          />
        </div>
      </div>
    </div>

    <DeleteConfirmModal
      :open="Boolean(siteToDelete)"
      title="Delete Site"
      :item-name="siteToDelete?.name || ''"
      description="This site, its runner container, and all downstream VNC and WebView data will be permanently deleted."
      @close="siteToDelete = null"
      @confirm="confirmDeleteSite"
    />
  </div>
</template>
