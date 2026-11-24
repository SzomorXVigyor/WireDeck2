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
  <div class="w-full space-y-5 sm:space-y-6">
    <div>
      <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">Profile & Password</h1>
      <p class="mt-0.5 text-xs text-slate-400">View your account details and update your sign-in credentials.</p>
    </div>

    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
      <h2 class="text-base font-semibold text-white">Account Profile</h2>
      <dl class="mt-4 space-y-3 text-sm">
        <div class="flex flex-col gap-0.5 sm:flex-row sm:items-center sm:justify-between">
          <dt class="text-xs text-slate-400 sm:text-sm">Email</dt>
          <dd class="break-all font-mono text-sm text-slate-200">{{ auth.user?.email }}</dd>
        </div>
        <div class="flex flex-col gap-0.5 border-t border-slate-800/70 pt-3 sm:flex-row sm:items-center sm:justify-between">
          <dt class="text-xs text-slate-400 sm:text-sm">Platform Role</dt>
          <dd class="text-sm font-medium text-slate-200">{{ auth.isPlatformAdmin ? 'Platform Admin' : 'Standard User' }}</dd>
        </div>
      </dl>
    </section>

    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
      <div class="mb-4 flex items-center gap-2">
        <KeyRound class="h-4 w-4 text-sky-400" />
        <h2 class="text-base font-semibold text-white">Change Password</h2>
      </div>
      <form class="space-y-4" @submit.prevent="onChangePassword">
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-400">Current Password</label>
          <input
            v-model="currentPassword"
            type="password"
            autocomplete="current-password"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
          />
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="mb-1.5 block text-xs font-medium text-slate-400">New Password (min 8 chars)</label>
            <input
              v-model="newPassword"
              type="password"
              autocomplete="new-password"
              minlength="8"
              required
              class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
            />
          </div>
          <div>
            <label class="mb-1.5 block text-xs font-medium text-slate-400">Confirm New Password</label>
            <input
              v-model="confirmPassword"
              type="password"
              autocomplete="new-password"
              minlength="8"
              required
              class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
            />
          </div>
        </div>
        <p v-if="error" class="rounded-lg border border-rose-500/40 bg-rose-500/10 p-2.5 text-xs text-rose-200">{{ error }}</p>
        <div class="pt-1">
          <button
            type="submit"
            :disabled="saving"
            class="w-full rounded-lg bg-sky-500 px-4 py-2.5 text-xs font-semibold text-slate-950 transition hover:bg-sky-400 disabled:opacity-50 sm:w-auto sm:py-2"
          >
            {{ saving ? 'Updating...' : 'Update Password' }}
          </button>
        </div>
      </form>
    </section>
  </div>
</template>
