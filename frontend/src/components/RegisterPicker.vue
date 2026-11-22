<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ChevronDown, Search, X } from 'lucide-vue-next'
import type { RegisterMapping, WebviewDevice } from '../types/webview'

const props = withDefaults(
  defineProps<{
    modelValue: string
    devices: WebviewDevice[]
    mappings: RegisterMapping[]
    registerLabel?: string
    allowClear?: boolean
    placeholder?: string
  }>(),
  {
    registerLabel: 'Register',
    allowClear: true,
    placeholder: 'Search register...',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const selectedDeviceId = ref<string>('')
const searchQuery = ref('')
const dropdownOpen = ref(false)
const pickerRoot = ref<HTMLElement | null>(null)

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

const deviceOptions = computed(() => {
  const ids = new Set(props.mappings.map((m) => m.deviceId))
  return props.devices.filter((d) => ids.has(d.id))
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

function onDocClick(e: MouseEvent) {
  if (!dropdownOpen.value) return
  if (pickerRoot.value && !pickerRoot.value.contains(e.target as Node)) {
    dropdownOpen.value = false
  }
}

onMounted(() => document.addEventListener('click', onDocClick))
onUnmounted(() => document.removeEventListener('click', onDocClick))
</script>

<template>
  <div ref="pickerRoot" class="grid grid-cols-1 gap-3 sm:grid-cols-[4fr_6fr]">
    <!-- Separate Device filter -->
    <div>
      <label class="mb-1 block text-xs text-slate-400">Device</label>
      <select
        v-model="selectedDeviceId"
        class="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white focus:border-amber-500 focus:outline-none"
        @change="onDeviceChange"
      >
        <option value="">All devices</option>
        <option v-for="d in deviceOptions" :key="d.id" :value="d.id">{{ d.name }}</option>
      </select>
    </div>

    <!-- Searchable Register dropdown -->
    <div class="relative">
      <label class="mb-1 block text-xs text-slate-400">{{ registerLabel }}</label>
      <button
        type="button"
        class="flex w-full items-center justify-between gap-2 rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-left text-xs text-white hover:border-slate-600 focus:border-amber-500 focus:outline-none"
        @click="dropdownOpen = !dropdownOpen"
      >
        <span class="truncate" :class="selectedMapping ? 'text-white' : 'text-slate-500'">
          {{ selectedMapping ? selectedMapping.name : 'Select register...' }}
        </span>
        <span class="flex items-center gap-1 shrink-0">
          <span
            v-if="allowClear && selectedMapping"
            class="rounded p-0.5 text-slate-400 hover:bg-slate-800 hover:text-white"
            title="Clear register"
            @click.stop="clearSelection"
          >
            <X class="h-3 w-3" />
          </span>
          <ChevronDown class="h-3.5 w-3.5 text-slate-400" />
        </span>
      </button>

      <div
        v-if="dropdownOpen"
        class="absolute left-0 right-0 top-full z-40 mt-1 overflow-hidden rounded-xl border border-slate-700 bg-slate-900 shadow-2xl"
      >
        <div class="flex items-center gap-2 border-b border-slate-800 px-3 py-2">
          <Search class="h-3.5 w-3.5 shrink-0 text-slate-500" />
          <input
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
    </div>
  </div>
</template>
