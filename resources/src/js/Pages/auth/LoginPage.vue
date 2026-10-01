<template>
  <AuthLayout>
    <AppHead :title="props.title" />

    <template #title>Вход в систему</template>

    <template #subtitle> Войдите в административную панель </template>

    <AdminForm :form="form" :fields="['email', 'password']" @submit="handleLogin">
      <!-- Email -->
      <FormInput
        v-model="form.email"
        name="email"
        type="email"
        label="Email адрес"
        placeholder="admin@example.com"
        autocomplete="email"
        required
        :error="form.errors.email"
        size="md"
      >
        <template #leftIcon>
          <Mail class="w-4 h-4" />
        </template>
      </FormInput>

      <!-- Password -->
      <FormInput
        v-model="form.password"
        name="password"
        type="password"
        label="Пароль"
        placeholder="Введите пароль"
        autocomplete="current-password"
        required
        :error="form.errors.password"
        size="md"
      >
        <template #leftIcon>
          <Lock class="w-4 h-4" />
        </template>
      </FormInput>

      <!-- Remember me and forgot password -->
      <div class="flex items-center justify-between px-2 py-2">
        <FormCheckbox v-model="form.remember" label="Запомнить меня" size="sm" />

        <div v-if="authmail" class="text-sm">
          <Link href="/auth/forgot-password" class="text-sm text-primary hover:text-primary-dark">
            Забыли пароль?
          </Link>
        </div>
      </div>

      <!-- Submit button -->
      <FormButton
        type="submit"
        variant="primary"
        size="md"
        :loading="form.processing"
        loading-text="Вход..."
        full-width
        class="mt-6"
      >
        <template #leftIcon>
          <Lock class="w-5 h-5" />
        </template>
        Войти
      </FormButton>
    </AdminForm>

    <template v-if="props.canRegisterFirstAdmin" #footer>
      <div class="text-center">
        <Link href="/auth/register" class="text-sm text-primary hover:text-primary-dark">
          Зарегистрировать первого администратора
        </Link>
      </div>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { Link, useForm, usePage } from '@inertiajs/vue3'
import { Lock, Mail } from 'lucide-vue-next'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AppHead from '@/components/layout/AppHead.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import FormCheckbox from '@/components/form/FormCheckbox.vue'
import AdminForm from '@/components/form/AdminForm.vue'

const { authmail } = useAdminCapabilities()

const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  canRegisterFirstAdmin: {
    type: Boolean,
    default: false,
  },
})

const page = usePage()
const pageErrors = (page.props.errors as Record<string, string>) || {}
const oldData = (page.props.old as Record<string, string | null>) || {}

const form = useForm({
  email: oldData.email || '',
  password: '',
  remember: false,
})

form.errors = { ...pageErrors }

function handleLogin() {
  form.post(`/auth/login`)
}
</script>
