<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { KeyRound } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref<string | null>(null)
const saving = ref(false)

async function onChangePassword() {
  error.value = null
  if (newPassword.value !== confirmPassword.value) {
    error.value = 'New passwords do not match'
    return
  }
  saving.value = true
  try {
    await auth.changePassword(currentPassword.value, newPassword.value)
    await router.push('/password-changed')
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Password change failed'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="mx-auto max-w-lg space-y-6">
    <div class="rounded-xl border border-slate-800 bg-slate-900/60 p-5">
      <h2 class="text-lg font-semibold text-white">Account Profile</h2>
      <dl class="mt-4 space-y-2 text-sm">
        <div class="flex justify-between">
          <dt class="text-slate-400">Email</dt>
          <dd class="font-mono text-slate-200">{{ auth.user?.email }}</dd>
        </div>
        <div class="flex justify-between">
          <dt class="text-slate-400">Platform Role</dt>
          <dd class="text-slate-200">{{ auth.isPlatformAdmin ? 'Platform Admin' : 'Standard User' }}</dd>
        </div>
      </dl>
    </div>

    <div class="rounded-xl border border-slate-800 bg-slate-900/60 p-5">
      <div class="mb-4 flex items-center gap-2">
        <KeyRound class="h-4 w-4 text-sky-400" />
        <h3 class="text-base font-semibold text-white">Change Password</h3>
      </div>
      <form class="space-y-4" @submit.prevent="onChangePassword">
        <div>
          <label class="mb-1 block text-xs text-slate-400">Current Password</label>
          <input
            v-model="currentPassword"
            type="password"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white"
          />
        </div>
        <div>
          <label class="mb-1 block text-xs text-slate-400">New Password (min 8 chars)</label>
          <input
            v-model="newPassword"
            type="password"
            minlength="8"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white"
          />
        </div>
        <div>
          <label class="mb-1 block text-xs text-slate-400">Confirm New Password</label>
          <input
            v-model="confirmPassword"
            type="password"
            minlength="8"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white"
          />
        </div>
        <p v-if="error" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-2.5 text-xs text-rose-200">{{ error }}</p>
        <button
          type="submit"
          :disabled="saving"
          class="rounded-lg bg-sky-500 px-4 py-2 text-sm font-medium text-slate-950 hover:bg-sky-400 disabled:opacity-50"
        >
          {{ saving ? 'Updating...' : 'Update Password' }}
        </button>
      </form>
    </div>
  </div>
</template>
