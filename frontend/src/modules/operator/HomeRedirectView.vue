<script setup lang="ts">
import { watchEffect } from 'vue'
import { useRouter } from 'vue-router'
import { Server, Shield } from 'lucide-vue-next'
import { useSiteStore } from '../../stores/sites'
import { useAuthStore } from '../../stores/auth'

const siteStore = useSiteStore()
const auth = useAuthStore()
const router = useRouter()

watchEffect(() => {
  if (!siteStore.loading && siteStore.sites.length > 0) {
    void router.replace(`/sites/${siteStore.sites[0].id}/overview`)
  }
})
</script>

<template>
  <div class="mx-auto mt-20 max-w-md rounded-2xl border border-slate-800 bg-slate-900/60 p-8 text-center">
    <Server class="mx-auto h-10 w-10 text-slate-600" />
    <h2 class="mt-4 text-lg font-semibold text-white">No Sites Available</h2>
    <p class="mt-2 text-sm text-slate-400">
      You do not have access to any sites yet. Contact a platform administrator to assign site membership.
    </p>
    <RouterLink
      v-if="auth.isPlatformAdmin"
      to="/admin/sites"
      class="mt-6 inline-flex items-center gap-2 rounded-lg bg-indigo-500 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-400"
    >
      <Shield class="h-4 w-4" />
      Onboard First Site in Admin Console
    </RouterLink>
  </div>
</template>
