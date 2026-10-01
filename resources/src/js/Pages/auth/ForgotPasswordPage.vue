<template>
  <AuthLayout>
    <template #title>Восстановление пароля</template>

    <template #subtitle>
      Введите ваш email адрес и мы отправим вам ссылку для восстановления пароля
    </template>

    <AdminForm :form="form" :fields="['email']" @submit="handleReset" class="space-y-6">
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
        help-text="Мы отправим ссылку для восстановления на этот адрес"
      >
        <template #leftIcon>
          <Mail class="w-4 h-4" />
        </template>
      </FormInput>

      <!-- Submit button -->
      <FormButton
        type="submit"
        variant="primary"
        size="md"
        :loading="form.processing"
        loading-text="Отправка..."
        full-width
        class="mt-6"
      >
        <template #leftIcon>
          <Mail class="w-5 h-5" />
        </template>
        Отправить ссылку
      </FormButton>
    </AdminForm>

    <template #footer>
      <div class="text-center space-y-2">
        <div>
          <span class="text-sm text-text-secondary"> Вспомнили пароль? </span>
          <Link href="/auth/login" class="text-sm text-primary hover:text-primary-dark">
            Войти
          </Link>
        </div>
      </div>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { Link, useForm, usePage } from '@inertiajs/vue3'
import { Mail } from 'lucide-vue-next'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import AdminForm from '@/components/form/AdminForm.vue'

const page = usePage()
const oldData = (page.props.old as Record<string, string | null>) || {}

const form = useForm({
  email: oldData.email || '',
})

function handleReset() {
  form.post('/auth/forgot-password')
}
</script>
