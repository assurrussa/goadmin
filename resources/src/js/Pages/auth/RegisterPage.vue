<template>
  <AuthLayout>
    <AppHead :title="props.title" />

    <template #title>Первый администратор</template>

    <template #subtitle
      >Создайте первого администратора, после этого публичная регистрация закроется.</template
    >

    <AdminForm
      :form="form"
      :fields="[
        'name',
        'lastName',
        'username',
        'email',
        'password',
        'passwordConfirm',
        'setupToken',
      ]"
      @submit="handleRegister"
    >
      <FormInput
        v-model="form.name"
        name="name"
        type="text"
        label="Имя"
        placeholder="Администратор"
        required
        :error="form.errors.name"
      />

      <FormInput
        v-model="form.lastName"
        name="lastName"
        type="text"
        label="Фамилия"
        placeholder="Фамилия"
        :error="form.errors.lastName"
      />

      <FormInput
        v-model="form.email"
        name="email"
        type="email"
        label="Email адрес"
        placeholder="admin@example.com"
        autocomplete="email"
        required
        :error="form.errors.email"
      />

      <FormInput
        v-model="form.username"
        name="username"
        type="text"
        label="Логин"
        placeholder="admin"
        autocomplete="username"
        :error="form.errors.username"
        help-text="Необязательно. Если оставить пустым, возьмем значение из email."
      />

      <PasswordInput
        v-model="form.password"
        name="password"
        label="Пароль"
        autocomplete="new-password"
        required
        :error="form.errors.password"
      />

      <PasswordInput
        v-model="form.passwordConfirm"
        name="passwordConfirm"
        label="Повтор пароля"
        autocomplete="new-password"
        required
        :error="form.errors.passwordConfirm"
      />

      <PasswordInput
        v-model="form.setupToken"
        name="setupToken"
        label="Setup-токен"
        autocomplete="off"
        required
        :error="form.errors.setupToken"
        help-text="Введите безопасный токен, настроенный владельцем приложения."
      />

      <FormButton
        type="submit"
        variant="primary"
        size="md"
        :loading="form.processing"
        loading-text="Создание..."
        full-width
        class="mt-6"
      >
        Создать первого администратора
      </FormButton>
    </AdminForm>

    <template #footer>
      <div class="text-center space-y-2">
        <span class="text-sm text-text-secondary"> Уже есть администратор? </span>
        <div>
          <Link href="/auth/login" class="text-sm text-primary hover:text-primary-dark">
            Вернуться ко входу
          </Link>
        </div>
      </div>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { Link, useForm, usePage } from '@inertiajs/vue3'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AppHead from '@/components/layout/AppHead.vue'
import FormInput from '@/components/form/FormInput.vue'
import PasswordInput from '@/components/form/PasswordInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import AdminForm from '@/components/form/AdminForm.vue'

const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
})

const page = usePage()
const pageErrors = (page.props.errors as Record<string, string>) || {}
const oldData = (page.props.old as Record<string, string | null>) || {}

function takeSetupTokenFromFragment(): string {
  if (typeof window === 'undefined') return ''

  const fragment = new URLSearchParams(window.location.hash.slice(1))
  const token = fragment.get('setup_token') || ''
  if (window.location.hash) {
    window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}`)
  }

  return token
}

const form = useForm({
  name: oldData.name || '',
  lastName: oldData.lastName || '',
  username: oldData.username || '',
  email: oldData.email || '',
  password: '',
  passwordConfirm: '',
  setupToken: takeSetupTokenFromFragment(),
})

form.errors = { ...pageErrors }

function handleRegister() {
  form.post('/auth/register')
}
</script>
