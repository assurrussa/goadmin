<template>
  <AuthLayout>
    <AppHead title="Привязка SSO" />
    <template #title>Привязать учётную запись SSO</template>
    <template #subtitle>Свяжите подтверждённый аккаунт с текущим администратором</template>
    <p class="text-sm text-muted-foreground">Это не изменит ваши роли и права доступа.</p>
    <dl class="my-4 break-all text-sm">
      <dt class="font-medium">Сервер авторизации</dt>
      <dd>{{ issuer }}</dd>
      <dt class="mt-2 font-medium">Учётная запись</dt>
      <dd>{{ subject }}</dd>
    </dl>
    <form method="post" action="/sso-link" @submit.prevent="confirmLink">
      <input type="hidden" name="intent" :value="intent" />
      <input type="hidden" name="csrf" :value="csrf" />
      <input type="hidden" name="confirm" value="link-this-identity" />
      <FormButton type="submit" full-width :disabled="submitting"
        >Подтвердить привязку и продолжить</FormButton
      >
    </form>
    <p v-if="error" role="alert" class="mt-4 text-sm text-danger">{{ error }}</p>
    <a v-if="error" href="/sso-link" class="mt-3 block text-primary">Открыть новую форму</a>
    <a href="/" class="mt-4 block text-center text-primary">Отмена</a>
  </AuthLayout>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AppHead from '@/components/layout/AppHead.vue'
import FormButton from '@/components/form/FormButton.vue'
const props = defineProps<{ issuer: string; subject: string; intent: string; csrf: string }>()
const submitting = ref(false)
const error = ref('')
async function confirmLink() {
  if (submitting.value) return
  submitting.value = true
  error.value = ''
  try {
    const response = await fetch('/sso-link', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-CSRF-Token': props.csrf,
        Accept: 'application/json',
      },
      body: new URLSearchParams({
        intent: props.intent,
        csrf: props.csrf,
        confirm: 'link-this-identity',
      }),
    })
    const body = response.headers.get('content-type')?.includes('application/json')
      ? await response.json()
      : null
    if (
      response.ok &&
      typeof body?.next === 'string' &&
      body.next.startsWith('/sso?redirectback=')
    ) {
      window.location.assign(body.next)
      return
    }
    error.value =
      typeof body?.message === 'string'
        ? body.message
        : 'Привязка не подтверждена. Откройте новую форму или войдите локально заново.'
  } catch {
    error.value =
      'Результат запроса неизвестен. Не повторяйте отправку: откройте новую форму для проверки.'
  }
  // An intent can be consumed even on failure. Never silently repeat the POST.
}
</script>
