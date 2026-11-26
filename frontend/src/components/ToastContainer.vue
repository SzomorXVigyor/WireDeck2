<script setup lang="ts">
import { AlertCircle, AlertTriangle, CheckCircle2, Info, X } from 'lucide-vue-next'
import { useToastStore, type ToastKind } from '../stores/toast'

const toast = useToastStore()

function barClass(kind: ToastKind): string {
  switch (kind) {
    case 'success':
      return 'bg-emerald-400'
    case 'error':
      return 'bg-rose-500'
    case 'warning':
      return 'bg-amber-400'
    default:
      return 'bg-sky-400'
  }
}

function borderClass(kind: ToastKind): string {
  switch (kind) {
    case 'success':
      return 'border-emerald-500/35'
    case 'error':
      return 'border-rose-500/45'
    case 'warning':
      return 'border-amber-500/40'
    default:
      return 'border-sky-500/35'
  }
}

function iconClass(kind: ToastKind): string {
  switch (kind) {
    case 'success':
      return 'text-emerald-400'
    case 'error':
      return 'text-rose-400'
    case 'warning':
      return 'text-amber-400'
    default:
      return 'text-sky-400'
  }
}

function defaultTitle(kind: ToastKind): string {
  switch (kind) {
    case 'success':
      return 'Done'
    case 'error':
      return 'Error'
    case 'warning':
      return 'Warning'
    default:
      return 'Notice'
  }
}
</script>

<template>
  <Teleport to="body">
    <div
      aria-live="polite"
      aria-atomic="false"
      class="safe-bottom pointer-events-none fixed inset-x-3 bottom-3 z-[80] flex flex-col gap-2 sm:inset-x-auto sm:right-5 sm:bottom-5 sm:w-96 sm:max-w-[calc(100vw-2.5rem)]"
    >
      <TransitionGroup name="wd-toast">
        <div
          v-for="item in toast.items"
          :key="item.id"
          role="status"
          class="pointer-events-auto relative flex items-start gap-3 overflow-hidden rounded-xl border bg-slate-900/95 px-3.5 py-3 shadow-2xl backdrop-blur-md"
          :class="borderClass(item.kind)"
          @mouseenter="toast.pause(item.id)"
          @mouseleave="toast.resume(item.id)"
        >
          <div class="pointer-events-none absolute inset-y-0 left-0 w-1" :class="barClass(item.kind)" />

          <div class="mt-0.5 shrink-0 pl-1" :class="iconClass(item.kind)">
            <CheckCircle2 v-if="item.kind === 'success'" class="h-4 w-4" />
            <AlertCircle v-else-if="item.kind === 'error'" class="h-4 w-4" />
            <AlertTriangle v-else-if="item.kind === 'warning'" class="h-4 w-4" />
            <Info v-else class="h-4 w-4" />
          </div>

          <div class="min-w-0 flex-1">
            <p class="text-xs font-semibold text-white">
              {{ item.title || defaultTitle(item.kind) }}
            </p>
            <p class="mt-0.5 break-words text-xs leading-relaxed text-slate-300">
              {{ item.message }}
            </p>
          </div>

          <button
            type="button"
            aria-label="Dismiss notification"
            class="flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white"
            @click="toast.dismiss(item.id)"
          >
            <X class="h-3.5 w-3.5" />
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.wd-toast-enter-active,
.wd-toast-leave-active {
  transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}
.wd-toast-enter-from {
  opacity: 0;
  transform: translateY(12px) scale(0.97);
}
.wd-toast-leave-to {
  opacity: 0;
  transform: translateX(16px) scale(0.97);
}
.wd-toast-move {
  transition: transform 0.2s ease;
}
</style>
