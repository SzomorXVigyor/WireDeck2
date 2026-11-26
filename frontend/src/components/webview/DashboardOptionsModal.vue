<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { X } from 'lucide-vue-next'
import type { DashboardLayoutConfig, TileDensity, WebviewDashboard } from '../../types/webview'
import { resolveDashboardLayout } from '../../utils/tiles'
import { firstError, validateIntegerRange, validateRequiredText } from '../../utils/validation'
import { useToastStore } from '../../stores/toast'
import NumericTextInput from '../NumericTextInput.vue'

export interface DashboardOptionsPayload {
  name: string
  pollIntervalMs: number
  isDefault: boolean
  layout: DashboardLayoutConfig
}

const props = defineProps<{
  open: boolean
  dashboard: WebviewDashboard | null
  defaultIfNew?: boolean
  submitLabel?: string
}>()

const emit = defineEmits<{
  close: []
  save: [payload: DashboardOptionsPayload]
}>()

const toast = useToastStore()

const name = ref('')
const pollIntervalMs = ref('1000')
const isDefault = ref(false)
const density = ref<TileDensity>('comfortable')
const submitted = ref(false)
const touched = ref<Record<string, boolean>>({})

const fieldErrors = computed(() => ({
  name: validateRequiredText(name.value, 'Dashboard name', { min: 1, max: 100 }),
  pollIntervalMs: validateIntegerRange(pollIntervalMs.value, 'Poll interval', 1000, 60000, { required: true }),
}))

function showErr(field: 'name' | 'pollIntervalMs'): string | null {
  return submitted.value || touched.value[field] ? fieldErrors.value[field] : null
}

function hydrate() {
  submitted.value = false
  touched.value = {}
  if (props.dashboard) {
    name.value = props.dashboard.name
    pollIntervalMs.value = String(props.dashboard.pollIntervalMs || 1000)
    isDefault.value = props.dashboard.isDefault
    const layout = resolveDashboardLayout(props.dashboard.layout)
    density.value = layout.density
  } else {
    name.value = ''
    pollIntervalMs.value = '1000'
    isDefault.value = Boolean(props.defaultIfNew)
    density.value = 'comfortable'
  }
}

watch(
  () => [props.open, props.dashboard] as const,
  ([isOpen]) => {
    if (isOpen) hydrate()
  },
  { immediate: true },
)

function handleSubmit() {
  submitted.value = true
  const err = firstError(fieldErrors.value)
  if (err) {
    toast.error(err, 'Validation Error')
    return
  }
  emit('save', {
    name: name.value.trim(),
    pollIntervalMs: Math.round(Number(pollIntervalMs.value)),
    isDefault: isDefault.value,
    layout: {
      density: density.value,
    },
  })
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
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-xs"
      @click.self="emit('close')"
    >
      <form
        novalidate
        class="max-h-[90dvh] w-full max-w-md overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl space-y-4 sm:p-6"
        @submit.prevent="handleSubmit"
      >
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-semibold text-white">
            {{ dashboard ? 'Dashboard Settings' : 'New Dashboard' }}
          </h3>
          <button
            type="button"
            aria-label="Close modal"
            class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
            @click="emit('close')"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div>
          <label class="mb-1 block text-xs text-slate-400">Dashboard Name</label>
          <input
            v-model="name"
            placeholder="Main Boiler"
            class="w-full rounded-lg border bg-slate-950 px-3 py-2 text-xs text-white focus:outline-none"
            :class="showErr('name') ? 'border-rose-500 focus:border-rose-400' : 'border-slate-700 focus:border-amber-500'"
            @blur="touched.name = true"
          />
          <p v-if="showErr('name')" class="mt-1 text-[11px] text-rose-400">{{ showErr('name') }}</p>
        </div>

        <div>
          <label class="mb-1 block text-xs text-slate-400">Poll Interval (ms, 1000-60000)</label>
          <NumericTextInput
            v-model="pollIntervalMs"
            :min="1000"
            :max="60000"
            required
            placeholder="1000"
            :external-error="showErr('pollIntervalMs')"
            @blur="touched.pollIntervalMs = true"
          />
          <p class="mt-1 text-[11px] text-slate-500">
            Applies up to 100 active registers per device; above 100, the minimum rate increases automatically to protect the device.
          </p>
        </div>

        <!-- Tile Scale / Density -->
        <div class="space-y-2 rounded-xl border border-slate-800 bg-slate-950/60 p-3.5">
          <p class="text-[11px] font-semibold uppercase tracking-wider text-slate-400">Tile Scale</p>
          <div class="grid grid-cols-3 gap-1.5">
            <button
              v-for="opt in [
                { value: 'compact', label: 'Compact', sub: 'High density' },
                { value: 'comfortable', label: 'Balanced', sub: 'Recommended' },
                { value: 'spacious', label: 'Spacious', sub: 'Large view' },
              ] as const"
              :key="opt.value"
              type="button"
              class="rounded-lg border px-2 py-2 text-center transition"
              :class="
                density === opt.value
                  ? 'border-amber-500/70 bg-amber-500/15 text-amber-200'
                  : 'border-slate-800 bg-slate-900/80 text-slate-300 hover:border-slate-700'
              "
              @click="density = opt.value"
            >
              <span class="block text-xs font-semibold">{{ opt.label }}</span>
              <span class="mt-0.5 block text-[10px] text-slate-400">{{ opt.sub }}</span>
            </button>
          </div>
        </div>

        <div class="space-y-1">
          <label class="inline-flex items-center gap-2 text-xs text-slate-300">
            <input v-model="isDefault" type="checkbox" /> Set as default dashboard
          </label>
          <p class="text-[11px] text-slate-500">
            Only one dashboard can be default per site. Enabling this replaces any previous default.
          </p>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button
            type="button"
            class="h-9 rounded-lg border border-slate-700 px-3.5 text-xs font-medium text-slate-300 hover:bg-slate-800"
            @click="emit('close')"
          >
            Cancel
          </button>
          <button
            type="submit"
            class="h-9 rounded-lg bg-amber-500 px-4 text-xs font-semibold text-slate-950 hover:bg-amber-400"
          >
            {{ submitLabel || 'Save' }}
          </button>
        </div>
      </form>
    </div>
  </Teleport>
</template>
