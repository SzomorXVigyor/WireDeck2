import { computed, ref, watch, type Ref } from 'vue'

export const PAGE_SIZE_OPTIONS = [25, 50, 100] as const

export function usePagination<T>(items: Ref<T[]> | (() => T[]), initialSize = 25) {
  const page = ref(1)
  const pageSize = ref(initialSize)

  const list = computed<T[]>(() => (typeof items === 'function' ? items() : items.value))
  const totalItems = computed(() => list.value.length)
  const totalPages = computed(() => Math.max(1, Math.ceil(totalItems.value / pageSize.value)))

  watch([totalItems, pageSize], () => {
    if (page.value > totalPages.value) {
      page.value = totalPages.value
    }
    if (page.value < 1) {
      page.value = 1
    }
  })

  const startIndex = computed(() => (page.value - 1) * pageSize.value)
  const paginatedItems = computed(() => list.value.slice(startIndex.value, startIndex.value + pageSize.value))

  function resetPage() {
    page.value = 1
  }

  return {
    page,
    pageSize,
    totalItems,
    totalPages,
    startIndex,
    paginatedItems,
    resetPage,
  }
}
