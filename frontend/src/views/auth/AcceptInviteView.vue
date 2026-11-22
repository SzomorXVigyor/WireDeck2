<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { KeyRound } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const token = ref(typeof route.query.token === 'string' ? route.query.token : '')
const password = ref('')
const confirm = ref('')
const error = ref<string | null>(null)
const submitting = ref(false)

async function onSubmit() {
  error.value = null
  if (password.value !== confirm.value) {
    error.value = 'Passwords do not match'
    return
  }
  submitting.value = true
  try {
    await auth.acceptInvite(token.value, password.value)
    await router.push('/')
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Failed to activate account'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-slate-950 p-4">
    <div class="w-full max-w-sm rounded-2xl border border-slate-800 bg-slate-900/90 p-7 shadow-2xl">
      <div class="mb-6 flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-500/20 text-emerald-400">
          <KeyRound class="h-5 w-5" />
        </div>
        <div>
          <h1 class="text-lg font-semibold text-white">Accept Invitation</h1>
          <p class="text-xs text-slate-400">Set your WireDeck2 account password</p>
        </div>
      </div>

      <form class="space-y-4" @submit.prevent="onSubmit">
        <div v-if="!route.query.token">
          <label class="mb-1.5 block text-xs font-medium text-slate-300">Invite Token</label>
          <input
            v-model="token"
            type="text"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-white focus:border-sky-500 focus:outline-none"
          />
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">New Password (min 8 chars)</label>
          <input
            v-model="password"
            type="password"
            minlength="8"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
          />
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">Confirm Password</label>
          <input
            v-model="confirm"
            type="password"
            minlength="8"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
          />
        </div>
        <p v-if="error" class="rounded-lg border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-xs text-rose-200">
          {{ error }}
        </p>
        <button
          type="submit"
          :disabled="submitting"
          class="w-full rounded-lg bg-emerald-500 py-2 text-sm font-medium text-slate-950 hover:bg-emerald-400 disabled:opacity-50"
        >
          {{ submitting ? 'Activating...' : 'Activate Account' }}
        </button>
      </form>
    </div>
  </div>
</template>
