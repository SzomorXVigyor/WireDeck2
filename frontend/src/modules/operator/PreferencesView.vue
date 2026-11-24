<script setup lang="ts">
import { computed } from 'vue'
import WireDeckLogo from '../../components/WireDeckLogo.vue'
import { useThemeStore, type ThemeMode } from '../../stores/theme'

const theme = useThemeStore()
const appVersion = __APP_VERSION__

const systemLabel = computed(() => (theme.systemPrefersDark ? 'Dark' : 'Light'))

function onThemeChange(e: Event) {
  const val = (e.target as HTMLSelectElement).value as ThemeMode
  theme.setMode(val)
}
</script>

<template>
  <div class="w-full space-y-5 sm:space-y-6">
    <div>
      <h1 class="text-lg font-semibold tracking-tight text-white sm:text-xl">Preferences</h1>
      <p class="mt-0.5 text-xs text-slate-400">Personalize your WireDeck workspace settings.</p>
    </div>

    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 class="text-sm font-semibold text-white">Color Theme</h2>
          <p class="mt-0.5 text-xs text-slate-400">
            Choose how WireDeck looks on this device. Active:
            <span class="font-medium text-slate-200">{{ theme.resolvedTheme === 'dark' ? 'Dark' : 'Light' }}</span>.
          </p>
        </div>

        <select
          :value="theme.mode"
          aria-label="Color theme"
          class="h-9 w-full rounded-lg border border-slate-700 bg-slate-950 px-3 text-xs font-medium text-white focus:border-sky-500 focus:outline-none sm:w-48"
          @change="onThemeChange"
        >
          <option value="system">System ({{ systemLabel }})</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </div>
    </section>

    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-4 sm:p-5">
      <div class="flex items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <WireDeckLogo size="md" />
          <div>
            <h2 class="text-sm font-semibold text-white">WireDeck</h2>
            <p class="text-xs text-slate-400">Industrial Site Control Plane</p>
          </div>
        </div>
        <div class="shrink-0 font-mono text-xs text-slate-200">
          {{ appVersion }}
        </div>
      </div>
    </section>
  </div>
</template>
