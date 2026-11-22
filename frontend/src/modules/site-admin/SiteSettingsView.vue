<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { MailPlus, Trash2 } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useAuthStore } from '../../stores/auth'
import type { SiteMember } from '../../types/iam'
import StatusBadge from '../../components/StatusBadge.vue'

const route = useRoute()
const auth = useAuthStore()
const siteId = computed(() => String(route.params.siteId || ''))

const members = ref<SiteMember[]>([])
const inviteEmail = ref('')
const submitting = ref(false)
const errMsg = ref<string | null>(null)
const okMsg = ref<string | null>(null)

async function loadMembers() {
  errMsg.value = null
  try {
    const res = await apiRequest<{ members: SiteMember[] }>(
      `/api/v1/iam/sites/${siteId.value}/operator/members`,
    )
    members.value = res.members || []
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to load site members'
  }
}

onMounted(loadMembers)
watch(siteId, loadMembers)

async function inviteMember() {
  errMsg.value = null
  okMsg.value = null
  submitting.value = true
  try {
    const res = await apiRequest<{ member: SiteMember; createdNew: boolean; inviteSent: boolean }>(
      `/api/v1/iam/sites/${siteId.value}/operator/members/invite`,
      {
        method: 'POST',
        body: JSON.stringify({ email: inviteEmail.value }),
      },
    )
    if (res.createdNew) {
      okMsg.value = res.inviteSent
        ? `Invited ${res.member.email} as SITE_MEMBER and dispatched activation email.`
        : `Created ${res.member.email} as SITE_MEMBER, but MailGopher failed to queue email.`
    } else {
      okMsg.value = `Added existing user ${res.member.email} to this site (${res.member.role}).`
    }
    inviteEmail.value = ''
    await loadMembers()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to add member'
  } finally {
    submitting.value = false
  }
}

async function removeMember(m: SiteMember) {
  if (!window.confirm(`Remove ${m.email} from this site?`)) return
  errMsg.value = null
  okMsg.value = null
  try {
    await apiRequest(`/api/v1/iam/sites/${siteId.value}/operator/members/${m.userId}`, {
      method: 'DELETE',
    })
    await loadMembers()
  } catch (err) {
    errMsg.value = err instanceof Error ? err.message : 'Failed to remove member'
  }
}
</script>

<template>
  <div class="mx-auto max-w-4xl space-y-6">
    <!-- Invite or Add Member as SITE_MEMBER -->
    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-5">
      <h2 class="text-base font-semibold text-white">Add or Invite Site Member</h2>
      <p class="mt-1 text-xs text-slate-400">
        Enter an email address to grant <code class="font-mono text-slate-200">SITE_MEMBER</code> access.
        New email addresses automatically receive an account activation link via email.
      </p>
      <form class="mt-4 flex flex-wrap gap-3" @submit.prevent="inviteMember">
        <input
          v-model="inviteEmail"
          type="email"
          required
          placeholder="member@company.com"
          class="flex-1 min-w-[240px] rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-amber-400 focus:outline-none"
        />
        <button
          type="submit"
          :disabled="submitting"
          class="inline-flex items-center gap-1.5 rounded-lg bg-amber-500 px-4 py-2 text-xs font-semibold text-slate-950 hover:bg-amber-400 disabled:opacity-50"
        >
          <MailPlus class="h-4 w-4" />
          {{ submitting ? 'Adding...' : 'Add / Invite Member' }}
        </button>
      </form>
      <p v-if="errMsg" class="mt-3 rounded-lg border border-rose-500/40 bg-rose-500/10 p-2.5 text-xs text-rose-200">{{ errMsg }}</p>
      <p v-if="okMsg" class="mt-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-2.5 text-xs text-emerald-200">{{ okMsg }}</p>
    </section>

    <!-- Current Site Members -->
    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-5">
      <h2 class="text-base font-semibold text-white">Site Members</h2>
      <div class="mt-3 overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="py-2 pr-4">Email</th>
              <th class="py-2 pr-4">Site Role</th>
              <th class="py-2 pr-4">Account Status</th>
              <th class="py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="m in members" :key="m.userId">
              <td class="py-2.5 pr-4 font-mono text-slate-200">{{ m.email }}</td>
              <td class="py-2.5 pr-4">
                <span
                  class="rounded px-1.5 py-0.5 font-mono text-[11px]"
                  :class="m.role === 'SITE_OPERATOR' ? 'bg-amber-500/15 text-amber-300' : 'bg-slate-800 text-slate-300'"
                >
                  {{ m.role }}
                </span>
              </td>
              <td class="py-2.5 pr-4"><StatusBadge :status="m.status" size="sm" /></td>
              <td class="py-2.5 text-right">
                <button
                  v-if="m.role === 'SITE_MEMBER' || auth.isPlatformAdmin"
                  type="button"
                  class="inline-flex items-center gap-1 rounded border border-rose-500/30 bg-rose-500/10 px-2 py-1 text-[11px] text-rose-300 hover:bg-rose-500/20"
                  @click="removeMember(m)"
                >
                  <Trash2 class="h-3 w-3" /> Remove
                </button>
              </td>
            </tr>
            <tr v-if="members.length === 0">
              <td colspan="4" class="py-6 text-center text-slate-500">No members assigned yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
