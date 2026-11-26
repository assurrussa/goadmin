<template>
  <AppHead :title="props.title" />
  <div class="space-y-4">
    <section
      class="rounded-xl border border-border-primary bg-card px-5 py-4 text-sm"
      :aria-label="t('Как работают роли')"
    >
      <h2 class="font-semibold text-text-primary">{{ t('Роли складываются по разрешениям') }}</h2>
      <p class="mt-1 max-w-4xl leading-6 text-text-secondary">
        {{
          t(
            'Если назначено несколько ролей, их разрешения складываются. Супер-администратор получает весь объединённый каталог прав. Область доступа остальных ролей определяется назначенными разрешениями.',
          )
        }}
      </p>
    </section>
    <DataGrid
      navigation-mode="inertia"
      ref="dataGridRef"
      :api-url="apiUrl"
      :initial-data="apiResponseData"
      :resolve-row-action-href="rowActionHref"
      @action-table="handleActionClick"
      @action-create="handleCreate"
      @action-row="handleRowAction"
    />
  </div>
</template>

<script setup lang="ts">
import { useConfirmation } from '@/composables/useConfirmation'
const confirm = useConfirmation()
import { t } from '@/i18n'

import { computed, ref } from 'vue'
import { router } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import DataGrid from '@/components/datagrid/DataGrid.vue'
import type { ApiResponse } from '@/composables/useDataGrid'
import { recordActionHref } from '@/composables/useRecordNavigation'

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

const apiUrl = computed(() => props.data?.config?.routePath || '/roles')

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

function handleActionClick(actionId: string) {
  switch (actionId) {
    case 'refresh':
      router.visit(`${apiUrl.value}/refresh`, {
        method: 'post',
      })
      break
    default:
      console.warn('Unknown table action:', actionId)
  }
}

function handleCreate() {
  router.get(`${apiUrl.value}/create`)
}

function rowActionHref(key: string, row: Record<string, unknown>, source: string) {
  return recordActionHref('/roles', row.idExact || row.id, key === 'edit', source)
}

async function handleRowAction(actionKey: string, item: Record<string, unknown>) {
  const roleId = item.idExact || item.id
  if (!roleId) {
    return
  }

  switch (actionKey) {
    case 'delete': {
      const target = `${apiUrl.value}/${roleId}`
      await confirm(
        {
          message: 'Удалить роль #{id}?',
          params: { id: String(roleId) },
          confirmLabel: 'Удалить',
          variant: 'danger',
        },
        () => {
          router.delete(target)
        },
      )
      break
    }
    default:
      console.warn('Unknown row action:', actionKey, item)
  }
}
</script>
