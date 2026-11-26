import { defineStore } from 'pinia'
import { ref } from 'vue'
import { formatErrorMessage } from '../services/api'

export type ToastKind = 'success' | 'error' | 'warning' | 'info'

export interface ToastItem {
  id: number
  kind: ToastKind
  title?: string
  message: string
  durationMs: number
}

const DEFAULT_DURATIONS: Record<ToastKind, number> = {
  success: 4000,
  info: 4500,
  warning: 5500,
  error: 6500,
}

const MAX_TOASTS = 5

export const useToastStore = defineStore('toast', () => {
  const items = ref<ToastItem[]>([])
  const timers = new Map<number, number>()
  let seq = 0

  function clearTimer(id: number) {
    const handle = timers.get(id)
    if (handle !== undefined) {
      window.clearTimeout(handle)
      timers.delete(id)
    }
  }

  function scheduleDismiss(id: number, durationMs: number) {
    clearTimer(id)
    if (durationMs <= 0) return
    const handle = window.setTimeout(() => {
      dismiss(id)
    }, durationMs)
    timers.set(id, handle)
  }

  function push(kind: ToastKind, message: string, opts?: { title?: string; durationMs?: number }): number {
    const clean = message.trim()
    if (!clean) return 0

    // Deduplicate identical active toast by resetting its timer
    const existing = items.value.find((t) => t.kind === kind && t.message === clean)
    if (existing) {
      scheduleDismiss(existing.id, opts?.durationMs ?? existing.durationMs)
      return existing.id
    }

    const id = ++seq
    const durationMs = opts?.durationMs ?? DEFAULT_DURATIONS[kind]
    const next: ToastItem = {
      id,
      kind,
      title: opts?.title,
      message: clean,
      durationMs,
    }
    items.value = [...items.value, next].slice(-MAX_TOASTS)
    scheduleDismiss(id, durationMs)
    return id
  }

  function dismiss(id: number) {
    clearTimer(id)
    items.value = items.value.filter((t) => t.id !== id)
  }

  function pause(id: number) {
    clearTimer(id)
  }

  function resume(id: number) {
    const item = items.value.find((t) => t.id === id)
    if (item) {
      scheduleDismiss(id, Math.min(item.durationMs, 2500))
    }
  }

  function success(message: string, title?: string, durationMs?: number) {
    return push('success', message, { title, durationMs })
  }

  function error(err: unknown, fallbackOrTitle?: string, durationMs?: number) {
    const msg = formatErrorMessage(err, fallbackOrTitle || 'Operation failed.')
    const title = err instanceof Error && fallbackOrTitle ? fallbackOrTitle : 'Error'
    return push('error', msg, { title, durationMs })
  }

  function warning(message: string, title?: string, durationMs?: number) {
    return push('warning', message, { title, durationMs })
  }

  function info(message: string, title?: string, durationMs?: number) {
    return push('info', message, { title, durationMs })
  }

  return {
    items,
    push,
    dismiss,
    pause,
    resume,
    success,
    error,
    warning,
    info,
  }
})
