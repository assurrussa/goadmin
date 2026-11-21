<script setup lang="ts">
import { nextTick, computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import { router, useForm, usePage } from '@inertiajs/vue3'
import { adminFetch, AdminFetchNotSentError } from '@/http'
import { t } from '@/i18n'
import AppHead from '@/components/layout/AppHead.vue'
import FormCheckbox from '@/components/form/FormCheckbox.vue'
import FormInput from '@/components/form/FormInput.vue'
import RecordEditForm from '@/components/form/RecordEditForm.vue'
import RoleEditConflict from '@/components/roles/RoleEditConflict.vue'
import RolePermissionsSection from '@/components/roles/RolePermissionsSection.vue'
import { permissionKey, type RolePermission } from '@/components/roles/permissionLabels'
import { useNotifications } from '@/composables/useNotifications'
import { useRecordNavigation } from '@/composables/useRecordNavigation'

interface RoleSnapshot {
  idExact: string
  revision?: string
  slug: string
  name: string
  description?: string | null
  isSystem: boolean
  permissions?: RolePermission[]
}

const props = defineProps<{
  title?: string
  role: RoleSnapshot
  permissions: RolePermission[]
}>()

const basePath = '/roles'
const { listHref } = useRecordNavigation(basePath)
const { success } = useNotifications()
const page = usePage()
const owner = () => {
  const actor = page.props.authuser as { idExact?: string; id?: number } | undefined
  return String(actor?.idExact ?? actor?.id ?? '')
}

function fields(role: RoleSnapshot) {
  return {
    slug: role.slug,
    name: role.name,
    description: role.description ?? '',
    isSystem: role.isSystem,
    permissions: (role.permissions ?? []).map(permissionKey).sort(),
  }
}

const baseline = shallowRef(props.role)
const saved = shallowRef<RoleSnapshot>()
const unavailable = ref(false)
const reloadRequired = ref(false)
const form = useForm(fields(props.role))
let pending: AbortController | undefined

const protectedSlug = computed(() => (saved.value ?? baseline.value).slug === 'super_admin')
const protectedSystem = computed(() => (saved.value ?? baseline.value).isSystem)
const catalog = computed(() => {
  const permissions = new Map<string, RolePermission>()
  for (const permission of [
    ...(baseline.value.permissions ?? []),
    ...(saved.value?.permissions ?? []),
    ...props.permissions,
  ]) {
    permissions.set(permissionKey(permission), permission)
  }
  return [...permissions.values()]
})

function cancel() {
  pending?.abort()
  pending = undefined
  form.processing = false
}

function hideEditor() {
  unavailable.value = true
  saved.value = undefined
  form.defaults({ slug: '', name: '', description: '', isSystem: false, permissions: [] }).reset()
  form.clearErrors()
}

watch([() => props.role, owner], ([role, actor], [previousRole, previousActor]) => {
  cancel()
  if (actor !== previousActor && role === previousRole) {
    hideEditor()
    return
  }
  baseline.value = role
  saved.value = undefined
  unavailable.value = false
  reloadRequired.value = false
  form.defaults(fields(role)).reset().clearErrors()
})

onScopeDispose(cancel)

function resolveConflict(keepDraft: boolean) {
  if (!saved.value) return
  const current = saved.value
  baseline.value = current
  saved.value = undefined
  form.defaults(fields(current)).clearErrors()
  if (!keepDraft) form.reset()
  else {
    if (current.slug === 'super_admin') form.slug = current.slug
    if (current.isSystem) form.isSystem = true
    // Recalculate dirty state even when keeping a previously unmodified draft.
    form.permissions = [...form.permissions]
  }
}

function unknownOutcome() {
  reloadRequired.value = true
  formError('Результат операции неизвестен. Обновите страницу перед повтором.')
}

function formError(message: string) {
  const errors: Record<string, string> = { form: message }
  form.setError(errors)
}

async function submit(_event: Event, returnToList = false) {
  const { revision, idExact } = baseline.value
  if (pending || saved.value || unavailable.value || reloadRequired.value || !revision) return
  const ownerAtStart = owner()
  const componentAtStart = page.component
  const destination = listHref.value
  const attempt = new AbortController()
  const current = () =>
    pending === attempt &&
    owner() === ownerAtStart &&
    page.component === componentAtStart &&
    props.role.idExact === idExact
  pending = attempt
  form.processing = true
  form.clearErrors()

  let goToList = false
  try {
    const response = await adminFetch(`${basePath}/${encodeURIComponent(idExact)}`, {
      method: 'PUT',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...form.data(), revision }),
      redirect: 'error',
      signal: attempt.signal,
    })
    const result = (await response.json().catch(() => null)) as {
      role?: RoleSnapshot
      message?: string
      access?: 'list' | 'denied'
    } | null
    if (!current()) return

    if ([401, 403, 404].includes(response.status) || (response.ok && result?.access)) {
      hideEditor()
      goToList = result?.access === 'list' || response.status === 404
      if (response.ok) success(t('Роль обновлена'))
      return
    }
    const role = result?.role
    if ((response.ok || response.status === 409) && role?.idExact === idExact && role.revision) {
      if (response.status === 409) {
        saved.value = role
      } else {
        baseline.value = role
        form.defaults(fields(role)).reset()
        success(t('Роль обновлена'))
        goToList = returnToList
      }
    } else if ([400, 422].includes(response.status) && result?.message) {
      formError(result.message)
    } else {
      unknownOutcome()
    }
  } catch (error) {
    if (!current()) return
    if (error instanceof AdminFetchNotSentError) {
      formError('Не удалось отправить изменения роли. Повторите попытку.')
    } else {
      unknownOutcome()
    }
  } finally {
    if (current()) {
      form.processing = false
      await nextTick()
      if (current()) {
        pending = undefined
        if (goToList && !form.isDirty) router.get(destination)
      }
    }
  }
}
</script>

<template>
  <AppHead :title="unavailable ? t('Редактирование') : props.title" />
  <p v-if="unavailable" class="rounded-lg border border-error/30 p-4 text-error" role="alert">
    {{ t('Редактирование этой роли недоступно.') }}
  </p>
  <RecordEditForm
    v-else
    :form="form"
    :fields="['slug', 'name', 'description']"
    :return-href="listHref"
    :label="t('Редактирование')"
    can-save-and-return
    :submit-disabled="Boolean(saved) || reloadRequired || !baseline.revision"
    @submit="submit"
  >
    <RoleEditConflict
      v-if="saved"
      :saved="fields(saved)"
      :draft="form"
      @discard="resolveConflict(false)"
      @reapply="resolveConflict(true)"
    />

    <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
      <FormInput
        v-model="form.slug"
        name="slug"
        label="Slug"
        placeholder="role-manager"
        required
        :readonly="protectedSlug"
        :error="form.errors.slug"
      />
      <FormInput
        v-model="form.name"
        name="name"
        :label="t('Название')"
        :placeholder="t('Менеджер')"
        required
        :error="form.errors.name"
      />
    </div>

    <FormInput
      v-model="form.description"
      name="description"
      :label="t('Описание')"
      :placeholder="t('Краткое описание роли')"
      :error="form.errors.description"
    />

    <FormCheckbox
      v-model="form.isSystem"
      :label="t('Системная роль')"
      :disabled="protectedSystem"
      :help-text="t('Изменение системных ролей может влиять на работу платформы.')"
    />

    <RolePermissionsSection
      v-model="form.permissions"
      :permissions="catalog"
      :title="t('Права доступа')"
      subtitle="Выберите действия для доменов, чтобы задать доступы роли."
    />
  </RecordEditForm>
</template>
