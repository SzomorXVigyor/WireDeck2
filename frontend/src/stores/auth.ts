import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { apiRequest, configureApiAuth } from '../services/api'
import type { AuthResponse, User } from '../types/iam'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const accessToken = ref<string | null>(null)
  const initialized = ref(false)

  const isAuthenticated = computed(() => !!accessToken.value && !!user.value)
  const isPlatformAdmin = computed(() => user.value?.isPlatformAdmin ?? false)

  function applySession(res: AuthResponse) {
    accessToken.value = res.accessToken
    user.value = res.user
  }

  function clearSession() {
    accessToken.value = null
    user.value = null
  }

  async function refresh(): Promise<string | null> {
    try {
      const res = await apiRequest<AuthResponse>('/api/v1/iam/auth/refresh', {
        method: 'POST',
        skipAuth: true,
      })
      applySession(res)
      return res.accessToken
    } catch {
      clearSession()
      return null
    }
  }

  configureApiAuth(() => accessToken.value, refresh)

  async function init() {
    if (initialized.value) return
    await refresh()
    initialized.value = true
  }

  async function login(email: string, password: string) {
    const res = await apiRequest<AuthResponse>('/api/v1/iam/auth/login', {
      method: 'POST',
      skipAuth: true,
      body: JSON.stringify({ email, password }),
    })
    applySession(res)
  }

  async function acceptInvite(token: string, password: string) {
    const res = await apiRequest<AuthResponse>('/api/v1/iam/auth/accept-invite', {
      method: 'POST',
      skipAuth: true,
      body: JSON.stringify({ token, password }),
    })
    applySession(res)
  }

  async function changePassword(currentPassword: string, newPassword: string) {
    const res = await apiRequest<AuthResponse>('/api/v1/iam/me/change-password', {
      method: 'POST',
      body: JSON.stringify({ currentPassword, newPassword }),
    })
    applySession(res)
  }

  async function forgotPassword(email: string) {
    await apiRequest<void>('/api/v1/iam/auth/forgot-password', {
      method: 'POST',
      skipAuth: true,
      body: JSON.stringify({ email }),
    })
  }

  async function resetPassword(token: string, newPassword: string) {
    await apiRequest<void>('/api/v1/iam/auth/reset-password', {
      method: 'POST',
      skipAuth: true,
      body: JSON.stringify({ token, newPassword }),
    })
  }

  async function logout() {
    try {
      await apiRequest<void>('/api/v1/iam/auth/logout', { method: 'POST', skipAuth: true })
    } finally {
      clearSession()
    }
  }

  return {
    user,
    accessToken,
    initialized,
    isAuthenticated,
    isPlatformAdmin,
    init,
    login,
    refresh,
    acceptInvite,
    changePassword,
    forgotPassword,
    resetPassword,
    logout,
  }
})
