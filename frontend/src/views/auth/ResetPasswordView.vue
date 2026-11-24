<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertTriangle, CheckCircle2, KeyRound, Loader2 } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const token = ref(typeof route.query.token === 'string' ? route.query.token.trim() : '')
const stage = ref<'checking' | 'invalid' | 'ready' | 'success'>('checking')
const invalidReason = ref<string>('INVALID')

const newPassword = ref('')
const confirm = ref('')
const error = ref<string | null>(null)
const submitting = ref(false)

const invalidTitle = computed(() => {
  if (invalidReason.value === 'EXPIRED') return 'Reset Link Expired'
  if (invalidReason.value === 'USED') return 'Reset Link Already Used'
  return 'Invalid Reset Link'
})

const invalidMessage = computed(() => {
  if (invalidReason.value === 'EXPIRED') {
    return 'This password reset link has expired. Request a new password reset link from the login page.'
  }
  if (invalidReason.value === 'USED') {
    return 'This password reset link has already been used. You can sign in with your new password or request another reset link.'
  }
  return 'This password reset link is invalid or incomplete. Please check the link from your email or request a new one.'
})

onMounted(async () => {
  if (!token.value) {
    invalidReason.value = 'INVALID'
    stage.value = 'invalid'
    return
  }
  try {
    const res = await auth.verifyToken(token.value, 'PASSWORD_RESET')
    if (res.valid) {
      stage.value = 'ready'
    } else {
      invalidReason.value = res.reason || 'INVALID'
      stage.value = 'invalid'
    }
  } catch {
    invalidReason.value = 'INVALID'
    stage.value = 'invalid'
  }
})

async function onSubmit() {
  error.value = null
  if (newPassword.value !== confirm.value) {
    error.value = 'Passwords do not match'
    return
  }
  submitting.value = true
  try {
    await auth.resetPassword(token.value, newPassword.value)
    stage.value = 'success'
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Reset failed'
    if (msg.toLowerCase().includes('invalid or expired token')) {
      invalidReason.value = 'EXPIRED'
      stage.value = 'invalid'
    } else {
      error.value = msg
    }
  } finally {
    submitting.value = false
  }
}

async function goToLogin() {
  if (auth.isAuthenticated) {
    await auth.logout()
  }
  await router.push('/login')
}
</script>

<template>
  <div class="safe-top safe-bottom flex min-h-dvh items-center justify-center bg-slate-950 p-4">
    <div class="w-full max-w-sm rounded-2xl border border-slate-800 bg-slate-900/90 p-6 shadow-2xl sm:p-7">
      <!-- Checking token -->
      <div v-if="stage === 'checking'" class="flex flex-col items-center py-6 text-center">
        <Loader2 class="mb-3 h-8 w-8 animate-spin text-sky-400" />
        <h1 class="text-base font-semibold text-white">Verifying Reset Link</h1>
        <p class="mt-1 text-xs text-slate-400">Please wait while we check your reset link...</p>
      </div>

      <!-- Invalid / Expired / Used -->
      <div v-else-if="stage === 'invalid'" class="space-y-5">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-rose-500/20 text-rose-400">
            <AlertTriangle class="h-5 w-5" />
          </div>
          <div>
            <h1 class="text-lg font-semibold text-white">{{ invalidTitle }}</h1>
            <p class="text-xs text-slate-400">Unable to reset password</p>
          </div>
        </div>
        <p class="rounded-lg border border-rose-500/30 bg-rose-500/10 p-3 text-xs leading-relaxed text-rose-200">
          {{ invalidMessage }}
        </p>
        <button
          type="button"
          class="w-full rounded-lg bg-sky-500 py-2 text-sm font-medium text-slate-950 hover:bg-sky-400"
          @click="goToLogin"
        >
          Go to Login
        </button>
        <div class="text-center">
          <RouterLink to="/forgot-password" class="text-xs text-slate-400 hover:text-sky-300">
            Request a new reset link
          </RouterLink>
        </div>
      </div>

      <!-- Password Change Success -->
      <div v-else-if="stage === 'success'" class="space-y-5">
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
          Your password has been changed successfully and all previous sessions have been signed out. You can now sign in with your new password.
        </p>
        <button
          type="button"
          class="w-full rounded-lg bg-sky-500 py-2 text-sm font-medium text-slate-950 hover:bg-sky-400"
          @click="goToLogin"
        >
          Go to Login
        </button>
      </div>

      <!-- Valid link: Choose New Password Form -->
      <template v-else>
        <div class="mb-5 flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-500/20 text-sky-400">
            <KeyRound class="h-5 w-5" />
          </div>
          <div>
            <h1 class="text-lg font-semibold text-white">Choose a New Password</h1>
            <p class="text-xs text-slate-400">All previous sessions will be signed out.</p>
          </div>
        </div>
        <form class="space-y-4" @submit.prevent="onSubmit">
          <div>
            <label class="mb-1.5 block text-xs font-medium text-slate-300">New Password (min 8 chars)</label>
            <input
              v-model="newPassword"
              type="password"
              minlength="8"
              required
              autocomplete="new-password"
              class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white focus:border-sky-500 focus:outline-none"
            />
          </div>
          <div>
            <label class="mb-1.5 block text-xs font-medium text-slate-300">Confirm New Password</label>
            <input
              v-model="confirm"
              type="password"
              minlength="8"
              required
              autocomplete="new-password"
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
            {{ submitting ? 'Updating...' : 'Update Password' }}
          </button>
        </form>
      </template>
    </div>
  </div>
</template>
