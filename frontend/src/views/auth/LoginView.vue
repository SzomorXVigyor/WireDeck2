<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Server } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const email = ref('')
const password = ref('')
const error = ref<string | null>(null)
const submitting = ref(false)

async function onSubmit() {
  error.value = null
  submitting.value = true
  try {
    await auth.login(email.value, password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.push(redirect)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Login failed'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-slate-950 p-4">
    <div class="w-full max-w-sm rounded-2xl border border-slate-800 bg-slate-900/90 p-7 shadow-2xl">
      <div class="mb-6 flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-500/20 text-sky-400">
          <Server class="h-5 w-5" />
        </div>
        <div>
          <h1 class="text-lg font-semibold text-white">WireDeck2</h1>
          <p class="text-xs text-slate-400">Sign in to your control plane</p>
        </div>
      </div>

      <form class="space-y-4" @submit.prevent="onSubmit">
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">Email</label>
          <input
            v-model="email"
            type="email"
            required
            autocomplete="email"
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
          />
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">Password</label>
          <input
            v-model="password"
            type="password"
            required
            autocomplete="current-password"
            class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
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
          {{ submitting ? 'Signing in...' : 'Sign In' }}
        </button>
      </form>

      <div class="mt-4 text-center">
        <RouterLink to="/forgot-password" class="text-xs text-slate-400 hover:text-sky-300">
          Forgot your password?
        </RouterLink>
      </div>
    </div>
  </div>
</template>
