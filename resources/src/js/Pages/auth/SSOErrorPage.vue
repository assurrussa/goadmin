<template>
  <AuthLayout>
    <AppHead :title="title" />
    <template #title>{{ title }}</template>
    <p role="alert" class="text-sm">{{ message }}</p>
    <p class="my-3 text-xs text-muted-foreground">Код: {{ code }}</p>
    <SSOButton v-if="startAvailable" :return-to="returnTarget" />
    <a
      v-if="linkAvailable"
      href="/auth/login?local=1&amp;redirectback=%2Fsso-link"
      class="my-3 block text-primary"
      >Войти локально и открыть привязку</a
    >
    <a href="/auth/login?local=1" class="block text-primary">Вернуться к входу</a>
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
}>()
</script>
