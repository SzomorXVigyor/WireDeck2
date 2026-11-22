<script setup lang="ts">
import { ref } from 'vue'
import { Upload, AlertTriangle } from 'lucide-vue-next'

const props = defineProps<{
  modelValue: string
  overrideSubnets: string
  needsNarrowing: boolean
  warnings?: string[]
}>()

const emit = defineEmits<{
  'update:modelValue': [val: string]
  'update:overrideSubnets': [val: string]
}>()

const fileInput = ref<HTMLInputElement | null>(null)

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    if (typeof reader.result === 'string') {
      emit('update:modelValue', reader.result)
    }
  }
  reader.readAsText(file)
}
</script>

<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <label class="text-xs font-medium uppercase tracking-wider text-slate-400">
        WireGuard Client Config (.conf)
      </label>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md border border-slate-700 bg-slate-800 px-2.5 py-1 text-xs text-slate-200 hover:bg-slate-700"
        @click="fileInput?.click()"
      >
        <Upload class="h-3.5 w-3.5" />
        Load .conf file
      </button>
      <input ref="fileInput" type="file" accept=".conf,.txt" class="hidden" @change="onFileChange" />
    </div>

    <textarea
      :value="props.modelValue"
      rows="8"
      placeholder="[Interface]&#10;PrivateKey = ...&#10;Address = 10.200.0.2/32&#10;&#10;[Peer]&#10;PublicKey = ...&#10;Endpoint = site.example.com:51820&#10;AllowedIPs = 192.168.10.0/24"
      class="w-full rounded-lg border border-slate-700 bg-slate-950 p-3 font-mono text-xs text-slate-200 placeholder-slate-600 focus:border-sky-500 focus:outline-none"
      @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
    />

    <div
      v-if="props.needsNarrowing"
      class="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3.5 text-sm text-amber-200 space-y-2"
    >
      <div class="flex items-start gap-2">
        <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
        <div>
          <p class="font-medium text-amber-100">Catch-all AllowedIPs (0.0.0.0/0) detected</p>
          <p class="mt-0.5 text-xs text-amber-200/80">
            A catch-all route would break the runner container's network. Enter the site's actual LAN
            subnets below (comma-separated) to narrow <code class="font-mono">AllowedIPs</code>
            automatically before saving.
          </p>
        </div>
      </div>
      <input
        :value="props.overrideSubnets"
        type="text"
        placeholder="e.g. 192.168.10.0/24, 10.200.1.0/24"
        class="w-full rounded-md border border-amber-500/50 bg-slate-950 px-3 py-2 font-mono text-xs text-slate-100 placeholder-slate-500 focus:border-amber-400 focus:outline-none"
        @input="emit('update:overrideSubnets', ($event.target as HTMLInputElement).value)"
      />
    </div>

    <div v-if="props.warnings && props.warnings.length" class="rounded-lg border border-sky-500/30 bg-sky-500/10 p-3 text-xs text-sky-200">
      <p class="font-medium mb-1">Normalization Notes:</p>
      <ul class="list-disc pl-4 space-y-0.5">
        <li v-for="(w, i) in props.warnings" :key="i">{{ w }}</li>
      </ul>
    </div>
  </div>
</template>
