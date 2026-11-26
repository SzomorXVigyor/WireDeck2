<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ChevronDown, Search, X } from 'lucide-vue-next'
import type { RegisterMapping, WebviewDevice } from '../types/webview'
import CustomSelect, { type SelectOption } from './CustomSelect.vue'

const props = withDefaults(
  defineProps<{
    modelValue: string
    devices: WebviewDevice[]
    mappings: RegisterMapping[]
    registerLabel?: string
    allowClear?: boolean
    placeholder?: string
    invalid?: boolean
  }>(),
  {
    registerLabel: 'Register',
    allowClear: true,
    placeholder: 'Search register...',
    invalid: false,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const selectedDeviceId = ref<string>('')
const searchQuery = ref('')
const dropdownOpen = ref(false)
const triggerBtnRef = ref<HTMLButtonElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)
const menuStyle = ref<Record<string, string>>({})

const deviceMap = computed<Record<string, WebviewDevice>>(() => {
  const out: Record<string, WebviewDevice> = {}
  for (const d of props.devices) out[d.id] = d
  return out
})

const selectedMapping = computed(() => props.mappings.find((m) => m.id === props.modelValue) ?? null)

watch(
  () => props.modelValue,
  (val) => {
    if (val) {
      const m = props.mappings.find((item) => item.id === val)
      if (m) {
        selectedDeviceId.value = m.deviceId
      }
    }
  },
  { immediate: true },
)

const deviceSelectOptions = computed<SelectOption<string>[]>(() => {
  const ids = new Set(props.mappings.map((m) => m.deviceId))
  const list = props.devices.filter((d) => ids.has(d.id))
  return [
    { value: '', label: 'All devices' },
    ...list.map((d) => ({ value: d.id, label: d.name })),
  ]
})

const filteredMappings = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return props.mappings.filter((m) => {
    if (selectedDeviceId.value && m.deviceId !== selectedDeviceId.value) {
      return false
    }
    if (!q) return true
    const a = m.protocolAttributes
    const devName = deviceMap.value[m.deviceId]?.name || ''
    const haystack = `${m.name} ${devName} ${a?.functionCode || ''} ${a?.registerAddress ?? ''} ${a?.dataType || ''} unit ${a?.slaveAddress ?? ''}`.toLowerCase()
    return haystack.includes(q)
  })
})

function onDeviceChange() {
  if (props.modelValue && selectedDeviceId.value) {
    const cur = props.mappings.find((m) => m.id === props.modelValue)
    if (cur && cur.deviceId !== selectedDeviceId.value) {
      emit('update:modelValue', '')
    }
  }
}

async function updateMenuPosition() {
  await nextTick()
  const trigger = triggerBtnRef.value
  const menu = menuRef.value
  if (!trigger || !menu) return

  const rect = trigger.getBoundingClientRect()
  const vw = window.innerWidth
  const vh = window.innerHeight
  const pad = 10
  const width = Math.min(Math.max(rect.width, 240), vw - pad * 2)

  let left = rect.left
  if (left + width > vw - pad) {
    left = Math.max(pad, vw - pad - width)
  }

  const maxH = 264
  const spaceBelow = vh - rect.bottom - pad
  const spaceAbove = rect.top - pad
  let top = rect.bottom + 4
  if (spaceBelow < maxH && spaceAbove > spaceBelow) {
    const actualH = Math.min(menu.scrollHeight || maxH, maxH)
    top = Math.max(pad, rect.top - actualH - 4)
  }

  menuStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    width: `${Math.round(width)}px`,
  }
}

async function toggleDropdown() {
  dropdownOpen.value = !dropdownOpen.value
  if (dropdownOpen.value) {
    await updateMenuPosition()
    searchInputRef.value?.focus()
  }
}

function selectMapping(m: RegisterMapping) {
  selectedDeviceId.value = m.deviceId
  emit('update:modelValue', m.id)
  searchQuery.value = ''
  dropdownOpen.value = false
}

function clearSelection() {
  emit('update:modelValue', '')
  searchQuery.value = ''
}

function metaText(m: RegisterMapping): string {
  const a = m.protocolAttributes
  const access = a?.accessMode === 'READ_WRITE' ? 'R/W' : 'R'
  const devName = !selectedDeviceId.value ? `${deviceMap.value[m.deviceId]?.name || ''} | ` : ''
  return `${devName}unit ${a?.slaveAddress ?? 1} | ${a?.functionCode ?? ''}@${a?.registerAddress ?? 0} [${a?.dataType ?? ''}] (${access})`
}

function onDocMouseDown(e: MouseEvent) {
  if (!dropdownOpen.value) return
  const target = e.target as Node | null
  if (triggerBtnRef.value?.contains(target) || menuRef.value?.contains(target)) return
  dropdownOpen.value = false
}

function onDocKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && dropdownOpen.value) {
    e.stopPropagation()
    dropdownOpen.value = false
    triggerBtnRef.value?.focus()
  }
}

function onScrollOrResize() {
  if (dropdownOpen.value) void updateMenuPosition()
}

onMounted(() => {
  document.addEventListener('mousedown', onDocMouseDown)
  document.addEventListener('keydown', onDocKeydown, true)
  window.addEventListener('resize', onScrollOrResize)
  window.addEventListener('scroll', onScrollOrResize, true)
})
onUnmounted(() => {
  document.removeEventListener('mousedown', onDocMouseDown)
  document.removeEventListener('keydown', onDocKeydown, true)
  window.removeEventListener('resize', onScrollOrResize)
  window.removeEventListener('scroll', onScrollOrResize, true)
})
</script>

<template>
  <div class="grid grid-cols-1 gap-3 sm:grid-cols-[4fr_6fr]">
    <!-- Separate Device filter (Unified CustomSelect) -->
    <div>
      <label class="mb-1 block text-xs text-slate-400">Device</label>
      <CustomSelect
        v-model="selectedDeviceId"
        :options="deviceSelectOptions"
        aria-label="Filter by device"
        @change="onDeviceChange"
      />
    </div>

    <!-- Searchable Register dropdown (Teleported above modal footers) -->
    <div class="relative">
      <label class="mb-1 block text-xs text-slate-400">{{ registerLabel }}</label>
      <button
        ref="triggerBtnRef"
        type="button"
        class="flex min-h-[36px] w-full items-center justify-between gap-2 rounded-lg border bg-slate-950 px-3 py-2 text-left text-xs text-white transition focus:outline-none"
        :class="
          invalid
            ? 'border-rose-500 focus:border-rose-400'
            : dropdownOpen
              ? 'border-amber-500/80'
              : 'border-slate-700 hover:border-slate-600 focus:border-amber-500'
        "
        @click="toggleDropdown"
      >
        <span class="truncate" :class="selectedMapping ? 'text-white' : 'text-slate-500'">
          {{ selectedMapping ? selectedMapping.name : 'Select register...' }}
        </span>
        <span class="flex shrink-0 items-center gap-1">
          <span
            v-if="allowClear && selectedMapping"
            class="rounded p-0.5 text-slate-400 hover:bg-slate-800 hover:text-white"
            title="Clear register"
            @click.stop="clearSelection"
          >
            <X class="h-3 w-3" />
          </span>
          <ChevronDown
            class="h-3.5 w-3.5 text-slate-400 transition-transform duration-150"
            :class="dropdownOpen ? 'rotate-180 text-amber-300' : ''"
          />
        </span>
      </button>

      <Teleport to="body">
        <div
          v-if="dropdownOpen"
          ref="menuRef"
          :style="menuStyle"
          class="fixed z-[75] overflow-hidden rounded-xl border border-slate-700 bg-slate-900 shadow-2xl"
        >
          <div class="flex items-center gap-2 border-b border-slate-800 px-3 py-2">
            <Search class="h-3.5 w-3.5 shrink-0 text-slate-500" />
            <input
              ref="searchInputRef"
              v-model="searchQuery"
              type="text"
              :placeholder="placeholder"
              class="w-full bg-transparent text-xs text-white placeholder-slate-500 focus:outline-none"
              @click.stop
            />
          </div>
          <div class="max-h-52 overflow-y-auto py-1">
            <button
              v-for="m in filteredMappings"
              :key="m.id"
              type="button"
              class="flex w-full flex-col px-3 py-2 text-left transition hover:bg-slate-800"
              :class="m.id === modelValue ? 'bg-amber-500/15' : ''"
              @click="selectMapping(m)"
            >
              <span class="truncate text-xs font-medium text-white">{{ m.name }}</span>
              <span class="truncate font-mono text-[10px] text-slate-400">{{ metaText(m) }}</span>
            </button>
            <p v-if="filteredMappings.length === 0" class="px-3 py-4 text-center text-xs text-slate-500">
              No matching registers
            </p>
          </div>
        </div>
      </Teleport>
    </div>
  </div>
</template>
