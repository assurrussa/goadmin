<template>
  <FormLayout ref="formLayout" :loading="form.processing" :spacing="spacing" @submit="handleSubmit">
    <slot />

    <div
      v-if="showSummary && summaryErrors.length"
      role="alert"
      class="rounded-md bg-error/10 border border-error/30 p-3 my-2 mx-4"
    >
      <slot name="summary" :errors="summaryErrors">
        <p class="mb-1 font-medium text-error">{{ t('Исправьте ошибки формы:') }}</p>
        <ul class="list-disc list-inside text-error text-sm">
          <li v-for="(e, i) in summaryErrors" :key="`${e.field}-${i}`">
            <strong>{{ e.field }}</strong
            >: {{ t(e.message) }}
          </li>
        </ul>
      </slot>
    </div>

    <slot name="actions" />
  </FormLayout>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { router, usePage } from '@inertiajs/vue3'
import FormLayout from '@/components/form/FormLayout.vue'

interface InertiaFormLike {
  errors: Record<string, string>
  processing: boolean
  isDirty?: boolean
}

interface Props {
  form: InertiaFormLike
  fields?: string[]
  spacing?: 'sm' | 'md' | 'lg'
  focusOnError?: boolean
  showSummary?: boolean
  warnUnsaved?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  fields: undefined,
  spacing: 'md',
  focusOnError: true,
  showSummary: true,
  warnUnsaved: false,
})

const emit = defineEmits<{ submit: [e: Event] }>()

const page = usePage()
const pageErrors = computed(() => (page.props.errors as Record<string, string>) || {})

// Helper: snake_case/PascalCase -> camelCase
const toCamel = (key: string) => {
  if (!key) return key
  if (key.includes('_')) {
    const parts = key.split('_').filter(Boolean)
    return (
      parts[0].toLowerCase() +
      parts
        .slice(1)
        .map((p) => (p ? p[0].toUpperCase() + p.slice(1) : ''))
        .join('')
    )
  }
  return key[0].toLowerCase() + key.slice(1)
}

const formLayout = ref<InstanceType<typeof FormLayout> | null>(null)
let unmounted = false
const focusErrors = computed(() =>
  props.form.errors && Object.keys(props.form.errors).length > 0
    ? props.form.errors
    : pageErrors.value,
)

// Inertia can deliver errors before processing ends and the fieldset is enabled.
const focusFirstError = async () => {
  if (!props.focusOnError || props.form.processing) return
  await nextTick()
  if (unmounted || !props.focusOnError || props.form.processing) return
  const rawKey = Object.keys(focusErrors.value)[0]
  const formElement = formLayout.value?.$el as HTMLFormElement | undefined
  if (!rawKey || !formElement?.isConnected) return
  const controls = Array.from(
    formElement.querySelectorAll<HTMLElement>(
      'input:not([type="hidden"]), select, textarea, button, a[href], [tabindex], [contenteditable="true"]',
    ),
  ).filter((element) => !element.matches(':disabled') && !element.closest('[hidden], [inert]'))
  for (const key of [rawKey, toCamel(rawKey)]) {
    const element =
      controls.find((control) => control.getAttribute('name') === key) ??
      controls.find((control) => control.id === key)
    if (element) {
      element.focus()
      break
    }
  }
}

watch(
  [focusErrors, () => props.form.processing, () => props.focusOnError],
  () => {
    void focusFirstError()
  },
  { immediate: true, deep: true },
)

// Summary errors: anything not in known fields (if provided)
type SummaryError = { field: string; message: string }
const summaryErrors = computed<SummaryError[]>(() => {
  const keys = props.fields

  // Prefer form.errors if present, else fall back to pageErrors
  const errs: Record<string, string> =
    props.form?.errors && Object.keys(props.form.errors).length > 0
      ? (props.form.errors as Record<string, string>)
      : Object.fromEntries(Object.entries(pageErrors.value).map(([k, v]) => [toCamel(k), v]))

  return Object.entries(errs)
    .filter(([k]) => !keys?.includes(k))
    .map(([k, v]) => ({ field: k, message: v }))
})

function handleSubmit(e: Event) {
  if (!props.form.processing) emit('submit', e)
}

function beforeUnload(event: BeforeUnloadEvent) {
  if (!props.warnUnsaved || !props.form.isDirty || props.form.processing) return
  event.preventDefault()
  event.returnValue = ''
}

let removeInertiaGuard: (() => void) | undefined
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload)
  if (props.warnUnsaved) {
    removeInertiaGuard = router.on('before', (event) => {
      if (!props.form.isDirty || props.form.processing || event.detail.visit.method !== 'get')
        return
      return window.confirm(t('Есть несохранённые изменения. Покинуть страницу?'))
    })
  }
})
onUnmounted(() => {
  unmounted = true
  window.removeEventListener('beforeunload', beforeUnload)
  removeInertiaGuard?.()
})
</script>
