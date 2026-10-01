<template>
  <AppHead :title="props.title" />
  <PageActionBar :back-href="basePath" label="Создание" />

  <AdminForm
    :form="form"
    :fields="['username', 'name', 'lastName', 'email', 'password', 'passwordConfirm']"
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
            required
            :error="form.errors.name"
          />

          <!-- Last Name -->
          <FormInput
            v-model="form.lastName"
            name="lastName"
            type="text"
            label="Фамилия"
            :error="form.errors.lastName"
          />

          <!-- Email -->
          <FormInput
            v-model="form.email"
            name="email"
            type="email"
            label="E-mail"
            required
            :error="form.errors.email"
          />

          <!-- Username -->
          <FormInput
            v-model="form.username"
            name="username"
            type="text"
            label="Nickname"
            :error="form.errors.username"
          />

          <!-- Password -->
          <PasswordInput
            v-model="form.password"
            name="password"
            label="Пароль"
            autocomplete="new-password"
            required
            :error="form.errors.password"
          />

          <!-- Password Confirm -->
          <PasswordInput
            v-model="form.passwordConfirm"
            name="passwordConfirm"
            label="Пароль"
            required
            :error="form.errors.passwordConfirm"
          />
        </div>
      </div>

      <div class="px-4 py-3 bg-surface border-t border-border-primary sm:px-6 flex justify-end">
        <FormButton
          type="submit"
          :loading="form.processing"
          loading-text="Добавление..."
          variant="primary"
        >
          Добавить
        </FormButton>
      </div>
    </div>
  </AdminForm>
</template>

<script setup lang="ts">
import { useForm, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import PasswordInput from '@/components/form/PasswordInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import AdminForm from '@/components/form/AdminForm.vue'

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
})

// Получаем данные из page props
const page = usePage()
const oldData = (page.props.old as Record<string, string | null>) || {}

const form = useForm({
  username: oldData.username || null,
  name: oldData.name || null,
  lastName: oldData.lastName || null,
  email: oldData.email || null,
  password: null, // Пароль всегда пустой для безопасности
  passwordConfirm: null, // Пароль всегда пустой для безопасности
})

function handleForm() {
  form.post(`${basePath}/create`)
}
</script>
