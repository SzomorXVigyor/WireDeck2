<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Edit3, KeyRound, Monitor, Plus, Trash2, X } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import type { VncSession, VncTarget } from '../../types/vnc'
import { firstError, validateIpAddress, validatePort, validateRequiredText } from '../../utils/validation'
import StatusBadge from '../../components/StatusBadge.vue'
import NumericTextInput from '../../components/NumericTextInput.vue'
import DeleteConfirmModal from '../../components/DeleteConfirmModal.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const route = useRoute()
const toast = useToastStore()
const siteId = computed(() => String(route.params.siteId || ''))

const targets = ref<VncTarget[]>([])
const sessions = ref<VncSession[]>([])
const targetPag = usePagination(targets)
const sessionPag = usePagination(sessions)

// Create / Edit modal state
const showModal = ref(false)
const editingId = ref<string | null>(null)
const formName = ref('')
const formHost = ref('')
const formPort = ref('5900')
const formPassword = ref('')
const formClearPassword = ref(false)
const formActive = ref(true)
const formSubmitted = ref(false)
const formTouched = ref<Record<string, boolean>>({})
const saving = ref(false)
const deleteTarget = ref<VncTarget | null>(null)

const formErrors = computed(() => ({
  name: validateRequiredText(formName.value, 'Display name', { min: 1, max: 100 }),
  host: validateIpAddress(formHost.value, 'Host IP'),
  port: validatePort(formPort.value, 'Port'),
}))

function showFormErr(field: 'name' | 'host' | 'port'): string | null {
  return formSubmitted.value || formTouched.value[field] ? formErrors.value[field] : null
}

async function loadData() {
  try {
    const [tRes, sRes] = await Promise.all([
      apiRequest<{ targets: VncTarget[] }>(`/api/v1/vnc/sites/${siteId.value}/operator/targets`),
      apiRequest<{ sessions: VncSession[] }>(`/api/v1/vnc/sites/${siteId.value}/operator/sessions`),
    ])
    targets.value = tRes.targets || []
    sessions.value = sRes.sessions || []
  } catch (err) {
    toast.error(err, 'Failed to load VNC edge points')
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && showModal.value) {
    showModal.value = false
  }
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  void loadData()
})
onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
})
watch(siteId, loadData)

function openCreate() {
  editingId.value = null
  formName.value = ''
  formHost.value = ''
  formPort.value = '5900'
  formPassword.value = ''
  formClearPassword.value = false
  formActive.value = true
  formSubmitted.value = false
  formTouched.value = {}
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
  formSubmitted.value = false
  formTouched.value = {}
  showModal.value = true
}

async function saveTarget() {
  formSubmitted.value = true
  const vErr = firstError(formErrors.value)
  if (vErr) {
    toast.error(vErr, 'Validation Error')
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await apiRequest(`/api/v1/vnc/sites/${siteId.value}/operator/targets/${editingId.value}`, {
        method: 'PATCH',
        body: JSON.stringify({
          name: formName.value.trim(),
          host: formHost.value.trim(),
          port: Number(formPort.value),
          isActive: formActive.value,
          clearPassword: formClearPassword.value,
          ...(formPassword.value ? { password: formPassword.value } : {}),
        }),
      })
      toast.success(`Updated VNC edge point "${formName.value.trim()}".`)
    } else {
      await apiRequest(`/api/v1/vnc/sites/${siteId.value}/operator/targets`, {
        method: 'POST',
        body: JSON.stringify({
          name: formName.value.trim(),
          host: formHost.value.trim(),
          port: Number(formPort.value),
          ...(formPassword.value ? { password: formPassword.value } : {}),
        }),
      })
      toast.success(`Added VNC edge point "${formName.value.trim()}".`)
    }
    showModal.value = false
    await loadData()
  } catch (err) {
    toast.error(err, 'Failed to save VNC edge point')
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
  try {
    await apiRequest(`/api/v1/vnc/sites/${siteId.value}/operator/targets/${t.id}`, {
      method: 'DELETE',
    })
    toast.success(`Deleted VNC edge point "${t.name}".`)
    await loadData()
  } catch (err) {
    toast.error(err, 'Failed to delete VNC edge point')
  }
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">VNC Edge Points</h1>
        <p class="mt-0.5 text-xs text-slate-400">
          Host IPs are validated against the site LAN subnets. Passwords are AES-256-GCM encrypted and used strictly server-side.
        </p>
      </div>
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 self-start rounded-lg bg-amber-500 px-3.5 text-xs font-semibold text-slate-950 transition hover:bg-amber-400 sm:self-auto"
        @click="openCreate"
      >
        <Plus class="h-4 w-4" />
        Add VNC Edge Point
      </button>
    </div>

    <!-- Targets Table & Mobile Cards -->
    <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <!-- Mobile Cards -->
      <div class="divide-y divide-slate-800/70 md:hidden">
        <div v-for="t in targetPag.paginatedItems.value" :key="t.id" class="space-y-2.5 p-4 text-xs">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <p class="flex items-center gap-2 text-sm font-semibold text-white">
                <Monitor class="h-4 w-4 shrink-0 text-emerald-400" />
                <span class="truncate">{{ t.name }}</span>
              </p>
              <p class="mt-0.5 font-mono text-slate-400">{{ t.host }}:{{ t.port }}</p>
            </div>
            <StatusBadge :status="t.isActive ? 'ACTIVE' : 'DISABLED'" size="sm" />
          </div>
          <div>
            <span
              v-if="t.hasPassword"
              class="inline-flex items-center gap-1 rounded bg-emerald-500/15 px-2 py-0.5 text-[11px] text-emerald-300"
            >
              <KeyRound class="h-3 w-3" /> Server-side password set
            </span>
            <span v-else class="text-slate-500">No stored password</span>
          </div>
          <div class="flex items-center gap-2 pt-0.5">
            <button
              type="button"
              class="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-lg border border-slate-700 bg-slate-800 px-2.5 text-[11px] font-medium text-slate-200 hover:bg-slate-700"
              @click="openEdit(t)"
            >
              <Edit3 class="h-3 w-3" /> Edit
            </button>
            <button
              type="button"
              aria-label="Delete VNC edge point"
              class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
              @click="removeTarget(t)"
            >
              <Trash2 class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
        <div v-if="targets.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">
          No VNC edge points configured yet.
        </div>
      </div>

      <!-- Desktop Table -->
      <div class="hidden overflow-x-auto md:block">
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
              <td class="px-4 py-3 text-right">
                <div class="inline-flex items-center justify-end gap-1.5">
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 transition hover:bg-slate-700"
                    @click="openEdit(t)"
                  >
                    <Edit3 class="h-3 w-3" /> Edit
                  </button>
                  <button
                    type="button"
                    aria-label="Delete VNC edge point"
                    class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 transition hover:bg-rose-500/20"
                    @click="removeTarget(t)"
                  >
                    <Trash2 class="h-3.5 w-3.5" />
                  </button>
                </div>
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

    <!-- Session Visibility Table & Mobile Cards -->
    <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <div class="px-4 pt-4 pb-3 sm:px-5 sm:pt-5">
        <h2 class="text-sm font-semibold text-white">Recent VNC Sessions</h2>
      </div>

      <!-- Mobile Cards -->
      <div class="divide-y divide-slate-800/70 md:hidden">
        <div v-for="s in sessionPag.paginatedItems.value" :key="s.id" class="space-y-1.5 p-4 text-xs">
          <div class="flex items-center justify-between gap-2">
            <p class="truncate font-medium text-white">{{ s.targetName || s.targetId }}</p>
            <span class="shrink-0 font-mono text-[11px] text-slate-300">{{ formatBytes(s.bytesIn) }} / {{ formatBytes(s.bytesOut) }}</span>
          </div>
          <p class="font-mono text-[11px] text-slate-400">Started: {{ new Date(s.startedAt).toLocaleString() }}</p>
          <p class="font-mono text-[11px] text-slate-400">Ended: {{ s.endedAt ? new Date(s.endedAt).toLocaleString() : 'OPEN' }}</p>
          <p v-if="s.closeReason" class="font-mono text-[11px] text-slate-500">Reason: {{ s.closeReason }}</p>
        </div>
        <div v-if="sessions.length === 0" class="px-4 py-6 text-center text-xs text-slate-500">
          No sessions recorded yet.
        </div>
      </div>

      <!-- Desktop Table -->
      <div class="hidden overflow-x-auto md:block">
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
              <td class="whitespace-nowrap px-4 py-2.5 font-mono tabular-nums text-slate-400">{{ new Date(s.startedAt).toLocaleString() }}</td>
              <td class="whitespace-nowrap px-4 py-2.5 font-mono tabular-nums text-slate-400">{{ s.endedAt ? new Date(s.endedAt).toLocaleString() : 'OPEN' }}</td>
              <td class="whitespace-nowrap px-4 py-2.5 font-mono tabular-nums text-slate-300">{{ formatBytes(s.bytesIn) }} / {{ formatBytes(s.bytesOut) }}</td>
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
    <Teleport to="body">
      <div
        v-if="showModal"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
        @click.self="showModal = false"
      >
        <div class="max-h-[90dvh] w-full max-w-md overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl sm:p-6">
          <div class="flex items-center justify-between gap-2">
            <h3 class="text-base font-semibold text-white">
              {{ editingId ? 'Edit VNC Edge Point' : 'Add VNC Edge Point' }}
            </h3>
            <button
              type="button"
              aria-label="Close modal"
              class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
              @click="showModal = false"
            >
              <X class="h-4 w-4" />
            </button>
          </div>
          <form novalidate class="mt-4 space-y-4" @submit.prevent="saveTarget">
            <div>
              <label class="mb-1 block text-xs text-slate-400">Display Name</label>
              <input
                v-model="formName"
                placeholder="HMI Panel 1"
                class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-sm text-white focus:outline-none"
                :class="showFormErr('name') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
                @blur="formTouched.name = true"
              />
              <p v-if="showFormErr('name')" class="mt-1 text-[11px] text-rose-400">{{ showFormErr('name') }}</p>
            </div>
            <div class="grid gap-3 sm:grid-cols-3">
              <div class="sm:col-span-2">
                <label class="mb-1 block text-xs text-slate-400">Host IP (inside site LAN)</label>
                <input
                  v-model="formHost"
                  placeholder="192.168.10.25"
                  class="w-full rounded-lg border bg-slate-950 px-3 py-2 font-mono text-xs text-white focus:outline-none"
                  :class="showFormErr('host') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
                  @blur="formTouched.host = true"
                />
                <p v-if="showFormErr('host')" class="mt-1 text-[11px] text-rose-400">{{ showFormErr('host') }}</p>
              </div>
              <div>
                <label class="mb-1 block text-xs text-slate-400">Port</label>
                <NumericTextInput
                  v-model="formPort"
                  :min="1"
                  :max="65535"
                  required
                  placeholder="5900"
                  :external-error="showFormErr('port')"
                  @blur="formTouched.port = true"
                />
              </div>
            </div>
            <div>
              <label class="mb-1 block text-xs text-slate-400">
                VNC Password (write-only{{ editingId ? ', leave blank to keep current' : ', optional' }})
              </label>
              <input v-model="formPassword" type="password" autocomplete="new-password" class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-amber-500 focus:outline-none" />
            </div>
            <div v-if="editingId" class="flex flex-wrap gap-4 text-xs text-slate-300">
              <label class="inline-flex items-center gap-1.5"><input v-model="formActive" type="checkbox" /> Active</label>
              <label class="inline-flex items-center gap-1.5"><input v-model="formClearPassword" type="checkbox" /> Remove stored password</label>
            </div>
            <div class="flex items-center justify-end gap-2 pt-2">
              <button type="button" class="inline-flex h-9 items-center justify-center rounded-lg border border-slate-700 px-3.5 text-xs font-medium leading-none text-slate-300 hover:bg-slate-800" @click="showModal = false">Cancel</button>
              <button type="submit" :disabled="saving" class="inline-flex h-9 items-center justify-center rounded-lg bg-amber-500 px-4 text-xs font-semibold leading-none text-slate-950 hover:bg-amber-400 disabled:opacity-50">
                {{ saving ? 'Saving...' : 'Save Edge Point' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </Teleport>

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
