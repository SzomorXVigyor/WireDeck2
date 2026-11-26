<script setup lang="ts">
import { computed, ref } from 'vue'
import { GripVertical, Maximize2, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import type { RegisterMapping, TelemetrySample, TileSize, WebviewWidget } from '../../types/webview'
import {
  nextTileSize,
  resolveDashboardLayout,
  resolveTileSize,
  tileSizeShortLabel,
  tileSlotClass,
} from '../../utils/tiles'
import WidgetTileRenderer from '../widgets/WidgetTileRenderer.vue'

const props = defineProps<{
  widgets: WebviewWidget[]
  siteId?: string
  layout?: Record<string, unknown>
  mappingsById: Record<string, RegisterMapping>
  samples?: Record<string, TelemetrySample>
  canWrite: boolean
  editing: boolean
}>()

const emit = defineEmits<{
  write: [mappingId: string, value: number, pulseMs?: number]
  add: []
  edit: [widget: WebviewWidget]
  delete: [widget: WebviewWidget]
  resize: [widget: WebviewWidget, nextSize: TileSize]
  reorder: [orderedWidgets: WebviewWidget[]]
}>()

const resolvedLayout = computed(() => resolveDashboardLayout(props.layout))
const gridClass = computed(() => `wd-density-${resolvedLayout.value.density}`)

// --- Live Drag-to-Reorder ---
const draggedId = ref<string | null>(null)
const dragList = ref<WebviewWidget[] | null>(null)

const renderedWidgets = computed(() => dragList.value ?? props.widgets)

function onDragStart(e: DragEvent, widgetId: string) {
  if (!props.editing) return
  draggedId.value = widgetId
  dragList.value = [...props.widgets]
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', widgetId)
  }
}

function onDragOver(targetId: string) {
  if (!props.editing || !draggedId.value || draggedId.value === targetId || !dragList.value) return
  const fromIdx = dragList.value.findIndex((w) => w.id === draggedId.value)
  const toIdx = dragList.value.findIndex((w) => w.id === targetId)
  if (fromIdx === -1 || toIdx === -1) return
  const next = [...dragList.value]
  const [moved] = next.splice(fromIdx, 1)
  if (!moved) return
  next.splice(toIdx, 0, moved)
  dragList.value = next
}

function finishDrag(commit: boolean) {
  const finalOrder = dragList.value
  draggedId.value = null
  dragList.value = null
  if (!commit || !finalOrder) return
  const changed =
    finalOrder.length === props.widgets.length &&
    finalOrder.some((w, i) => w.id !== props.widgets[i]?.id)
  if (changed) {
    emit('reorder', finalOrder)
  }
}

function cycleSize(w: WebviewWidget) {
  const cur = resolveTileSize(w)
  emit('resize', w, nextTileSize(cur))
}
</script>

<template>
  <div class="wd-tile-canvas">
    <!-- Live Mode Grid -->
    <div v-if="!editing" class="wd-tile-grid" :class="gridClass">
      <div
        v-for="w in widgets"
        :key="w.id"
        class="wd-tile-slot"
        :class="tileSlotClass(resolveTileSize(w))"
      >
        <WidgetTileRenderer
          :widget="w"
          :site-id="siteId"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :mappings-by-id="mappingsById"
          :sample="w.registerMappingId && samples ? samples[w.registerMappingId] : undefined"
          :samples="samples"
          :can-write="canWrite"
          @write="(mid, val, pulse) => emit('write', mid, val, pulse)"
        />
      </div>

      <div
        v-if="widgets.length === 0"
        class="col-span-full flex flex-col items-center justify-center rounded-xl border border-dashed border-slate-800 bg-slate-900/35 px-6 py-14 text-center text-xs text-slate-500"
      >
        This dashboard has no tiles yet.
      </div>
    </div>

    <!-- WYSIWYG Edit Mode Grid -->
    <div
      v-else
      class="wd-tile-grid"
      :class="[gridClass, draggedId ? 'cursor-grabbing select-none' : '']"
    >
      <div
        v-for="w in renderedWidgets"
        :key="w.id"
        draggable="true"
        class="wd-tile-slot group relative transition-all duration-150"
        :class="[
          tileSlotClass(resolveTileSize(w)),
          draggedId === w.id ? 'scale-[0.98] opacity-35 cursor-grabbing' : 'cursor-grab',
        ]"
        @dragstart="onDragStart($event, w.id)"
        @dragover.prevent="onDragOver(w.id)"
        @drop.prevent="finishDrag(true)"
        @dragend="finishDrag(true)"
        @click="emit('edit', w)"
      >
        <!-- Exact End-Result Tile Preview -->
        <WidgetTileRenderer
          class="pointer-events-none"
          :widget="w"
          :site-id="siteId"
          :mapping="w.registerMappingId ? mappingsById[w.registerMappingId] : undefined"
          :mappings-by-id="mappingsById"
          :sample="w.registerMappingId && samples ? samples[w.registerMappingId] : undefined"
          :samples="samples"
          :can-write="canWrite"
          :preview="true"
        />

        <!-- Interactive Edit Frame Overlay -->
        <div
          class="pointer-events-none absolute inset-0 z-10 rounded-lg border-2 border-transparent transition-all group-hover:border-amber-400/75 group-hover:bg-amber-500/[0.04] group-focus-within:border-amber-400/75"
        />

        <!-- Drag Grip Hint (Top-Left) -->
        <div
          class="pointer-events-none absolute top-2 left-2 z-20 flex h-6 w-6 items-center justify-center rounded-md bg-slate-950/75 text-slate-300 opacity-75 backdrop-blur-xs transition group-hover:bg-amber-500 group-hover:text-slate-950 group-hover:opacity-100 sm:opacity-0 sm:group-hover:opacity-100"
          title="Drag to reorder"
        >
          <GripVertical class="h-3.5 w-3.5" />
        </div>

        <!-- Quick Action Bar (Top-Right: Resize, Edit, Delete) -->
        <div
          class="absolute top-2 right-2 z-20 flex items-center gap-1 rounded-lg border border-slate-700/90 bg-slate-950/90 p-0.5 shadow-lg backdrop-blur-xs transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
          @click.stop
        >
          <button
            type="button"
            :title="`Cycle tile size (currently ${tileSizeShortLabel(resolveTileSize(w))})`"
            :aria-label="`Cycle tile size, currently ${tileSizeShortLabel(resolveTileSize(w))}`"
            class="inline-flex h-7 items-center gap-1 rounded-md px-1.5 font-mono text-[10px] font-semibold text-amber-300 transition hover:bg-amber-500/20"
            @click="cycleSize(w)"
          >
            <Maximize2 class="h-3 w-3" />
            <span>{{ tileSizeShortLabel(resolveTileSize(w)) }}</span>
          </button>

          <button
            type="button"
            title="Edit tile"
            aria-label="Edit tile"
            class="flex h-7 w-7 items-center justify-center rounded-md text-sky-300 transition hover:bg-sky-500/20"
            @click="emit('edit', w)"
          >
            <Pencil class="h-3.5 w-3.5" />
          </button>

          <button
            type="button"
            title="Remove tile"
            aria-label="Remove tile"
            class="flex h-7 w-7 items-center justify-center rounded-md text-rose-300 transition hover:bg-rose-500/20"
            @click="emit('delete', w)"
          >
            <Trash2 class="h-3.5 w-3.5" />
          </button>
        </div>

        <!-- Subtle Bottom Edit Hint on Desktop Hover -->
        <div
          class="pointer-events-none absolute inset-x-0 bottom-2 z-20 hidden justify-center opacity-0 transition-opacity group-hover:opacity-100 sm:flex"
        >
          <span class="rounded-full border border-amber-500/40 bg-slate-950/85 px-2.5 py-0.5 text-[10px] font-medium text-amber-200 shadow">
            Click to configure - Drag to move
          </span>
        </div>
      </div>

      <!-- Inline Ghost Tile: Add New Tile -->
      <button
        type="button"
        class="wd-tile-slot wd-tile-wide flex h-full w-full flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-slate-700/80 bg-slate-900/35 px-4 py-6 text-slate-400 transition hover:border-amber-400/75 hover:bg-amber-500/[0.06] hover:text-amber-200"
        @click="emit('add')"
      >
        <span class="flex h-10 w-10 items-center justify-center rounded-full border border-current/30 bg-slate-900/60">
          <Plus class="h-5 w-5" />
        </span>
        <span class="text-xs font-semibold">Add Tile</span>
      </button>
    </div>
  </div>
</template>
