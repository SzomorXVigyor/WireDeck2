<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Pencil, X } from 'lucide-vue-next'
import type { RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import {
  clampPrecision,
  effectiveDisplayBounds,
  encodePrecision,
  formatPrecisionValue,
  isSignedDataType,
  normalizeNumericText,
  validateDisplayNumber,
} from '../../utils/precision'

const props = defineProps<{
  open: boolean
  widget: WebviewWidget
  mapping?: RegisterMapping
  sample?: TelemetrySample
}>()

const emit = defineEmits<{
  close: []
  submit: [rawValue: number]
}>()

const inputText = ref('')
const touched = ref(false)
const inputEl = ref<HTMLInputElement | null>(null)

const dataType = computed(() => props.mapping?.protocolAttributes?.dataType)
const precision = computed(() => clampPrecision(props.widget.extra?.precision ?? 0))
const prefix = computed(() => String(props.widget.extra?.prefix || ''))
const unit = computed(() => String(props.widget.extra?.unit || ''))
const cardMin = computed(() => {
  const v = props.widget.extra?.min
  return v !== undefined && v !== null && v !== '' ? Number(v) : undefined
})
const cardMax = computed(() => {
  const v = props.widget.extra?.max
  return v !== undefined && v !== null && v !== '' ? Number(v) : undefined
})

const allowDecimal = computed(() => precision.value > 0 || dataType.value === 'FLOAT32')
const allowNegative = computed(() => isSignedDataType(dataType.value))

const bounds = computed(() => effectiveDisplayBounds(dataType.value, precision.value, cardMin.value, cardMax.value))
const currentFormatted = computed(() => formatPrecisionValue(props.sample?.value, precision.value))

const validation = computed(() =>
  validateDisplayNumber(inputText.value, dataType.value, precision.value, cardMin.value, cardMax.value),
)

watch(
  () => props.open,
  async (isOpen) => {
    if (isOpen) {
      touched.value = false
      const cur = props.sample?.value
      inputText.value = cur !== null && cur !== undefined && Number.isFinite(cur)
        ? formatPrecisionValue(cur, precision.value)
        : ''
      await nextTick()
      inputEl.value?.focus()
      inputEl.value?.select()
    }
  },
)

function onInput(e: Event) {
  touched.value = true
  const target = e.target as HTMLInputElement
  const cleaned = normalizeNumericText(target.value, allowDecimal.value, allowNegative.value)
  inputText.value = cleaned
  if (target.value !== cleaned) {
    target.value = cleaned
  }
}

function handleSave() {
  touched.value = true
  if (!validation.value.valid || validation.value.value === null) return
  const rawValue = encodePrecision(validation.value.value, precision.value, dataType.value)
  emit('submit', rawValue)
  emit('close')
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
      @click.self="emit('close')"
    >
      <form
        class="w-full max-w-sm overflow-hidden rounded-2xl border border-slate-700 bg-slate-900 text-slate-100 shadow-2xl"
        @submit.prevent="handleSave"
      >
        <div class="flex items-center justify-between border-b border-slate-800 px-5 py-4">
          <div class="flex items-center gap-3 min-w-0">
            <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-sky-500/15 text-sky-400">
              <Pencil class="h-4 w-4" />
            </div>
            <div class="min-w-0">
              <h3 class="truncate text-sm font-semibold text-white">Set Value</h3>
              <p class="truncate text-xs text-slate-400">{{ widget.name }}</p>
            </div>
          </div>
          <button
            type="button"
            class="rounded-lg p-1 text-slate-400 hover:bg-slate-800 hover:text-white"
            @click="emit('close')"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="space-y-4 px-5 py-4">
          <div>
            <p class="text-[11px] font-medium uppercase tracking-wider text-slate-500">Current Value</p>
            <div class="mt-1 flex items-baseline gap-1 font-mono text-2xl font-bold text-white">
              <span v-if="prefix" class="text-slate-300">{{ prefix }}</span>
              <span>{{ currentFormatted }}</span>
              <span v-if="unit" class="text-base font-medium text-slate-400">{{ unit }}</span>
            </div>
          </div>

          <div class="border-t border-slate-800 pt-4">
            <label class="block text-xs font-medium text-slate-300">New Value</label>
            <div class="mt-1.5 flex items-center gap-2">
              <span v-if="prefix" class="text-sm text-slate-400">{{ prefix }}</span>
              <input
                ref="inputEl"
                :value="inputText"
                type="text"
                inputmode="decimal"
                placeholder="Enter value"
                class="w-full rounded-lg border bg-slate-950 px-3 py-2 font-mono text-sm text-white focus:outline-none"
                :class="
                  touched && !validation.valid
                    ? 'border-rose-500 focus:border-rose-400'
                    : 'border-slate-700 focus:border-sky-500'
                "
                @input="onInput"
              />
              <span v-if="unit" class="shrink-0 text-xs font-medium text-slate-400">{{ unit }}</span>
            </div>
            <p v-if="touched && validation.error" class="mt-1.5 text-xs text-rose-400">
              {{ validation.error }}
            </p>
            <p class="mt-1.5 text-[11px] text-slate-500">
              Allowed range: {{ bounds.min }} to {{ bounds.max }} ({{ dataType || 'UINT16' }}, 10^{{ -precision }})
            </p>
          </div>
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
            type="submit"
            :disabled="!validation.valid"
            class="rounded-lg bg-sky-500 px-4 py-1.5 text-xs font-semibold text-slate-950 hover:bg-sky-400 disabled:cursor-not-allowed disabled:opacity-40"
          >
            Set Value
          </button>
        </div>
      </form>
    </div>
  </Teleport>
</template>
