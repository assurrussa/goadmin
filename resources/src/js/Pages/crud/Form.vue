<script setup lang="ts">
defineOptions({ name: 'CrudFormPage' })
import { computed, onUnmounted, watch, ref } from 'vue'
import { useForm, usePage, router } from '@inertiajs/vue3'
import { t } from '@/i18n'
import AppHead from '@/components/layout/AppHead.vue'
import RecordEditForm from '@/components/form/RecordEditForm.vue'
import Field from '@/crud/Field.vue'
import AppButton from '@/components/ui/AppButton.vue'
import {
  crudReturnHref,
  crudValue,
  type CrudValue,
  type CrudResource,
  type CrudRecord,
} from '@/crud'
const props = defineProps<{
  resource: CrudResource
  record?: CrudRecord
  values: Record<string, unknown>
  mode: 'create' | 'update'
}>()
const page = usePage()
const listHref = computed(() => crudReturnHref(props.resource.basePath, page.url))
const activeFields = computed(() => props.resource.fields.filter((field) => field[props.mode]))
function initial(): Record<string, CrudValue> {
  return Object.fromEntries(
    activeFields.value.map((field) => [
      field.key,
      Object.hasOwn(props.values, field.key)
        ? crudValue(props.values[field.key])
        : field.type === 'boolean'
          ? false
          : field.type === 'number'
            ? 0
            : field.type === 'select' && field.required
              ? (field.options?.[0]?.value ?? '')
              : '',
    ]),
  )
}
const form = useForm({ values: initial() })
const draftRevision = ref(props.record?.revision)
const errors = computed(() => form.errors as Record<string, string>)
function submit() {
  if (form.processing) return
  form.transform((data) =>
    props.mode === 'update' ? { ...data.values, _revision: draftRevision.value } : data.values,
  )
  const options = {
    onSuccess: () => {
      draftRevision.value = props.record?.revision
      form.defaults({ values: initial() })
      form.reset()
    },
  }
  const suffix =
    listHref.value === props.resource.basePath
      ? ''
      : `?${new URLSearchParams({ returnTo: listHref.value })}`
  if (props.mode === 'create') form.post(props.resource.basePath + suffix, options)
  else if (props.record)
    form.put(
      `${props.resource.basePath}/records/${encodeURIComponent(props.record.id)}${suffix}`,
      options,
    )
}
watch(
  () => `${props.resource.key}:${props.record?.id ?? ''}`,
  () => {
    draftRevision.value = props.record?.revision
    form.defaults({ values: initial() })
    form.reset()
    form.clearErrors()
  },
)
function reload() {
  if (!props.record) return
  router.get(
    `${props.resource.basePath}/records/${encodeURIComponent(props.record.id)}/edit`,
    { returnTo: listHref.value },
    { preserveState: false },
  )
}
onUnmounted(() => form.cancel())
</script>
<template>
  <AppHead :title="resource.title" />
  <RecordEditForm
    :form="form"
    :fields="activeFields.map((field) => field.key)"
    :return-href="listHref"
    :label="t(mode === 'create' ? 'Создание' : 'Редактирование')"
    @submit="submit"
  >
    <Field
      v-for="field in activeFields"
      :key="field.key"
      :field="field"
      v-model="form.values[field.key]"
      :error="errors[field.key]"
      :disabled="form.processing"
    />
    <AppButton
      v-if="mode === 'update' && errors.form"
      variant="secondary"
      :disabled="form.processing"
      @click="reload"
      >{{ t('Загрузить актуальную запись') }}</AppButton
    >
  </RecordEditForm>
</template>
