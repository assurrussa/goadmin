<script setup lang="ts">
import { computed, nextTick, onScopeDispose, ref, shallowRef, watch } from 'vue'
import { router, usePage } from '@inertiajs/vue3'
import { appearance } from '@/appearance/state'
import { adminFetch, AdminFetchNotSentError, reportSessionAuthFailure } from '@/http'
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import Field from './Field.vue'
import { constraintError } from './constraints'
import { displayFieldValue } from './display'
import { crudRecordHref, type CrudAction, type CrudRecord, type CrudResource } from './index'

const props = defineProps<{ resource: CrudResource; recordId?: string }>()
const page = usePage()
function actorOf(props: Record<string, unknown>) {
  const actor = props.authuser as { id?: number | string; idExact?: string } | undefined
  return String(actor?.idExact || actor?.id || '')
}

type Request = {
  action: CrudAction
  record: CrudRecord
  actor: string
  href: string
  controller: AbortController
}
const request = shallowRef<Request>()
const draft = ref<Record<string, string>>(Object.create(null))
const fieldErrors = ref<Record<string, string>>(Object.create(null))
const notice = ref('')
const pending = ref(false)
const attempted = ref(false)
const refreshed = ref(false)
const recovery = ref<'unknown' | 'refresh' | 'unavailable'>()
let trigger: HTMLElement | null = null
const fields = computed(() =>
  (request.value?.action.params || []).map((param) => ({
    ...param,
    read: false,
    create: false,
    update: false,
  })),
)
function trimActionText(raw: string) {
  // Go unicode.IsSpace uses Unicode White_Space. JS trim() also strips U+FEFF.
  const space = /\p{White_Space}/u
  let start = 0
  let end = raw.length
  while (start < end && space.test(raw.charAt(start))) start++
  while (end > start && space.test(raw.charAt(end - 1))) end--
  return raw.slice(start, end)
}
const values = computed(() =>
  Object.fromEntries(
    fields.value.map((field) => {
      const raw = draft.value[field.key] ?? ''
      return [field.key, field.trimSpace ? trimActionText(raw) : raw]
    }),
  ),
)
const localErrors = computed<Record<string, string | undefined>>(() =>
  props.resource.serverValidateActionParams
    ? {}
    : Object.fromEntries(
        fields.value.map((field) => {
          const value = values.value[field.key] ?? ''
          // In Unicode mode a surrogate pair is one scalar, so only lone surrogates match.
          if (value.includes('\0') || /[\uD800-\uDFFF]/u.test(value))
            return [field.key, t('Invalid field value')]
          if (attempted.value && field.required && !value)
            return [field.key, t('This field is required')]
          return [field.key, constraintError(field, value)]
        }),
      ),
)
const recordName = computed(() => {
  const field = props.resource.fields.find((field) => field.read && field.type === 'text')
  return field && request.value?.record.data[field.key]
})
function clear(message = '') {
  request.value?.controller.abort()
  request.value = undefined
  draft.value = Object.create(null)
  fieldErrors.value = Object.create(null)
  attempted.value = refreshed.value = pending.value = false
  recovery.value = undefined
  notice.value = message
}
function open(action: CrudAction, record: CrudRecord, returnTo: string) {
  const actor = actorOf(page.props)
  if (request.value || !actor || !action.params?.length) return
  clear()
  trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null
  request.value = {
    action,
    record,
    actor,
    href: crudRecordHref(props.resource.basePath, record.id, false, returnTo),
    controller: new AbortController(),
  }
}
defineExpose({ open })
function restoreFocus(event: Event) {
  event.preventDefault()
  if (trigger?.isConnected) trigger.focus()
}
function edit(key: string, value: unknown) {
  if (pending.value || recovery.value === 'unknown') return
  draft.value[key] = String(value ?? '')
  delete fieldErrors.value[key]
}
function viewRecord(target: Request) {
  clear()
  router.get(target.href)
}
function requestSignal(target: Request) {
  return AbortSignal.any([target.controller.signal, AbortSignal.timeout(15_000)])
}
async function readCurrent(target: Request) {
  const response = await adminFetch(target.href, {
    headers: {
      'X-Inertia': 'true',
      'X-Inertia-Version': page.version ?? '',
      Accept: 'text/html, application/xhtml+xml',
    },
    cache: 'no-store',
    signal: requestSignal(target),
    redirect: 'error',
  })
  if (target.controller.signal.aborted) return
  if ([401, 403, 404].includes(response.status)) return clear('Record access denied')
  if (!response.ok) throw new Error('Record could not be refreshed')
  const result = await response.json()
  if (target.controller.signal.aborted) return
  const record: CrudRecord | undefined = result.props?.record
  if (!result.props || actorOf(result.props) !== target.actor) return clear('Record access denied')
  if (
    result.component !== 'crud/Show' ||
    result.props.resource?.basePath !== props.resource.basePath ||
    result.props.resource?.key !== props.resource.key ||
    !record ||
    record.id !== target.record.id ||
    typeof record.revision !== 'string' ||
    !record.revision ||
    !record.data ||
    typeof record.data !== 'object' ||
    Array.isArray(record.data) ||
    !Array.isArray(record.actions)
  )
    throw new Error('Unexpected record')
  if (!record.actions.includes(target.action.key)) {
    if (!props.resource.serverValidateActionParams) return viewRecord(target)
    request.value = { ...target, record }
    refreshed.value = true
    pending.value = false
    recovery.value = 'unavailable'
    notice.value = 'The record changed. This action is no longer available.'
    return
  }
  request.value = { ...target, record }
  refreshed.value = true
  recovery.value = undefined
  pending.value = false
  notice.value = 'The record changed. Review its current values and confirm again.'
}
async function recheck(target: Request) {
  pending.value = true
  try {
    await readCurrent(target)
  } catch (error) {
    if (target.controller.signal.aborted) return
    if (error instanceof AdminFetchNotSentError && error.code !== 'authentication_unavailable')
      return clear('Record access denied')
    pending.value = false
    recovery.value = 'refresh'
    notice.value = 'The record changed. Its current values could not be loaded.'
  }
}
async function focusField(form: HTMLFormElement, key: string) {
  await nextTick()
  const input = Array.from(document.getElementsByName(key)).find(
    (control) => control.closest('form') === form,
  )
  input?.focus()
}
async function submit(event: SubmitEvent) {
  const target = request.value
  if (!target || pending.value || recovery.value) return
  const form = event.currentTarget as HTMLFormElement
  attempted.value = true
  const invalid = fields.value.find((field) => localErrors.value[field.key])
  if (invalid) return focusField(form, invalid.key)
  pending.value = true
  fieldErrors.value = Object.create(null)
  notice.value = ''
  const path = `${props.resource.basePath}/records/${encodeURIComponent(target.record.id)}/actions/${encodeURIComponent(target.action.key)}`
  try {
    const response = await adminFetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ _revision: target.record.revision, params: values.value }),
      redirect: 'error',
      signal: requestSignal(target),
    })
    if (target.controller.signal.aborted) return
    if (response.status === 204) return viewRecord(target)
    if ([401, 403, 404].includes(response.status)) return clear('Record access denied')
    if (response.status === 419 || response.status === 409) {
      const marked = response.headers.get('x-goadmin-auth-error') === '1'
      if (marked) {
        const body = await response.json().catch(() => null)
        if (target.controller.signal.aborted) return
        if (reportSessionAuthFailure(response.status, response.headers, body, false)) {
          if (response.status === 409) return clear('Record access denied')
          // adminFetch started CSRF repair. A manual submit joins its readiness gate.
          pending.value = false
          notice.value = 'The action was not performed. Confirm again after access is restored.'
          return
        }
      } else if (response.status === 409) return await recheck(target)
    }
    if (response.status === 422) {
      const result = await response.json()
      if (target.controller.signal.aborted) return
      for (const field of fields.value) {
        const error = result.errors?.[field.key]?.[0]
        if (typeof error === 'string') fieldErrors.value[field.key] = error
      }
      pending.value = false
      const first = fields.value.find((field) => fieldErrors.value[field.key])
      if (first) await focusField(form, first.key)
      else notice.value = result.errors?._form?.[0] || 'Invalid field value'
      return
    }
  } catch (error) {
    if (target.controller.signal.aborted) return
    if (error instanceof AdminFetchNotSentError) {
      if (error.code !== 'authentication_unavailable') return clear('Record access denied')
      pending.value = false
      notice.value = error.message
      return
    }
  }
  pending.value = false
  recovery.value = 'unknown'
  notice.value = 'The outcome is unknown. Check the record and its activity before trying again.'
}
const leave = () => clear()
const stopNavigation = router.on('start', leave)
if (typeof window !== 'undefined') window.addEventListener('popstate', leave)
watch(
  [
    () => page.props,
    () => page.url,
    () => props.resource.basePath,
    () => props.recordId,
    () => appearance.owner,
    () => appearance.readOnly,
    () => appearance.resolving,
    () => actorOf(page.props),
  ],
  leave,
  { flush: 'sync' },
)
onScopeDispose(() => {
  clear()
  stopNavigation()
  if (typeof window !== 'undefined') window.removeEventListener('popstate', leave)
})
</script>

<template>
  <p v-if="!request && notice" role="alert" class="mb-4 text-error">{{ t(notice) }}</p>
  <Dialog
    v-if="request"
    :open="true"
    @update:open="
      (open) => {
        if (!open && !pending) clear()
      }
    "
  >
    <DialogContent
      data-crud-action-dialog
      aria-modal="true"
      class="max-h-[calc(100dvh-2rem)] overflow-y-auto"
      @close-auto-focus="restoreFocus"
    >
      <DialogHeader>
        <DialogTitle>{{ t(request.action.label) }}</DialogTitle>
        <DialogDescription class="break-words">
          {{ t(resource.title) }} #{{ request.record.id
          }}<template v-if="recordName"> · {{ recordName }}</template>
          <span v-if="request.action.description" class="mt-1 block">{{
            t(request.action.description)
          }}</span>
          <span v-if="request.action.confirmation" class="mt-1 block">{{
            t(request.action.confirmation)
          }}</span>
        </DialogDescription>
      </DialogHeader>
      <p v-if="notice" role="alert" class="text-sm">{{ t(notice) }}</p>
      <dl v-if="refreshed" class="grid gap-2 text-sm">
        <div
          v-for="field in resource.fields.filter((f) => f.read && f.type !== 'custom')"
          :key="field.key"
        >
          <dt class="text-text-secondary">{{ t(field.label) }}</dt>
          <dd class="whitespace-pre-wrap break-words">
            {{ displayFieldValue(field, request.record.data[field.key]) }}
          </dd>
        </div>
      </dl>
      <form novalidate class="space-y-4" @submit.prevent="submit">
        <Field
          v-for="field in fields"
          :key="field.key"
          :field="field"
          :model-value="draft[field.key] ?? ''"
          :constraint-error="localErrors[field.key]"
          :error="fieldErrors[field.key]"
          :disabled="pending || recovery === 'unknown' || recovery === 'unavailable'"
          @update:model-value="(value) => edit(field.key, value)"
        />
        <DialogFooter class="gap-2">
          <AppButton
            variant="secondary"
            data-confirmation-cancel
            :disabled="pending"
            @click="clear()"
            >{{ t('Отмена') }}</AppButton
          >
          <AppButton v-if="recovery === 'refresh'" :disabled="pending" @click="recheck(request)">{{
            t('Проверить ещё раз')
          }}</AppButton>
          <AppButton
            v-else-if="recovery === 'unknown' || recovery === 'unavailable'"
            @click="viewRecord(request)"
            >{{ t('Открыть запись') }}</AppButton
          >
          <AppButton v-else type="submit" data-confirmation-accept :disabled="pending">{{
            t(request.action.label)
          }}</AppButton>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
