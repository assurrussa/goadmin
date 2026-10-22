<template>
  <AuthLayout>
    <AppHead :title="title" />
    <template #title>{{ title }}</template>
    <p role="alert" class="text-sm">{{ message }}</p>
    <p class="my-3 text-xs text-muted-foreground">Код: {{ code }}</p>
    <form
      v-if="code === 'local_auth_stale' && reauthCSRF"
      method="post"
      action="/sso-link/reauth"
      class="my-3"
      @submit.prevent="reauthenticate"
    >
      <input type="hidden" name="csrf" :value="reauthCSRF" />
      <input type="hidden" name="confirm" value="reauthenticate-local" />
      <button type="submit" class="text-primary" :disabled="reauthSubmitting">Войти заново</button>
    </form>
    <p v-if="reauthError" role="alert" class="text-sm text-danger">{{ reauthError }}</p>
    <SSOButton v-if="startAvailable" :return-to="returnTarget" />
    <a v-if="linkAvailable" href="/sso-link" class="my-3 block text-primary"
      >Войти локально и открыть привязку</a
    >
    <a
      :href="code === 'local_auth_required' ? '/auth/profile/settings' : '/auth/login?local=1'"
      class="block text-primary"
      >{{ code === 'local_auth_required' ? 'К настройкам профиля' : 'Вернуться к входу' }}</a
    >
  </AuthLayout>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import SSOButton from '@/components/auth/SSOButton.vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AppHead from '@/components/layout/AppHead.vue'
import {
  localReauthenticationDestination,
  requestLocalReauthentication,
} from '@/auth/localReauthentication'
const props = defineProps<{
  title: string
  message: string
  code: string
  linkAvailable?: boolean
  startAvailable?: boolean
  returnTarget?: string
  reauthCSRF?: string
}>()
const reauthSubmitting = ref(false)
const reauthError = ref('')
async function reauthenticate() {
  if (reauthSubmitting.value || !props.reauthCSRF) return
  reauthSubmitting.value = true
  try {
    if (await requestLocalReauthentication(props.reauthCSRF)) {
      window.location.assign(localReauthenticationDestination)
      return
    }
    reauthError.value = 'Не удалось начать локальный вход. Обновите страницу и попробуйте снова.'
  } catch {
    reauthError.value = 'Результат запроса неизвестен. Обновите страницу перед новой попыткой.'
  }
}
</script>
