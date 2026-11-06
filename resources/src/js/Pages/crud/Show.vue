<script setup lang="ts">
defineOptions({ name: 'CrudShowPage' })
import { computed, ref } from 'vue'
import { Link, router, usePage } from '@inertiajs/vue3'
import { t } from '@/i18n'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import AppButton from '@/components/ui/AppButton.vue'
import Field from '@/crud/Field.vue'
import { useConfirmation } from '@/composables/useConfirmation'
import {
  crudReturnHref,
  crudRecordHref,
  crudDisplay,
  crudValue,
  type CrudResource,
  type CrudRecord,
  type CrudPermissions,
} from '@/crud'
const props = defineProps<{
  resource: CrudResource
  record: CrudRecord
  permissions: CrudPermissions
}>()
const page = usePage()
const mutating = ref(false)
const confirming = ref(false)
const confirm = useConfirmation()
const listHref = computed(() => crudReturnHref(props.resource.basePath, page.url))
const error = computed(() => (page.props.errors as Record<string, string> | undefined)?.form)
async function run(key: string) {
  if (mutating.value || confirming.value) return
  const action = props.resource.actions.find((action) => action.key === key)
  if (key !== 'delete' && !action) return
  if (!props.record.actions?.includes(key)) return
  const basePath = props.resource.basePath
  const id = props.record.id
  const revision = props.record.revision
  const returnTo = listHref.value
  confirming.value = true
  try {
    await confirm(
      {
        message: action?.confirmation || 'Вы уверены, что хотите удалить запись?',
        confirmLabel: action?.label || 'Удалить',
        variant: key === 'delete' ? 'danger' : 'primary',
      },
      () => {
        if (
          mutating.value ||
          props.resource.basePath !== basePath ||
          props.record.id !== id ||
          props.record.revision !== revision
        )
          return
        const base = `${basePath}/records/${encodeURIComponent(id)}`
        const suffix = returnTo === basePath ? '' : `?${new URLSearchParams({ returnTo })}`
        const data = { _revision: revision }
        const onStart = () => {
          mutating.value = true
        }
        const onFinish = () => {
          mutating.value = false
        }
        if (key === 'delete') router.delete(base + suffix, { data, onStart, onFinish })
        else
          router.post(`${base}/actions/${encodeURIComponent(key)}${suffix}`, data, {
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
  <PageActionBar
    :back-href="listHref"
    :back-text="t('К списку')"
    :label="resource.title"
    :use-history="false"
  />
  <p v-if="error" role="alert" class="mb-4 text-error">{{ t(error) }}</p>
  <section class="space-y-6 rounded-xl border border-border-primary bg-card p-6">
    <dl class="grid gap-6 sm:grid-cols-2">
      <div v-for="field in resource.fields.filter((field) => field.read)" :key="field.key">
        <Field
          v-if="field.type === 'custom'"
          :field="field"
          :model-value="crudValue(record.data[field.key])"
          readonly
          disabled
        />
        <template v-else
          ><dt class="text-sm text-text-secondary">{{ t(field.label) }}</dt>
          <dd class="mt-1 whitespace-pre-wrap break-words">
            {{
              record.data[field.key] == null
                ? '—'
                : field.type === 'boolean'
                  ? t(record.data[field.key] ? 'Да' : 'Нет')
                  : field.type === 'select'
                    ? t(crudDisplay(field, record.data[field.key]))
                    : crudDisplay(field, record.data[field.key])
            }}
          </dd></template
        >
      </div>
    </dl>
    <div class="flex flex-wrap gap-3" :inert="mutating || undefined" :aria-busy="mutating">
      <AppButton v-if="record.actions?.includes('edit')" as-child
        ><Link :href="crudRecordHref(resource.basePath, record.id, true, listHref)">{{
          t('Редактировать')
        }}</Link></AppButton
      >
      <AppButton
        v-for="action in resource.actions.filter((action) => record.actions?.includes(action.key))"
        :key="action.key"
        variant="secondary"
        :disabled="mutating"
        @click="run(action.key)"
        >{{ t(action.label) }}</AppButton
      >
      <AppButton
        v-if="record.actions?.includes('delete')"
        variant="danger"
        :disabled="mutating"
        @click="run('delete')"
        >{{ t('Удалить') }}</AppButton
      >
    </div>
  </section>
</template>
