<script setup lang="ts">
import { computed } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { PAGE_SIZE_OPTIONS } from '../composables/usePagination'
import CustomSelect, { type SelectOption } from './CustomSelect.vue'

const props = withDefaults(
  defineProps<{
    page: number
    pageSize: number
    totalItems: number
    pageSizeOptions?: readonly number[]
    compact?: boolean
  }>(),
  {
    pageSizeOptions: () => PAGE_SIZE_OPTIONS,
    compact: false,
  },
)

const emit = defineEmits<{
  'update:page': [value: number]
  'update:pageSize': [value: number]
}>()

const totalPages = computed(() => Math.max(1, Math.ceil(props.totalItems / props.pageSize)))
const rangeStart = computed(() => (props.totalItems === 0 ? 0 : (props.page - 1) * props.pageSize + 1))
const rangeEnd = computed(() => Math.min(props.totalItems, props.page * props.pageSize))

const visiblePages = computed<(number | 'ellipsis')[]>(() => {
  const total = totalPages.value
  const current = props.page
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1)
  }
  const pages: (number | 'ellipsis')[] = [1]
  const start = Math.max(2, current - 1)
  const end = Math.min(total - 1, current + 1)
  if (start > 2) pages.push('ellipsis')
  for (let p = start; p <= end; p++) pages.push(p)
  if (end < total - 1) pages.push('ellipsis')
  pages.push(total)
  return pages
})

const sizeSelectOptions = computed<SelectOption<number>[]>(() =>
  props.pageSizeOptions.map((opt) => ({ value: opt, label: String(opt) })),
)

function onSizeChange(nextVal: string | number) {
  const next = Number(nextVal)
  if (Number.isFinite(next) && next > 0) {
    emit('update:pageSize', next)
    emit('update:page', 1)
  }
}

function go(next: number) {
  const clamped = Math.max(1, Math.min(totalPages.value, next))
  if (clamped !== props.page) {
    emit('update:page', clamped)
  }
}
</script>

<template>
  <div
    class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 border-t border-slate-800 px-3.5 py-2.5 text-xs text-slate-400 sm:px-4"
    :class="compact ? 'px-2.5 py-2' : ''"
  >
    <div class="text-[11px] tabular-nums text-slate-400">
      <template v-if="totalItems > 0">
        Showing <span class="font-medium text-slate-200">{{ rangeStart }}-{{ rangeEnd }}</span> of
        <span class="font-medium text-slate-200">{{ totalItems }}</span>
      </template>
      <template v-else>0 items</template>
    </div>

    <div class="flex flex-wrap items-center gap-2.5 sm:gap-3">
      <div class="inline-flex items-center gap-1.5 text-[11px] text-slate-400">
        <span>Rows</span>
        <CustomSelect
          :model-value="pageSize"
          :options="sizeSelectOptions"
          size="sm"
          :menu-min-width="76"
          aria-label="Rows per page"
          trigger-class="w-16"
          @change="onSizeChange"
        />
      </div>

      <div class="inline-flex items-center gap-1">
        <button
          type="button"
          :disabled="page <= 1"
          title="Previous page"
          aria-label="Previous page"
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-700 bg-slate-900 text-slate-300 transition hover:bg-slate-800 hover:text-white disabled:cursor-not-allowed disabled:opacity-40 sm:h-7 sm:w-7"
          @click="go(page - 1)"
        >
          <ChevronLeft class="h-3.5 w-3.5" />
        </button>

        <!-- Compact page readout on mobile or when compact prop is enabled -->
        <span
          class="px-1.5 font-mono text-[11px] tabular-nums text-slate-300"
          :class="compact ? 'inline' : 'inline sm:hidden'"
        >
          {{ page }} / {{ totalPages }}
        </span>

        <!-- Numbered page pills on tablet/desktop -->
        <div v-if="!compact" class="hidden items-center gap-1 sm:inline-flex">
          <template v-for="(item, idx) in visiblePages" :key="idx">
            <span v-if="item === 'ellipsis'" class="px-1 text-slate-500">...</span>
            <button
              v-else
              type="button"
              class="inline-flex h-7 min-w-[28px] items-center justify-center rounded-md border px-2 text-xs font-medium tabular-nums transition"
              :class="
                item === page
                  ? 'border-sky-500/60 bg-sky-500/20 text-sky-200'
                  : 'border-slate-800 bg-slate-900 text-slate-400 hover:bg-slate-800 hover:text-white'
              "
              @click="go(item)"
            >
              {{ item }}
            </button>
          </template>
        </div>

        <button
          type="button"
          :disabled="page >= totalPages"
          title="Next page"
          aria-label="Next page"
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-700 bg-slate-900 text-slate-300 transition hover:bg-slate-800 hover:text-white disabled:cursor-not-allowed disabled:opacity-40 sm:h-7 sm:w-7"
          @click="go(page + 1)"
        >
          <ChevronRight class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  </div>
</template>
