<script setup lang="ts">
import { ref } from 'vue'
import { Mail } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const email = ref('')
const submitted = ref(false)
const submitting = ref(false)

async function onSubmit() {
  submitting.value = true
  try {
    await auth.forgotPassword(email.value)
    submitted.value = true
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="safe-top safe-bottom flex min-h-dvh items-center justify-center bg-slate-950 p-4">
    <div class="w-full max-w-sm rounded-2xl border border-slate-800 bg-slate-900/90 p-6 shadow-2xl sm:p-7">
      <div class="mb-5 flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-500/20 text-sky-400">
          <Mail class="h-5 w-5" />
        </div>
        <div>
          <h1 class="text-lg font-semibold text-white">Reset Password</h1>
          <p class="text-xs text-slate-400">We will email you a 1-hour reset link</p>
        </div>
      </div>

      <div v-if="submitted" class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-200">
        If an active account exists for <strong>{{ email }}</strong>, a reset link has been sent.
      </div>

      <form v-else class="space-y-4" @submit.prevent="onSubmit">
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">Email</label>
          <input
            v-model="email"
            type="email"
            required
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
          />
        </div>
        <button
          type="submit"
          :disabled="submitting"
          class="w-full rounded-lg bg-sky-500 py-2 text-sm font-medium text-slate-950 hover:bg-sky-400 disabled:opacity-50"
        >
          {{ submitting ? 'Sending...' : 'Send Reset Link' }}
        </button>
      </form>

      <div class="mt-4 text-center">
        <RouterLink to="/login" class="text-xs text-slate-400 hover:text-sky-300">Back to login</RouterLink>
      </div>
    </div>
  </div>
</template>
