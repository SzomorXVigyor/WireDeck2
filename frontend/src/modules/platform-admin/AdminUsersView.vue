<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Mail, Plus, Trash2 } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import type { User } from '../../types/iam'
import StatusBadge from '../../components/StatusBadge.vue'

const users = ref<User[]>([])
const errBanner = ref<string | null>(null)
const notice = ref<string | null>(null)

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

onMounted(loadUsers)

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

async function deleteUser(u: User) {
  if (!window.confirm(`Permanently delete user ${u.email}?`)) return
  await apiRequest(`/api/v1/iam/admin/users/${u.id}`, { method: 'DELETE' })
  await loadUsers()
}
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold text-white">Users & Email Invites</h1>
        <p class="text-xs text-slate-400">Create users via single-use 48-hour email invitations dispatched by MailGopher.</p>
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg bg-indigo-500 px-3.5 py-2 text-xs font-semibold text-white hover:bg-indigo-400"
        @click="showInvite = true"
      >
        <Plus class="h-4 w-4" />
        Invite User
      </button>
    </div>

    <p v-if="errBanner" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-xs text-rose-200">{{ errBanner }}</p>
    <p v-if="notice" class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-200">{{ notice }}</p>

    <div class="overflow-x-auto rounded-xl border border-slate-800 bg-slate-900/60">
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
          <tr v-for="u in users" :key="u.id" class="hover:bg-slate-800/30">
            <td class="px-4 py-3 font-mono text-slate-200">{{ u.email }}</td>
            <td class="px-4 py-3"><StatusBadge :status="u.status" size="sm" /></td>
            <td class="px-4 py-3 text-slate-300">{{ u.isPlatformAdmin ? 'Platform Admin' : 'Standard User' }}</td>
            <td class="px-4 py-3 text-right space-x-1.5">
              <button
                v-if="u.status === 'INVITED'"
                type="button"
                class="inline-flex items-center gap-1 rounded border border-amber-500/40 bg-amber-500/10 px-2 py-1 text-[11px] text-amber-200"
                @click="resendInvite(u)"
              >
                <Mail class="h-3 w-3" /> Resend Invite
              </button>
              <button
                v-if="u.status !== 'INVITED'"
                type="button"
                class="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200"
                @click="toggleDisabled(u)"
              >
                {{ u.status === 'DISABLED' ? 'Enable' : 'Disable' }}
              </button>
              <button
                type="button"
                class="inline-flex items-center rounded border border-rose-500/30 bg-rose-500/10 p-1 text-rose-300"
                title="Delete user"
                @click="deleteUser(u)"
              >
                <Trash2 class="h-3.5 w-3.5" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="showInvite" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div class="w-full max-w-md rounded-2xl border border-slate-700 bg-slate-900 p-6 shadow-2xl">
        <h2 class="text-base font-semibold text-white">Invite New User</h2>
        <form class="mt-4 space-y-4" @submit.prevent="createInvite">
          <div>
            <label class="mb-1 block text-xs text-slate-400">User Email</label>
            <input v-model="inviteEmail" type="email" required class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white" />
          </div>
          <label class="inline-flex items-center gap-2 text-xs text-slate-200">
            <input v-model="inviteAdmin" type="checkbox" />
            Grant Platform Administrator privileges
          </label>
          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="rounded-lg border border-slate-700 px-3.5 py-2 text-xs text-slate-300" @click="showInvite = false">Cancel</button>
            <button type="submit" :disabled="inviting" class="rounded-lg bg-indigo-500 px-4 py-2 text-xs font-semibold text-white">
              {{ inviting ? 'Sending Invite...' : 'Create & Send Invite' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
