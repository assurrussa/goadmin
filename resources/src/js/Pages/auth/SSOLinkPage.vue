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
    <form
      v-if="errorCode === 'local_auth_stale'"
      method="post"
      action="/sso-link/reauth"
      class="mt-3"
      @submit.prevent="reauthenticate"
    >
      <input type="hidden" name="csrf" :value="csrf" />
      <input type="hidden" name="confirm" value="reauthenticate-local" />
      <button type="submit" class="text-primary" :disabled="reauthSubmitting">Войти заново</button>
    </form>
    <p v-if="reauthError" role="alert" class="mt-3 text-sm text-danger">{{ reauthError }}</p>
    <a v-if="error" href="/sso-link" class="mt-3 block text-primary">Открыть новую форму</a>
    <a href="/" class="mt-4 block text-center text-primary">Отмена</a>
  </AuthLayout>
</template>
<script setup lang="ts">
import {
  adminFetch,
  ensureSessionReady,
  getCurrentCSRFToken,
  sessionCoordinator,
  SessionReadinessError,
} from '@/http'
import { ref } from 'vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AppHead from '@/components/layout/AppHead.vue'
import FormButton from '@/components/form/FormButton.vue'
import {
  localReauthenticationDestination,
  requestLocalReauthentication,
} from '@/auth/localReauthentication'
const props = defineProps<{ issuer: string; subject: string; intent: string; csrf: string }>()
const submitting = ref(false)
const error = ref('')
const errorCode = ref('')
const reauthSubmitting = ref(false)
const reauthError = ref('')
async function reauthenticate() {
  if (reauthSubmitting.value) return
  reauthSubmitting.value = true
  try {
    if (await requestLocalReauthentication(props.csrf)) {
      window.location.assign(localReauthenticationDestination)
      return
    }
    reauthError.value = 'Не удалось начать локальный вход. Обновите страницу и попробуйте снова.'
  } catch (cause) {
    if (cause instanceof SessionReadinessError) {
      reauthSubmitting.value = false
      reauthError.value = cause.message
      return
    }
    reauthError.value = 'Результат запроса неизвестен. Обновите страницу перед новой попыткой.'
  }
}
async function confirmLink() {
  if (submitting.value) return
  submitting.value = true
  error.value = ''
  errorCode.value = ''
  try {
    let csrf = props.csrf
    if (sessionCoordinator.isEnabled()) {
      await ensureSessionReady()
      csrf = getCurrentCSRFToken() ?? csrf
    }
    const response = await adminFetch('/sso-link', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-CSRF-Token': csrf,
        Accept: 'application/json',
      },
      body: new URLSearchParams({
        intent: props.intent,
        csrf,
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
    errorCode.value = typeof body?.code === 'string' ? body.code : ''
  } catch (cause) {
    if (cause instanceof SessionReadinessError) {
      submitting.value = false
      error.value = cause.message
      return
    }
    error.value =
      'Результат запроса неизвестен. Не повторяйте отправку: откройте новую форму для проверки.'
  }
  // An intent can be consumed even on failure. Never silently repeat the POST.
}
</script>
