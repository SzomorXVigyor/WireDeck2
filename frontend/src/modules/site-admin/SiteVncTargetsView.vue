<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { KeyRound, Monitor, Plus, Trash2, Edit3 } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import type { VncSession, VncTarget } from '../../types/vnc'
import StatusBadge from '../../components/StatusBadge.vue'
import NumericTextInput from '../../components/NumericTextInput.vue'
import DeleteConfirmModal from '../../components/DeleteConfirmModal.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const route = useRoute()
const siteId = computed(() => String(route.params.siteId || ''))

const targets = ref<VncTarget[]>([])
const sessions = ref<VncSession[]>([])
const targetPag = usePagination(targets)
const sessionPag = usePagination(sessions)
const errMsg = ref<string | null>(null)
const okMsg = ref<string | null>(null)

// Create / Edit modal state
const showModal = ref(false)
const editingId = ref<string | null>(null)
const formName = ref('')
const formHost = ref('')
const formPort = ref('5900')
const formPassword = ref('')
const formClearPassword = ref(false)
const formActive = ref(true)
const saving = ref(false)
const deleteTarget = ref<VncTarget | null>(null)

async function loadData() {
  errMsg.value = null
  try {
    const [tRes, sRes] = await Promise.all([
      apiRequest<{ targets: VncTarget[] }>(`/api/v1/vnc/sites/${siteId.value}/operator/targets`),
      apiRequest<{ sessions: VncSession[] }>(`/api/v1/vnc/sites/${siteId.value}/operator/sessions`),
    ])
    targets.value = tRes.targets || []
    sessions.value = sRes.sessions || []
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to load VNC edge points'
  }
}

onMounted(loadData)
watch(siteId, loadData)

function openCreate() {
  editingId.value = null
  formName.value = ''
  formHost.value = ''
  formPort.value = '5900'
  formPassword.value = ''
  formClearPassword.value = false
  formActive.value = true
  showModal.value = true
}

function openEdit(t: VncTarget) {
  editingId.value = t.id
  formName.value = t.name
  formHost.value = t.host
  formPort.value = String(t.port)
  formPassword.value = ''
  formClearPassword.value = false
  formActive.value = t.isActive
  showModal.value = true
}

async function saveTarget() {
  errMsg.value = null
  okMsg.value = null
  saving.value = true
  try {
    if (editingId.value) {
      await apiRequest(`/api/v1/vnc/sites/${siteId.value}/operator/targets/${editingId.value}`, {
        method: 'PATCH',
        body: JSON.stringify({
          name: formName.value,
          host: formHost.value,
          port: Number(formPort.value),
          isActive: formActive.value,
          clearPassword: formClearPassword.value,
          ...(formPassword.value ? { password: formPassword.value } : {}),
        }),
      })
      okMsg.value = `Updated VNC edge point "${formName.value}".`
    } else {
      await apiRequest(`/api/v1/vnc/sites/${siteId.value}/operator/targets`, {
        method: 'POST',
        body: JSON.stringify({
          name: formName.value,
          host: formHost.value,
          port: Number(formPort.value),
          ...(formPassword.value ? { password: formPassword.value } : {}),
        }),
      })
      okMsg.value = `Added VNC edge point "${formName.value}".`
    }
    showModal.value = false
    await loadData()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to save VNC edge point'
  } finally {
    saving.value = false
  }
}

function removeTarget(t: VncTarget) {
  deleteTarget.value = t
}

async function confirmRemoveTarget() {
  const t = deleteTarget.value
  deleteTarget.value = null
  if (!t) return
  errMsg.value = null
  try {
    await apiRequest(`/api/v1/vnc/sites/${siteId.value}/operator/targets/${t.id}`, {
      method: 'DELETE',
    })
    await loadData()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to delete VNC edge point'
  }
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h2 class="text-lg font-semibold text-white">VNC Edge Points</h2>
        <p class="text-xs text-slate-400">
          Host IPs are validated against the site LAN subnets. Passwords are AES-256-GCM encrypted and used strictly server-side.
        </p>
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg bg-amber-500 px-3.5 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400"
        @click="openCreate"
      >
        <Plus class="h-4 w-4" />
        Add VNC Edge Point
      </button>
    </div>

    <p v-if="errMsg" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-xs text-rose-200">{{ errMsg }}</p>
    <p v-if="okMsg" class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-200">{{ okMsg }}</p>

    <!-- Targets Table -->
    <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-4 py-3">Name</th>
              <th class="px-4 py-3">Endpoint</th>
              <th class="px-4 py-3">Auth</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="t in targetPag.paginatedItems.value" :key="t.id" class="hover:bg-slate-800/30">
              <td class="px-4 py-3 font-medium text-white">
                <span class="inline-flex items-center gap-2">
                  <Monitor class="h-3.5 w-3.5 text-emerald-400" />
                  {{ t.name }}
                </span>
              </td>
              <td class="px-4 py-3 font-mono text-slate-300">{{ t.host }}:{{ t.port }}</td>
              <td class="px-4 py-3">
                <span
                  v-if="t.hasPassword"
                  class="inline-flex items-center gap-1 rounded bg-emerald-500/15 px-2 py-0.5 text-[11px] text-emerald-300"
                >
                  <KeyRound class="h-3 w-3" /> Server-side password set
                </span>
                <span v-else class="text-slate-500">No password</span>
              </td>
              <td class="px-4 py-3"><StatusBadge :status="t.isActive ? 'ACTIVE' : 'DISABLED'" size="sm" /></td>
              <td class="px-4 py-3 text-right space-x-1.5">
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 hover:bg-slate-700"
                  @click="openEdit(t)"
                >
                  <Edit3 class="h-3 w-3" /> Edit
                </button>
                <button
                  type="button"
                  class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 hover:bg-rose-500/20"
                  @click="removeTarget(t)"
                >
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
              </td>
            </tr>
            <tr v-if="targets.length === 0">
              <td colspan="5" class="px-4 py-8 text-center text-slate-500">No VNC edge points configured yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <TablePagination
        v-model:page="targetPag.page.value"
        v-model:page-size="targetPag.pageSize.value"
        :total-items="targetPag.totalItems.value"
      />
    </section>

    <!-- Session Visibility Table -->
    <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <div class="px-5 pt-5 pb-3">
        <h3 class="text-sm font-semibold text-white">Recent VNC Sessions</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-5 py-2.5">Target</th>
              <th class="px-4 py-2.5">Started</th>
              <th class="px-4 py-2.5">Ended</th>
              <th class="px-4 py-2.5">Traffic (In / Out)</th>
              <th class="px-5 py-2.5">Close Reason</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="s in sessionPag.paginatedItems.value" :key="s.id" class="hover:bg-slate-800/30">
              <td class="px-5 py-2.5 text-slate-200">{{ s.targetName || s.targetId }}</td>
              <td class="px-4 py-2.5 font-mono text-slate-400">{{ new Date(s.startedAt).toLocaleString() }}</td>
              <td class="px-4 py-2.5 font-mono text-slate-400">{{ s.endedAt ? new Date(s.endedAt).toLocaleString() : 'OPEN' }}</td>
              <td class="px-4 py-2.5 font-mono text-slate-300">{{ formatBytes(s.bytesIn) }} / {{ formatBytes(s.bytesOut) }}</td>
              <td class="px-5 py-2.5 font-mono text-slate-400">{{ s.closeReason || '-' }}</td>
            </tr>
            <tr v-if="sessions.length === 0">
              <td colspan="5" class="px-5 py-6 text-center text-slate-500">No sessions recorded yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <TablePagination
        v-model:page="sessionPag.page.value"
        v-model:page-size="sessionPag.pageSize.value"
        :total-items="sessionPag.totalItems.value"
      />
    </section>

    <!-- Create / Edit Target Modal -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div class="w-full max-w-md rounded-2xl border border-slate-700 bg-slate-900 p-6 shadow-2xl">
        <h3 class="text-base font-semibold text-white">
          {{ editingId ? 'Edit VNC Edge Point' : 'Add VNC Edge Point' }}
        </h3>
        <form class="mt-4 space-y-4" @submit.prevent="saveTarget">
          <div>
            <label class="mb-1 block text-xs text-slate-400">Display Name</label>
            <input v-model="formName" required placeholder="HMI Panel 1" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white" />
          </div>
          <div class="grid gap-3 sm:grid-cols-3">
            <div class="sm:col-span-2">
              <label class="mb-1 block text-xs text-slate-400">Host IP (inside site LAN)</label>
              <input v-model="formHost" required placeholder="192.168.10.25" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white" />
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">Port</label>
              <NumericTextInput v-model="formPort" :min="1" :max="65535" required placeholder="5900" />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-xs text-slate-400">
              VNC Password (write-only{{ editingId ? ', leave blank to keep current' : ', optional' }})
            </label>
            <input v-model="formPassword" type="password" autocomplete="new-password" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white" />
          </div>
          <div v-if="editingId" class="flex flex-wrap gap-4 text-xs text-slate-300">
            <label class="inline-flex items-center gap-1.5"><input v-model="formActive" type="checkbox" /> Active</label>
            <label class="inline-flex items-center gap-1.5"><input v-model="formClearPassword" type="checkbox" /> Remove stored password</label>
          </div>
          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300" @click="showModal = false">Cancel</button>
            <button type="submit" :disabled="saving" class="rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400">
              {{ saving ? 'Saving...' : 'Save Edge Point' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <DeleteConfirmModal
      :open="Boolean(deleteTarget)"
      title="Delete VNC Edge Point"
      :item-name="deleteTarget?.name || ''"
      description="This VNC endpoint will be removed from the site."
      @close="deleteTarget = null"
      @confirm="confirmRemoveTarget"
    />
  </div>
</template>
