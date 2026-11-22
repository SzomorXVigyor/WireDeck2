<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { Trash2, X } from 'lucide-vue-next'

const props = defineProps<{
  open: boolean
  title: string
  itemName: string
  description?: string
  confirmLabel?: string
}>()

const emit = defineEmits<{
  close: []
  confirm: []
}>()

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.open) {
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
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
      @click.self="emit('close')"
    >
      <div class="w-full max-w-sm overflow-hidden rounded-2xl border border-slate-700 bg-slate-900 text-slate-100 shadow-2xl">
        <div class="flex items-center justify-between border-b border-slate-800 px-5 py-4">
          <div class="flex items-center gap-3">
            <div class="flex h-9 w-9 items-center justify-center rounded-xl bg-rose-500/15 text-rose-400">
              <Trash2 class="h-4 w-4" />
            </div>
            <h3 class="text-sm font-semibold text-white">{{ title }}</h3>
          </div>
          <button
            type="button"
            class="rounded-lg p-1 text-slate-400 hover:bg-slate-800 hover:text-white"
            @click="emit('close')"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="px-5 py-4 text-xs leading-relaxed text-slate-300">
          <p>Are you sure you want to delete this item?</p>
          <p class="mt-2 rounded-lg border border-slate-800 bg-slate-950/70 px-3 py-2 font-semibold text-white">
            {{ itemName }}
          </p>
          <p class="mt-2 text-slate-400">
            {{ description || 'This action cannot be undone.' }}
          </p>
        </div>

        <div class="flex justify-end gap-2 border-t border-slate-800 px-5 py-3.5">
          <button
            type="button"
            class="rounded-lg border border-slate-700 bg-slate-800 px-3.5 py-1.5 text-xs font-medium text-slate-200 hover:bg-slate-700"
            @click="emit('close')"
          >
            Cancel
          </button>
          <button
            type="button"
            class="rounded-lg bg-rose-600 px-3.5 py-1.5 text-xs font-semibold text-white hover:bg-rose-500"
            @click="emit('confirm')"
          >
            {{ confirmLabel || 'Delete' }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
