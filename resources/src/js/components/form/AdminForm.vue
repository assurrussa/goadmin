<template>
  <FormLayout :loading="form.processing" :spacing="spacing" @submit="handleSubmit">
    <slot />

    <div
      v-if="showSummary && summaryErrors.length"
      role="alert"
      class="rounded-md bg-error/10 border border-error/30 p-3 my-2 mx-4"
    >
      <slot name="summary" :errors="summaryErrors">
        <p class="mb-1 font-medium text-error">Исправьте ошибки формы:</p>
        <ul class="list-disc list-inside text-error text-sm">
          <li v-for="(e, i) in summaryErrors" :key="`${e.field}-${i}`">
            <strong>{{ e.field }}</strong
            >: {{ e.message }}
          </li>
        </ul>
      </slot>
    </div>

    <slot name="actions" />
  </FormLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
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

// Focus first field with error when errors change
const focusFirstError = async (errors: Record<string, string> | undefined) => {
  if (!props.focusOnError || !errors) return
  const entries = Object.entries(errors)
  if (!entries.length) return
  await nextTick()
  const rawKey = entries[0][0]
  const candidates = [rawKey, toCamel(rawKey)]
  let el: HTMLElement | null = null
  for (const key of candidates) {
    el = (document.querySelector(`[name="${key}"]`) ||
      document.getElementById(key)) as HTMLElement | null
    if (el) break
  }
  el?.focus()
}

// When errors update after submit, focus first error field (optional)
watch(
  () => props.form?.errors,
  (errs) => {
    focusFirstError(errs as Record<string, string>)
  },
  { deep: true },
)

// Also react to pageErrors when form.errors is empty (initial GET with errors)
watch(
  pageErrors,
  (errs) => {
    if (!props.form?.errors || Object.keys(props.form.errors).length === 0) {
      focusFirstError(errs)
    }
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
      return window.confirm('Есть несохранённые изменения. Покинуть страницу?')
    })
  }
})
onUnmounted(() => {
  window.removeEventListener('beforeunload', beforeUnload)
  removeInertiaGuard?.()
})
</script>
