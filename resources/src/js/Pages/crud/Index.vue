<script setup lang="ts">
defineOptions({ name: 'CrudIndexPage' })
import { computed, ref } from 'vue'
import { router, usePage } from '@inertiajs/vue3'
import { t } from '@/i18n'
import AppHead from '@/components/layout/AppHead.vue'
import ActionDialog from '@/crud/ActionDialog.vue'
import DataGrid from '@/components/datagrid/DataGrid.vue'
import type { ApiResponse } from '@/composables/useDataGrid'
import { recordListHref } from '@/composables/useRecordNavigation'
import { crudRecordHref, type CrudResource } from '@/crud'
import { useConfirmation } from '@/composables/useConfirmation'
const props = defineProps<{ resource: CrudResource; data: ApiResponse }>()
const page = usePage()
const grid = ref<InstanceType<typeof DataGrid> | null>(null)
const actionDialog = ref<InstanceType<typeof ActionDialog>>()
const mutating = ref(false)
const confirming = ref(false)
const confirm = useConfirmation()
const error = computed(() => (page.props.errors as Record<string, string> | undefined)?.form)
function source() {
  return recordListHref(props.resource.basePath, grid.value?.listLocation ?? null)
}
function rowActionHref(key: string, row: Record<string, unknown>, listLocation: string) {
  if (key !== 'view' && key !== 'edit') return
  if (typeof row.id !== 'string' && typeof row.id !== 'number') return
  return crudRecordHref(props.resource.basePath, String(row.id), key === 'edit', listLocation)
}
function create() {
  if (mutating.value) return
  router.get(`${props.resource.basePath}/create`, { returnTo: source() })
}
function tableAction(key: string) {
  if (key === 'refresh' && !mutating.value && !confirming.value) grid.value?.refreshData()
}
async function action(key: string, row: Record<string, unknown>) {
  if (mutating.value || confirming.value) return
  const id = String(row.id)
  const command = props.resource.actions.find((item) => item.key === key)
  if (key !== 'delete' && !command) return
  const basePath = props.resource.basePath
  const revision = row._revision
  if (typeof revision !== 'string' || !revision) return
  const returnTo = source()
  if (command?.params?.length) {
    actionDialog.value?.open(command, { id, revision, data: row }, returnTo)
    return
  }
  confirming.value = true
  try {
    await confirm(
      {
        title: command?.description ? command.confirmation || command.label : undefined,
        message:
          command?.description ||
          command?.confirmation ||
          (key === 'delete'
            ? 'Вы уверены, что хотите удалить запись?'
            : 'Вы уверены, что хотите выполнить действие?'),
        confirmLabel: command?.label || 'Удалить',
        variant: key === 'delete' ? 'danger' : 'primary',
      },
      () => {
        if (mutating.value || props.resource.basePath !== basePath) return
        const path = `${basePath}/records/${encodeURIComponent(id)}`
        const suffix = returnTo === basePath ? '' : `?${new URLSearchParams({ returnTo })}`
        const data = { _revision: revision }
        const onStart = () => {
          mutating.value = true
        }
        const onFinish = () => {
          mutating.value = false
        }
        if (key === 'delete') router.delete(path + suffix, { data, onStart, onFinish })
        else
          router.post(`${path}/actions/${encodeURIComponent(key)}${suffix}`, data, {
            onStart,
            onFinish,
          })
      },
    )
  } finally {
    confirming.value = false
  }
}
</script>
<template>
  <AppHead :title="resource.title" />
  <ActionDialog
    v-if="resource.actions.some((item) => item.params?.length)"
    ref="actionDialog"
    :resource="resource"
  />
  <p v-if="resource.description" class="mb-4 text-sm text-text-secondary">
    {{ t(resource.description) }}
  </p>
  <p v-if="error" role="alert" class="mb-4 text-error">{{ t(error) }}</p>
  <div :inert="mutating || undefined" :aria-busy="mutating">
    <DataGrid
      ref="grid"
      navigation-mode="inertia"
      :api-url="resource.basePath"
      :initial-data="data"
      :resolve-row-action-href="rowActionHref"
      @action-create="create"
      @action-table="tableAction"
      @action-row="action"
    />
  </div>
</template>
