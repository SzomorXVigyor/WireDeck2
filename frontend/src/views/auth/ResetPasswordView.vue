<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const token = ref(typeof route.query.token === 'string' ? route.query.token : '')
const newPassword = ref('')
const confirm = ref('')
const error = ref<string | null>(null)
const submitting = ref(false)

async function onSubmit() {
  error.value = null
  if (newPassword.value !== confirm.value) {
    error.value = 'Passwords do not match'
    return
  }
  submitting.value = true
  try {
    await auth.resetPassword(token.value, newPassword.value)
    await router.push('/login')
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Reset failed'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-slate-950 p-4">
    <div class="w-full max-w-sm rounded-2xl border border-slate-800 bg-slate-900/90 p-7 shadow-2xl">
      <h1 class="mb-1 text-lg font-semibold text-white">Choose a New Password</h1>
      <p class="mb-5 text-xs text-slate-400">All previous sessions will be signed out.</p>
      <form class="space-y-4" @submit.prevent="onSubmit">
        <div v-if="!route.query.token">
          <label class="mb-1.5 block text-xs font-medium text-slate-300">Reset Token</label>
          <input
            v-model="token"
            type="text"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white"
          />
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">New Password</label>
          <input
            v-model="newPassword"
            type="password"
            minlength="8"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white"
          />
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">Confirm New Password</label>
          <input
            v-model="confirm"
            type="password"
            minlength="8"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white"
          />
        </div>
        <p v-if="error" class="rounded-lg border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-xs text-rose-200">
          {{ error }}
        </p>
        <button
          type="submit"
          :disabled="submitting"
          class="w-full rounded-lg bg-sky-500 py-2 text-sm font-medium text-slate-950 hover:bg-sky-400 disabled:opacity-50"
        >
          {{ submitting ? 'Updating...' : 'Update Password' }}
        </button>
      </form>
    </div>
  </div>
</template>
