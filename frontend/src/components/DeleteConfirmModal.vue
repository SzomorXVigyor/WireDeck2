<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Trash2, X } from 'lucide-vue-next'

const props = defineProps<{
  open: boolean
  title: string
  itemName: string
  description?: string
  confirmLabel?: string
  requireConfirmText?: string
  confirmInputLabel?: string
}>()

const emit = defineEmits<{
  close: []
  confirm: []
}>()

const typedText = ref('')
const inputEl = ref<HTMLInputElement | null>(null)

const requiresTyping = computed(() => Boolean(props.requireConfirmText && props.requireConfirmText.length > 0))
const canConfirm = computed(() => {
  if (!requiresTyping.value) return true
  return typedText.value.trim() === props.requireConfirmText
})

watch(
  () => props.open,
  async (isOpen) => {
    typedText.value = ''
    if (isOpen && requiresTyping.value) {
      await nextTick()
      inputEl.value?.focus()
    }
  },
)

function handleConfirm() {
  if (!canConfirm.value) return
  emit('confirm')
}

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
      role="dialog"
      aria-modal="true"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
      @click.self="emit('close')"
    >
      <div
        class="max-h-[90dvh] w-full max-w-sm overflow-hidden rounded-2xl border border-slate-700 bg-slate-900 text-slate-100 shadow-2xl"
      >
        <div class="flex items-center justify-between border-b border-slate-800 px-5 py-4">
          <div class="flex min-w-0 items-center gap-3">
            <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-rose-500/15 text-rose-400">
              <Trash2 class="h-4 w-4" />
            </div>
            <h3 class="truncate text-sm font-semibold text-white">{{ title }}</h3>
          </div>
          <button
            type="button"
            aria-label="Close modal"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-800 hover:text-white"
            @click="emit('close')"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <form class="contents" @submit.prevent="handleConfirm">
          <div class="space-y-3 px-5 py-4 text-xs leading-relaxed text-slate-300">
            <p>Are you sure you want to delete this item?</p>
            <p class="break-words rounded-lg border border-slate-800 bg-slate-950/70 px-3 py-2 font-semibold text-white">
              {{ itemName }}
            </p>
            <p class="text-slate-400">
              {{ description || 'This action cannot be undone.' }}
            </p>

            <div v-if="requiresTyping" class="space-y-1.5 pt-1">
              <label class="block text-[11px] text-slate-300">
                {{ confirmInputLabel || 'Type the confirmation value below to continue' }}:
                <code class="ml-1 rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[11px] font-semibold text-rose-300">{{ requireConfirmText }}</code>
              </label>
              <input
                ref="inputEl"
                v-model="typedText"
                type="text"
                autocomplete="off"
                spellcheck="false"
                :placeholder="requireConfirmText"
                class="h-9 w-full rounded-lg border border-slate-700 bg-slate-950 px-3 font-mono text-xs text-white placeholder:text-slate-600 focus:border-rose-400 focus:outline-none"
              />
            </div>
          </div>

          <div class="flex items-center justify-end gap-2 border-t border-slate-800 px-5 py-3.5">
            <button
              type="button"
              class="inline-flex h-9 items-center justify-center rounded-lg border border-slate-700 bg-slate-800 px-4 text-xs font-medium leading-none text-slate-200 transition hover:bg-slate-700"
              @click="emit('close')"
            >
              Cancel
            </button>
            <button
              type="submit"
              :disabled="!canConfirm"
              class="inline-flex h-9 items-center justify-center rounded-lg bg-rose-600 px-4 text-xs font-semibold leading-none text-white transition hover:bg-rose-500 disabled:opacity-40"
            >
              {{ confirmLabel || 'Delete' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </Teleport>
</template>
