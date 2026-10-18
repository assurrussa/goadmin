<script setup lang="ts">
import { computed } from 'vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AppHead from '@/components/layout/AppHead.vue'
import { t, registerTranslations } from '@/i18n'
registerTranslations({
  ru: {
    'Admission waiting title': 'Доступ к админке',
    'Admission waiting pending': 'Учётная запись создана. Ожидайте выдачи доступа администратором.',
    'Admission waiting approved': 'Доступ одобрен. Войдите через SSO ещё раз.',
    'Admission waiting active': 'Доступ выдан. Продолжите через SSO.',
    'Admission waiting rejected': 'Заявка отклонена. Обратитесь к администратору.',
    'Admission retry': 'Войти через SSO',
  },
  en: {
    'Admission waiting title': 'Admin access',
    'Admission waiting pending':
      'Your account has been created. Wait for an administrator to grant access.',
    'Admission waiting approved': 'Access approved. Sign in through SSO again.',
    'Admission waiting active': 'Access granted. Continue through SSO.',
    'Admission waiting rejected': 'Request rejected. Contact your administrator.',
    'Admission retry': 'Sign in through SSO',
  },
})
const props = defineProps<{ admissionStatus: string; signInPath: string }>()
const known = computed(() =>
  ['pending', 'approved', 'active', 'rejected'].includes(props.admissionStatus)
    ? props.admissionStatus
    : 'rejected',
)
const safePath = computed(() =>
  props.signInPath.startsWith('/') &&
  !props.signInPath.startsWith('//') &&
  !/[\\\r\n]/.test(props.signInPath)
    ? props.signInPath
    : '/auth/login',
)
</script>
<template>
  <AuthLayout>
    <AppHead :title="t('Admission waiting title')" />
    <template #title>{{ t('Admission waiting title') }}</template>
    <p role="status">{{ t('Admission waiting ' + known) }}</p>
    <a :href="safePath" class="mt-4 inline-block rounded-lg bg-primary px-4 py-2 text-white">
      {{ t('Admission retry') }}
    </a>
  </AuthLayout>
</template>
