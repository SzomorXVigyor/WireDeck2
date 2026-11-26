<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'

export interface SelectOption<T extends string | number = string> {
  value: T
  label: string
  description?: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    modelValue: string | number
    options: readonly SelectOption<string | number>[]
    placeholder?: string
    disabled?: boolean
    invalid?: boolean
    ariaLabel?: string
    size?: 'sm' | 'md'
    triggerClass?: string
    menuMinWidth?: number
  }>(),
  {
    placeholder: 'Select...',
    disabled: false,
    invalid: false,
    ariaLabel: undefined,
    size: 'md',
    triggerClass: '',
    menuMinWidth: 0,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: any]
  change: [value: any]
}>()

const open = ref(false)
const triggerRef = ref<HTMLButtonElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const activeIndex = ref(-1)
const menuStyle = ref<Record<string, string>>({})

const selectedOption = computed(() => props.options.find((o) => o.value === props.modelValue) ?? null)

async function updatePosition() {
  await nextTick()
  const trigger = triggerRef.value
  const menu = menuRef.value
  if (!trigger || !menu) return

  const rect = trigger.getBoundingClientRect()
  const vw = window.innerWidth
  const vh = window.innerHeight
  const pad = 10
  const width = Math.max(rect.width, props.menuMinWidth || 0)
  const clampedWidth = Math.min(width, vw - pad * 2)

  let left = rect.left
  if (left + clampedWidth > vw - pad) {
    left = Math.max(pad, vw - pad - clampedWidth)
  }

  const menuHeight = Math.min(menu.scrollHeight || 220, 260)
  const spaceBelow = vh - rect.bottom - pad
  const spaceAbove = rect.top - pad
  let top = rect.bottom + 4
  if (spaceBelow < menuHeight && spaceAbove > spaceBelow) {
    top = Math.max(pad, rect.top - menuHeight - 4)
  }

  menuStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    width: `${Math.round(clampedWidth)}px`,
  }
}

function toggle() {
  if (props.disabled) return
  if (open.value) {
    open.value = false
    return
  }
  open.value = true
  activeIndex.value = props.options.findIndex((o) => o.value === props.modelValue)
  void updatePosition()
}

function selectOption(opt: SelectOption<string | number>) {
  if (opt.disabled) return
  emit('update:modelValue', opt.value)
  emit('change', opt.value)
  open.value = false
  triggerRef.value?.focus()
}

function onTriggerKeydown(e: KeyboardEvent) {
  if (props.disabled) return
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    if (!open.value) {
      open.value = true
      activeIndex.value = Math.max(
        0,
        props.options.findIndex((o) => o.value === props.modelValue),
      )
      void updatePosition()
      return
    }
    if (e.key === 'Enter' || e.key === ' ') {
      const opt = props.options[activeIndex.value]
      if (opt) selectOption(opt)
      return
    }
    const dir = e.key === 'ArrowDown' ? 1 : -1
    const len = props.options.length
    if (len === 0) return
    let idx = activeIndex.value
    for (let step = 0; step < len; step++) {
      idx = (idx + dir + len) % len
      if (!props.options[idx]?.disabled) {
        activeIndex.value = idx
        break
      }
    }
  } else if (e.key === 'Escape' && open.value) {
    e.preventDefault()
    e.stopPropagation()
    open.value = false
  }
}

function onGlobalMouseDown(e: MouseEvent) {
  if (!open.value) return
  const target = e.target as Node | null
  if (triggerRef.value?.contains(target) || menuRef.value?.contains(target)) return
  open.value = false
}

function onGlobalKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) {
    e.stopPropagation()
    open.value = false
    triggerRef.value?.focus()
  }
}

function onScrollOrResize() {
  if (open.value) void updatePosition()
}

watch(
  () => props.disabled,
  (isDisabled) => {
    if (isDisabled) open.value = false
  },
)

onMounted(() => {
  document.addEventListener('mousedown', onGlobalMouseDown)
  document.addEventListener('keydown', onGlobalKeydown, true)
  window.addEventListener('resize', onScrollOrResize)
  window.addEventListener('scroll', onScrollOrResize, true)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', onGlobalMouseDown)
  document.removeEventListener('keydown', onGlobalKeydown, true)
  window.removeEventListener('resize', onScrollOrResize)
  window.removeEventListener('scroll', onScrollOrResize, true)
})
</script>

<template>
  <div class="relative min-w-0">
    <button
      ref="triggerRef"
      type="button"
      role="combobox"
      :aria-label="ariaLabel"
      :aria-expanded="open"
      :disabled="disabled"
      class="flex w-full items-center justify-between gap-2 rounded-lg border bg-slate-950 text-left text-xs text-white transition focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
      :class="[
        size === 'sm' ? 'h-8 px-2.5 py-1 sm:h-7' : 'min-h-[36px] px-3 py-2',
        invalid
          ? 'border-rose-500 focus:border-rose-400'
          : open
            ? 'border-amber-500/80'
            : 'border-slate-700 hover:border-slate-600 focus:border-amber-500',
        triggerClass,
      ]"
      @click="toggle"
      @keydown="onTriggerKeydown"
    >
      <span class="min-w-0 truncate" :class="selectedOption ? 'text-white' : 'text-slate-500'">
        {{ selectedOption ? selectedOption.label : placeholder }}
      </span>
      <ChevronDown
        class="h-3.5 w-3.5 shrink-0 text-slate-400 transition-transform duration-150"
        :class="open ? 'rotate-180 text-amber-300' : ''"
      />
    </button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="menuRef"
        role="listbox"
        :style="menuStyle"
        class="fixed z-[75] max-h-[260px] overflow-y-auto rounded-xl border border-slate-700 bg-slate-900 py-1 shadow-2xl"
      >
        <button
          v-for="(opt, idx) in options"
          :key="String(opt.value)"
          type="button"
          role="option"
          :aria-selected="opt.value === modelValue"
          :disabled="opt.disabled"
          class="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-xs transition disabled:cursor-not-allowed disabled:opacity-40"
          :class="[
            opt.value === modelValue
              ? 'bg-amber-500/15 font-medium text-amber-200'
              : idx === activeIndex
                ? 'bg-slate-800 text-white'
                : 'text-slate-200 hover:bg-slate-800/80 hover:text-white',
          ]"
          @mouseenter="activeIndex = idx"
          @click="selectOption(opt)"
        >
          <div class="min-w-0">
            <p class="truncate">{{ opt.label }}</p>
            <p v-if="opt.description" class="truncate text-[10px] text-slate-400">{{ opt.description }}</p>
          </div>
          <Check v-if="opt.value === modelValue" class="h-3.5 w-3.5 shrink-0 text-amber-300" />
        </button>

        <p v-if="options.length === 0" class="px-3 py-3 text-center text-xs text-slate-500">
          No options available
        </p>
      </div>
    </Teleport>
  </div>
</template>
