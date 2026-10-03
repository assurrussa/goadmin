<template>
  <AppHead :title="props.title" />
  <DataGrid
    ref="dataGridRef"
    :api-url="apiUrl"
    :initial-data="apiResponseData"
    @action-table="handleActionClick"
    @action-create="handleDataCreate"
    @action-row="handleDataAction"
  />
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import DataGrid from '@/components/datagrid/DataGrid.vue'
import type { ApiResponse } from '@/composables/useDataGrid'
import AppHead from '@/components/layout/AppHead.vue'
import { router } from '@inertiajs/vue3'
import { downloadUserExport } from '@/services/userExport'
import { useNotifications } from '@/composables/useNotifications'

const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  data: {
    type: [Object],
    default: undefined,
  },
})

const dataGridRef = ref<InstanceType<typeof DataGrid> | null>(null)

const apiUrl = computed(() => {
  return props.data?.config?.routePath || ''
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

const notifications = useNotifications()
let exportController: AbortController | null = null
onUnmounted(() => exportController?.abort())

async function exportUsers() {
  if (exportController) return
  const controller = new AbortController()
  exportController = controller
  try {
    const truncated = await downloadUserExport(
      apiUrl.value,
      window.location.href,
      controller.signal,
    )
    if (truncated && !controller.signal.aborted) {
      notifications.warning(
        'Экспорт содержит первые 10 000 пользователей. Уточните фильтры для полного результата.',
      )
    }
  } catch (error) {
    if (!controller.signal.aborted) {
      notifications.error(
        error instanceof Error ? error.message : 'Не удалось экспортировать пользователей.',
      )
    }
  } finally {
    if (exportController === controller) exportController = null
  }
}

function handleActionClick(actionId: string, val: string | number | null) {
  switch (actionId) {
    case 'export':
      void exportUsers()
      break
    case 'refresh':
      router.visit(`${apiUrl.value}/refresh`, {
        method: 'post',
        onFinish: (visit) => {
          if (visit.completed && dataGridRef.value) {
            dataGridRef.value.refreshData()
          }
        },
      })
      break
    default:
      console.warn('Unknown action:', actionId, val)
  }
}

function handleDataCreate() {
  router.get(`${apiUrl.value}/create`)
}

function handleDataAction(actionKey: string, item: Record<string, unknown>) {
  switch (actionKey) {
    case 'view':
      router.get(`${apiUrl.value}/${item.id}`)
      break
    case 'edit':
      router.get(`${apiUrl.value}/${item.id}/edit`)
      break
    case 'delete':
      if (confirm(`Вы уверены, что хотите удалить запись #${item.id}?`)) {
        router.delete(`${apiUrl.value}/${item.id}`, {
          onFinish: (visit) => {
            if (visit.completed && dataGridRef.value) {
              dataGridRef.value.refreshData()
            }
          },
        })
      }
      break
    case 'restore':
      if (confirm(`Вы уверены, что хотите восстановить запись #${item.id}?`)) {
        router.post(
          `${apiUrl.value}/${item.id}/restore`,
          {},
          {
            onFinish: (visit) => {
              if (visit.completed && dataGridRef.value) {
                dataGridRef.value.refreshData()
              }
            },
          },
        )
      }
      break
    default:
      console.warn('Unknown action:', actionKey, 'for item:', item)
  }
}
</script>
