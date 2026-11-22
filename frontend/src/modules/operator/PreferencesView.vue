<script setup lang="ts">
import { computed } from 'vue'
import { Check, Monitor, Moon, Sun } from 'lucide-vue-next'
import { useThemeStore, type ThemeMode } from '../../stores/theme'

const theme = useThemeStore()

const systemLabel = computed(() => (theme.systemPrefersDark ? 'Dark' : 'Light'))

interface ThemeOption {
  value: ThemeMode
  label: string
  description: string
  icon: typeof Monitor
}

const options: ThemeOption[] = [
  {
    value: 'system',
    label: 'System',
    description: 'Automatically match your operating system light or dark appearance.',
    icon: Monitor,
  },
  {
    value: 'light',
    label: 'Light',
    description: 'Clean high-contrast light surfaces for bright workspaces.',
    icon: Sun,
  },
  {
    value: 'dark',
    label: 'Dark',
    description: 'Low-glare dark surfaces designed for control rooms.',
    icon: Moon,
  },
]
</script>

<template>
  <div class="mx-auto max-w-2xl space-y-6">
    <div>
      <h1 class="text-lg font-semibold text-white">Preferences</h1>
      <p class="mt-0.5 text-xs text-slate-400">Personalize your WireDeck2 workspace settings.</p>
    </div>

    <section class="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 class="text-base font-semibold text-white">Appearance</h2>
          <p class="mt-0.5 text-xs text-slate-400">
            Choose how WireDeck2 looks on this device. System is selected by default.
          </p>
        </div>
        <span class="rounded-full border border-slate-700 bg-slate-800 px-2.5 py-1 text-[11px] text-slate-300">
          Active: {{ theme.resolvedTheme === 'dark' ? 'Dark' : 'Light' }}
        </span>
      </div>

      <div class="grid gap-3 sm:grid-cols-3">
        <button
          v-for="opt in options"
          :key="opt.value"
          type="button"
          class="relative flex flex-col items-start rounded-xl border p-4 text-left transition"
          :class="
            theme.mode === opt.value
              ? 'border-sky-500/70 bg-sky-500/15'
              : 'border-slate-800 bg-slate-950/60 hover:border-slate-700 hover:bg-slate-800/40'
          "
          @click="theme.setMode(opt.value)"
        >
          <div class="flex w-full items-center justify-between">
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg"
              :class="theme.mode === opt.value ? 'bg-sky-500/20 text-sky-300' : 'bg-slate-800 text-slate-400'"
            >
              <component :is="opt.icon" class="h-4 w-4" />
            </div>
            <span
              v-if="theme.mode === opt.value"
              class="flex h-5 w-5 items-center justify-center rounded-full bg-sky-500 text-slate-950"
            >
              <Check class="h-3 w-3" />
            </span>
          </div>

          <p class="mt-3 text-sm font-semibold text-white">
            {{ opt.label }}
            <span v-if="opt.value === 'system'" class="ml-1 text-[11px] font-normal text-slate-400">
              ({{ systemLabel }})
            </span>
          </p>
          <p class="mt-1 text-xs leading-relaxed text-slate-400">{{ opt.description }}</p>
        </button>
      </div>

      <p class="text-[11px] text-slate-500">
        Your theme preference is saved automatically in this browser.
      </p>
    </section>
  </div>
</template>
