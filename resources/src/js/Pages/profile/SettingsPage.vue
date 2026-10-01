<template>
  <AppHead :title="props.title" />
  <PageActionBar back-href="/auth/profile" label="Настройки" />

  <div class="space-y-8">
    <Card v-if="authmail">
      <CardHeader class="border-b border-border-secondary bg-surface-variant/40">
        <CardTitle>Смена email</CardTitle>
        <CardDescription>
          Для смены email мы отправим код подтверждения на новый адрес и уведомим старый.
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-6 pt-6">
        <div class="flex flex-col gap-2 text-sm">
          <span class="text-text-tertiary">Текущий email</span>
          <span class="text-text-primary font-medium">{{ props.data.email }}</span>
        </div>

        <PasswordInput
          v-model="emailRequestForm.currentPassword"
          name="currentPassword"
          label="Текущий пароль"
          placeholder="Введите текущий пароль"
          autocomplete="current-password"
          :error="emailRequestForm.errors.currentPassword"
        />

        <div class="grid gap-4 lg:grid-cols-[1fr_auto] lg:items-end">
          <FormInput
            v-model="emailRequestForm.email"
            name="email"
            type="email"
            label="Новый email"
            placeholder="name@example.com"
            :error="emailRequestForm.errors.email"
          />
          <FormButton
            type="button"
            variant="secondary"
            :loading="emailRequestForm.processing"
            loading-text="Отправляем..."
            class="lg:mb-1"
            @click="sendEmailCode"
          >
            Отправить код
          </FormButton>
        </div>

        <div
          v-if="pendingEmailChange"
          class="rounded-xl border border-border-secondary bg-surface px-4 py-3 text-sm text-text-secondary"
        >
          <p>
            Код отправлен на
            <span class="font-medium text-text-primary">{{ pendingEmailChange.newEmail }}</span
            >. Действует до {{ formatDateTime(pendingEmailChange.expiresAt) }}.
          </p>
        </div>

        <div class="grid gap-4 lg:grid-cols-[1fr_auto] lg:items-end">
          <FormInput
            v-model="emailConfirmForm.code"
            name="code"
            label="Код подтверждения"
            placeholder="123456"
            :error="emailConfirmForm.errors.code"
            help-text="Введите 6-значный код из письма"
          />
          <FormButton
            type="button"
            variant="primary"
            :loading="emailConfirmForm.processing"
            loading-text="Проверяем..."
            class="lg:mb-1"
            @click="confirmEmail"
          >
            Подтвердить email
          </FormButton>
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader class="border-b border-border-secondary bg-surface-variant/40">
        <CardTitle>Смена пароля</CardTitle>
        <CardDescription>Укажите текущий пароль и задайте новый.</CardDescription>
      </CardHeader>
      <CardContent class="space-y-6 pt-6">
        <PasswordInput
          v-model="passwordForm.currentPassword"
          name="currentPassword"
          label="Текущий пароль"
          placeholder="Введите текущий пароль"
          autocomplete="current-password"
          :error="passwordForm.errors.currentPassword"
        />
        <PasswordInput
          v-model="passwordForm.password"
          name="password"
          label="Новый пароль"
          placeholder="Введите новый пароль"
          autocomplete="new-password"
          :error="passwordForm.errors.password"
        />
        <PasswordInput
          v-model="passwordForm.passwordConfirmation"
          name="passwordConfirmation"
          label="Подтвердите новый пароль"
          placeholder="Повторите новый пароль"
          autocomplete="new-password"
          :error="
            passwordForm.errors.passwordConfirmation || passwordConfirmationError || undefined
          "
        />

        <PasswordStrengthIndicator :password="passwordForm.password" />

        <div class="flex justify-end">
          <FormButton
            type="button"
            variant="primary"
            :loading="passwordForm.processing"
            loading-text="Сохраняем..."
            :disabled="!isPasswordFormValid"
            @click="changePassword"
          >
            Обновить пароль
          </FormButton>
        </div>
      </CardContent>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { useForm } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import PasswordInput from '@/components/form/PasswordInput.vue'
import PasswordStrengthIndicator from '@/components/form/PasswordStrengthIndicator.vue'
import { Card, CardHeader, CardContent, CardTitle, CardDescription } from '@/components/ui/card'

interface PendingEmailChange {
  newEmail: string
  expiresAt: string
}

interface SettingsProps {
  title?: string
  data: {
    email: string
  }
  pendingEmailChange?: PendingEmailChange | null
}

const props = defineProps<SettingsProps>()
const { authmail } = useAdminCapabilities()

const emailRequestForm = useForm({
  currentPassword: '',
  email: props.pendingEmailChange?.newEmail || props.data.email || '',
})

const emailConfirmForm = useForm({
  code: '',
})

const passwordForm = useForm({
  currentPassword: '',
  password: '',
  passwordConfirmation: '',
})

const pendingEmailChange = computed(() => props.pendingEmailChange || null)

const passwordConfirmationError = computed(() => {
  if (
    passwordForm.passwordConfirmation &&
    passwordForm.password !== passwordForm.passwordConfirmation
  ) {
    return 'Пароли не совпадают'
  }
  return null
})

const isPasswordFormValid = computed(() => {
  return (
    passwordForm.currentPassword.length > 0 &&
    passwordForm.password.length >= 8 &&
    passwordForm.password === passwordForm.passwordConfirmation &&
    !passwordConfirmationError.value
  )
})

const sendEmailCode = () => {
  if (!authmail.value) return
  emailRequestForm.post('/auth/profile/settings/email/request', {
    preserveScroll: true,
    onFinish: () => {
      emailRequestForm.reset('currentPassword')
    },
    onSuccess: () => {
      emailConfirmForm.reset('code')
    },
  })
}

const confirmEmail = () => {
  if (!authmail.value) return
  emailConfirmForm.post('/auth/profile/settings/email/confirm', {
    preserveScroll: true,
    onSuccess: () => {
      emailConfirmForm.reset('code')
    },
  })
}

const changePassword = () => {
  passwordForm.post('/auth/profile/settings/password', {
    preserveScroll: true,
    onSuccess: () => {
      passwordForm.reset('currentPassword', 'password', 'passwordConfirmation')
    },
  })
}

const formatDateTime = (value: string) => {
  if (!value) return '—'
  const date = new Date(value)
  return date.toLocaleString('ru-RU', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>
