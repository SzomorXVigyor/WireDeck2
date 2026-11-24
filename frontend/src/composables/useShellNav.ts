import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

const STORAGE_KEY = 'wd2_sidebar_collapsed'

// Shared across layouts so collapsing the desktop rail persists across surfaces
const desktopCollapsed = ref<boolean>(readInitialCollapsed())

function readInitialCollapsed(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

export function useShellNav() {
  const route = useRoute()
  const mobileDrawerOpen = ref(false)

  function toggleDesktopCollapsed() {
    desktopCollapsed.value = !desktopCollapsed.value
    try {
      localStorage.setItem(STORAGE_KEY, desktopCollapsed.value ? '1' : '0')
    } catch {
      // ignore storage errors
    }
  }

  function openMobileDrawer() {
    mobileDrawerOpen.value = true
  }

  function closeMobileDrawer() {
    mobileDrawerOpen.value = false
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && mobileDrawerOpen.value) {
      mobileDrawerOpen.value = false
    }
  }

  // Close mobile drawer automatically whenever navigation occurs
  watch(
    () => route.fullPath,
    () => {
      mobileDrawerOpen.value = false
    },
  )

  // Prevent background page scroll while mobile drawer is open
  watch(mobileDrawerOpen, (open) => {
    if (typeof document !== 'undefined') {
      document.body.style.overflow = open ? 'hidden' : ''
    }
  })

  onMounted(() => {
    document.addEventListener('keydown', onKeydown)
  })

  onUnmounted(() => {
    document.removeEventListener('keydown', onKeydown)
    if (typeof document !== 'undefined') {
      document.body.style.overflow = ''
    }
  })

  return {
    desktopCollapsed,
    mobileDrawerOpen,
    toggleDesktopCollapsed,
    openMobileDrawer,
    closeMobileDrawer,
  }
}
