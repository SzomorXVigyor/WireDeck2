import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

export type ThemeMode = 'system' | 'light' | 'dark'
export type ResolvedTheme = 'light' | 'dark'

const STORAGE_KEY = 'wd2_theme'

function readStoredMode(): ThemeMode {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw === 'light' || raw === 'dark' || raw === 'system') return raw
  } catch {
    // ignore storage errors
  }
  return 'system'
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(readStoredMode())
  const systemPrefersDark = ref<boolean>(
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
      : true,
  )

  const resolvedTheme = computed<ResolvedTheme>(() => {
    if (mode.value === 'system') {
      return systemPrefersDark.value ? 'dark' : 'light'
    }
    return mode.value
  })

  function applyDomTheme() {
    if (typeof document === 'undefined') return
    const root = document.documentElement
    const resolved = resolvedTheme.value
    root.classList.toggle('dark', resolved === 'dark')
    root.classList.toggle('light', resolved === 'light')
    root.dataset.theme = resolved
    root.style.colorScheme = resolved
  }

  function setMode(next: ThemeMode) {
    mode.value = next
    try {
      window.localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // ignore storage errors
    }
    applyDomTheme()
  }

  let initialized = false
  function init() {
    if (initialized) return
    initialized = true
    applyDomTheme()
    if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
      const mql = window.matchMedia('(prefers-color-scheme: dark)')
      systemPrefersDark.value = mql.matches
      applyDomTheme()
      const onChange = (ev: MediaQueryListEvent) => {
        systemPrefersDark.value = ev.matches
        applyDomTheme()
      }
      if (typeof mql.addEventListener === 'function') {
        mql.addEventListener('change', onChange)
      }
    }
  }

  return {
    mode,
    systemPrefersDark,
    resolvedTheme,
    setMode,
    init,
  }
})
