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
      props.mode === 'update' && !field.read
        ? null
        : Object.hasOwn(props.values, field.key)
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
function fieldError(key: string): string | undefined {
  // Track absent keys too, so later setError calls update the field.
  const error = errors.value[key]
  return Object.hasOwn(errors.value, key) && typeof error === 'string' ? error : undefined
}
const fieldValidity = ref<Record<string, boolean>>({})
const editedFields = ref(new Set<string>())
const formState = computed(() => ({
  errors: form.errors,
  processing: form.processing,
  isDirty:
    form.isDirty ||
    (props.mode === 'update' &&
      activeFields.value.some((field) => !field.read && editedFields.value.has(field.key))),
}))
function submit() {
  if (
    form.processing ||
    activeFields.value.some((field) => fieldValidity.value[field.key] === false)
  )
    return
  form.transform((data) => {
    if (props.mode !== 'update') return data.values
    const values = Object.fromEntries(
      activeFields.value
        .filter((field) => field.read || editedFields.value.has(field.key))
        .map((field) => [field.key, data.values[field.key]]),
    )
    return { ...values, _revision: draftRevision.value }
  })
  const options = {
    onSuccess: () => {
      draftRevision.value = props.record?.revision
      form.defaults({ values: initial() })
      form.reset()
      editedFields.value.clear()
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
    fieldValidity.value = {}
    editedFields.value.clear()
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
    :form="formState"
    :fields="activeFields.map((field) => field.key)"
    :return-href="listHref"
    :label="t(mode === 'create' ? 'Создание' : 'Редактирование')"
    @submit="submit"
  >
    <Field
      v-for="field in activeFields"
      :key="`${resource.key}:${record?.id ?? ''}:${field.key}`"
      :field="field"
      v-model="form.values[field.key]"
      :error="fieldError(field.key)"
      :disabled="form.processing"
      @update:model-value="editedFields.add(field.key)"
      @update:valid="fieldValidity[field.key] = $event"
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
