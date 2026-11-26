<script setup lang="ts">
import type { RegisterMapping, TelemetrySample, WebviewWidget } from '../../types/webview'
import WidgetDisplay from './WidgetDisplay.vue'
import WidgetLamp from './WidgetLamp.vue'
import WidgetSwitch from './WidgetSwitch.vue'
import WidgetButton from './WidgetButton.vue'
import WidgetNumberInput from './WidgetNumberInput.vue'
import WidgetGraph from './WidgetGraph.vue'

defineProps<{
  widget: WebviewWidget
  siteId?: string
  mapping?: RegisterMapping
  mappingsById?: Record<string, RegisterMapping>
  sample?: TelemetrySample
  samples?: Record<string, TelemetrySample>
  canWrite: boolean
  preview?: boolean
  previewState?: boolean
  previewTriggered?: boolean
}>()

const emit = defineEmits<{
  write: [mappingId: string, value: number, pulseMs?: number]
}>()
</script>

<template>
  <WidgetDisplay
    v-if="widget.widgetType === 'display'"
    :widget="widget"
    :mapping="mapping"
    :sample="sample"
    :preview="preview"
  />
  <WidgetLamp
    v-else-if="widget.widgetType === 'lamp'"
    :widget="widget"
    :mapping="mapping"
    :sample="sample"
    :preview="preview"
    :preview-state="previewState"
  />
  <WidgetSwitch
    v-else-if="widget.widgetType === 'switch'"
    :widget="widget"
    :mapping="mapping"
    :sample="sample"
    :can-write="canWrite"
    :preview="preview"
    :preview-state="previewState"
    @write="(mid, val) => emit('write', mid, val)"
  />
  <WidgetButton
    v-else-if="widget.widgetType === 'button'"
    :widget="widget"
    :mapping="mapping"
    :sample="sample"
    :can-write="canWrite"
    :preview="preview"
    :preview-triggered="previewTriggered"
    @write="(mid, val, pulse) => emit('write', mid, val, pulse)"
  />
  <WidgetNumberInput
    v-else-if="widget.widgetType === 'number_input'"
    :widget="widget"
    :mapping="mapping"
    :sample="sample"
    :can-write="canWrite"
    :preview="preview"
    @write="(mid, val) => emit('write', mid, val)"
  />
  <WidgetGraph
    v-else-if="widget.widgetType === 'graph'"
    :widget="widget"
    :site-id="siteId"
    :mappings-by-id="mappingsById"
    :samples="samples"
    :preview="preview"
  />
</template>
