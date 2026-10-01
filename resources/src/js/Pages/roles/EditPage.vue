<template>
  <AppHead :title="props.title" />
  <PageActionBar back-href="/roles" label="Редактирование" />

  <AdminForm :form="form" warn-unsaved @submit="handleSubmit">
    <Card class="overflow-hidden">
      <CardContent class="p-6 space-y-6">
        <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
          <FormInput
            v-model="form.slug"
            name="slug"
            label="Slug"
            placeholder="role-manager"
            required
            :error="form.errors.slug"
          />
          <FormInput
            v-model="form.name"
            name="name"
            label="Название"
            placeholder="Менеджер"
            required
            :error="form.errors.name"
          />
        </div>

        <FormInput
          v-model="form.description"
          name="description"
          label="Описание"
          placeholder="Краткое описание роли"
          :error="form.errors.description"
        />

        <div class="flex items-center gap-3">
          <FormCheckbox v-model="form.isSystem" label="Системная роль" />
          <p class="text-sm text-text-secondary">
            Изменение системных ролей может влиять на работу платформы.
          </p>
        </div>

        <RolePermissionsSection
          v-model="form.permissions"
          :permissions="props.permissions"
          title="Права доступа"
          subtitle="Выберите действия для доменов, чтобы задать доступы роли."
          header-eyebrow="Доступы роли"
        />
        <p v-if="form.errors.permissions" class="text-sm text-error">
          {{ form.errors.permissions }}
        </p>
      </CardContent>

      <CardFooter class="bg-surface border-t border-border-primary px-6 py-3 flex justify-end">
        <FormButton
          type="submit"
          :loading="form.processing"
          loading-text="Сохранение..."
          variant="primary"
        >
          Обновить
        </FormButton>
      </CardFooter>
    </Card>
  </AdminForm>
</template>

<script setup lang="ts">
import { useForm, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import AdminForm from '@/components/form/AdminForm.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import FormCheckbox from '@/components/form/FormCheckbox.vue'
import RolePermissionsSection from '@/components/roles/RolePermissionsSection.vue'
import { Card, CardContent, CardFooter } from '@/components/ui/card'

interface PermissionRecord {
  id: number
  uuid: string
  domain: string
  action: string
  description?: string | null
}

interface RoleDetail {
  id: number
  slug: string
  name: string
  description?: string | null
  isSystem: boolean
  permissions?: {
    domain: string
    action: string
    description?: string | null
  }[]
}

const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  role: {
    type: Object as () => RoleDetail,
    required: true,
  },
  permissions: {
    type: Array as () => PermissionRecord[],
    default: () => [],
  },
})

const oldData = (usePage().props.old as Record<string, unknown>) || {}

const normalizeBool = (value: unknown, fallback: boolean) => {
  if (typeof value === 'boolean') {
    return value
  }
  if (typeof value === 'string') {
    return value === 'true'
  }
  return fallback
}

const normalizePermissions = (): string[] => {
  if (Array.isArray(oldData.permissions)) {
    return oldData.permissions.map((item) => String(item))
  }
  return (props.role.permissions || []).map((perm) => `${perm.domain}:${perm.action}`)
}

const form = useForm({
  slug: (oldData.slug as string) || props.role.slug,
  name: (oldData.name as string) || props.role.name,
  description: (oldData.description as string) || props.role.description || '',
  isSystem: normalizeBool(oldData.isSystem, props.role.isSystem),
  permissions: normalizePermissions(),
})

function handleSubmit() {
  form.put(`/roles/${props.role.id}`)
}
</script>
