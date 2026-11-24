<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Mail, Plus, Trash2, X } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import type { User } from '../../types/iam'
import StatusBadge from '../../components/StatusBadge.vue'
import DeleteConfirmModal from '../../components/DeleteConfirmModal.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const users = ref<User[]>([])
const usersPag = usePagination(users)
const errBanner = ref<string | null>(null)
const notice = ref<string | null>(null)
const userToDelete = ref<User | null>(null)

const showInvite = ref(false)
const inviteEmail = ref('')
const inviteAdmin = ref(false)
const inviting = ref(false)

async function loadUsers() {
  try {
    const res = await apiRequest<{ users: User[] }>('/api/v1/iam/admin/users')
    users.value = res.users || []
  } catch (err) {
    errBanner.value = err instanceof Error ? err.message : 'Failed to load users'
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && showInvite.value) {
    showInvite.value = false
  }
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  void loadUsers()
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
})

async function createInvite() {
  errBanner.value = null
  notice.value = null
  inviting.value = true
  try {
    const res = await apiRequest<{ user: User; inviteSent: boolean }>('/api/v1/iam/admin/users', {
      method: 'POST',
      body: JSON.stringify({
        email: inviteEmail.value,
        isPlatformAdmin: inviteAdmin.value,
      }),
    })
    showInvite.value = false
    inviteEmail.value = ''
    inviteAdmin.value = false
    notice.value = res.inviteSent
      ? `Invite email dispatched to ${res.user.email}.`
      : `User ${res.user.email} created, but MailGopher failed to queue email - use Resend Invite once SMTP is ready.`
    await loadUsers()
  } catch (err) {
    errBanner.value = err instanceof Error ? err.message : 'Invite failed'
  } finally {
    inviting.value = false
  }
}

async function resendInvite(u: User) {
  notice.value = null
  const res = await apiRequest<{ inviteSent: boolean }>(`/api/v1/iam/admin/users/${u.id}/resend-invite`, {
    method: 'POST',
  })
  notice.value = res.inviteSent ? `Re-sent invite email to ${u.email}.` : `Failed to send invite to ${u.email}.`
}

async function toggleDisabled(u: User) {
  const nextStatus = u.status === 'DISABLED' ? 'ACTIVE' : 'DISABLED'
  await apiRequest(`/api/v1/iam/admin/users/${u.id}`, {
    method: 'PATCH',
    body: JSON.stringify({ status: nextStatus }),
  })
  await loadUsers()
}

function deleteUser(u: User) {
  userToDelete.value = u
}

async function confirmDeleteUser() {
  const u = userToDelete.value
  userToDelete.value = null
  if (!u) return
  await apiRequest(`/api/v1/iam/admin/users/${u.id}`, { method: 'DELETE' })
  await loadUsers()
}
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">Users & Email Invites</h1>
        <p class="mt-0.5 text-xs text-slate-400">Create users via single-use 48-hour email invitations dispatched by MailGopher.</p>
      </div>
      <button
        type="button"
        class="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 self-start rounded-lg bg-indigo-500 px-3.5 text-xs font-semibold text-white transition hover:bg-indigo-400 sm:self-auto"
        @click="showInvite = true"
      >
        <Plus class="h-4 w-4" />
        Invite User
      </button>
    </div>

    <p v-if="errBanner" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-xs text-rose-200">{{ errBanner }}</p>
    <p v-if="notice" class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-200">{{ notice }}</p>

    <div class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <!-- Mobile Cards -->
      <div class="divide-y divide-slate-800/70 md:hidden">
        <div v-for="u in usersPag.paginatedItems.value" :key="u.id" class="space-y-3 p-4 text-xs">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <p class="break-all font-mono text-sm font-medium text-white">{{ u.email }}</p>
              <p class="mt-0.5 text-slate-400">{{ u.isPlatformAdmin ? 'Platform Admin' : 'Standard User' }}</p>
            </div>
            <StatusBadge :status="u.status" size="sm" />
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button
              v-if="u.status === 'INVITED'"
              type="button"
              class="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-lg border border-amber-500/40 bg-amber-500/10 px-2.5 text-[11px] font-medium text-amber-200 hover:bg-amber-500/20"
              @click="resendInvite(u)"
            >
              <Mail class="h-3.5 w-3.5" /> Resend Invite
            </button>
            <button
              v-if="u.status !== 'INVITED'"
              type="button"
              class="inline-flex h-8 flex-1 items-center justify-center rounded-lg border border-slate-700 bg-slate-800 px-2.5 text-[11px] font-medium text-slate-200 hover:bg-slate-700"
              @click="toggleDisabled(u)"
            >
              {{ u.status === 'DISABLED' ? 'Enable Account' : 'Disable Account' }}
            </button>
            <button
              type="button"
              aria-label="Delete user"
              class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
              @click="deleteUser(u)"
            >
              <Trash2 class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
        <div v-if="users.length === 0" class="px-4 py-8 text-center text-xs text-slate-500">No users found.</div>
      </div>

      <!-- Desktop Table -->
      <div class="hidden overflow-x-auto md:block">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-4 py-3">Email</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3">Role</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="u in usersPag.paginatedItems.value" :key="u.id" class="hover:bg-slate-800/30">
              <td class="px-4 py-3 font-mono text-slate-200">{{ u.email }}</td>
              <td class="px-4 py-3"><StatusBadge :status="u.status" size="sm" /></td>
              <td class="px-4 py-3 text-slate-300">{{ u.isPlatformAdmin ? 'Platform Admin' : 'Standard User' }}</td>
              <td class="px-4 py-3 text-right">
                <div class="inline-flex items-center justify-end gap-1.5">
                  <button
                    v-if="u.status === 'INVITED'"
                    type="button"
                    class="inline-flex items-center gap-1 rounded border border-amber-500/40 bg-amber-500/10 px-2 py-1 text-[11px] text-amber-200 transition hover:bg-amber-500/20"
                    @click="resendInvite(u)"
                  >
                    <Mail class="h-3 w-3" /> Resend Invite
                  </button>
                  <button
                    v-if="u.status !== 'INVITED'"
                    type="button"
                    class="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 transition hover:bg-slate-700"
                    @click="toggleDisabled(u)"
                  >
                    {{ u.status === 'DISABLED' ? 'Enable' : 'Disable' }}
                  </button>
                  <button
                    type="button"
                    class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300 transition hover:bg-rose-500/20"
                    title="Delete user"
                    aria-label="Delete user"
                    @click="deleteUser(u)"
                  >
                    <Trash2 class="h-3.5 w-3.5" />
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="users.length === 0">
              <td colspan="4" class="px-4 py-8 text-center text-slate-500">No users found.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <TablePagination
        v-model:page="usersPag.page.value"
        v-model:page-size="usersPag.pageSize.value"
        :total-items="usersPag.totalItems.value"
      />
    </div>

    <Teleport to="body">
      <div
        v-if="showInvite"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
        @click.self="showInvite = false"
      >
        <div class="max-h-[90dvh] w-full max-w-md overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl sm:p-6">
          <div class="flex items-center justify-between gap-2">
            <h2 class="text-base font-semibold text-white">Invite New User</h2>
            <button
              type="button"
              aria-label="Close modal"
              class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
              @click="showInvite = false"
            >
              <X class="h-4 w-4" />
            </button>
          </div>
          <form class="mt-4 space-y-4" @submit.prevent="createInvite">
            <div>
              <label class="mb-1 block text-xs text-slate-400">User Email</label>
              <input v-model="inviteEmail" type="email" required class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none" />
            </div>
            <label class="inline-flex items-center gap-2 text-xs text-slate-200">
              <input v-model="inviteAdmin" type="checkbox" />
              Grant Platform Administrator privileges
            </label>
            <div class="flex items-center justify-end gap-2 pt-2">
              <button type="button" class="inline-flex h-9 items-center justify-center rounded-lg border border-slate-700 px-3.5 text-xs font-medium leading-none text-slate-300 hover:bg-slate-800" @click="showInvite = false">Cancel</button>
              <button type="submit" :disabled="inviting" class="inline-flex h-9 items-center justify-center rounded-lg bg-indigo-500 px-4 text-xs font-semibold leading-none text-white hover:bg-indigo-400 disabled:opacity-50">
                {{ inviting ? 'Sending Invite...' : 'Create & Send Invite' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </Teleport>

    <DeleteConfirmModal
      :open="Boolean(userToDelete)"
      title="Delete User"
      :item-name="userToDelete?.email || ''"
      description="This user account and all of its site memberships will be permanently removed."
      @close="userToDelete = null"
      @confirm="confirmDeleteUser"
    />
  </div>
</template>
