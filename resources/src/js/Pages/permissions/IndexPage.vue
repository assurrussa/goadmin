<template>
  <AppHead :title="props.title" />
  <DataGrid
    navigation-mode="inertia"
    ref="dataGridRef"
    :api-url="apiUrl"
    :initial-data="apiResponseData"
    @action-table="handleAction"
    @action-row="handleDataAction"
  />
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { router } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import DataGrid from '@/components/datagrid/DataGrid.vue'
import type { ApiResponse } from '@/composables/useDataGrid'

const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  data: {
    type: Object,
    default: () => ({}),
  },
})

const dataGridRef = ref<InstanceType<typeof DataGrid> | null>(null)

const apiUrl = computed(() => {
  return props.data?.config?.routePath || '/permissions'
})

const isValidApiResponse = (data: unknown): data is ApiResponse => {
  return !!(
    data &&
    typeof data === 'object' &&
    Array.isArray((data as Record<string, unknown>).data)
  )
}

const apiResponseData = computed(() => {
  if (isValidApiResponse(props.data)) {
    return props.data as ApiResponse
  }
  return null
})

function handleAction(actionId: string) {
  switch (actionId) {
    case 'refresh':
      router.visit(`${apiUrl.value}/refresh`, {
        method: 'post',
      })
      break
    default:
      console.warn('Unknown action:', actionId)
  }
}
function handleDataAction(actionKey: string, item: Record<string, unknown>) {
  switch (actionKey) {
    case 'sync':
      if (confirm('Синхронизировать разрешения с кодовой базой?')) {
        router.visit(`${apiUrl.value}/sync`, {
          method: 'post',
        })
      }
      break
    default:
      console.warn('Unknown action:', actionKey, 'for item:', item)
  }
}
</script>
