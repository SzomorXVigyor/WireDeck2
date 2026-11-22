<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  size?: 'sm' | 'md'
}>()

const style = computed(() => {
  switch (props.status) {
    case 'ONLINE':
    case 'ACTIVE':
    case 'RUNNING':
      return {
        bg: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
        dot: 'bg-emerald-400',
      }
    case 'DEGRADED':
    case 'INVITED':
      return {
        bg: 'bg-amber-500/15 text-amber-300 border-amber-500/30',
        dot: 'bg-amber-400',
      }
    case 'OFFLINE':
    case 'DISABLED':
    case 'STOPPED':
      return {
        bg: 'bg-rose-500/15 text-rose-300 border-rose-500/30',
        dot: 'bg-rose-400',
      }
    default:
      return {
        bg: 'bg-slate-700/40 text-slate-300 border-slate-600/40',
        dot: 'bg-slate-400',
      }
  }
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full border font-medium tracking-wide uppercase"
    :class="[style.bg, size === 'sm' ? 'px-2 py-0.5 text-[10px]' : 'px-2.5 py-0.5 text-xs']"
  >
    <span class="h-1.5 w-1.5 rounded-full" :class="style.dot" />
    {{ status }}
  </span>
</template>
