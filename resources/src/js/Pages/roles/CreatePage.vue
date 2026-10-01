<template>
  <AppHead :title="props.title" />
  <PageActionBar back-href="/roles" label="Создание" />

  <AdminForm :form="form" @submit="handleSubmit">
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
            Системные роли защищены от удаления и используются платформой.
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
          Сохранить
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

const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  permissions: {
    type: Array as () => PermissionRecord[],
    default: () => [],
  },
})

const oldData = (usePage().props.old as Record<string, unknown>) || {}

const normalizePermissions = (val: unknown): string[] => {
  if (Array.isArray(val)) {
    return val.map((item) => String(item))
  }
  return []
}

const form = useForm({
  slug: (oldData.slug as string) || '',
  name: (oldData.name as string) || '',
  description: (oldData.description as string) || '',
  isSystem: typeof oldData.isSystem === 'boolean' ? oldData.isSystem : oldData.isSystem === 'true',
  permissions: normalizePermissions(oldData.permissions),
})

function handleSubmit() {
  form.post('/roles')
}
</script>
