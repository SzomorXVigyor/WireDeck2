<script setup lang="ts">
import { Construction } from 'lucide-vue-next'

defineProps<{
  title: string
  phase: string
  description: string
}>()
</script>

<template>
  <div class="mx-auto mt-16 max-w-lg rounded-2xl border border-slate-800 bg-slate-900/60 p-8 text-center">
    <Construction class="mx-auto h-9 w-9 text-sky-400" />
    <span class="mt-3 inline-block rounded-full border border-sky-500/30 bg-sky-500/15 px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-sky-300">
      Scheduled for {{ phase }}
    </span>
    <h2 class="mt-3 text-lg font-semibold text-white">{{ title }}</h2>
    <p class="mt-2 text-sm text-slate-400">{{ description }}</p>
  </div>
</template>
