<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { normalizeNumericText } from '../utils/precision'

const props = withDefaults(
  defineProps<{
    modelValue: string | number | null | undefined
    min?: number
    max?: number
    allowDecimal?: boolean
    allowNegative?: boolean
    required?: boolean
    placeholder?: string
    disabled?: boolean
    inputClass?: string
  }>(),
  {
    allowDecimal: false,
    allowNegative: false,
    required: false,
    placeholder: '',
    disabled: false,
    inputClass: '',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const rawText = ref('')
const touched = ref(false)

watch(
  () => props.modelValue,
  (val) => {
    const next = val === null || val === undefined ? '' : String(val)
    if (rawText.value !== next) {
      rawText.value = next
    }
  },
  { immediate: true },
)

const validationError = computed<string | null>(() => {
  const s = rawText.value.trim()
  if (s === '') {
    return props.required ? 'Value is required' : null
  }
  if (s === '-' || s === '.' || s === '-.') {
    return 'Enter a valid number'
  }
  const n = Number(s)
  if (!Number.isFinite(n)) {
    return 'Invalid number'
  }
  if (!props.allowNegative && n < 0) {
    return 'Must be 0 or greater'
  }
  if (!props.allowDecimal && !Number.isInteger(n)) {
    return 'Must be a whole number'
  }
  if (props.min !== undefined && n < props.min) {
    return `Must be at least ${props.min}`
  }
  if (props.max !== undefined && n > props.max) {
    return `Must be at most ${props.max}`
  }
  return null
})

function onKeyDown(e: KeyboardEvent) {
  if (
    e.ctrlKey ||
    e.metaKey ||
    e.key === 'Backspace' ||
    e.key === 'Delete' ||
    e.key === 'ArrowLeft' ||
    e.key === 'ArrowRight' ||
    e.key === 'Tab' ||
    e.key === 'Home' ||
    e.key === 'End' ||
    e.key === 'Enter'
  ) {
    return
  }
  if (e.key >= '0' && e.key <= '9') return
  if (e.key === '-') {
    const el = e.target as HTMLInputElement
    if (props.allowNegative && el.selectionStart === 0 && !el.value.startsWith('-')) return
    e.preventDefault()
    return
  }
  if (e.key === '.' || e.key === ',') {
    if (props.allowDecimal) return
    e.preventDefault()
    return
  }
  e.preventDefault()
}

function onInput(e: Event) {
  touched.value = true
  const el = e.target as HTMLInputElement
  const cleaned = normalizeNumericText(el.value, props.allowDecimal, props.allowNegative)
  rawText.value = cleaned
  if (el.value !== cleaned) {
    el.value = cleaned
  }
  emit('update:modelValue', cleaned)
}
</script>

<template>
  <div>
    <input
      :value="rawText"
      type="text"
      :inputmode="allowDecimal ? 'decimal' : 'numeric'"
      :placeholder="placeholder"
      :disabled="disabled"
      :required="required"
      class="w-full rounded-lg border px-3 py-2 font-mono text-xs text-white focus:outline-none disabled:opacity-50"
      :class="[
        inputClass || 'bg-slate-950',
        touched && validationError
          ? 'border-rose-500 focus:border-rose-400'
          : 'border-slate-700 focus:border-amber-500',
      ]"
      @keydown="onKeyDown"
      @input="onInput"
      @blur="touched = true"
    />
    <p v-if="touched && validationError" class="mt-1 text-[11px] text-rose-400">
      {{ validationError }}
    </p>
  </div>
</template>
