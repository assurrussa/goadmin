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
    >
      <input type="hidden" name="csrf" :value="reauthCSRF" />
      <input type="hidden" name="confirm" value="reauthenticate-local" />
      <button type="submit" class="text-primary">Войти заново</button>
    </form>
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
import SSOButton from '@/components/auth/SSOButton.vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AppHead from '@/components/layout/AppHead.vue'
defineProps<{
  title: string
  message: string
  code: string
  linkAvailable?: boolean
  startAvailable?: boolean
  returnTarget?: string
  reauthCSRF?: string
}>()
</script>
