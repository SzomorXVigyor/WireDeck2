<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { AlertTriangle, Power, X } from 'lucide-vue-next'

type ActionTone = 'warning' | 'danger' | 'success' | 'primary'

const props = defineProps<{
  open: boolean
  title: string
  itemName?: string
  description: string
  confirmLabel?: string
  tone?: ActionTone
  busy?: boolean
}>()

const emit = defineEmits<{
  close: []
  confirm: []
}>()

const toneConfig = computed(() => {
  switch (props.tone) {
    case 'danger':
      return {
        iconBox: 'bg-rose-500/15 text-rose-400',
        button: 'bg-rose-600 text-white hover:bg-rose-500',
        useAlertIcon: true,
      }
    case 'success':
      return {
        iconBox: 'bg-emerald-500/15 text-emerald-400',
        button: 'bg-emerald-600 text-white hover:bg-emerald-500',
        useAlertIcon: false,
      }
    case 'primary':
      return {
        iconBox: 'bg-indigo-500/15 text-indigo-400',
        button: 'bg-indigo-500 text-white hover:bg-indigo-400',
        useAlertIcon: false,
      }
    default:
      return {
        iconBox: 'bg-amber-500/15 text-amber-400',
        button: 'bg-amber-500 text-slate-950 hover:bg-amber-400',
        useAlertIcon: true,
      }
  }
})

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.open && !props.busy) {
    emit('close')
  }
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      role="dialog"
      aria-modal="true"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
      @click.self="!busy && emit('close')"
    >
      <div
        class="max-h-[90dvh] w-full max-w-sm overflow-hidden rounded-2xl border border-slate-700 bg-slate-900 text-slate-100 shadow-2xl"
      >
        <div class="flex items-center justify-between border-b border-slate-800 px-5 py-4">
          <div class="flex min-w-0 items-center gap-3">
            <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl" :class="toneConfig.iconBox">
              <AlertTriangle v-if="toneConfig.useAlertIcon" class="h-4 w-4" />
              <Power v-else class="h-4 w-4" />
            </div>
            <h3 class="truncate text-sm font-semibold text-white">{{ title }}</h3>
          </div>
          <button
            type="button"
            aria-label="Close modal"
            :disabled="busy"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-800 hover:text-white disabled:opacity-40"
            @click="emit('close')"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="space-y-2.5 px-5 py-4 text-xs leading-relaxed text-slate-300">
          <p
            v-if="itemName"
            class="break-words rounded-lg border border-slate-800 bg-slate-950/70 px-3 py-2 font-semibold text-white"
          >
            {{ itemName }}
          </p>
          <p class="text-slate-400">{{ description }}</p>
        </div>

        <div class="flex items-center justify-end gap-2 border-t border-slate-800 px-5 py-3.5">
          <button
            type="button"
            :disabled="busy"
            class="inline-flex h-9 items-center justify-center rounded-lg border border-slate-700 bg-slate-800 px-4 text-xs font-medium leading-none text-slate-200 transition hover:bg-slate-700 disabled:opacity-40"
            @click="emit('close')"
          >
            Cancel
          </button>
          <button
            type="button"
            :disabled="busy"
            class="inline-flex h-9 items-center justify-center rounded-lg px-4 text-xs font-semibold leading-none transition disabled:opacity-40"
            :class="toneConfig.button"
            @click="emit('confirm')"
          >
            {{ busy ? 'Working...' : confirmLabel || 'Confirm' }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
