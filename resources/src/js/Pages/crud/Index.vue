<script setup lang="ts">
defineOptions({ name: 'CrudIndexPage' })
import { computed, ref } from 'vue'
import { router, usePage } from '@inertiajs/vue3'
import { t } from '@/i18n'
import AppHead from '@/components/layout/AppHead.vue'
import DataGrid from '@/components/datagrid/DataGrid.vue'
import type { ApiResponse } from '@/composables/useDataGrid'
import { recordListHref } from '@/composables/useRecordNavigation'
import { crudRecordHref, type CrudResource } from '@/crud'
const props = defineProps<{ resource: CrudResource; data: ApiResponse }>()
const page = usePage()
const mutating = ref(false)
const error = computed(() => (page.props.errors as Record<string, string> | undefined)?.form)
function source() {
  return recordListHref(props.resource.basePath, window.location.pathname + window.location.search)
}
function create() {
  if (mutating.value) return
  router.get(`${props.resource.basePath}/create`, { returnTo: source() })
}
function action(key: string, row: Record<string, unknown>) {
  if (mutating.value) return
  const id = String(row.id)
  if (key === 'view' || key === 'edit') {
    router.get(crudRecordHref(props.resource.basePath, id, key === 'edit', source()))
    return
  }
  const command = props.resource.actions.find((item) => item.key === key)
  if (key !== 'delete' && !command) return
  if (!window.confirm(t(command?.confirmation || 'Вы уверены, что хотите удалить запись?'))) return
  const path = `${props.resource.basePath}/records/${encodeURIComponent(id)}`
  const suffix =
    source() === props.resource.basePath ? '' : `?${new URLSearchParams({ returnTo: source() })}`
  const data = { _revision: String(row._revision) }
  const onStart = () => {
    mutating.value = true
  }
  const onFinish = () => {
    mutating.value = false
  }
  if (key === 'delete') router.delete(path + suffix, { data, onStart, onFinish })
  else
    router.post(`${path}/actions/${encodeURIComponent(key)}${suffix}`, data, { onStart, onFinish })
}
</script>
<template>
  <AppHead :title="resource.title" />
  <p v-if="error" role="alert" class="mb-4 text-error">{{ t(error) }}</p>
  <div :inert="mutating || undefined" :aria-busy="mutating">
    <DataGrid
      navigation-mode="inertia"
      :api-url="resource.basePath"
      :initial-data="data"
      @action-create="create"
      @action-row="action"
    />
  </div>
</template>
