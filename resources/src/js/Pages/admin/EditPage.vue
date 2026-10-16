<template>
  <AppHead :title="props.title" />
  <PageActionBar :back-href="basePath" label="Редактирование администратора" :use-history="false" />

  <AdminForm
    :form="form"
    :fields="['username', 'name', 'lastName', 'email']"
    warn-unsaved
    @submit="handleForm"
  >
    <div class="bg-card shadow-sm border border-border-primary overflow-hidden sm:rounded-lg">
      <div class="px-4 py-5 sm:p-6">
        <div class="space-y-6">
          <!-- Name -->
          <FormInput
            v-model="form.name"
            name="name"
            type="text"
            label="Имя"
            autocomplete="given-name"
            required
            :error="form.errors.name"
          />

          <!-- Last Name -->
          <FormInput
            v-model="form.lastName"
            name="lastName"
            type="text"
            label="Фамилия (необязательно)"
            autocomplete="family-name"
            :error="form.errors.lastName"
          />

          <!-- Email -->
          <FormInput
            v-model="form.email"
            name="email"
            type="email"
            label="Электронная почта"
            required
            :error="form.errors.email"
            readonly
            help-text="Адрес связан с учётной записью. Администратор меняет свой email в настройках профиля с подтверждением пароля."
          />

          <!-- Username -->
          <FormInput
            v-model="form.username"
            name="username"
            type="text"
            label="Имя пользователя (необязательно)"
            autocomplete="username"
            help-text="Можно оставить пустым."
            :error="form.errors.username"
          />
        </div>
      </div>

      <div
        class="px-4 py-3 bg-surface border-t border-border-primary sm:px-6 flex flex-wrap justify-end gap-3"
      >
        <AppButton as-child variant="secondary">
          <Link :href="basePath">Отмена</Link>
        </AppButton>
        <FormButton
          type="submit"
          :loading="form.processing"
          loading-text="Сохранение..."
          variant="primary"
        >
          Сохранить
        </FormButton>
      </div>
    </div>
  </AdminForm>
</template>

<script setup lang="ts">
import { Link, useForm, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import AdminForm from '@/components/form/AdminForm.vue'
import AppButton from '@/components/ui/AppButton.vue'

const basePath = '/admins'
const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  old: {
    type: Object,
    default: () => ({}),
  },
  data: {
    type: Object,
    default: () => ({}),
  },
})

// Получаем данные из page props
const page = usePage()
const oldData = (page.props.old as Record<string, string | null> | undefined) ?? props.old
const oldValue = (field: string) =>
  Object.prototype.hasOwnProperty.call(oldData, field) ? oldData[field] : (props.data[field] ?? '')

const form = useForm({
  name: oldValue('name'),
  email: props.data.email ?? '',
  lastName: oldValue('lastName'),
  username: oldValue('username'),
})

function handleForm() {
  if (form.processing) return
  form.put(`${basePath}/${props.data.id}`)
}
</script>
