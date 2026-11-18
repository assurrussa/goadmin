<script setup lang="ts">
defineOptions({ name: 'CrudFormPage' })
import { computed, nextTick, onUnmounted, reactive, ref, watch } from 'vue'
import { useForm, usePage, router } from '@inertiajs/vue3'
import { v4 as uuid } from 'uuid'
import { t } from '@/i18n'
import AppHead from '@/components/layout/AppHead.vue'
import RecordEditForm from '@/components/form/RecordEditForm.vue'
import Field from '@/crud/Field.vue'
import AppButton from '@/components/ui/AppButton.vue'
import {
  crudDisplay,
  crudRecordHref,
  crudReturnHref,
  crudValue,
  type CrudField,
  type CrudResource,
  type CrudRecord,
  type CrudValue,
} from '@/crud'

type Values = Record<string, CrudValue>
type Choice = 'mine' | 'latest'
type Dispute = { mine: CrudValue; latest: CrudValue; choice?: Choice }
type HeldSecret = { value: CrudValue; reapply: boolean }
type Flash = {
  success?: string
  error?: string
  recordEditAttempt?: string
  crudEditConflict?: string
}
const conflictNotice = 'Запись изменилась. Проверьте изменения и сохраните запись снова.'

const props = defineProps<{
  resource: CrudResource
  record?: CrudRecord
  values: Record<string, unknown>
  mode: 'create' | 'update'
}>()
const page = usePage()
const listHref = computed(() => crudReturnHref(props.resource.basePath, page.url))
const activeFields = computed(() => props.resource.fields.filter((field) => field[props.mode]))

function editableValues(values = props.values): Values {
  return Object.fromEntries(
    activeFields.value.map((field) => [
      field.key,
      props.mode === 'update' && !field.read
        ? null
        : Object.hasOwn(values, field.key)
          ? crudValue(values[field.key])
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

function editSnapshot(values = props.values) {
  return {
    values: editableValues(values),
    complete: activeFields.value.every((field) => !field.read || Object.hasOwn(values, field.key)),
  }
}

let baseline = editSnapshot()
let draftRevision = props.record?.revision
const form = useForm({ values: baseline.values })
const errors = computed(() => form.errors as Record<string, string>)
function fieldError(key: string): string | undefined {
  // Track absent keys too, so later setError calls update the field.
  const error = errors.value[key]
  return Object.hasOwn(errors.value, key) && typeof error === 'string' ? error : undefined
}

function setFormError(message: string) {
  const errors: Record<string, string> = { form: message }
  form.setError(errors)
}

const fieldValidity = ref<Record<string, boolean>>({})
const editedSecrets = reactive(new Set<string>())
const disputedFields = reactive(new Map<string, Dispute>())
const heldSecrets = reactive(new Map<string, HeldSecret>())
const fieldsWithChoices = computed(() =>
  activeFields.value.map((field) => ({
    field,
    dispute: disputedFields.get(field.key),
    heldSecret: heldSecrets.get(field.key),
  })),
)
const needsReload = ref(false)
const activeAttempt = ref<string>()
let cancelUpdate: (() => void) | undefined

const formState = computed(() => ({
  errors: form.errors,
  processing: form.processing || !!activeAttempt.value,
  isDirty: form.isDirty || editedSecrets.size > 0 || heldSecrets.size > 0,
}))
const submitDisabled = computed(
  () =>
    needsReload.value ||
    Array.from(heldSecrets.values()).some(({ reapply }) => !reapply) ||
    Array.from(disputedFields.values()).some(({ choice }) => !choice) ||
    activeFields.value.some((field) => fieldValidity.value[field.key] === false),
)

function choose(key: string, dispute: Dispute, choice: Choice) {
  dispute.choice = choice
  form.values[key] = dispute[choice]
}

function displayChoice(field: CrudField, value: CrudValue) {
  if (value === null) return t('Не указано')
  if (value === '') return t('Пустая строка')
  return crudDisplay(field, value)
}

function replaceSecret(key: string) {
  heldSecrets.delete(key)
  editedSecrets.delete(key)
  form.values[key] = null
  delete fieldValidity.value[key]
}

function reconcile(latest: ReturnType<typeof editSnapshot>, revision: string) {
  // A host JSON marshaler may intentionally omit a readable field. A display
  // default cannot stand in for its value when comparing concurrent writes.
  if (!baseline.complete || !latest.complete) {
    needsReload.value = true
    setFormError('Не удалось сравнить все поля. Загрузите актуальную запись.')
    return
  }
  disputedFields.clear()
  for (const field of activeFields.value) {
    const { key } = field
    if (!field.read) {
      const value = editedSecrets.delete(key) ? form.values[key] : heldSecrets.get(key)?.value
      if (value !== undefined) {
        heldSecrets.set(key, { value, reapply: false })
        form.values[key] = null
        delete fieldValidity.value[key]
      }
      continue
    }

    const mine = form.values[key]!
    const theirs = latest.values[key]!
    if (mine === baseline.values[key]) form.values[key] = theirs
    else if (theirs !== baseline.values[key] && mine !== theirs) {
      disputedFields.set(key, { mine, latest: theirs })
    }
  }
  baseline = latest
  draftRevision = revision
  form.defaults({ values: baseline.values })
  setFormError(conflictNotice)
}

function ownerOf(pageProps: Record<string, unknown>): string {
  const actor = pageProps.authuser as { id?: string | number; idExact?: string } | undefined
  return String(actor?.idExact || actor?.id || '')
}
const identity = computed(() =>
  JSON.stringify([
    ownerOf(page.props),
    props.resource.key,
    props.resource.basePath,
    props.record?.id,
    props.mode,
  ]),
)
watch(identity, () => {
  cancelUpdate?.()
  activeAttempt.value = undefined
  cancelUpdate = undefined
  baseline = editSnapshot()
  draftRevision = props.record?.revision
  form.defaults({ values: baseline.values }).reset().clearErrors()
  fieldValidity.value = {}
  editedSecrets.clear()
  disputedFields.clear()
  heldSecrets.clear()
  needsReload.value = false
})

function submit(_event: Event, returnToList = false) {
  if (formState.value.processing || submitDisabled.value) return
  if (props.mode === 'create') {
    const suffix =
      listHref.value === props.resource.basePath
        ? ''
        : `?${new URLSearchParams({ returnTo: listHref.value })}`
    form
      .transform((data) => data.values)
      .post(props.resource.basePath + suffix, {
        onSuccess: () => form.defaults({ values: editableValues() }).reset(),
      })
  } else if (props.record) {
    update(props.record.id, returnToList)
  }
}

function update(id: string, returnToList: boolean) {
  const requestId = uuid()
  const owner = ownerOf(page.props)
  const currentIdentity = identity.value
  const component = page.component
  const submitted = { ...form.values }
  const submittedSecrets = new Set(editedSecrets)
  const values = Object.fromEntries(
    activeFields.value
      .filter((field) => field.read || editedSecrets.has(field.key) || heldSecrets.has(field.key))
      .map(({ key }) => [key, heldSecrets.get(key)?.value ?? submitted[key]]),
  )
  const current = () =>
    activeAttempt.value === requestId &&
    identity.value === currentIdentity &&
    page.component === component

  function acceptedResponse(responseComponent: string, responseProps: Record<string, unknown>) {
    const record = responseProps.record as Partial<CrudRecord> | undefined
    const resource = responseProps.resource as Partial<CrudResource> | undefined
    const flash = responseProps.flash as Flash | undefined
    const latest = responseProps.values
    if (
      responseComponent !== component ||
      ownerOf(responseProps) !== owner ||
      resource?.key !== props.resource.key ||
      resource?.basePath !== props.resource.basePath ||
      responseProps.mode !== 'update' ||
      record?.id !== id ||
      typeof record?.revision !== 'string' ||
      !record.revision ||
      !latest ||
      typeof latest !== 'object' ||
      Array.isArray(latest) ||
      flash?.recordEditAttempt !== requestId
    ) {
      return
    }
    return {
      flash,
      latest: editSnapshot(latest as Record<string, unknown>),
      revision: record.revision,
    }
  }

  function unconfirmed() {
    needsReload.value = true
    setFormError('Результат операции неизвестен. Обновите страницу перед повтором.')
  }

  let destination: string | undefined
  let receivedResult = false
  router.put(
    crudRecordHref(props.resource.basePath, id, false, listHref.value),
    {
      ...values,
      _revision: draftRevision ?? null,
    },
    {
      headers: {
        'X-GoAdmin-Record-Attempt': requestId,
        'X-GoAdmin-Record-ID': encodeURIComponent(id),
      },
      onCancelToken: (token) => {
        cancelUpdate = token.cancel
      },
      onStart: () => {
        activeAttempt.value = requestId
        disputedFields.clear()
        form.clearErrors()
      },
      onSuccess: (response) => {
        if (!current()) return
        receivedResult = true
        const accepted = acceptedResponse(response.component, response.props)
        if (!accepted?.flash.success || accepted.flash.error || accepted.flash.crudEditConflict) {
          unconfirmed()
          return
        }
        baseline = accepted.latest
        draftRevision = accepted.revision
        for (const { key, read } of activeFields.value) {
          if (read) {
            if (form.values[key] === submitted[key]) form.values[key] = baseline.values[key]!
          } else {
            if (heldSecrets.get(key)?.value === values[key]) heldSecrets.delete(key)
            if (submittedSecrets.has(key) && form.values[key] === submitted[key]) {
              editedSecrets.delete(key)
              form.values[key] = null
            }
          }
        }
        form.defaults({ values: baseline.values })
        form.values = { ...form.values }
        if (returnToList) destination = crudReturnHref(props.resource.basePath, response.url)
      },
      onError: (visitErrors) => {
        if (!current()) return
        receivedResult = true
        form.setError(visitErrors)
        const flash = page.props.flash as Flash | undefined
        if (flash?.crudEditConflict === '1') {
          const accepted = acceptedResponse(page.component, page.props)
          if (accepted && visitErrors.form) reconcile(accepted.latest, accepted.revision)
          else unconfirmed()
        } else if (visitErrors.form === 'Result unknown; check record before retrying') {
          needsReload.value = true
        }
      },
      onFinish: async (visit) => {
        await nextTick()
        if (activeAttempt.value !== requestId) return
        const stillCurrent = current()
        activeAttempt.value = undefined
        cancelUpdate = undefined
        if (!stillCurrent || visit.cancelled || visit.interrupted) return
        if (!receivedResult) unconfirmed()
        if (destination && owner && !formState.value.isDirty) {
          router.get(destination)
        }
      },
    },
  )
}

function reload() {
  if (!props.record) return
  router.get(
    crudRecordHref(props.resource.basePath, props.record.id, true, listHref.value),
    {},
    {
      preserveState: false,
    },
  )
}

onUnmounted(() => {
  activeAttempt.value = undefined
  cancelUpdate?.()
  form.cancel()
})
</script>

<template>
  <AppHead :title="resource.title" />
  <RecordEditForm
    :form="formState"
    :fields="activeFields.map((field) => field.key)"
    :return-href="listHref"
    :label="t(mode === 'create' ? 'Создание' : 'Редактирование')"
    :can-save-and-return="mode === 'update'"
    :submit-disabled="mode === 'update' && submitDisabled"
    @submit="submit"
  >
    <template
      v-for="{ field, dispute, heldSecret } in fieldsWithChoices"
      :key="`${identity}:${field.key}`"
    >
      <fieldset v-if="dispute" class="space-y-2 rounded-lg border border-border-primary p-3">
        <legend class="px-1 text-sm font-medium">{{ t(field.label) }}</legend>
        <label
          v-for="choice in ['mine', 'latest'] as const"
          :key="choice"
          class="flex items-start gap-2"
        >
          <input
            type="radio"
            :name="`crud-conflict:${field.key}`"
            :checked="dispute.choice === choice"
            class="mt-1 accent-primary"
            @change="choose(field.key, dispute, choice)"
          />
          <span class="min-w-0 text-sm">
            <span class="font-medium">{{
              t(choice === 'mine' ? 'Моё значение' : 'Актуальное значение') + ': '
            }}</span>
            <span class="whitespace-pre-wrap break-words">{{
              displayChoice(field, dispute[choice])
            }}</span>
          </span>
        </label>
      </fieldset>
      <fieldset
        v-else-if="heldSecret"
        class="space-y-2 rounded-lg border border-border-primary p-3"
      >
        <legend class="px-1 text-sm font-medium">{{ t(field.label) }}</legend>
        <p class="text-sm">
          {{
            t(
              !heldSecret.reapply
                ? 'Подтвердите повторное применение введённого значения.'
                : 'Введённое значение будет применено при сохранении.',
            )
          }}
        </p>
        <div class="flex flex-wrap gap-2">
          <AppButton
            v-if="!heldSecret.reapply"
            variant="secondary"
            @click="heldSecret.reapply = true"
          >
            {{ t('Повторно применить моё значение') }}
          </AppButton>
          <AppButton
            variant="secondary"
            :data-field-name="field.key"
            @click="replaceSecret(field.key)"
          >
            {{ t(field.required ? 'Ввести новое значение' : 'Пропустить поле') }}
          </AppButton>
        </div>
        <p v-if="fieldError(field.key)" class="text-sm text-error" role="alert">
          {{ t('Проверьте значение.') }}
        </p>
      </fieldset>
      <Field
        v-else
        :field="field"
        v-model="form.values[field.key]"
        :error="fieldError(field.key)"
        :disabled="formState.processing"
        @update:model-value="mode === 'update' && !field.read && editedSecrets.add(field.key)"
        @update:valid="fieldValidity[field.key] = $event"
      />
    </template>
    <AppButton
      v-if="mode === 'update' && errors.form && errors.form !== conflictNotice"
      variant="secondary"
      :disabled="formState.processing"
      @click="reload"
      >{{ t('Загрузить актуальную запись') }}</AppButton
    >
  </RecordEditForm>
</template>
