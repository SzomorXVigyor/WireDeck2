<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { MailPlus, Trash2 } from 'lucide-vue-next'
import { apiRequest } from '../../services/api'
import { useAuthStore } from '../../stores/auth'
import { useToastStore } from '../../stores/toast'
import type { SiteMember } from '../../types/iam'
import { validateEmail } from '../../utils/validation'
import StatusBadge from '../../components/StatusBadge.vue'
import DeleteConfirmModal from '../../components/DeleteConfirmModal.vue'
import TablePagination from '../../components/TablePagination.vue'
import { usePagination } from '../../composables/usePagination'

const route = useRoute()
const auth = useAuthStore()
const toast = useToastStore()
const siteId = computed(() => String(route.params.siteId || ''))

const members = ref<SiteMember[]>([])
const memberPag = usePagination(members)
const inviteEmail = ref('')
const emailTouched = ref(false)
const submitting = ref(false)
const memberToRemove = ref<SiteMember | null>(null)

const emailError = computed(() => (emailTouched.value ? validateEmail(inviteEmail.value, 'Member email') : null))

async function loadMembers() {
  try {
    const res = await apiRequest<{ members: SiteMember[] }>(
      `/api/v1/iam/sites/${siteId.value}/operator/members`,
    )
    members.value = res.members || []
  } catch (err) {
    toast.error(err, 'Failed to load site members')
  }
}

onMounted(loadMembers)
watch(siteId, loadMembers)

async function inviteMember() {
  emailTouched.value = true
  const vErr = validateEmail(inviteEmail.value, 'Member email')
  if (vErr) {
    toast.error(vErr, 'Validation Error')
    return
  }
  submitting.value = true
  try {
    const res = await apiRequest<{ member: SiteMember; createdNew: boolean; inviteSent: boolean }>(
      `/api/v1/iam/sites/${siteId.value}/operator/members/invite`,
      {
        method: 'POST',
        body: JSON.stringify({ email: inviteEmail.value.trim() }),
      },
    )
    if (res.createdNew) {
      if (res.inviteSent) {
        toast.success(`Invited ${res.member.email} as SITE_MEMBER and dispatched activation email.`)
      } else {
        toast.warning(`Created ${res.member.email} as SITE_MEMBER, but MailGopher failed to queue email.`)
      }
    } else {
      toast.success(`Added existing user ${res.member.email} to this site (${res.member.role}).`)
    }
    inviteEmail.value = ''
    emailTouched.value = false
    await loadMembers()
  } catch (err) {
    toast.error(err, 'Failed to add member')
  } finally {
    submitting.value = false
  }
}

function removeMember(m: SiteMember) {
  memberToRemove.value = m
}

async function confirmRemoveMember() {
  const m = memberToRemove.value
  memberToRemove.value = null
  if (!m) return
  try {
    await apiRequest(`/api/v1/iam/sites/${siteId.value}/operator/members/${m.userId}`, {
      method: 'DELETE',
    })
    toast.success(`Removed ${m.email} from this site.`)
    await loadMembers()
  } catch (err) {
    toast.error(err, 'Failed to remove member')
  }
}
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <div>
      <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">Site Members & Invites</h1>
      <p class="mt-0.5 text-xs text-slate-400">
        Grant <code class="font-mono text-slate-200">SITE_MEMBER</code> access or send account activation invites for this site.
      </p>
    </div>

    <!-- Invite or Add Member as SITE_MEMBER -->
    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
      <h2 class="text-sm font-semibold text-white">Add or Invite Site Member</h2>
      <p class="mt-1 text-xs text-slate-400">
        New email addresses automatically receive an account activation link via email.
      </p>
      <form novalidate class="mt-4 flex flex-col gap-2.5 sm:flex-row sm:items-start" @submit.prevent="inviteMember">
        <div class="w-full min-w-0 sm:flex-1">
          <input
            v-model="inviteEmail"
            type="email"
            placeholder="member@company.com"
            class="h-10 w-full min-w-0 rounded-lg border bg-slate-950 px-3 text-sm text-white focus:outline-none"
            :class="emailError ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-400'"
            @blur="emailTouched = true"
          />
          <p v-if="emailError" class="mt-1 text-[11px] text-rose-400">{{ emailError }}</p>
        </div>
        <button
          type="submit"
          :disabled="submitting"
          class="inline-flex h-10 shrink-0 items-center justify-center gap-1.5 rounded-lg bg-amber-500 px-4 text-xs font-semibold leading-none text-slate-950 transition hover:bg-amber-400 disabled:opacity-50"
        >
          <MailPlus class="h-4 w-4" />
          {{ submitting ? 'Adding...' : 'Add / Invite Member' }}
        </button>
      </form>
    </section>

    <!-- Current Site Members -->
    <section class="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60">
      <div class="px-4 pt-4 pb-3 sm:px-5 sm:pt-5">
        <h2 class="text-sm font-semibold text-white">Site Members</h2>
      </div>

      <!-- Mobile Cards -->
      <div class="divide-y divide-slate-800/70 md:hidden">
        <div v-for="m in memberPag.paginatedItems.value" :key="m.userId" class="space-y-2.5 p-4 text-xs">
          <div class="flex items-start justify-between gap-2">
            <p class="break-all font-mono text-sm text-slate-100">{{ m.email }}</p>
            <StatusBadge :status="m.status" size="sm" />
          </div>
          <div class="flex items-center justify-between gap-2">
            <span
              class="rounded px-2 py-0.5 font-mono text-[11px]"
              :class="m.role === 'SITE_OPERATOR' ? 'bg-amber-500/15 text-amber-300' : 'bg-slate-800 text-slate-300'"
            >
              {{ m.role }}
            </span>
            <button
              v-if="m.role === 'SITE_MEMBER' || auth.isPlatformAdmin"
              type="button"
              class="inline-flex h-8 items-center gap-1 rounded-lg border border-rose-500/30 bg-rose-500/10 px-2.5 text-[11px] font-medium text-rose-300 hover:bg-rose-500/20"
              @click="removeMember(m)"
            >
              <Trash2 class="h-3 w-3" /> Remove
            </button>
          </div>
        </div>
        <div v-if="members.length === 0" class="px-4 py-6 text-center text-xs text-slate-500">No members assigned yet.</div>
      </div>

      <!-- Desktop Table -->
      <div class="hidden overflow-x-auto md:block">
        <table class="w-full text-left text-xs">
          <thead class="border-b border-slate-800 text-slate-400">
            <tr>
              <th class="px-5 py-2.5">Email</th>
              <th class="px-4 py-2.5">Site Role</th>
              <th class="px-4 py-2.5">Account Status</th>
              <th class="px-5 py-2.5 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            <tr v-for="m in memberPag.paginatedItems.value" :key="m.userId" class="hover:bg-slate-800/30">
              <td class="px-5 py-2.5 font-mono text-slate-200">{{ m.email }}</td>
              <td class="px-4 py-2.5">
                <span
                  class="rounded px-1.5 py-0.5 font-mono text-[11px]"
                  :class="m.role === 'SITE_OPERATOR' ? 'bg-amber-500/15 text-amber-300' : 'bg-slate-800 text-slate-300'"
                >
                  {{ m.role }}
                </span>
              </td>
              <td class="px-4 py-2.5"><StatusBadge :status="m.status" size="sm" /></td>
              <td class="px-5 py-2.5 text-right">
                <button
                  v-if="m.role === 'SITE_MEMBER' || auth.isPlatformAdmin"
                  type="button"
                  class="inline-flex items-center gap-1 rounded border border-rose-500/30 bg-rose-500/10 px-2 py-1 text-[11px] text-rose-300 transition hover:bg-rose-500/20"
                  @click="removeMember(m)"
                >
                  <Trash2 class="h-3 w-3" /> Remove
                </button>
              </td>
            </tr>
            <tr v-if="members.length === 0">
              <td colspan="4" class="px-5 py-6 text-center text-slate-500">No members assigned yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <TablePagination
        v-model:page="memberPag.page.value"
        v-model:page-size="memberPag.pageSize.value"
        :total-items="memberPag.totalItems.value"
      />
    </section>

    <DeleteConfirmModal
      :open="Boolean(memberToRemove)"
      title="Remove Site Member"
      :item-name="memberToRemove?.email || ''"
      description="This user will lose access to this site immediately."
      @close="memberToRemove = null"
      @confirm="confirmRemoveMember"
    />
  </div>
</template>
