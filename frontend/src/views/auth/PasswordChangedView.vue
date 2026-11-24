<script setup lang="ts">
import { useRouter } from 'vue-router'
import { CheckCircle2 } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function goToLogin() {
  if (auth.isAuthenticated) {
    await auth.logout()
  }
  await router.push('/login')
}
</script>

<template>
  <div class="safe-top safe-bottom flex min-h-dvh items-center justify-center bg-slate-950 p-4">
    <div class="w-full max-w-sm rounded-2xl border border-slate-800 bg-slate-900/90 p-6 shadow-2xl space-y-5 sm:p-7">
      <div class="flex items-center gap-3">
        <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-emerald-500/20 text-emerald-400">
          <CheckCircle2 class="h-5 w-5" />
        </div>
        <div>
          <h1 class="text-lg font-semibold text-white">Password Changed</h1>
          <p class="text-xs text-slate-400">Your password has been updated</p>
        </div>
      </div>

      <p class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs leading-relaxed text-emerald-200">
        Your password has been changed successfully and all active sessions have been signed out. Please sign in with your new password.
      </p>

      <button
        type="button"
        class="w-full rounded-lg bg-sky-500 py-2 text-sm font-medium text-slate-950 hover:bg-sky-400"
        @click="goToLogin"
      >
        Go to Login
      </button>
    </div>
  </div>
</template>
