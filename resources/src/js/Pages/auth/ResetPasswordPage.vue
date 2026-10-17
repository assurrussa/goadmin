<template>
  <AuthLayout>
    <AppHead :title="props.title" />

    <template #title>{{ props.title }}</template>

    <template #subtitle>
      {{ t('Введите ваш новый пароль для завершения процедуры восстановления') }}
    </template>

    <AdminForm :form="form" :fields="['password', 'password_confirmation']" @submit="handleReset">
      <!-- Email (disabled, из URL) -->
      <FormInput
        v-model="form.email"
        type="email"
        :label="t('Email адрес')"
        :disabled="true"
        size="md"
        class="opacity-75"
      >
        <template #leftIcon>
          <Mail class="w-4 h-4" />
        </template>
      </FormInput>

      <!-- Token (disabled, из URL) -->
      <FormInput
        v-model="form.token"
        type="text"
        :label="t('Токен сброса')"
        :disabled="true"
        size="md"
        class="opacity-75"
        help-text="Токен из ссылки восстановления"
      >
        <template #leftIcon>
          <KeyRound class="w-4 h-4" />
        </template>
      </FormInput>

      <!-- Password -->
      <PasswordInput
        v-model="form.password"
        name="password"
        :label="t('Новый пароль')"
        :placeholder="t('Введите новый пароль')"
        autocomplete="new-password"
        required
        :error="form.errors.password"
        size="md"
        help-text="Пароль должен содержать минимум 8 символов"
      >
        <template #leftIcon>
          <KeyRound class="w-4 h-4" />
        </template>
      </PasswordInput>

      <!-- Password Confirmation -->
      <PasswordInput
        v-model="form.passwordConfirmation"
        name="passwordConfirmation"
        :label="t('Подтверждение пароля')"
        :placeholder="t('Повторите новый пароль')"
        autocomplete="new-password"
        required
        :error="form.errors.passwordConfirmation || passwordConfirmationError || undefined"
        size="md"
        help-text="Пароли должны совпадать"
      >
        <template #leftIcon>
          <KeyRound class="w-4 h-4" />
        </template>
      </PasswordInput>

      <!-- Password strength indicator -->
      <PasswordStrengthIndicator :password="form.password" />

      <!-- Submit button -->
      <FormButton
        type="submit"
        variant="primary"
        size="md"
        :loading="form.processing"
        :disabled="!isFormValid"
        :loading-text="t('Сброс пароля...')"
        full-width
        class="mt-6"
      >
        <template #leftIcon>
          <KeyRound class="w-5 h-5" />
        </template>
        {{ t('Сбросить пароль') }}
      </FormButton>
    </AdminForm>

    <template #footer>
      <div class="text-center">
        <span class="text-sm text-text-secondary"> {{ t('Вспомнили пароль?') }} </span>
        <a
          v-if="props.loginURL"
          :href="props.loginURL"
          class="text-sm text-primary hover:text-primary-dark"
        >
          {{ t('Войти') }}
        </a>
      </div>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { useForm } from '@inertiajs/vue3'
import { KeyRound, Mail } from 'lucide-vue-next'
import { computed } from 'vue'
import AppHead from '@/components/layout/AppHead.vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import PasswordInput from '@/components/form/PasswordInput.vue'
import PasswordStrengthIndicator from '@/components/form/PasswordStrengthIndicator.vue'
import AdminForm from '@/components/form/AdminForm.vue'

interface Props {
  token: string
  email: string
  title?: string
  formAction?: string
  loginURL?: string
}

const props = withDefaults(defineProps<Props>(), {
  title: 'Сброс пароля',
  formAction: '/auth/reset-password',
  loginURL: '/auth/login',
})

const form = useForm({
  token: props.token,
  email: props.email,
  password: '',
  passwordConfirmation: '',
})

// Валидация подтверждения пароля
const passwordConfirmationError = computed(() => {
  if (form.passwordConfirmation && form.password !== form.passwordConfirmation) {
    return t('Пароли не совпадают')
  }
  return null
})

// Проверка валидности формы
const isFormValid = computed(() => {
  return (
    form.password.length >= 8 &&
    form.password === form.passwordConfirmation &&
    !passwordConfirmationError.value
  )
})

function handleReset() {
  form.post(props.formAction)
}
</script>
